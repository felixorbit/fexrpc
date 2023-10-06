package main

import (
	"context"
	"encoding/json"
	"felixorb/fexrpc/client"
	"felixorb/fexrpc/codec"
	"felixorb/fexrpc/common"
	"felixorb/fexrpc/registry"
	"felixorb/fexrpc/server"
	"felixorb/fexrpc/xclient"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// 启动注册中心
func startRegistry(wg *sync.WaitGroup) {
	l, _ := net.Listen("tcp", ":9999")
	registry.HandleHTTP()
	wg.Done()
	_ = http.Serve(l, nil)
}

// 启动服务，注册到注册中心并保持心跳
func startServer(registryAddr string, wg *sync.WaitGroup) {
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
	addr := l.Addr().String()

	registry.Heartbeat(registryAddr, "tcp@"+addr, 0)
	wg.Done()
	srv.Accept(l)
	// 使用 HTTP 协议
	// srv.HandleHTTP()
	// _ = http.Serve(l, nil)
}

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
	_ = json.NewEncoder(conn).Encode(common.DefaultOption)
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
	client, _ := client.Dial("tcp", <-addr)
	// 使用 HTTP 协议
	// client, _ := client.DialHTTP("tcp", <-addr)
	defer func() {
		client.Close()
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
			if err := client.Call(ctx, "FooSvc.Sum", args, &reply); err != nil {
				log.Fatal("call foo.Sum failed: ", err)
			}
			log.Printf("%d + %d = %d\n", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
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
func callWithLoadBalance(addr1 string, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer func() {
		xc.Close()
	}()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			callProxy(xc, context.Background(), "call", "FooSvc.Sum", &FooArgs{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

func broadcast(addr1 string, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer func() {
		xc.Close()
	}()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			callProxy(xc, context.Background(), "broadcast", "FooSvc.Sum", &FooArgs{Num1: i, Num2: i * i})
			ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
			callProxy(xc, ctx, "broadcast", "FooSvc.Sleep", &FooArgs{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

// 使用具有服务发现和负载均衡的 Client
func callWithDiscoveryAndLoadBalance(registry string) {
	d := xclient.NewFexRegistryDiscovery(registry, 0)
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer func() {
		xc.Close()
	}()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			callProxy(xc, context.Background(), "call", "FooSvc.Sum", &FooArgs{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

func broadcastWithDiscovery(registry string) {
	d := xclient.NewFexRegistryDiscovery(registry, 0)
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer func() {
		xc.Close()
	}()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			callProxy(xc, context.Background(), "broadcast", "FooSvc.Sum", &FooArgs{Num1: i, Num2: i * i})
			ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
			callProxy(xc, ctx, "broadcast", "FooSvc.Sleep", &FooArgs{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

func init() {
	// logFile, err := os.OpenFile("./log/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	// if err != nil {
	// 	fmt.Println("open log file failed, err:", err)
	// 	return
	// }
	// log.SetOutput(logFile)

	// log.SetFlags(log.Llongfile | log.Lmicroseconds | log.Ldate)
	log.SetFlags(log.Lshortfile | log.Ltime | log.Ldate)
}

func main() {
	// addr := make(chan string)
	// go syncStartServer(addr)
	// callSimple(addr)

	// 使用 debug 页面需要服务阻塞在主协程，调整了启动顺序
	// go call(addr)
	// syncStartServer(addr)

	// 多个服务实例，使用硬编码实现负载均衡
	// ch1 := make(chan string)
	// ch2 := make(chan string)
	// go syncStartServer(ch1)
	// go syncStartServer(ch2)
	// addr1 := <-ch1
	// addr2 := <-ch2
	// time.Sleep(time.Second)
	// callWithLoadBalance(addr1, addr2)
	// broadcast(addr1, addr2)

	// 使用注册中心+负载均衡
	registryAddr := "http://localhost:9999/_fexrpc_/registry"
	var wg sync.WaitGroup
	wg.Add(1)
	go startRegistry(&wg)
	wg.Wait()

	time.Sleep(time.Second)
	wg.Add(2)
	go startServer(registryAddr, &wg)
	go startServer(registryAddr, &wg)
	wg.Wait()

	time.Sleep(time.Second)
	callWithDiscoveryAndLoadBalance(registryAddr)
	broadcastWithDiscovery(registryAddr)
}
