package wsclient

import (
	"context"
	"errors"
	"fmt"
	"github.com/agiledragon/gomonkey/v2"
	"github.com/cloudwego/netpoll"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"strings"
	"testing"
	"time"
)

func TestSeekSeenFlags(t *testing.T) {
	tests := []struct {
		name        string
		inputFlags  byte     // 输入的标志位组合
		expectedErr error    // 预期的错误
		missing     []string // 预期缺失的头部列表
	}{
		// 成功用例
		{
			name:        "All flags present",
			inputFlags:  headerFlagAll, // 0b111
			expectedErr: nil,
			missing:     nil,
		},

		// 单个头部缺失
		{
			name:        "Missing Upgrade",
			inputFlags:  headerFlagConnection | headerFlagSecAccept, // 0b110
			expectedErr: fmt.Errorf("missing required headers: [Upgrade]"),
			missing:     []string{headerUpgradeCanonical},
		},
		{
			name:        "Missing Connection",
			inputFlags:  headerFlagUpgrade | headerFlagSecAccept, // 0b101
			expectedErr: fmt.Errorf("missing required headers: [Connection]"),
			missing:     []string{headerConnectionCanonical},
		},
		{
			name:        "Missing Sec-WebSocket-Accept",
			inputFlags:  headerFlagUpgrade | headerFlagConnection, // 0b011
			expectedErr: fmt.Errorf("missing required headers: [Sec-WebSocket-Accept]"),
			missing:     []string{headerSecAcceptCanonical},
		},

		// 多个头部缺失
		{
			name:        "Missing Upgrade and Connection",
			inputFlags:  headerFlagSecAccept, // 0b100
			expectedErr: fmt.Errorf("missing required headers: [Upgrade Connection]"),
			missing:     []string{headerUpgradeCanonical, headerConnectionCanonical},
		},
		{
			name:        "Missing Upgrade and Sec-WebSocket-Accept",
			inputFlags:  headerFlagConnection, // 0b010
			expectedErr: fmt.Errorf("missing required headers: [Upgrade Sec-WebSocket-Accept]"),
			missing:     []string{headerUpgradeCanonical, headerSecAcceptCanonical},
		},
		{
			name:        "Missing Connection and Sec-WebSocket-Accept",
			inputFlags:  headerFlagUpgrade, // 0b001
			expectedErr: fmt.Errorf("missing required headers: [Connection Sec-WebSocket-Accept]"),
			missing:     []string{headerConnectionCanonical, headerSecAcceptCanonical},
		},

		// 所有头部缺失
		{
			name:        "All headers missing",
			inputFlags:  0x00, // 0b000
			expectedErr: fmt.Errorf("missing required headers: [Upgrade Connection Sec-WebSocket-Accept]"),
			missing:     []string{headerUpgradeCanonical, headerConnectionCanonical, headerSecAcceptCanonical},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := seekSeenFlags(tt.inputFlags)

			// 错误存在性断言
			if (err != nil) != (tt.expectedErr != nil) {
				t.Fatalf("预期错误存在性: %v, 实际: %v", tt.expectedErr != nil, err != nil)
			}

			// 错误内容断言
			if err != nil {
				// 检查错误信息是否包含所有缺失的头部
				for _, h := range tt.missing {
					if !strings.Contains(err.Error(), h) {
						t.Errorf("错误信息缺少预期头部: %s, 实际错误: %v", h, err)
					}
				}

				// 检查错误信息格式
				expectedMsg := fmt.Sprintf("missing required headers: %v", tt.missing)
				if err.Error() != expectedMsg {
					t.Errorf("错误信息不匹配\n期望: %s\n实际: %s", expectedMsg, err.Error())
				}
			}
		})
	}
}

type DialerTestSuite struct {
	suite.Suite
	dialer   *Dialer
	connMock *MockConnection
	lb       *netpoll.LinkBuffer
	ctrl     *gomock.Controller
}

func TestDialerTestSuite(t *testing.T) {
	suite.Run(t, new(DialerTestSuite))
}

