package wsclient

import (
	"bytes"
	"github.com/cloudwego/netpoll"
	"github.com/someview/wsclient/httphead"
	"net"
	"net/http"
	"net/url"
)

const (
	crlf          = "\r\n"
	colonAndSpace = ": "
	commaAndSpace = ", "
)

const (
	textHeadUpgrade = "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
)

const (
	// Every new header must be added to TestHeaderNames test.
	headerHost          = "Host"
	headerUpgrade       = "Upgrade"
	headerConnection    = "Connection"
	headerSecVersion    = "Sec-WebSocket-Version"
	headerSecProtocol   = "Sec-WebSocket-Protocol"
	headerSecExtensions = "Sec-WebSocket-Extensions"
	headerSecKey        = "Sec-WebSocket-Key"
	headerSecAccept     = "Sec-WebSocket-Accept"

	headerHostCanonical          = headerHost
	headerUpgradeCanonical       = headerUpgrade
	headerConnectionCanonical    = headerConnection
	headerSecVersionCanonical    = "Sec-Websocket-Version"
	headerSecProtocolCanonical   = "Sec-Websocket-Protocol"
	headerSecExtensionsCanonical = "Sec-Websocket-Extensions"
	headerSecKeyCanonical        = "Sec-Websocket-Key"
	headerSecAcceptCanonical     = "Sec-Websocket-Accept"
)

var (
	specHeaderValueUpgrade         = []byte("websocket")
	specHeaderValueConnection      = []byte("Upgrade")
	specHeaderValueConnectionLower = []byte("upgrade")
	specHeaderValueSecVersion      = []byte("13")
)

var (
	httpVersion1_0    = []byte("HTTP/1.0")
	httpVersion1_1    = []byte("HTTP/1.1")
	httpVersionPrefix = []byte("HTTP/")
)

type httpRequestLine struct {
	method, uri  []byte
	major, minor int
}

type httpResponseLine struct {
	major, minor int
	status       int
	reason       []byte
}

// httpParseRequestLine parses http request line like "GET / HTTP/1.0".
func httpParseRequestLine(line []byte) (req httpRequestLine, err error) {
	var proto []byte
	req.method, req.uri, proto = bsplit3(line, ' ')

	var ok bool
	req.major, req.minor, ok = httpParseVersion(proto)
	if !ok {
		err = ErrMalformedRequest
	}
	return req, err
}

func httpParseResponseLine(line []byte) (resp httpResponseLine, err error) {
	var (
		proto  []byte
		status []byte
	)
	proto, status, resp.reason = bsplit3(line, ' ')

	var ok bool
	resp.major, resp.minor, ok = httpParseVersion(proto)
	if !ok {
		return resp, ErrMalformedResponse
	}

	var convErr error
	resp.status, convErr = asciiToInt(status)
	if convErr != nil {
		return resp, ErrMalformedResponse
	}

	return resp, nil
}

// httpParseVersion parses major and minor version of HTTP protocol. It returns
// parsed values and true if parse is ok.
func httpParseVersion(bts []byte) (major, minor int, ok bool) {
	switch {
	case bytes.Equal(bts, httpVersion1_0):
		return 1, 0, true
	case bytes.Equal(bts, httpVersion1_1):
		return 1, 1, true
	case len(bts) < 8:
		return 0, 0, false
	case !bytes.Equal(bts[:5], httpVersionPrefix):
		return 0, 0, false
	}

	bts = bts[5:]

	dot := bytes.IndexByte(bts, '.')
	if dot == -1 {
		return 0, 0, false
	}
	var err error
	major, err = asciiToInt(bts[:dot])
	if err != nil {
		return major, 0, false
	}
	minor, err = asciiToInt(bts[dot+1:])
	if err != nil {
		return major, minor, false
	}

	return major, minor, true
}

// httpParseHeaderLine parses HTTP header as key-value pair. It returns parsed
// values and true if parse is ok.
func httpParseHeaderLine(line []byte) (k, v []byte, ok bool) {
	colon := bytes.IndexByte(line, ':')
	if colon == -1 {
		return nil, nil, false
	}

	k = btrim(line[:colon])
	// TODO(gobwas): maybe use just lower here?
	canonicalizeHeaderKey(k)

	v = btrim(line[colon+1:])

	return k, v, true
}

