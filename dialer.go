package wsclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/cloudwego/netpoll"
	"github.com/someview/wsclient/httphead"
	"net/http"
	"strconv"
	"time"
)

var (
	ErrHandshakeBadStatus      = fmt.Errorf("unexpected http status")
	ErrHandshakeBadSubProtocol = fmt.Errorf("unexpected protocol in %q header", headerSecProtocol)
	ErrHandshakeBadExtensions  = fmt.Errorf("unexpected extensions in %q header", headerSecProtocol)
	ErrMalformedURL            = fmt.Errorf("malformed ws or wss URL")
)

var (
	// This variables are set like in net/net.go.
	// noDeadline is just zero value for readability.
	noDeadline = time.Time{}
	// aLongTimeAgo is a non-zero time, far in the past, used for immediate
	// cancelation of dials.
	aLongTimeAgo = time.Unix(42, 0)
)

// DefaultDialer is dialer that holds no options and is used by Dial function.
var DefaultDialer Dialer

// Dial is like Dialer{}.Dial().
func DialContext(ctx context.Context, urlstr string, header http.Header) (*WsClient, http.Header, error) {
	return DefaultDialer.Dial(ctx, urlstr, header)
}

type NetDial func(network, addr string, timeout time.Duration) (netpoll.Connection, error)

// Dialer contains options for establishing websocket connection to an url.
type Dialer struct {
	Timeout time.Duration
	// 子协议列表 (例如: []string{"chat", "notification"})
	Protocols []string
	// Extensions is the list of extensions that client wants to speak.
	// Note that if server decides to use some of this extensions, Dial() will
	// return Handshake struct containing a slice of items, which are the
	// shallow copies of the items from this list. That is, internals of
	// Extensions items are shared during Dial().
	// See https://tools.ietf.org/html/rfc6455#section-4.1
	// See https://tools.ietf.org/html/rfc6455#section-9.1
	// 扩展支持 (例如: []string{"permessage-deflate"})
	// todo 添加压缩扩展支持, 需要在netpoll.writer实现压缩算法，成本较高
	Extensions []httphead.Option
	// Host is an optional string that could be used to specify the host during
	// HTTP upgrade request by setting 'Host' header.
	//
	// Default value is an empty string, which results in setting 'Host' header
	// equal to the URL hostname given to Dialer.Dial().
	Host string
	// NetDial is the function that is used to get plain tcp connection.
	// If it is not nil, then it is used instead of net.Dialer.
	NetDial
}

// Dial connects to the url host and upgrades connection to WebSocket.
//
// If server has sent frames right after successful handshake then returned
// buffer will be non-nil. In other cases buffer is always nil. For better
// memory efficiency received non-nil bufio.Reader should be returned to the
// inner pool with PutReader() function after use.
//
// Note that Dialer does not implement IDNA (RFC5895) logic as net/http does.
// If you want to dial non-ascii host name, take care of its name serialization
// avoiding bad request issues. For more info see net/http Request.Write()
// implementation, especially cleanHost() function.
func (d *Dialer) Dial(ctx context.Context, urlstr string, reqHeader http.Header) (cli *WsClient, resHeader http.Header, err error) {
	u, err := parseAndValidateURL(urlstr)
	if err != nil {
		return nil, nil, err
	}
	deadline := time.Now().Add(d.Timeout)
	if d.NetDial == nil {
		d.NetDial = defaultNetDial
	}
	conn, err := d.NetDial("tcp", u.Host, d.Timeout)
	if err != nil {
		return nil, nil, err
	}
	nonce := newNonce()
	if err = sendUpgradeRequest(conn.Writer(), u, nonce, d.Protocols, d.Extensions, reqHeader, d.Host); err != nil {
		return nil, nil, err
	}
	conn.SetReadTimeout(deadline.Sub(time.Now()))
	defer conn.SetReadTimeout(0)

	stopFunc := startContextMonitoring(ctx, conn)
	defer func() {
		stopFunc()
		err = MergeContextError(ctx.Err(), err)
	}()

	resHeader, err = d.handleUpgradeResponse(conn.Reader(), nonce)
	if err != nil {
		return nil, nil, err
	}
	cli = newWsClient(conn)
	return
}

// StatusError contains an unexpected status-line code from the server.
type StatusError int

func (s StatusError) Error() string {
	return "unexpected HTTP response status: " + strconv.Itoa(int(s))
}

func isTimeoutError(err error) bool {
	// 检查 netpoll 的特定超时错误
	return errors.Is(err, netpoll.ErrReadTimeout) || errors.Is(err, netpoll.ErrWriteTimeout)
}