func (s *DialerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.connMock = NewMockConnection(s.ctrl)
	s.dialer = &Dialer{}
	s.lb = netpoll.NewLinkBuffer()
	s.connMock.EXPECT().Reader().Return(s.lb).AnyTimes()
	s.connMock.EXPECT().Writer().Return(s.lb).AnyTimes()
}

func (s *DialerTestSuite) TearDown() {
	s.ctrl.Finish()
}

func (s *DialerTestSuite) TestParseAndValidateHeaders_Normal() {
	nonce := newNonce()
	headers := strings.Join([]string{
		// "HTTP/1.1 101 Switching Protocols",
		"Upgrade: websocket",
		"Connection: Upgrade",
		"Sec-WebSocket-Accept:" + generateSecAccept(nonce),
		// "Sec-WebSocket-Protocol: chat",
	}, "\r\n") + "\r\n\r\n"
	s.lb.WriteString(headers)
	s.lb.Flush()
	resHeader, err := s.dialer.parseAndValidateHeaders(s.lb, nonce)
	s.Nil(err, "包含必须字段")
	s.Empty(resHeader)
}

func (s *DialerTestSuite) TestParseAndValidateHeaders_ConnectError() {
	nonce := newNonce()
	headers := strings.Join([]string{
		// "HTTP/1.1 101 Switching Protocols",
		"Upgrade: websocket",
		"Connection: Upgrade222",
		"Sec-WebSocket-Accept:" + generateSecAccept(nonce),
		// "Sec-WebSocket-Protocol: chat",
	}, "\r\n") + "\r\n\r\n"
	s.lb.WriteString(headers)
	s.lb.Flush()
	resHeader, err := s.dialer.parseAndValidateHeaders(s.lb, nonce)
	s.ErrorIs(err, ErrHandshakeBadConnection)
	s.Empty(resHeader)
}

func (s *DialerTestSuite) TestParseAndValidateHeaders_UpgradeError() {
	nonce := newNonce()
	headers := strings.Join([]string{
		// "HTTP/1.1 101 Switching Protocols",
		"Upgrade: websocket111",
		"Connection: Upgrade",
		"Sec-WebSocket-Accept:" + generateSecAccept(nonce),
		// "Sec-WebSocket-Protocol: chat",
	}, "\r\n") + "\r\n\r\n"
	s.lb.WriteString(headers)
	s.lb.Flush()
	resHeader, err := s.dialer.parseAndValidateHeaders(s.lb, nonce)
	s.ErrorIs(err, ErrHandshakeBadUpgrade)
	s.Empty(resHeader)
}

func (s *DialerTestSuite) TestParseAndValidateHeaders_SecAcceptError() {
	nonce := newNonce()
	headers := strings.Join([]string{
		// "HTTP/1.1 101 Switching Protocols",
		"Upgrade: websocket",
		"Connection: Upgrade",
		"Sec-WebSocket-Accept:" + generateSecAccept(nonce) + "1",
		// "Sec-WebSocket-Protocol: chat",
	}, "\r\n") + "\r\n\r\n"
	s.lb.WriteString(headers)
	s.lb.Flush()
	resHeader, err := s.dialer.parseAndValidateHeaders(s.lb, nonce)
	s.ErrorIs(err, ErrHandshakeBadSecAccept)
	s.Empty(resHeader)
}

func (s *DialerTestSuite) TestParseAndValidateHeaders_MissRequiredHeader() {
	nonce := newNonce()
	headers := strings.Join([]string{
		// "HTTP/1.1 101 Switching Protocols",
		// "Sec-WebSocket-Protocol: chat",
	}, "\r\n") + "\r\n\r\n"
	s.lb.WriteString(headers)
	s.lb.Flush()
	resHeader, err := s.dialer.parseAndValidateHeaders(s.lb, nonce)
	s.NotNil(err, "err不为空，缺少字段")
	s.Empty(resHeader)
}

