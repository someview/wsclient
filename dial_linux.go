//go:build !windows

package wsclient

import (
	"github.com/cloudwego/netpoll"
)

var defaultNetDial = netpoll.DialConnection
