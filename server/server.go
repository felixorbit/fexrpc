package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/felixorbit/fexrpc/codec"
	"github.com/felixorbit/fexrpc/common"
)

type Server struct {
	serviceMap sync.Map
}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

var invalidRequest = struct{}{}

// 表示一次 RPC 调用请求
type request struct {
	h            *codec.Header
	svc          *service
	mtype        *methodType
	argv, replyv reflect.Value
}

func (s *Server) findService(serviceMethod string) (svc *service, mtype *methodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svcInter, ok := s.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service " + serviceName)
		return
	}
	svc = svcInter.(*service)
	mtype = svc.method[methodName]
	if mtype == nil {
		err = errors.New("rpc server: can't find method " + methodName)
	}
	return
}

func (s *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error: ", err)
		}
		return nil, err
	}
	return &h, nil
}

// 读一次 RPC 调用的请求，包括：header、调用方法、参数、响应，返回一个 request 实例
func (s *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	req.svc, req.mtype, err = s.findService(h.ServiceMethod)
	if err != nil {
		return req, err
	}
	req.argv = req.mtype.newArgv()
	req.replyv = req.mtype.newReplyv()
	argvInter := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvInter = req.argv.Addr().Interface()
	}
	if err = cc.ReadBody(argvInter); err != nil {
		log.Println("rpc server: read argv error: ", err)
	}
	return req, nil
}

// 执行请求，并返回响应
func (s *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	// 执行阶段同步，超时控制
	called := make(chan struct{})
	// 发送阶段同步
	sent := make(chan struct{})
	go func() {
		err := req.svc.call(req.mtype, req.argv, req.replyv)
		called <- struct{}{}
		if err != nil {
			req.h.Error = err.Error()
			s.sendResponse(cc, req.h, invalidRequest, sending)
			sent <- struct{}{}
			return
		}
		s.sendResponse(cc, req.h, req.replyv.Interface(), sending)
		sent <- struct{}{}
	}()
	if timeout == 0 {
		<-called
		<-sent
		return
	}
	select {
	case <-time.After(timeout):
		req.h.Error = fmt.Sprintf("rpc server: request handle timeout: expect within %s", timeout)
		s.sendResponse(cc, req.h, invalidRequest, sending)
	case <-called:
		<-sent
	}
}

func (s *Server) sendResponse(cc codec.Codec, header *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(header, body); err != nil {
		log.Println("rpc server: write response error: ", err)
	}
}

func (s *Server) serveCodec(cc codec.Codec, opt *common.Option) {
	sending := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	for {
		req, err := s.readRequest(cc)
		if err != nil {
			if req == nil {
				break
			}
			req.h.Error = err.Error()
			s.sendResponse(cc, req.h, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		s.handleRequest(cc, req, sending, wg, opt.HandleTimeout)
	}
	wg.Wait()
	_ = cc.Close()
}

// 一次连接可以包含多次调用，报文格式：| Option | Header1 | Body1 | Header2 | Body2 | ...
func (s *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() {
		_ = conn.Close()
	}()
	// 反序列化 Option，检查
	var opt common.Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: options error: ", err)
		return
	}
	if opt.MagicNumber != common.MagicNumber {
		log.Printf("rpc server: invalid magic number: %v", opt.MagicNumber)
		return
	}
	// 根据 CodeType 选择解码器进行解码
	f, ok := codec.NewCodecFuncMap[opt.CodecType]
	if !ok {
		log.Printf("rpc server: invalid codec type: %v", opt.CodecType)
		return
	}
	s.serveCodec(f(conn), &opt)
}

// 直接使用 TCP 协议
func (s *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("rpc server accept error: ", err)
			return
		}
		go s.ServeConn(conn)
	}
}

func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "405 must CONNECT\n")
		return
	}
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Print("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
		return
	}
	_, _ = io.WriteString(conn, "HTTP/1.0 "+common.Connected+"\n\n")
	s.ServeConn(conn)
}

// 使用 HTTP 协议
func (s *Server) HandleHTTP() {
	http.Handle(common.DefaultRPCPath, s)
	http.Handle(common.DefaultDebugPath, debugHTTP{s})
	log.Println("rpc server debug path:", common.DefaultDebugPath)
}

func HandleHTTP() {
	DefaultServer.HandleHTTP()
}

// 服务注册为 Service 实例，对外支持 RPC 调用
func (s *Server) Register(obj interface{}) error {
	service := newService(obj)
	if _, dup := s.serviceMap.LoadOrStore(service.name, service); dup {
		return errors.New("rpc: service already registered: " + service.name)
	}
	return nil
}

func Register(obj interface{}) error {
	return DefaultServer.Register(obj)
}