// startContextMonitoring is a helper function that starts connection I/O
// interrupter goroutine.
//
// Started goroutine calls SetDeadline() with long time ago value when context
// become expired to make any I/O operations failed. It returns done function
// that stops started goroutine and maps error received from conn I/O methods
// to possible context expiration error.
//
// In concern with possible SetDeadline() call inside interrupter goroutine,
// caller passes pointer to its I/O error (even if it is nil) to done(&err).
// That is, even if I/O error is nil, context could be already expired and
// connection "poisoned" by SetDeadline() call. In that case done(&err) will
// store at *err ctx.Err() result. If err is caused not by timeout, it will
// leaved untouched.
// setupContextDeadline
func startContextMonitoring(ctx context.Context, conn netpoll.Connection) (stopFunc func()) {
	done := make(chan struct{})
	// 启动监控协程
	go func() {
		select {
		case <-done:
		case <-ctx.Done():
			_ = conn.Close()
		}
	}()
	return func() { close(done) }
}

// MergeContextError Map Upgrade() error to a possible context expiration error. That
// is, even if Upgrade() err is nil, context could be already
// expired and connection be "poisoned" by SetDeadline() call.
// In that case we must not return ctx.Err() error.
func MergeContextError(ctxError error, err error) error {
	// ctxError优先级比正常处理流程的优先级高
	if ctxError != nil {
		return ctxError
	}
	return err
}

func (d *Dialer) handleUpgradeResponse(br netpoll.Reader, nonce []byte) (resHeader http.Header, err error) {

	// Begin validation of the response.
	// See https://tools.ietf.org/html/rfc6455#section-4.2.2
	// Parse request line data like HTTP version, uri and method.
	// 阶段1: 读取并验证响应行
	respLine, err := readResponseLine(br)
	if err != nil {
		return nil, fmt.Errorf("failed to read response line: %w", err)
	}
	// 阶段2: 验证HTTP协议版本
	// Even if RFC says "1.1 or higher" without mentioning the part of the
	// version, we apply it only to minor part.
	if err = validateHTTPVersion(respLine.major, respLine.minor); err != nil {
		return nil, err
	}

	// 阶段3: 验证状态码是否为101
	if respLine.status != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("statusCode:%v,reason:%s\n", respLine.status, respLine.reason)
	}

	// 阶段4: 验证必要头部字段
	return d.parseAndValidateHeaders(br, nonce)
}

// 定义头部标记位
const (
	headerFlagUpgrade = 1 << iota
	headerFlagConnection
	headerFlagSecAccept
	headerFlagAll = headerFlagUpgrade | headerFlagConnection | headerFlagSecAccept
)

// 阶段4: 使用位标记的头部验证
func (d *Dialer) parseAndValidateHeaders(r netpoll.Reader, nonce []byte) (resHeader http.Header, err error) {
	resHeader, seenFlags := make(http.Header), byte(0)
	var line []byte
	for {
		line, err = readLine(r)
		if err != nil {
			return nil, err
		}
		if len(line) == 0 {
			break
		}
		k, v, ok := httpParseHeaderLine(line)
		if !ok {
			return nil, ErrMalformedResponse
		}
		key := btsToString(k)
		switch key {
		case headerUpgradeCanonical:
			if !bytes.EqualFold(v, specHeaderValueUpgrade) {
				return nil, ErrHandshakeBadUpgrade
			}
			seenFlags |= headerFlagUpgrade

		case headerConnectionCanonical:
			if !bytes.EqualFold(v, specHeaderValueConnection) {
				return nil, ErrHandshakeBadConnection
			}
			seenFlags |= headerFlagConnection

		case headerSecAcceptCanonical:
			if !checkAcceptFromNonce(v, nonce) {
				return nil, ErrHandshakeBadSecAccept
			}
			seenFlags |= headerFlagSecAccept

		case headerSecProtocolCanonical:
			if err = processSubProtocol(d.Protocols, v, resHeader); err != nil {
				return nil, err
			}

		case headerSecExtensionsCanonical:
			// TODO: 扩展处理逻辑
		default:
			resHeader.Set(key, string(v))
		}
	}
	err = seekSeenFlags(seenFlags)
	return
}

func seekSeenFlags(seen byte) error {
	if seen != headerFlagAll {
		var missingHeaders []string
		if seen&headerFlagUpgrade == 0 {
			missingHeaders = append(missingHeaders, headerUpgradeCanonical)
		}
		if seen&headerFlagConnection == 0 {
			missingHeaders = append(missingHeaders, headerConnectionCanonical)
		}
		if seen&headerFlagSecAccept == 0 {
			missingHeaders = append(missingHeaders, headerSecAcceptCanonical)
		}
		return newMissHeaderError(missingHeaders)
	}
	return nil
}

func newMissHeaderError(missingHeaders []string) error {
	return fmt.Errorf("missing required headers: %v", missingHeaders)
}
