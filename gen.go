package wsclient

//go:generate mockgen -destination=netpoll_mock.go -package=github.com/someview/wsclient github.com/cloudwego/netpoll Reader,Writer,Connection
