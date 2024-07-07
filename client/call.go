package client

// Call 一次调用 Call 包含：方法名、参数、响应
// 支持异步调用，通过 Done 通道通知调用方
type Call struct {
	Seq           uint64
	ServiceMethod string
	Args          interface{}
	Reply         interface{}
	Error         error
	Done          chan *Call
}

func (c *Call) done() {
	c.Done <- c
}
