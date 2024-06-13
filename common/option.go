package common

import (
	"time"

	"github.com/felixorbit/fexrpc/codec"
)

type Option struct {
	MagicNumber    int
	CodecType      codec.CodecType
	ConnectTimeout time.Duration // 连接超时控制。0 代表没有限制
	HandleTimeout  time.Duration
}

const MagicNumber = 0x3bef5c

var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      codec.GobType,
	ConnectTimeout: 10 * time.Second,
}
