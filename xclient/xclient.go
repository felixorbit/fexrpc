package xclient

import (
	"context"
	"github.com/felixorbit/fexrpc/option"
	"reflect"
	"sync"

	fexClient "github.com/felixorbit/fexrpc/client"
)

// XClient 封装 Client，屏蔽建立连接的实现，同时支持服务发现和负载均衡
type XClient struct {
	d       Discovery
	mode    SelectMode
	opt     *option.Option
	mu      sync.Mutex
	clients map[string]*fexClient.Client // 保存已建立的连接
}

func NewXClient(d Discovery, mode SelectMode, opt *option.Option) *XClient {
	return &XClient{
		d:       d,
		mode:    mode,
		opt:     opt,
		clients: make(map[string]*fexClient.Client),
	}
}

func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	for key, client := range xc.clients {
		_ = client.Close()
		delete(xc.clients, key)
	}
	return nil
}

func (xc *XClient) dial(addr string) (*fexClient.Client, error) {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	client, ok := xc.clients[addr]
	if ok && !client.IsAvailable() {
		_ = client.Close()
		delete(xc.clients, addr)
		client = nil
	}
	if client == nil {
		var err error
		client, err = fexClient.XDial(addr, xc.opt)
		if err != nil {
			return nil, err
		}
		xc.clients[addr] = client
	}
	return client, nil
}

func (xc *XClient) call(rpcAddr string, ctx context.Context, serviceMethod string, args, reply interface{}) error {
	client, err := xc.dial(rpcAddr)
	if err != nil {
		return err
	}
	return client.Call(ctx, serviceMethod, args, reply)
}

func (xc *XClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	rpcAddr, err := xc.d.Get(xc.mode)
	if err != nil {
		return err
	}
	return xc.call(rpcAddr, ctx, serviceMethod, args, reply)
}

// Broadcast 将请求广播到所有服务实例
// 任意一个实例发生错误，则返回错误；调用成功则返回其中一个结果
func (xc *XClient) Broadcast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	servers, err := xc.d.GetAll()
	if err != nil {
		return err
	}

	var mu sync.Mutex
	var e error
	replyDone := reply == nil // reply 为 nil 时调用没有返回值，无需设置

	ctx, cancel := context.WithCancel(ctx) // 有错误时快速失效
	var wg sync.WaitGroup
	for _, addr := range servers {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			var cloneReply interface{}
			if reply != nil {
				cloneReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			reqErr := xc.call(addr, ctx, serviceMethod, args, cloneReply)

			mu.Lock()
			if reqErr != nil && e == nil {
				e = reqErr
				cancel()
			}
			if reqErr == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(cloneReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(addr)
	}
	wg.Wait()
	return e
}