func (s *DialerTestSuite) TestParseAndValidateHeaders_subProtocolFail() {
	nonce := newNonce()
	headers := strings.Join([]string{
		// "HTTP/1.1 101 Switching Protocols",
		"Upgrade: websocket",
		"Connection: Upgrade",
		"Sec-WebSocket-Accept:" + generateSecAccept(nonce),
		"Sec-WebSocket-Protocol: chat",
	}, "\r\n") + "\r\n\r\n"
	s.lb.WriteString(headers)
	s.lb.Flush()
	resHeader, err := s.dialer.parseAndValidateHeaders(s.lb, nonce)
	s.NotNil(err, "err不为空，子协议协商未通过")
	s.ErrorIs(err, ErrHandshakeBadSubProtocol)
	s.Empty(resHeader)
}

func (s *DialerTestSuite) TestParseAndValidateHeaders_subProtocolSucceed() {
	nonce := newNonce()
	headers := strings.Join([]string{
		// "HTTP/1.1 101 Switching Protocols",
		"Upgrade: websocket",
		"Connection: Upgrade",
		"Sec-WebSocket-Accept:" + generateSecAccept(nonce),
		"Sec-WebSocket-Protocol: chat",
	}, "\r\n") + "\r\n\r\n"
	s.lb.WriteString(headers)
	s.lb.Flush()
	s.dialer.Protocols = []string{"chat"}
	resHeader, err := s.dialer.parseAndValidateHeaders(s.lb, nonce)
	s.Nil(err, "err为空，协商成功")
	s.NotEmpty(resHeader)
	// fixme 添加http header优雅的比较形式
}

func (s *DialerTestSuite) TestDial_DailTimeout() {
	s.dialer.Timeout = 1 * time.Second
	err := netpoll.ErrDialTimeout
	s.dialer.NetDial = func(network, addr string, timeout time.Duration) (netpoll.Connection, error) {
		return nil, err
	}
	_, _, dialError := s.dialer.Dial(context.Background(), "ws://127.0.0.1:8080", nil)
	s.ErrorIs(dialError, err)
}

func (s *DialerTestSuite) TestDial_ReadTimeout() {
	s.dialer.Timeout = 1 * time.Second
	s.dialer.NetDial = func(network, addr string, timeout time.Duration) (netpoll.Connection, error) {
		return s.connMock, nil
	}
	timeout := time.Millisecond * 10
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	exit := make(chan struct{}, 1)
	patch := gomonkey.ApplyMethod(s.lb, "Until", func(b *netpoll.LinkBuffer, delimiter byte) (line []byte, err error) {
		<-exit
		return nil, errors.New("mock reader error")
	})
	defer patch.Reset()
	s.connMock.EXPECT().SetReadTimeout(gomock.Any()).AnyTimes()
	s.connMock.EXPECT().Close().DoAndReturn(func() error { exit <- struct{}{}; return nil })
	_, _, err := s.dialer.Dial(ctx, "ws://baidu.com/path", nil)
	s.ErrorIs(err, context.DeadlineExceeded)
}

func (s *DialerTestSuite) TestDial_ContextCancel() {
	s.dialer.Timeout = 1 * time.Second
	s.dialer.NetDial = func(network, addr string, timeout time.Duration) (netpoll.Connection, error) {
		return s.connMock, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		cancel()
	}()
	errChan := make(chan error, 1)
	patch := gomonkey.ApplyMethod(s.lb, "Until", func(b *netpoll.LinkBuffer, delimiter byte) (line []byte, err error) {
		err = <-errChan
		return nil, err
	})
	defer patch.Reset()
	s.connMock.EXPECT().SetReadTimeout(gomock.Any()).AnyTimes()
	s.connMock.EXPECT().Close().DoAndReturn(func() error { errChan <- fmt.Errorf("执行错误"); return nil })
	_, _, err := s.dialer.Dial(ctx, "ws://baidu.com/path", nil)
	s.ErrorIs(err, context.Canceled)
}
