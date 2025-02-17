package wsclient

import (
	"encoding/binary"
	"github.com/cloudwego/netpoll"
	"sync"
)

// Constants defined by specification.
const (
	// All control frames MUST have a payload length of 125 bytes or less and MUST NOT be fragmented.
	MaxControlFramePayloadSize = 125
)

const (
	bit0 = 0x01 // 第0位
	bit1 = 0x02 // 第1位
	bit2 = 0x04 // 第2位
	bit3 = 0x08 // 第3位
	bit4 = 0x10 // 第4位
	bit5 = 0x20 // 第5位
	bit6 = 0x40 // 第6位
	bit7 = 0x80 // 第7位

	len7  = int64(125)
	len16 = int64(^(uint16(0)))
	len64 = int64(^(uint64(0)) >> 1)
)

// OpCode represents operation code.
type OpCode byte

// Operation codes defined by specification.
// See https://tools.ietf.org/html/rfc6455#section-5.2
const (
	OpContinuation OpCode = 0x0
	OpText         OpCode = 0x1
	OpBinary       OpCode = 0x2
	OpClose        OpCode = 0x8
	OpPing         OpCode = 0x9
	OpPong         OpCode = 0xa
)

// IsControl checks whether the c is control operation code.
// See https://tools.ietf.org/html/rfc6455#section-5.5
func (c OpCode) IsControl() bool {
	// RFC6455: Control frames are identified by opcodes where
	// the most significant bit of the opcode is 1.
	//
	// Note that OpCode is only 4 bit length.
	return c&0x8 != 0
}

// IsData checks whether the c is data operation code.
// See https://tools.ietf.org/html/rfc6455#section-5.6
func (c OpCode) IsData() bool {
	// RFC6455: Data frames (e.g., non-control frames) are identified by opcodes
	// where the most significant bit of the opcode is 0.
	//
	// Note that OpCode is only 4 bit length.
	return c&0x8 == 0
}

// IsReserved checks whether the c is reserved operation code.
// See https://tools.ietf.org/html/rfc6455#section-5.2
func (c OpCode) IsReserved() bool {
	// RFC6455:
	// %x3-7 are reserved for further non-control frames
	// %xB-F are reserved for further control frames
	return (0x3 <= c && c <= 0x7) || (0xb <= c && c <= 0xf)
}

// StatusCode represents the encoded reason for closure of websocket connection.
//
// There are few helper methods on StatusCode that helps to define a range in
// which given code is lay in. accordingly to ranges defined in specification.
//
// See https://tools.ietf.org/html/rfc6455#section-7.4
type StatusCode uint16

// StatusCodeRange describes range of StatusCode values.
type StatusCodeRange struct {
	Min, Max StatusCode
}

// Status code ranges defined by specification.
// See https://tools.ietf.org/html/rfc6455#section-7.4.2
var (
	StatusRangeNotInUse    = StatusCodeRange{0, 999}
	StatusRangeProtocol    = StatusCodeRange{1000, 2999}
	StatusRangeApplication = StatusCodeRange{3000, 3999}
	StatusRangePrivate     = StatusCodeRange{4000, 4999}
)

// Status codes defined by specification.
// See https://tools.ietf.org/html/rfc6455#section-7.4.1
const (
	StatusNormalClosure           StatusCode = 1000
	StatusGoingAway               StatusCode = 1001
	StatusProtocolError           StatusCode = 1002
	StatusUnsupportedData         StatusCode = 1003
	StatusNoMeaningYet            StatusCode = 1004
	StatusInvalidFramePayloadData StatusCode = 1007
	StatusPolicyViolation         StatusCode = 1008
	StatusMessageTooBig           StatusCode = 1009
	StatusMandatoryExt            StatusCode = 1010
	StatusInternalServerError     StatusCode = 1011
	StatusTLSHandshake            StatusCode = 1015

	// StatusAbnormalClosure is a special code designated for use in
	// applications.
	StatusAbnormalClosure StatusCode = 1006

	// StatusNoStatusRcvd is a special code designated for use in applications.
	StatusNoStatusRcvd StatusCode = 1005
)

// In reports whether the code is defined in given range.
func (s StatusCode) In(r StatusCodeRange) bool {
	return r.Min <= s && s <= r.Max
}

// Empty reports whether the code is empty.
// Empty code has no any meaning neither app level codes nor other.
// This method is useful just to check that code is golang default value 0.
func (s StatusCode) Empty() bool {
	return s == 0
}

// IsNotUsed reports whether the code is predefined in not used range.
func (s StatusCode) IsNotUsed() bool {
	return s.In(StatusRangeNotInUse)
}

// IsApplicationSpec reports whether the code should be defined by
// application, framework or libraries specification.
func (s StatusCode) IsApplicationSpec() bool {
	return s.In(StatusRangeApplication)
}

// IsPrivateSpec reports whether the code should be defined privately.
func (s StatusCode) IsPrivateSpec() bool {
	return s.In(StatusRangePrivate)
}

// IsProtocolSpec reports whether the code should be defined by protocol specification.
func (s StatusCode) IsProtocolSpec() bool {
	return s.In(StatusRangeProtocol)
}

// IsProtocolDefined reports whether the code is already defined by protocol specification.
func (s StatusCode) IsProtocolDefined() bool {
	switch s {
	case StatusNormalClosure,
		StatusGoingAway,
		StatusProtocolError,
		StatusUnsupportedData,
		StatusInvalidFramePayloadData,
		StatusPolicyViolation,
		StatusMessageTooBig,
		StatusMandatoryExt,
		StatusInternalServerError,
		StatusNoStatusRcvd,
		StatusAbnormalClosure,
		StatusTLSHandshake:
		return true
	}
	return false
}

