package wsclient

import (
	"bytes"
	"fmt"
	"github.com/someview/wsclient/httphead"
	"unsafe"

	"github.com/cloudwego/netpoll"
)

// do not modify returned buffer
func readLine(r netpoll.Reader) (line []byte, err error) {
	line, err = r.Until('\n')
	if err != nil {
		return
	}
	l := len(line)
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return nil, fmt.Errorf("invalid HTTP line ending, got %q", line)
	}
	return line[:l-2], nil
}

// SelectFromSlice creates accept function that could be used as Protocol/Extension
// select during upgrade.
func SelectFromSlice(accept []string) func(string) bool {
	if len(accept) > 16 {
		mp := make(map[string]struct{}, len(accept))
		for _, p := range accept {
			mp[p] = struct{}{}
		}
		return func(p string) bool {
			_, ok := mp[p]
			return ok
		}
	}
	return func(p string) bool {
		for _, ok := range accept {
			if p == ok {
				return true
			}
		}
		return false
	}
}

// SelectEqual creates accept function that could be used as Protocol/Extension
// select during upgrade.
func SelectEqual(v string) func(string) bool {
	return func(p string) bool {
		return v == p
	}
}

// asciiToInt converts bytes to int.
func asciiToInt(bts []byte) (ret int, err error) {
	// ASCII numbers all start with the high-order bits 0011.
	// If you see that, and the next bits are 0-9 (0000 - 1001) you can grab those
	// bits and interpret them directly as an integer.
	var n int
	if n = len(bts); n < 1 {
		return 0, fmt.Errorf("converting empty bytes to int")
	}
	for i := 0; i < n; i++ {
		if bts[i]&0xf0 != 0x30 {
			return 0, fmt.Errorf("%s is not a numeric character", string(bts[i]))
		}
		ret += int(bts[i]&0xf) * pow(10, n-i-1)
	}
	return ret, nil
}

// pow for integers implementation.
// See Donald Knuth, The Art of Computer Programming, Volume 2, Section 4.6.3.
func pow(a, b int) int {
	p := 1
	for b > 0 {
		if b&1 != 0 {
			p *= a
		}
		b >>= 1
		a *= a
	}
	return p
}

func bsplit3(bts []byte, sep byte) (b1, b2, b3 []byte) {
	a := bytes.IndexByte(bts, sep)
	b := bytes.IndexByte(bts[a+1:], sep)
	if a == -1 || b == -1 {
		return bts, nil, nil
	}
	b += a + 1
	return bts[:a], bts[a+1 : b], bts[b+1:]
}

func btrim(bts []byte) []byte {
	var i, j int
	for i = 0; i < len(bts) && (bts[i] == ' ' || bts[i] == '\t'); {
		i++
	}
	for j = len(bts); j > i && (bts[j-1] == ' ' || bts[j-1] == '\t'); {
		j--
	}
	return bts[i:j]
}

func strHasToken(header, token string) (has bool) {
	return btsHasToken(strToBytes(header), strToBytes(token))
}

func btsHasToken(header, token []byte) (has bool) {
	httphead.ScanTokens(header, func(v []byte) bool {
		has = bytes.EqualFold(v, token)
		return !has
	})
	return has
}

const (
	toLower  = 'a' - 'A'      // for use with OR.
	toUpper  = ^byte(toLower) // for use with AND.
	toLower8 = uint64(toLower) |
		uint64(toLower)<<8 |
		uint64(toLower)<<16 |
		uint64(toLower)<<24 |
		uint64(toLower)<<32 |
		uint64(toLower)<<40 |
		uint64(toLower)<<48 |
		uint64(toLower)<<56
)

// Algorithm below is like standard textproto/CanonicalMIMEHeaderKey, except
// that it operates with slice of bytes and modifies it inplace without copying.
func canonicalizeHeaderKey(k []byte) {
	upper := true
	for i, c := range k {
		if upper && 'a' <= c && c <= 'z' {
			k[i] &= toUpper
		} else if !upper && 'A' <= c && c <= 'Z' {
			k[i] |= toLower
		}
		upper = c == '-'
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func nonZero(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

func strToBytes(str string) []byte {
	return unsafe.Slice(unsafe.StringData(str), len(str))
}

func btsToString(bts []byte) (str string) {
	return unsafe.String(unsafe.SliceData(bts), len(bts))
}
