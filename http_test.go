package wsclient

import (
	"errors"
	"github.com/cloudwego/netpoll"
	"net/http"
	"net/textproto"
	"net/url"
	"testing"

	"github.com/someview/wsclient/httphead"
)

type httpVersionCase struct {
	in    []byte
	major int
	minor int
	ok    bool
}

var httpVersionCases = []httpVersionCase{
	{[]byte("HTTP/1.1"), 1, 1, true},
	{[]byte("HTTP/1.0"), 1, 0, true},
	{[]byte("HTTP/1.2"), 1, 2, true},
	{[]byte("HTTP/42.1092"), 42, 1092, true},
}

func TestParseHttpVersion(t *testing.T) {
	for _, c := range httpVersionCases {
		t.Run(string(c.in), func(t *testing.T) {
			major, minor, ok := httpParseVersion(c.in)
			if major != c.major || minor != c.minor || ok != c.ok {
				t.Errorf(
					"parseHttpVersion([]byte(%q)) = %v, %v, %v; want %v, %v, %v",
					string(c.in), major, minor, ok, c.major, c.minor, c.ok,
				)
			}
		})
	}
}

func TestHeaderNames(t *testing.T) {
	testCases := []struct {
		have, want string
	}{
		{
			have: headerHost,
			want: headerHostCanonical,
		},
		{
			have: headerUpgrade,
			want: headerUpgradeCanonical,
		},
		{
			have: headerConnection,
			want: headerConnectionCanonical,
		},
		{
			have: headerSecVersion,
			want: headerSecVersionCanonical,
		},
		{
			have: headerSecProtocol,
			want: headerSecProtocolCanonical,
		},
		{
			have: headerSecExtensions,
			want: headerSecExtensionsCanonical,
		},
		{
			have: headerSecKey,
			want: headerSecKeyCanonical,
		},
		{
			have: headerSecAccept,
			want: headerSecAcceptCanonical,
		},
	}

	for _, tc := range testCases {
		if have := textproto.CanonicalMIMEHeaderKey(tc.have); have != tc.want {
			t.Errorf("have %q want %q,", have, tc.want)
		}
	}
}

func BenchmarkParseHttpVersion(b *testing.B) {
	for _, c := range httpVersionCases {
		b.Run(string(c.in), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, _ = httpParseVersion(c.in)
			}
		})
	}
}

// goos: windows
// goarch: amd64
// pkg: github.com/someview/wsclient
// cpu: 12th Gen Intel(R) Core(TM) i9-12900K
// BenchmarkHttpWriteUpgradeRequest
// BenchmarkHttpWriteUpgradeRequest/#00
// BenchmarkHttpWriteUpgradeRequest/#00-24         	 6539334	       170.0 ns/op
// BenchmarkHttpWriteUpgradeRequest/#01
// BenchmarkHttpWriteUpgradeRequest/#01-24         	 8899290	       140.6 ns/op
func BenchmarkHttpWriteUpgradeRequest(b *testing.B) {
	for _, test := range []struct {
		url        *url.URL
		protocols  []string
		extensions []httphead.Option
		headers    http.Header
		host       string
	}{
		{
			url: makeURL("ws://example.org"),
		},
		{
			url:  makeURL("ws://example.org"),
			host: "test-host",
		},
	} {
		bw := netpoll.NewLinkBuffer()
		nonce := make([]byte, nonceSize)
		initNonce(nonce)

		var headers http.Header
		if test.headers != nil {
			headers = test.headers
		}

		b.ResetTimer()
		b.Run("", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				sendUpgradeRequest(bw,
					test.url,
					nonce,
					test.protocols,
					test.extensions,
					headers,
					test.host,
				)
			}
		})
	}
}

func makeURL(s string) *url.URL {
	ret, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return ret
}

