//go:build windows

package wsclient

import (
	"github.com/cloudwego/netpoll"
	"time"
)

var defaultNetDial = func(network, address string, timeout time.Duration) (connection netpoll.Connection, err error) {
	return nil, netpoll.ErrDialTimeout
}
