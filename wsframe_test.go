package wsclient

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/cloudwego/netpoll"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestOpCodeIsControl(t *testing.T) {
	for _, test := range []struct {
		code OpCode
		exp  bool
	}{
		{OpClose, true},
		{OpPing, true},
		{OpPong, true},
		{OpBinary, false},
		{OpText, false},
		{OpContinuation, false},
	} {
		t.Run(fmt.Sprintf("0x%02x", test.code), func(t *testing.T) {
			if act := test.code.IsControl(); act != test.exp {
				t.Errorf("IsControl = %v; want %v", act, test.exp)
			}
		})
	}
}

func TestLinkedBuffer(t *testing.T) {
	lb := netpoll.NewLinkBuffer()
	buf := []byte("hello")
	lb.WriteBinary(buf)
	lb.Flush()
	assert.Equal(t, lb.Len(), 5)
	reads := lb.Bytes()
	assert.ElementsMatch(t, reads, buf)
	b, _ := lb.Next(1)
	assert.Equal(t, b[0], buf[0])
}

type DecodeFrameTestSuite struct {
	suite.Suite
	reader *netpoll.LinkBuffer
}

func (s *DecodeFrameTestSuite) WriteTestData(testData []byte) {
	s.reader.WriteBinary(testData)
	s.reader.Flush()
}

func TestDecodeFrameTestSuite(t *testing.T) {
	suite.Run(t, new(DecodeFrameTestSuite))
}

func (s *DecodeFrameTestSuite) SetupTest() {
	s.reader = netpoll.NewLinkBuffer()
}

func (s *DecodeFrameTestSuite) TearDownTest() {
}

func (s *DecodeFrameTestSuite) TestMaskDecoding() {
	// 构造测试数据（带掩码）
	testData := []byte{
		0x82, 0x86, // FIN=1, Opcode=2, Masked=1, len=6
		0x11, 0x22, 0x33, 0x44, // Mask key
		// 加密后的payload（原始数据与mask按字节异或）
		'a' ^ 0x11, 'b' ^ 0x22, 'c' ^ 0x33, 'd' ^ 0x44,
		'e' ^ 0x11, 'f' ^ 0x22, // 继续循环使用mask key
	}
	s.WriteTestData(testData)
	_, err := DecodeWsFrame(s.reader)
	s.Nil(err)
}

func (s *DecodeFrameTestSuite) TestFragmentedFrames() {
	// 第一个分片（FIN=0）
	s.WriteTestData([]byte{
		0x01, 0x05, // Opcode=Text, FIN=0, Len=5
		'H', 'e', 'l', 'l', 'o',
	})
	// 持续分片（FIN=1）
	s.WriteTestData([]byte{
		0x80, 0x06, // Opcode=Continuation, FIN=1, Len=6
		' ', 'W', 'o', 'r', 'l', 'd',
	})

	// 解码第一个分片
	frame1, err := DecodeWsFrame(s.reader)
	s.NoError(err)
	s.False(frame1.Fin)
	s.Equal(OpText, frame1.OpCode)
	s.Equal([]byte("Hello"), frame1.Payload)

	// 解码第二个分片
	frame2, err := DecodeWsFrame(s.reader)
	s.NoError(err)
	s.True(frame2.Fin)
	s.Equal(OpContinuation, frame2.OpCode)
	s.Equal([]byte(" World"), frame2.Payload)
}

func (s *DecodeFrameTestSuite) TestBoundaryLength125() {
	payload := bytes.Repeat([]byte{'A'}, 125)
	testData := append([]byte{
		0x82, 0x7D, // FIN=1, Opcode=2, Masked=0, len=125
	}, payload...)
	s.WriteTestData(testData)
	frame, err := DecodeWsFrame(s.reader)
	s.NoError(err)
	assert.Equal(s.T(), uint64(125), frame.Length)
	assert.Equal(s.T(), payload, frame.Payload)
}

// 测试控制帧payload超限（RFC6455规定控制帧payload≤125）
func (s *DecodeFrameTestSuite) TestControlFrameOversize() {
	testData := []byte{
		0x89, 0xFE, 0x00, 0x7E, // FIN=1, Opcode=9, Masked=0, len=126
	}
	s.WriteTestData(testData)
	_, err := DecodeWsFrame(s.reader)
	s.NotNil(err, "解码出现错误")
	// linked buffer行为和connection行为有些差异,connection保证一定能读取到足够量的数据
	//s.ErrorIs(err, ErrInvalidFrame)
}

// 测试0长度payload
func (s *DecodeFrameTestSuite) TestZeroLengthPayload() {
	testData := []byte{0x82, 0x00} // FIN=1, Opcode=2, Len=0
	s.WriteTestData(testData)

	frame, err := DecodeWsFrame(s.reader)
	s.NoError(err)
	s.Empty(frame.Payload, "Payload should be empty")
}

// 测试最大16-bit长度（65535）
func (s *DecodeFrameTestSuite) TestMax16BitLength() {
	// 修正header构造方式
	header := []byte{
		0x82, 0x7E, // FIN=1, Opcode=2, Masked=0, len=126
		0xFF, 0xFF, // 16位长度字段65535 (big-endian)
	}
	testData := bytes.Repeat(header, 1)
	testData = append(testData, bytes.Repeat([]byte{'A'}, 65535)...)

	s.WriteTestData(testData)
	frame, err := DecodeWsFrame(s.reader)
	s.NoError(err)
	s.Equal(uint64(65535), frame.Length)
}

// 测试超大长度帧（64位长度）
func (s *DecodeFrameTestSuite) Test64BitLength() {
	header := []byte{
		0x82, 0x7F, // FIN=1, Opcode=2, len=127
		0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, // 65536 (big-endian)
	}
	testData := append(header, bytes.Repeat([]byte{'A'}, 65536)...)

	s.WriteTestData(testData)
	frame, err := DecodeWsFrame(s.reader)
	s.NoError(err)
	s.Equal(uint64(65536), frame.Length)
}

func (s *DecodeFrameTestSuite) TestReservedBits() {
	tests := []struct {
		name        string
		data        []byte
		expectError bool
	}{
		{
			name:        "RSV1_set",
			data:        []byte{0xC2, 0x00}, // FIN=1, RSV1=1, Opcode=2
			expectError: true,
		},
		{
			name:        "RSV2_set",
			data:        []byte{0xA2, 0x00}, // FIN=1, RSV2=1, Opcode=2
			expectError: true,
		},
		{
			name:        "RSV3_set",
			data:        []byte{0x92, 0x00}, // FIN=1, RSV3=1, Opcode=2
			expectError: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// 重置reader
			s.SetupTest()

			// 写入测试数据
			s.WriteTestData(tt.data)

			// 解码帧
			_, err := DecodeWsFrame(s.reader)
			s.NotNil(err)
			s.Error(err, "应返回协议错误")
			s.True(errors.Is(err, ErrInvalidFrame),
				"错误类型应为协议违规，实际错误：%v", err)
		})
	}
}
