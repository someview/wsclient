package httphead

import (
	"github.com/cloudwego/netpoll"
)

var (
	comma     = []byte{','}
	equality  = []byte{'='}
	semicolon = []byte{';'}
	quote     = []byte{'"'}
	escape    = []byte{'\\'}
)

// WriteOptions write options list to the dest.
// It uses the same form as {Scan,Parse}Options functions:
// values = 1#value
// value = token *( ";" param )
// param = token [ "=" (token | quoted-string) ]
//
// It wraps valuse into the quoted-string sequence if it contains any
// non-token characters.
func WriteOptions(dest netpoll.Writer, options []Option) (n int, err error) {

	for i, opt := range options {
		if i > 0 {
			dest.WriteBinary(comma)
		}

		writeTokenSanitized(dest, opt.Name)

		for _, p := range opt.Parameters.data() {
			dest.WriteBinary(semicolon)
			writeTokenSanitized(dest, p.key)
			if len(p.value) != 0 {
				dest.WriteBinary(equality)
				writeTokenSanitized(dest, p.value)
			}
		}
	}
	//fixme  这里的返回结果没有用到，考虑不返回这个字段
	return 0, err
}

// writeTokenSanitized writes token as is or as quouted string if it contains
// non-token characters.
//
// Note that is is not expects LWS sequnces be in s, cause LWS is used only as
// header field continuation:
// "A CRLF is allowed in the definition of TEXT only as part of a header field
// continuation. It is expected that the folding LWS will be replaced with a
// single SP before interpretation of the TEXT value."
// See https://tools.ietf.org/html/rfc2616#section-2
//
// That is we sanitizing s for writing, so there could not be any header field
// continuation.
// That is any CRLF will be escaped as any other control characters not allowd in TEXT.
func writeTokenSanitized(bw netpoll.Writer, bts []byte) {
	var qt bool
	var pos int
	for i := 0; i < len(bts); i++ {
		c := bts[i]
		if !OctetTypes[c].IsToken() && !qt {
			qt = true
			bw.WriteBinary(quote)
		}
		if OctetTypes[c].IsControl() || c == '"' {
			if !qt {
				qt = true
				bw.WriteBinary(quote)
			}
			bw.WriteBinary(bts[pos:i])
			bw.WriteBinary(escape)
			bw.WriteBinary(bts[i : i+1])
			pos = i + 1
		}
	}
	if !qt {
		bw.WriteBinary(bts)
	} else {
		bw.WriteBinary(bts[pos:])
		bw.WriteBinary(quote)
	}
}

// todo 添加extension支持
//func matchSelectedExtensions(selected []byte, wanted, received []httphead.Option) ([]httphead.Option, error) {
//	if len(selected) == 0 {
//		return received, nil
//	}
//	var (
//		index  int
//		option httphead.Option
//		err    error
//	)
//	index = -1
//	match := func() (ok bool) {
//		for _, want := range wanted {
//			// A server accepts one or more extensions by including a
//			// |Sec-WebSocket-Extensions| header field containing one or more
//			// extensions that were requested by the client.
//			//
//			// The interpretation of any extension parameters, and what
//			// constitutes a valid response by a server to a requested set of
//			// parameters by a client, will be defined by each such extension.
//			if bytes.Equal(option.Name, want.Name) {
//				// Check parsed extension to be present in client
//				// requested extensions. We move matched extension
//				// from client list to avoid allocation of httphead.Option.Name,
//				// httphead.Option.Parameters have to be copied from the header
//				want.Parameters, _ = option.Parameters.Copy(make([]byte, option.Parameters.Size()))
//				received = append(received, want)
//				return true
//			}
//		}
//		return false
//	}
//	ok := httphead.ScanOptions(selected, func(i int, name, attr, val []byte) httphead.Control {
//		if i != index {
//			// Met next option.
//			index = i
//			if i != 0 && !match() {
//				// Server returned non-requested extension.
//				err = ErrHandshakeBadExtensions
//				return httphead.ControlBreak
//			}
//			option = httphead.Option{Name: name}
//		}
//		if attr != nil {
//			option.Parameters.Set(attr, val)
//		}
//		return httphead.ControlContinue
//	})
//	if !ok {
//		err = ErrMalformedResponse
//		return received, err
//	}
//	if !match() {
//		return received, ErrHandshakeBadExtensions
//	}
//	return received, err
//}
