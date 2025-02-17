package main

import (
	"bytes"
	"context"
	"github.com/gorilla/websocket"
	"github.com/someview/wsclient"
	"log"
	"log/slog"
	"net/http"
	"time"
)

var upgrader = websocket.Upgrader{}

func handler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error during connection upgrade:", err)
		return
	}
	defer conn.Close()
	for {
		tp, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error during message reading:", err)
			break
		}
		slog.Info("接收消息", slog.Int("server receive msg size", len(message)))
		if err = conn.WriteMessage(tp, message); err != nil {
			slog.Info("server发送消息", slog.String("server send error", string(message)))
			conn.Close()
			break
		}
	}
}

func runServer() {
	http.HandleFunc("/ws", handler)
	log.Println("Server started at ws://localhost:8080/ws")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

func main() {
	go runServer()
	time.Sleep(time.Second)
	cli, _, err := wsclient.DialContext(context.Background(), "ws://localhost:8080/ws", nil)
	if err != nil {
		slog.Info("dial err:", slog.String("err", err.Error()))
		return
	}
	cli.AddCallback(func(frame *wsclient.WsFrame) {
		slog.Info("frame:", slog.Int("opcode",
			int(frame.OpCode)), slog.Bool("masked", frame.Masked), slog.Int("payload size", len(frame.Payload)))
	}, func() {}, nil)
	slog.Info("cli:", slog.Bool("cli.IsActive", cli.IsActive()))
	err = cli.WriteBinary([]byte("hello")) // small size
	if err != nil {
		slog.Info("dial err:", slog.String("client write err", err.Error()))
		return
	}
	err = cli.WriteBinary(bytes.Repeat([]byte("hello"), 26)) // middle size
	if err != nil {
		slog.Info("dial err:", slog.String("client write err", err.Error()))
		return
	}

	err = cli.WriteBinary(bytes.Repeat([]byte("hello"), 20000)) // big size
	if err != nil {
		slog.Info("dial err:", slog.String("client write err", err.Error()))
		return
	}
	time.Sleep(time.Second)
}
