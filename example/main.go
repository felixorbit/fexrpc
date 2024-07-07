package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/felixorbit/fexrpc/option"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/felixorbit/fexrpc/client"
	"github.com/felixorbit/fexrpc/codec"
	"github.com/felixorbit/fexrpc/registry"
	"github.com/felixorbit/fexrpc/server"
	"github.com/felixorbit/fexrpc/xclient"
)

// 同步启动服务端，返回服务端监听地址进行测试
func syncStartServer(addr chan string) {
	srv := server.NewServer()
	var foo FooSvc
	if err := srv.Register(&foo); err != nil {
		log.Println("register error: ", err)
	}

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error: ", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()

	srv.Accept(l)
	// 使用 HTTP 协议
	// srv.HandleHTTP()
	// _ = http.Serve(l, nil)
}

// 实现简易 Client 完成调用
func callSimple(addr chan string) {
	conn, _ := net.Dial("tcp", <-addr)
	defer func() {
		_ = conn.Close()
	}()
	time.Sleep(time.Second)
	_ = json.NewEncoder(conn).Encode(option.DefaultOption)
	cc := codec.NewGobCodec(conn)
	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           uint64(i),
		}
		_ = cc.Write(h, fmt.Sprintf("fexrpc req %d", h.Seq))
		_ = cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply :", reply)
	}
}

// 通过 Client 完成调用
func call(addr chan string) {
	c, _ := client.Dial("tcp", <-addr)
	// 使用 HTTP 协议
	// client, _ := client.DialHTTP("tcp", <-addr)
	defer func() {
		c.Close()
	}()

	time.Sleep(time.Second)
	wg := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &FooArgs{Num1: i, Num2: i * i}
			ctx, _ := context.WithTimeout(context.Background(), time.Second)
			var reply int
			if err := c.Call(ctx, "FooSvc.Sum", args, &reply); err != nil {
				log.Fatal("call foo.Sum failed: ", err)
			}
			log.Printf("%d + %d = %d\n", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
}

// 启动注册中心
func startRegistry(wg *sync.WaitGroup) {
	regCenter := registry.NewFexRegistry(time.Minute * 5)
	regCenter.HandleHTTP("/_fexrpc_/registry")
	wg.Done()
	_ = http.ListenAndServe(":9999", regCenter)
}

// 启动服务，注册到注册中心并保持心跳
func startServer(registryAddr string, serveHttp bool, wg *sync.WaitGroup) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error: ", err)
	}
	addr := l.Addr().String()
	log.Println("start rpc server on", addr)

	srv := server.NewServer()
	srv.SetAddr(addr)
	var foo FooSvc
	if err = srv.Register(&foo); err != nil {
		log.Println("register error: ", err)
	}
	if serveHttp {
		registry.Heartbeat(registryAddr, "http@"+addr, 0)
		// 使用 HTTP 协议
		httpSrv := srv.HandleHTTP()
		wg.Done()
		_ = http.Serve(l, httpSrv)
	} else {
		registry.Heartbeat(registryAddr, "tcp@"+addr, 0)
		wg.Done()
		srv.Accept(l)
	}
}

func callProxy(xc *xclient.XClient, ctx context.Context, typ, serviceMethod string, args *FooArgs) {
	var reply int
	var err error
	switch typ {
	case "call":
		err = xc.Call(ctx, serviceMethod, args, &reply)
	case "broadcast":
		err = xc.Broadcast(ctx, serviceMethod, args, &reply)
	}
	if err != nil {
		log.Printf("%s %s error: %v", typ, serviceMethod, err)
	} else {
		log.Printf("%s %s success: %d + %d = %d\n", typ, serviceMethod, args.Num1, args.Num2, reply)
	}
}

// 使用具有负载均衡的 Client
func callWithLoadBalance(callType, callMethod string, useDiscovery bool, registry string, addrList []string) {
	var d xclient.Discovery
	if !useDiscovery {
		targets := make([]string, 0)
		for _, addr := range addrList {
			targets = append(targets, "tcp@"+addr)
		}
		d = xclient.NewMultiServerDiscovery(targets)
	} else {
		d = xclient.NewFexRegistryDiscovery(registry, 0)
	}
	opt := &option.Option{
		MagicNumber:    option.MagicNumber,
		CodecType:      codec.GobType,
		ConnectTimeout: time.Second,
		HandleTimeout:  0,
	}
	// 初始化一个 client，所有调用结束后关闭
	xc := xclient.NewXClient(d, xclient.RoundRobinSelect, opt)
	defer func() {
		_ = xc.Close()
	}()
	var wg sync.WaitGroup
	for i := 0; i < 9; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ctx := context.Background()
			//ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
			callProxy(xc, ctx, callType, callMethod, &FooArgs{Num1: n, Num2: 0})
		}(i)
	}
	wg.Wait()
}

func main() {
	// logFile, err := os.OpenFile("./log/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	// if err != nil {
	// 	fmt.Println("open log file failed, err:", err)
	// 	return
	// }
	// log.SetOutput(logFile)
	//log.SetFlags(log.Lshortfile | log.Ltime | log.Ldate)

	//addr := make(chan string)
	//go syncStartServer(addr)
	//callSimple(addr)

	// 使用 debug 页面需要服务阻塞在主协程，调整了启动顺序
	//go call(addr)
	//syncStartServer(addr)

	// 多个服务实例，使用硬编码实现负载均衡
	//ch1 := make(chan string)
	//ch2 := make(chan string)
	//go syncStartServer(ch1)
	//go syncStartServer(ch2)
	//addr1 := <-ch1
	//addr2 := <-ch2
	//time.Sleep(time.Second)
	//callWithLoadBalance([]string{addr1, addr2}, false, "")
	//broadcastWithLoadBalance([]string{addr1, addr2}, false, "")

	// 使用注册中心+负载均衡
	registryAddr := "http://localhost:9999/_fexrpc_/registry"
	var wg sync.WaitGroup
	wg.Add(1)
	go startRegistry(&wg)
	wg.Wait()

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go startServer(registryAddr, true, &wg)
	}
	wg.Wait()
	callWithLoadBalance("call", "FooSvc.Sum", true, registryAddr, []string{})
}
