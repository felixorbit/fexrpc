package codec

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
)

type JsonCodec struct {
	conn io.ReadWriteCloser
	dec  *json.Decoder
	buf  *bufio.Writer
	enc  *json.Encoder
}

// 确保接口被实现常用的方式。即利用强制类型转换，确保 struct 实现了接口
var _ Codec = (*JsonCodec)(nil)

func NewJsonCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &JsonCodec{
		conn: conn,                  // 建立 socket 时的链接实例
		dec:  json.NewDecoder(conn), // 从链接实例中读取并解码
		buf:  buf,                   // 带缓冲的 Writer, 防止阻塞
		enc:  json.NewEncoder(buf),  // 从写缓冲区读取并编码
	}
}

func (g *JsonCodec) ReadHeader(header *Header) error {
	return g.dec.Decode(header)
}

func (g *JsonCodec) ReadBody(body interface{}) error {
	return g.dec.Decode(body)
}

func (g *JsonCodec) Write(header *Header, body interface{}) (err error) {
	defer func() {
		_ = g.buf.Flush()
		if err != nil {
			_ = g.Close()
		}
	}()
	if err = g.enc.Encode(header); err != nil {
		log.Println("rpc codec: json error encoding header: ", err)
		return err
	}
	if err = g.enc.Encode(body); err != nil {
		log.Println("rpc codec: json error encoding body: ", err)
		return err
	}
	return nil
}

func (g *JsonCodec) Close() error {
	return g.conn.Close()
}
