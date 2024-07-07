# RPC 框架 - FexRPC

> 参考 [7 days golang programs from scratch](https://github.com/geektutu/7days-golang)

## 特性

- 协议：TCP / HTTP
- 序列化：Gob / Json
- 超时控制：连接超时 / 调用超时
- 注册中心：接收服务心跳
- 负载均衡：随机选择 / Round-Robin
- 服务发现：硬编码 / 基于注册中心

## 类图
```mermaid
classDiagram
    direction TD
    class FexRegistry {
        -putServer()
        -aliveServers()
        +ServeHTTP()
    }
    class Server {
        -map~string,service~ serviceMap
        +Accept()
        +ServeHTTP()
        +Register()
    }
    class service {
        -string name
        -map~string,methodType~ method
        -registerMethods()
        -call()
    }
    class methodType {
        -reflect.Method method
        +reflect.Type ArgType
        +reflect.Type ReplyType
        -uint64 numCalls
    }
    class Discovery {
        <<interface>>
        +Refresh()
        +Update()
        +Get(SelectMode)
        +GetAll()
    }
    class MultiServerDiscovery {
        -slice~string~ servers
        +Refresh()
        +Update()
        +Get(SelectMode)
        +GetAll()
    }
    class FexRegistryDiscovery {
        -string registry
        -time.Duration timeout
        -time.Time lastUpdate
        +Refresh()
        +Update()
        +Get(SelectMode)
        +GetAll()
    }
    class Client {
        -map~uint64,*Call~ pending
        -Codec cc
        -send()
        -receive()
        +Go()
        +Call()
        +Close()
    }
    class XClient {
        -Discovery d
        -SelectMode mode
        -Option opt
        -map~string,Client~ clients
        -call()
        -dial()
        +Call()
        +Broadcast()
        +Close()
    }
    class Call {
        +uint64 Seq
        +string ServiceMethod
        +interface Args
        +interface Reply
        +error Error
        +chan~Call~ Done
    }
    class Codec {
        <<interface>>
        +ReadHeader()
        +ReadBody()
        +Write()
    }
    class GobCodec {
        -io.ReadWriteCloser conn
        -*gob.Decoder dec
        -*bufio.Writer buf
        -*gob.Encoder enc
        +ReadHeader()
        +ReadBody()
        +Write()
    }
    class JsonCodec {
        -io.ReadWriteCloser conn
        -*json.Decoder dec
        -*bufio.Writer buf
        -*json.Encoder enc
        +ReadHeader()
        +ReadBody()
        +Write()
    }
    
    service o-- methodType
    XClient o-- Discovery
    XClient o-- Client
    Discovery <|.. MultiServerDiscovery
    Discovery <|.. FexRegistryDiscovery
    FexRegistryDiscovery *-- MultiServerDiscovery
    Client o-- Codec
    Client o-- Call
    Codec <|.. GobCodec
    Codec <|.. JsonCodec
    Server o-- service
    Server ..> Codec
```

## 示例
### 编译
```shell
cd example
go build -o fexrpc
```

### 运行
```shell
cd example
go run .
```