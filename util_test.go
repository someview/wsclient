package wsclient

import (
	"bytes"
	"github.com/cloudwego/netpoll"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"net/textproto"
	"testing"
)

func TestStrToBytes(t *testing.T) {
	str := "12345"
	assert.Equal(t, str, btsToString(strToBytes(str)))

}

func Test_readLine(t *testing.T) {
	suite.Run(t, new(ReadLineTestSuite))
}

type ReadLineTestSuite struct {
	suite.Suite
	ctrl *gomock.Controller
}

func (s *ReadLineTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
}

func (s *ReadLineTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ReadLineTestSuite) TestSucceedReadLine() {
	readerMock := NewMockReader(s.ctrl)
	mockData := []byte("12345\r\n")
	readerMock.EXPECT().Until(gomock.Any()).Return(mockData, nil).Times(1)
	res, err := readLine(readerMock)
	s.Nil(err, "没有错误发生")
	s.Len(res, 5)
}

func (s *ReadLineTestSuite) TestFailedReadLine() {
	readerMock := NewMockReader(s.ctrl)
	readerMock.EXPECT().Until(gomock.Any()).Return(nil, assert.AnError).Times(1)
	res, err := readLine(readerMock)
	s.NotNil(err, "有错误发生")
	s.Nil(res)
}

func (s *ReadLineTestSuite) TestMultiReadLine() {
	lb := netpoll.NewLinkBuffer()
	lb.WriteString("123\r\n456\r\n")
	lb.Flush()
	buf1, err := readLine(lb)
	s.Nil(err, "有错误发生")
	s.Equal(string(buf1), "123")
	buf2, err := readLine(lb)
	s.Nil(err, "有错误发生")
	s.Equal(string(buf2), "456")
}

func TestBSplit3(t *testing.T) {
	for _, test := range []struct {
		bts  []byte
		sep  byte
		exp1 []byte
		exp2 []byte
		exp3 []byte
	}{
		{[]byte(""), ' ', []byte{}, nil, nil},
		{[]byte("GET / HTTP/1.1"), ' ', []byte("GET"), []byte("/"), []byte("HTTP/1.1")},
	} {
		t.Run(string(test.bts), func(t *testing.T) {
			b1, b2, b3 := bsplit3(test.bts, test.sep)
			if !bytes.Equal(b1, test.exp1) || !bytes.Equal(b2, test.exp2) || !bytes.Equal(b3, test.exp3) {
				t.Errorf(
					"bsplit3(%q) = %q, %q, %q; want %q, %q, %q",
					string(test.bts), string(b1), string(b2), string(b3),
					string(test.exp1), string(test.exp2), string(test.exp3),
				)
			}
		})
	}
}

func TestAsciiToInt(t *testing.T) {
	for _, test := range []struct {
		bts []byte
		exp int
		err bool
	}{
		{[]byte{'0'}, 0, false},
		{[]byte{'1'}, 1, false},
		{[]byte("42"), 42, false},
		{[]byte("420"), 420, false},
		{[]byte("010050042"), 10050042, false},
	} {
		t.Run(string(test.bts), func(t *testing.T) {
			act, err := asciiToInt(test.bts)
			if (test.err && err == nil) || (!test.err && err != nil) {
				t.Errorf("unexpected error: %v", err)
			}
			if act != test.exp {
				t.Errorf("asciiToInt(%v) = %v; want %v", test.bts, act, test.exp)
			}
		})
	}
}

func TestBtrim(t *testing.T) {
	for _, test := range []struct {
		bts []byte
		exp []byte
	}{
		{[]byte("abc"), []byte("abc")},
		{[]byte(" abc"), []byte("abc")},
		{[]byte("abc "), []byte("abc")},
		{[]byte(" abc "), []byte("abc")},
	} {
		t.Run(string(test.bts), func(t *testing.T) {
			if act := btrim(test.bts); !bytes.Equal(act, test.exp) {
				t.Errorf("btrim(%v) = %v; want %v", test.bts, act, test.exp)
			}
		})
	}
}

var canonicalHeaderCases = [][]byte{
	[]byte("foo-"),
	[]byte("-foo"),
	[]byte("-"),
	[]byte("foo----bar"),
	[]byte("foo-bar"),
	[]byte("FoO-BaR"),
	[]byte("Foo-Bar"),
	[]byte("sec-websocket-extensions"),
}

func TestCanonicalizeHeaderKey(t *testing.T) {
	for _, bts := range canonicalHeaderCases {
		t.Run(string(bts), func(t *testing.T) {
			act := append([]byte(nil), bts...)
			canonicalizeHeaderKey(act)

			exp := strToBytes(textproto.CanonicalMIMEHeaderKey(string(bts)))

			if !bytes.Equal(act, exp) {
				t.Errorf(
					"canonicalizeHeaderKey(%v) = %v; want %v",
					string(bts), string(act), string(exp),
				)
			}
		})
	}
}

func BenchmarkCanonicalizeHeaderKey(b *testing.B) {
	for _, bts := range canonicalHeaderCases {
		b.Run(string(bts), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				canonicalizeHeaderKey(bts)
			}
		})
	}
}