func TestParseAndValidateURL(t *testing.T) {
	testCases := []struct {
		name    string
		url     string
		wantErr error
	}{
		// 有效用例
		{
			name: "valid ws url",
			url:  "ws://localhost:8080/chat",
		},
		{
			name: "valid wss url",
			url:  "wss://example.com/api",
		},

		// 无效协议
		{
			name:    "invalid scheme http",
			url:     "http://localhost",
			wantErr: ErrMalformedURL,
		},
		{
			name:    "invalid scheme ftp",
			url:     "ftp://files.com",
			wantErr: ErrMalformedURL,
		},

		// 用户信息
		{
			name:    "with user info",
			url:     "ws://user:pass@localhost",
			wantErr: ErrMalformedURL,
		},

		// 格式错误
		{
			name:    "malformed url",
			url:     "ws://:8080", // 缺少host
			wantErr: ErrMalformedURL,
		},
		{
			name:    "invalid characters",
			url:     "ws://loc{alhost/", // 非法字符
			wantErr: ErrMalformedURL,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAndValidateURL(tc.url)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("Expected error %v, got %v", tc.wantErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateHTTPVersion(t *testing.T) {
	tests := []struct {
		name    string
		major   int
		minor   int
		wantErr error
	}{
		// 有效用例
		{
			name:    "HTTP/1.1",
			major:   1,
			minor:   1,
			wantErr: nil,
		},
		{
			name:    "HTTP/1.2",
			major:   1,
			minor:   2,
			wantErr: nil,
		},

		// 无效用例
		{
			name:    "HTTP/1.0",
			major:   1,
			minor:   0,
			wantErr: ErrHandshakeBadProtocol,
		},
		{
			name:    "HTTP/2.0",
			major:   2,
			minor:   0,
			wantErr: ErrHandshakeBadProtocol,
		},
		{
			name:    "HTTP/0.9",
			major:   0,
			minor:   9,
			wantErr: ErrHandshakeBadProtocol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHTTPVersion(tt.major, tt.minor)

			// 错误类型断言
			if (err != nil) != (tt.wantErr != nil) {
				t.Fatalf("预期错误存在性: %v, 实际: %v", tt.wantErr != nil, err != nil)
			}

			// 错误内容匹配
			if err != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("期望错误: %v, 实际错误: %v", tt.wantErr, err)
			}
		})
	}
}

//| 测试场景                 | 预期结果                  |
//|-------------------------|--------------------------|
//| 精确匹配                 | 头信息添加成功           |
//| 无匹配协议               | 返回错误                 |
//| 客户端未配置协议         | 拒绝任何服务端协议       |
//| 服务端返回空协议         | 拒绝空值                 |
//| 大小写敏感匹配           | 严格区分大小写           |
//| 服务端返回多协议         | 拒绝非法格式             |

func TestProcessSubProtocol(t *testing.T) {
	tests := []struct {
		name        string   // 测试用例名称
		dialerProto []string // Dialer配置的协议列表
		respProto   []byte   // 服务端返回的协议值
		wantHeader  string   // 期望header中的协议值
		wantError   error    // 期望的错误类型
	}{
		// 用例1: 服务端返回协议匹配客户端配置
		{
			name:        "exact match",
			dialerProto: []string{"chat", "notification"},
			respProto:   []byte("chat"),
			wantHeader:  "chat",
			wantError:   nil,
		},
		// 用例2: 服务端协议不在客户端列表
		{
			name:        "no match",
			dialerProto: []string{"binary", "json"},
			respProto:   []byte("xml"),
			wantHeader:  "",
			wantError:   ErrHandshakeBadSubProtocol,
		},
		// 用例3: 客户端未配置协议
		{
			name:        "empty client protocols",
			dialerProto: nil,
			respProto:   []byte("proto1"),
			wantHeader:  "",
			wantError:   ErrHandshakeBadSubProtocol,
		},
		// 用例4: 服务端返回空协议
		{
			name:        "empty server protocol",
			dialerProto: []string{"a", "b"},
			respProto:   []byte(""),
			wantHeader:  "",
			wantError:   ErrHandshakeBadSubProtocol,
		},
		// 用例5: 大小写敏感匹配
		{
			name:        "case sensitive match",
			dialerProto: []string{"Chat"},
			respProto:   []byte("chat"), // 大小写不匹配
			wantHeader:  "",
			wantError:   ErrHandshakeBadSubProtocol,
		},
		// 用例6: 服务端返回多个协议（非法）
		{
			name:        "multiple protocols",
			dialerProto: []string{"a", "b", "c"},
			respProto:   []byte("a, b"), // 根据RFC应返回单个协议
			wantHeader:  "",
			wantError:   ErrHandshakeBadSubProtocol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 初始化Dialer和header
			header := make(http.Header)
			// 执行处理
			err := processSubProtocol(tt.dialerProto, tt.respProto, header)

			// 验证错误
			if (err != nil) != (tt.wantError != nil) {
				t.Fatalf("错误存在性不匹配: 预期错误=%v, 实际错误=%v", tt.wantError, err)
			}
			if tt.wantError != nil && !errors.Is(err, tt.wantError) {
				t.Fatalf("错误类型不匹配: 预期=%v, 实际=%v", tt.wantError, err)
			}

			// 验证header
			got := header.Get(headerSecProtocol)
			if got != tt.wantHeader {
				t.Fatalf("协议头不匹配: 预期=%q, 实际=%q", tt.wantHeader, got)
			}
		})
	}
}
