package codec

import "io"

type Header struct {
	ServiceMethod string
	Seq           uint64
	Error         string
}

// 编解码器接口
type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

// 编解码器的构造函数
// 将 IO 包起来，读/写都通过编解码器 codec 完成
type NewCodecFunc func(io.ReadWriteCloser) Codec

type CodecType string

const (
	// gob 和 json 都是 go 语言自带的序列化方式
	GobType CodecType = "application/gob"
)

var NewCodecFuncMap map[CodecType]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[CodecType]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
}