func sendUpgradeRequest(
	bw netpoll.Writer,
	u *url.URL,
	nonce []byte,
	protocols []string,
	extensions []httphead.Option,
	header http.Header,
	host string,
) error {
	bw.WriteString("GET ")
	bw.WriteString(u.RequestURI())
	bw.WriteString(" HTTP/1.1\r\n")

	if host == "" {
		host = u.Host
	}
	HttpWriteHeader(bw, headerHost, host)

	HttpWriteHeaderBts(bw, headerUpgrade, specHeaderValueUpgrade)
	HttpWriteHeaderBts(bw, headerConnection, specHeaderValueConnection)
	HttpWriteHeaderBts(bw, headerSecVersion, specHeaderValueSecVersion)

	// NOTE: write nonce bytes as a string to prevent heap allocation –
	// WriteString() copy given string into its inner buffer, unlike Write()
	// which may write p directly to the underlying io.Writer – which in turn
	// will lead to p escape.
	HttpWriteHeader(bw, headerSecKey, btsToString(nonce))

	if len(protocols) > 0 {
		HttpWriteHeaderKey(bw, headerSecProtocol)
		for i, p := range protocols {
			if i > 0 {
				bw.WriteString(commaAndSpace)
			}
			bw.WriteString(p)
		}
		bw.WriteString(crlf)
	}

	if len(extensions) > 0 {
		HttpWriteHeaderKey(bw, headerSecExtensions)
		httphead.WriteOptions(bw, extensions)
		bw.WriteString(crlf)
	}

	if header != nil {
		for key, values := range header {
			for _, value := range values {
				HttpWriteHeader(bw, key, value)
			}
		}
	}
	bw.WriteString(crlf)
	return bw.Flush()
}

func HttpWriteHeader(bw netpoll.Writer, key, value string) {
	HttpWriteHeaderKey(bw, key)
	bw.WriteString(value)
	bw.WriteString(crlf)
}

func HttpWriteHeaderBts(bw netpoll.Writer, key string, value []byte) {
	HttpWriteHeaderKey(bw, key)
	bw.WriteBinary(value)
	bw.WriteString(crlf)
}

func HttpWriteHeaderKey(bw netpoll.Writer, key string) {
	bw.WriteString(key)
	bw.WriteString(colonAndSpace)
}

func readResponseLine(br netpoll.Reader) (httpResponseLine, error) {
	lineBytes, err := readLine(br)
	if err != nil {
		return httpResponseLine{}, err
	}
	return httpParseResponseLine(lineBytes)
}

func validateHTTPVersion(major, minor int) (err error) {
	if major != 1 || minor < 1 {
		err = ErrHandshakeBadProtocol
	}
	return
}

// ------------------- 辅助函数 -------------------
func parseAndValidateURL(urlStr string) (*url.URL, error) {
	u, err := url.ParseRequestURI(urlStr)
	if err != nil {
		return nil, ErrMalformedURL
	}
	if u.Hostname() == "" {
		return nil, ErrMalformedURL
	}
	switch u.Scheme {
	case "ws", "wss":
		u.Scheme = map[string]string{"ws": "http", "wss": "https"}[u.Scheme]
	default:
		return nil, ErrMalformedURL
	}

	// 新增智能端口处理
	if u.Port() == "" {
		switch u.Scheme {
		case "http":
			u.Host = net.JoinHostPort(u.Hostname(), "80")
		case "https":
			u.Host = net.JoinHostPort(u.Hostname(), "443")
		}
	}

	if u.User != nil {
		return nil, ErrMalformedURL
	}
	return u, nil
}

func processSubProtocol(supportProtocols []string, resSubProtocol []byte, resHeader http.Header) error {
	selected := string(resSubProtocol)
	for _, proto := range supportProtocols {
		if proto == selected {
			resHeader.Set(headerSecProtocol, selected)
			return nil
		}
	}
	return ErrHandshakeBadSubProtocol
}
