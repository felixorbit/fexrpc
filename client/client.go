package client

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/felixorbit/fexrpc/option"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/felixorbit/fexrpc/codec"
	"github.com/felixorbit/fexrpc/common"
)

// Client 维护一次连接
// 1. 通过 Dial 创建一次连接，对应一个 Client 实例
// 2. 通过 Client 实例的 Call 接口完成调用
type Client struct {
	cc       codec.Codec
	opt      *option.Option
	sending  sync.Mutex // 保证发送一次完整请求
	mu       sync.Mutex
	seq      uint64
	pending  map[uint64]*Call // 等待响应的请求
	closing  bool             // 用户主动关闭
	shutdown bool             // 有错误发生
	target   string
}

var ErrShutDown = errors.New("connection is shut down")

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return ErrShutDown
	}
	c.closing = true
	return c.cc.Close()
}

func (c *Client) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.shutdown && !c.closing
}

func (c *Client) registerCall(call *Call) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing || c.shutdown {
		return 0, ErrShutDown
	}
	call.Seq = c.seq
	c.pending[call.Seq] = call
	c.seq++
	return call.Seq, nil
}

func (c *Client) removeCall(seq uint64) *Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	call := c.pending[seq]
	delete(c.pending, seq)
	return call
}

func (c *Client) terminateCalls(err error) {
	c.sending.Lock()
	defer c.sending.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdown = true
	for _, call := range c.pending {
		call.Error = err
		call.done()
	}
}

// 接收响应，处理待处理队列
func (c *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = c.cc.ReadHeader(&h); err != nil {
			break
		}
		call := c.removeCall(h.Seq)
		switch {
		case call == nil:
			err = c.cc.ReadBody(nil)
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = c.cc.ReadBody(nil)
			call.done()
		default:
			err = c.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}
	// 发生错误，中止连接，结束所有调用
	c.terminateCalls(err)
}

// 发送请求，Call 实例加入待处理队列
func (c *Client) send(call *Call) {
	seq, err := c.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}
	c.sending.Lock()
	defer c.sending.Unlock()
	header := &codec.Header{
		ServiceMethod: call.ServiceMethod,
		Seq:           seq,
	}
	if err = c.cc.Write(header, call.Args); err != nil {
		failedCall := c.removeCall(seq)
		if failedCall != nil {
			failedCall.Error = err
			failedCall.done()
		}
	}
}

// Go 异步调用，返回 Call 实例
func (c *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	c.send(call)
	return call
}

// Call 同步调用，对 Go 封装，阻塞在 Call.Done 等待响应返回。客户端通过 context 进行超时控制
func (c *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	call := c.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <-ctx.Done():
		c.removeCall(call.Seq)
		return errors.New("rpc client: call failed: " + ctx.Err().Error())
	case doneCall := <-call.Done:
		return doneCall.Error
	}
}

type newClientFunc func(conn net.Conn, opt *option.Option) (*Client, error)

func newClientCodec(cc codec.Codec, opt *option.Option) *Client {
	client := &Client{
		seq:     1,
		cc:      cc,
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}

// NewClient 创建连接。通过 Option 协商编码方式、超时时间
func NewClient(conn net.Conn, opt *option.Option) (*Client, error) {
	codecFunc := codec.NewCodecFuncMap[opt.CodecType]
	if codecFunc == nil {
		err := fmt.Errorf("invalid codec type: %v", opt.CodecType)
		log.Println("rpc client: codec error: ", err)
		return nil, err
	}
	switch option.OptCodecType {
	case common.OptionCodecBinary:
		if err := binary.Write(conn, binary.BigEndian, opt); err != nil {
			log.Println("rpc client: options error: ", err)
			_ = conn.Close()
			return nil, err
		}
	case common.OptionCodecJson:
		// 使用 JSON 协商选项，不定长，可能会读取多余的数据
		if err := json.NewEncoder(conn).Encode(opt); err != nil {
			log.Println("rpc client: options error: ", err)
			_ = conn.Close()
			return nil, err
		}
	}
	clientInst := newClientCodec(codecFunc(conn), opt)
	clientInst.target = conn.RemoteAddr().String()
	return clientInst, nil
}

// NewHTTPClient 创建 HTTP 连接
func NewHTTPClient(conn net.Conn, opt *option.Option) (*Client, error) {
	_, _ = io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", common.DefaultRPCPath))
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == common.Connected {
		return NewClient(conn, opt)
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	return nil, err
}

func parseOptions(opts ...*option.Option) (*option.Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return option.DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = option.DefaultOption.MagicNumber
	if opt.CodecType == 0 {
		opt.CodecType = option.DefaultOption.CodecType
	}
	return opt, nil
}

type clientResult struct {
	client *Client
	err    error
}

func dialTimeout(newFunc newClientFunc, network, address string, opts ...*option.Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	// 处理创建连接超时
	ch := make(chan clientResult)
	go func() {
		newClient, err := newFunc(conn, opt)
		ch <- clientResult{client: newClient, err: err}
	}()
	if opt.ConnectTimeout == 0 {
		result := <-ch
		return result.client, result.err
	}
	select {
	case <-time.After(opt.ConnectTimeout):
		return nil, fmt.Errorf("rpc client: connect timeout: expected within %s", opt.ConnectTimeout)
	case result := <-ch:
		return result.client, result.err
	}
}

func Dial(network, address string, opts ...*option.Option) (client *Client, err error) {
	return dialTimeout(NewClient, network, address, opts...)
}

func DialHTTP(network, address string, opts ...*option.Option) (client *Client, err error) {
	return dialTimeout(NewHTTPClient, network, address, opts...)
}

// XDial 根据协议建立连接，rpcAddr 格式： protocol@addr
func XDial(rpcAddr string, opts ...*option.Option) (*Client, error) {
	parts := strings.Split(rpcAddr, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("rpc client error: wrong format: '%s', expect protocol@addr", rpcAddr)
	}
	protocol, addr := parts[0], parts[1]
	switch protocol {
	case "http":
		return DialHTTP("tcp", addr, opts...)
	default:
		return Dial(protocol, addr, opts...)
	}
}
