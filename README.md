# github.com/someview/wsclient
websocket 高性能websocket客户端, 底层采用netpoll, 添加对象池和协程池的实现

## 特性说明
1. 发送数据并发安全
2. 内置协程池，ws帧池，bytes池
3. 不支持window
4. 不支持websocket extension

## 使用说明
参考example和test


## 开发说明
- mock文件生成
`mockgen -package mymock -destination mocks/reader_mock.go netpoll Reader,Writer`
- 测试包 `go test -v -gcflags=all=-l ./...`