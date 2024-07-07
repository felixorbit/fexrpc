package codec

import (
	"io"
)

type Header struct {
	ServiceMethod string
	Seq           uint64
	Error         string
}

// Codec 编解码器接口
type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

// NewCodecFunc 编解码器的构造函数
// 将 IO 包起来，读/写都通过编解码器 codec 完成
type NewCodecFunc func(io.ReadWriteCloser) Codec

type CType uint64

const (
	GobType CType = iota + 1
	JsonType
)

var NewCodecFuncMap map[CType]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[CType]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
	NewCodecFuncMap[JsonType] = NewJsonCodec
}
