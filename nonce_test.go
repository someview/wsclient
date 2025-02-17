package wsclient

import "testing"

func BenchmarkInitAcceptFromNonce(b *testing.B) {
	dst := make([]byte, acceptSize)
	nonce := make([]byte, nonceSize)
	initNonce(nonce)
	for i := 0; i < b.N; i++ {
		initAcceptFromNonce(dst, nonce)
	}
}
