package option

import (
	"github.com/felixorbit/fexrpc/common"
	"time"

	"github.com/felixorbit/fexrpc/codec"
)

type Option struct {
	MagicNumber    uint64        `json:"magic_number"`
	CodecType      codec.CType   `json:"codec_type"`
	ConnectTimeout time.Duration `json:"connect_timeout"` // 连接超时控制。0 代表没有限制
	HandleTimeout  time.Duration `json:"handle_timeout"`
}

// OptCodecType Option 的编码方式
var OptCodecType = common.OptionCodecBinary

const (
	MagicNumber = 0x3bef5c
)

var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      codec.GobType,
	ConnectTimeout: 10 * time.Second,
}
