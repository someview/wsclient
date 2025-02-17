package wsclient

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"github.com/cloudwego/netpoll"
	"sync"
	"time"
)

type WsClient struct {
	sync.Mutex
	conn    netpoll.Connection
	OnFrame func(frame *WsFrame)
	OnClose func()
	OnError func(error)
}

func newWsClient(conn netpoll.Connection) *WsClient {
	return &WsClient{conn: conn}
}

func (cli *WsClient) AddCallback(OnFrame func(frame *WsFrame), OnClose func(), OnError func(error)) {
	cli.Mutex.Lock()
	defer cli.Mutex.Unlock()
	cli.OnFrame = OnFrame
	cli.OnClose = OnClose
	if OnFrame == nil {
		cli.OnFrame = func(frame *WsFrame) {}
	}
	if cli.OnClose == nil {
		cli.OnClose = func() {}
	}
	if cli.OnError == nil {
		cli.OnError = func(err error) {}
	}
	_ = cli.conn.SetOnRequest(newOnRequest(cli))
	_ = cli.conn.AddCloseCallback(func(conn netpoll.Connection) error {
		cli.OnClose()
		return nil
	})
}

func newOnRequest(cli *WsClient) func(ctx context.Context, connection netpoll.Connection) error {
	return func(ctx context.Context, connection netpoll.Connection) error {
		reader := connection.Reader()
		defer reader.Release()
		frame, err := DecodeWsFrame(reader)
		if err != nil && errors.Is(err, ErrInvalidFrame) {
			// 违反websocket协议格式
			if errors.Is(err, ErrInvalidFrame) {
				return cli.Close(StatusProtocolError, "协议帧格式错误")
			}
			cli.OnError(err)
			return err
		}
		cli.OnFrame(frame)
		return nil
	}
}

func (cli *WsClient) WritePing(payload []byte) error {
	return cli.writeControlFrame(byte(OpPing), payload)
}

func (cli *WsClient) WritePong(payload []byte) error {
	return cli.writeControlFrame(byte(OpPong), payload)
}

func (cli *WsClient) Close(status StatusCode, reason string) error {
	reasonLen := len(reason)
	if reasonLen+2 > MaxControlFramePayloadSize {
		return ErrInvalidFrame
	}
	cli.Mutex.Lock()
	defer cli.Mutex.Unlock()
	wr := cli.conn.Writer()
	bufLen := 2 + 4 + 2 + reasonLen // 固定2个字节 +  4字节MASK + 2字节statusCode + reason
	buf, err := wr.Malloc(bufLen)   // 10 bytes for frame header and status code
	if err != nil {
		return err
	}
	buf[0] = byte(OpClose) | bit7 // 设置FIN位
	buf[1] = byte(reasonLen+2) | bit7
	maskKey := buf[2:6]
	if _, err = rand.Read(maskKey); err != nil {
		return err
	}
	binary.BigEndian.PutUint16(buf[6:], uint16(status))

	maskPayload := buf[8:]
	reasonBuf := strToBytes(reason)
	for i := 0; i < reasonLen; i++ {
		maskPayload[i] = reasonBuf[i] ^ maskKey[i%4]
	}
	_ = wr.Flush()
	return cli.conn.Close()
}

// 专用控制帧写入方法
func (cli *WsClient) writeControlFrame(opcode byte, payload []byte) error {
	if len(payload) > MaxControlFramePayloadSize {
		return ErrInvalidFrame
	}
	cli.Mutex.Lock()
	defer cli.Mutex.Unlock()
	wr := cli.conn.Writer()
	buf, err := wr.Malloc(6 + len(payload))
	if err != nil {
		return err
	}
	// 1. 构建帧头
	buf[0] = bit7 | opcode             // FIN=1 + OPCODE
	buf[1] = byte(len(payload)) | bit7 // MASK=1

	// 3. 生成掩码密钥
	maskKey := buf[2:6]
	if _, err = rand.Read(maskKey); err != nil {
		return err
	}
	maskPayload := buf[6:]
	for i := 0; i < len(payload); i++ {
		maskPayload[i] = payload[i] ^ maskKey[i%4]
	}
	err = wr.Flush()
	if OpCode(opcode) == OpClose {
		err = cli.conn.Close()
	}
	return err
}

// 公共数据写入方法
func (cli *WsClient) writeData(opcode OpCode, payload []byte) error {
	wr := cli.conn.Writer()

	// 1. 分配帧头空间
	header, err := wr.Malloc(14) // 最大可能头长度(2+8+4)
	if err != nil {
		return err
	}

	// 设置操作码和FIN位
	header[0] = byte(opcode) | bit7

	// 处理payload长度
	length := len(payload)
	ackLen := 2
	switch {
	case length <= 125:
		header[1] = byte(length) | bit7 // 设置MASK位
	case length <= 65535:
		binary.BigEndian.PutUint16(header[2:], uint16(length))
		header[1] = 0xFE // 126 + MASK位
		ackLen = 4
	default:
		binary.BigEndian.PutUint64(header[2:], uint64(length))
		header[1] = 0xFF // 127 + M
		ackLen = 10
	}

	// 处理掩码
	ackLen += 4
	maskKey := header[ackLen-4 : ackLen]
	if _, err = rand.Read(maskKey); err != nil {
		return err
	}
	_ = wr.MallocAck(ackLen)

	// 写入payload
	buf, err := wr.Malloc(len(payload))
	if err != nil {
		return err
	}
	for i := 0; i < len(payload); i++ {
		buf[i] = payload[i] ^ maskKey[i%4]
	}

	return wr.Flush()
}

func (cli *WsClient) WriteText(payload string) error {
	cli.Mutex.Lock()
	defer cli.Mutex.Unlock()
	return cli.writeData(OpText, strToBytes(payload))
}

func (cli *WsClient) WriteBinary(payload []byte) error {
	cli.Mutex.Lock()
	defer cli.Mutex.Unlock()
	return cli.writeData(OpBinary, payload)
}

func (cli *WsClient) IsActive() bool {
	return cli.conn.IsActive()
}

func (cli *WsClient) SetReadTimeout(duration time.Duration) error {
	return cli.conn.SetReadTimeout(duration)
}

func (cli *WsClient) SetWriteTimeout(duration time.Duration) error {
	return cli.conn.SetWriteTimeout(duration)
}