// IsProtocolReserved reports whether the code is defined by protocol specification
// to be reserved only for application usage purpose.
func (s StatusCode) IsProtocolReserved() bool {
	switch s {
	// [RFC6455]: {1005,1006,1015} is a reserved value and MUST NOT be set as a status code in a
	// Close control frame by an endpoint.
	case StatusNoStatusRcvd, StatusAbnormalClosure, StatusTLSHandshake:
		return true
	default:
		return false
	}
}

// Header represents websocket frame header.
// See https://tools.ietf.org/html/rfc6455#section-5.2
type Header struct {
	Fin    bool
	Rsv    byte
	OpCode OpCode
	Masked bool
	Mask   [4]byte
	Length uint64
}

// Rsv1 reports whether the header has first rsv bit set.
func (h Header) Rsv1() bool { return h.Rsv&bit5 != 0 }

// Rsv2 reports whether the header has second rsv bit set.
func (h Header) Rsv2() bool { return h.Rsv&bit6 != 0 }

// Rsv3 reports whether the header has third rsv bit set.
func (h Header) Rsv3() bool { return h.Rsv&bit7 != 0 }

// Rsv creates rsv byte representation from bits.
func Rsv(r1, r2, r3 bool) (rsv byte) {
	if r1 {
		rsv |= bit5
	}
	if r2 {
		rsv |= bit6
	}
	if r3 {
		rsv |= bit7
	}
	return rsv
}

// RsvBits returns rsv bits from bytes representation.
func RsvBits(rsv byte) (r1, r2, r3 bool) {
	r1 = rsv&bit5 != 0
	r2 = rsv&bit6 != 0
	r3 = rsv&bit7 != 0
	return r1, r2, r3
}

var wsFramePool = &sync.Pool{
	New: func() any {
		return &WsFrame{
			Header:  Header{},
			Payload: make([]byte, 0, 1024), // 这里是预估的payload大小
		}
	},
}

func SetWsFramePool(pool *sync.Pool) {
	wsFramePool = pool
}

// Frame represents websocket frame.
// See https://tools.ietf.org/html/rfc6455#section-5.2
type WsFrame struct {
	Header
	Payload []byte
}

func NewWsFrame() *WsFrame {
	return wsFramePool.Get().(*WsFrame)
}

func (f *WsFrame) Recycle() {
	wsFramePool.Put(f)
}

type Recycler interface {
	Recycle()
}

// 0                   1                   2                   3
// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-------+-+-------------+-------------------------------+
// |F|R|R|R| opcode|M| Payload len |    Extended payload length    |     baseHeader +  extraHeader
// |I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
// |N|V|V|V|       |S|             |   (if payload len==126/127)   |
// | |1|2|3|       |K|             |                               |
// +-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
// |     Extended payload length continued, if payload len == 127  |
// + - - - - - - - - - - - - - - - +-------------------------------+
// |                               |Masking-key, if MASK set to 1  |
// +-------------------------------+-------------------------------+
// | Masking-key (continued)       |          Payload Data         |
// +-------------------------------- - - - - - - - - - - - - - - - +
// :                     Payload Data continued ...                :
// + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
// |                     Payload Data continued ...                |
// +---------------------------------------------------------------+

const baseHeaderSize = 2 // 前两个字节

// DecodeWsFrame 同时适用于客户端和服务端的解码帧格式
func DecodeWsFrame(reader netpoll.Reader) (*WsFrame, error) {
	// 1. 解析header固定部分
	fixedHeader, err := reader.Next(baseHeaderSize)
	if err != nil {
		return nil, err
	}
	frame := NewWsFrame()
	frame.Fin = fixedHeader[0]&bit7 != 0
	frame.Rsv = (fixedHeader[0] & 0x70) >> 4
	frame.OpCode = OpCode(fixedHeader[0] & 0x0f)
	frame.Masked = fixedHeader[1]&bit7 != 0
	if frame.Rsv != 0 {
		return nil, ErrInvalidFrame
	}
	// 2. 计算header变长部分
	extraHeaderSize := 0
	payloadLenField := fixedHeader[1] & 0x7f
	switch {
	case payloadLenField == 126:
		extraHeaderSize = 2
	case payloadLenField < 126:
		frame.Length = uint64(payloadLenField)
	case payloadLenField == 127:
		extraHeaderSize = 8
	default:
		return nil, ErrInvalidFrame
	}
	if frame.Masked {
		extraHeaderSize += 4
	}
	extraHeader, err := reader.Next(extraHeaderSize)
	if err != nil {
		return nil, err
	}

	if payloadLenField == 126 {
		frame.Length = uint64(binary.BigEndian.Uint16(extraHeader[:2])) // 长度字节为[0:2]
	} else if payloadLenField == 127 {
		frame.Length = binary.BigEndian.Uint64(extraHeader[:8]) // 长度字节为[0:8]
	}
	if frame.OpCode.IsControl() && frame.Length > 125 { // fixHeader所能表示的最大长度
		return nil, ErrInvalidFrame
	}

	if frame.Masked {
		copy(frame.Mask[:], extraHeader[extraHeaderSize-4:])
	}

	frameBuf, err := reader.Next(int(frame.Length))
	if err != nil {
		return nil, err
	}
	if cap(frame.Payload) < int(frame.Length) {
		frame.Payload = make([]byte, frame.Length)
	}
	frame.Payload = frame.Payload[:frame.Length]
	if frame.Masked {
		mask := frame.Mask // 按协议规范进行异或解码
		for i := 0; i < len(frameBuf); i++ {
			frame.Payload[i] = frameBuf[i] ^ mask[i%4]
		}
	} else {
		copy(frame.Payload, frameBuf)
	}
	return frame, nil
}
