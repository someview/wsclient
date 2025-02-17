package wsclient

import (
	"bytes"
	"github.com/cloudwego/netpoll"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"testing"
)

type WsClientTestSuite struct {
	suite.Suite
	client   *WsClient
	connMock *MockConnection
	lb       *netpoll.LinkBuffer
	ctrl     *gomock.Controller
}

func TestWsClientTestSuite(t *testing.T) {
	suite.Run(t, new(WsClientTestSuite))
}

func (s *WsClientTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.connMock = NewMockConnection(s.ctrl)
	s.lb = netpoll.NewLinkBuffer()
	s.client = newWsClient(s.connMock)
	s.connMock.EXPECT().Reader().Return(s.lb).AnyTimes()
	s.connMock.EXPECT().Writer().Return(s.lb).AnyTimes()
}

func (s *WsClientTestSuite) TearDown() {
	s.ctrl.Finish()
}

// 编码和解码相互对应
func (s *WsClientTestSuite) TestWriteControlFrame_Normal() {
	s.connMock.EXPECT().Close().AnyTimes()
	opcode := byte(0x8) //测试关闭帧
	payload := []byte("test payload")
	err := s.client.writeControlFrame(opcode, payload)
	s.NoError(err, "writeControlFrame should not return error")
	// 使用decodeWsFrame方法进行验证
	frame, err := DecodeWsFrame(s.lb)
	s.NoError(err, "解码不应该出错")
	s.Equal(OpCode(opcode), frame.Header.OpCode, "OpCode should match")
	s.Equal(uint64(len(payload)), frame.Length, "Length should match")
}

func (s *WsClientTestSuite) TestWriteControlFrame_TooBig() {
	// 准备测试数据
	opcode := byte(0x8) // 假设我们测试关闭帧
	payload := bytes.Repeat([]byte("a"), MaxControlFramePayloadSize+1)
	err := s.client.writeControlFrame(opcode, payload)
	s.ErrorIs(err, ErrInvalidFrame)
}
