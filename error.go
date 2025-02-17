package wsclient

import (
	"fmt"
	"net/http"
)

// RejectOption represents an option used to control the way connection is
// rejected.
type RejectOption func(*ConnectionRejectedError)

// RejectionReason returns an option that makes connection to be rejected with
// given reason.
func RejectionReason(reason string) RejectOption {
	return func(err *ConnectionRejectedError) {
		err.reason = reason
	}
}

// RejectionStatus returns an option that makes connection to be rejected with
// given HTTP status code.
func RejectionStatus(code int) RejectOption {
	return func(err *ConnectionRejectedError) {
		err.code = code
	}
}

// RejectionHeader returns an option that makes connection to be rejected with
// given HTTP headers.
func RejectionHeader(h http.Header) RejectOption {
	return func(err *ConnectionRejectedError) {
		err.header = h
	}
}

// RejectConnectionError constructs an error that could be used to control the
// way handshake is rejected by Upgrader.
func RejectConnectionError(options ...RejectOption) error {
	err := new(ConnectionRejectedError)
	for _, opt := range options {
		opt(err)
	}
	return err
}

// ConnectionRejectedError represents a rejection of connection during
// WebSocket handshake error.
//
// It can be returned by Upgrader's On* hooks to indicate that WebSocket
// handshake should be rejected.
type ConnectionRejectedError struct {
	reason string
	code   int
	header http.Header
}

// Error implements error interface.
func (r *ConnectionRejectedError) Error() string {
	return r.reason
}

func (r *ConnectionRejectedError) StatusCode() int {
	return r.code
}

// Errors used by both client and server when preparing WebSocket handshake.
var (
	ErrHandshakeBadProtocol = RejectConnectionError(
		RejectionStatus(http.StatusHTTPVersionNotSupported),
		RejectionReason("handshake error: bad HTTP protocol version"),
	)
	ErrHandshakeBadMethod = RejectConnectionError(
		RejectionStatus(http.StatusMethodNotAllowed),
		RejectionReason("handshake error: bad HTTP request method"),
	)
	ErrHandshakeBadHost = RejectConnectionError(
		RejectionStatus(http.StatusBadRequest),
		RejectionReason(fmt.Sprintf("handshake error: bad %q header", headerHost)),
	)
	ErrHandshakeBadUpgrade = RejectConnectionError(
		RejectionStatus(http.StatusBadRequest),
		RejectionReason(fmt.Sprintf("handshake error: bad %q header", headerUpgrade)),
	)
	ErrHandshakeBadConnection = RejectConnectionError(
		RejectionStatus(http.StatusBadRequest),
		RejectionReason(fmt.Sprintf("handshake error: bad %q header", headerConnection)),
	)
	ErrHandshakeBadSecAccept = RejectConnectionError(
		RejectionStatus(http.StatusBadRequest),
		RejectionReason(fmt.Sprintf("handshake error: bad %q header", headerSecAccept)),
	)
	ErrHandshakeBadSecKey = RejectConnectionError(
		RejectionStatus(http.StatusBadRequest),
		RejectionReason(fmt.Sprintf("handshake error: bad %q header", headerSecKey)),
	)
	ErrHandshakeBadSecVersion = RejectConnectionError(
		RejectionStatus(http.StatusBadRequest),
		RejectionReason(fmt.Sprintf("handshake error: bad %q header", headerSecVersion)),
	)
)

// ErrMalformedResponse is returned by Dialer to indicate that server response
// can not be parsed.
var ErrMalformedResponse = fmt.Errorf("malformed HTTP response")

// ErrMalformedRequest is returned when HTTP request can not be parsed.
var ErrMalformedRequest = RejectConnectionError(
	RejectionStatus(http.StatusBadRequest),
	RejectionReason("malformed HTTP request"),
)

// ErrHandshakeUpgradeRequired is returned by Upgrader to indicate that
// connection is rejected because given WebSocket version is malformed.
//
// According to RFC6455:
// If this version does not match a version understood by the server, the
// server MUST abort the WebSocket handshake described in this section and
// instead send an appropriate HTTP error code (such as 426 Upgrade Required)
// and a |Sec-WebSocket-Version| header field indicating the version(s) the
// server is capable of understanding.
var ErrHandshakeUpgradeRequired = RejectConnectionError(
	RejectionStatus(http.StatusUpgradeRequired),
	RejectionHeader(http.Header{headerSecVersion: []string{"13"}}),
	RejectionReason(fmt.Sprintf("handshake error: bad %q header", headerSecVersion)),
)

// ErrNotHijacker is an error returned when http.ResponseWriter does not
// implement http.Hijacker interface.
var ErrNotHijacker = RejectConnectionError(
	RejectionStatus(http.StatusInternalServerError),
	RejectionReason("given http.ResponseWriter is not a http.Hijacker"),
)

// 协议违规（RSV/长度/操作码）     立即关闭连接              1002
// 掩码缺失                     立即关闭连接              1002
// IO错误/超时                  重试或返回应用层
// 应用层关闭                   正常关闭流程               1000
//

var (
	ErrInvalidFrame = fmt.Errorf("帧格式错误")
)
