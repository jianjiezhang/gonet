package web

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"game/lib/packet"
	"protocol/client"
)

// heartbeatEvery 是这条连接上发给玩家口的心跳间隔。
// 页面在后台时浏览器会拖慢定时器，改由本进程写，避免 15 秒被踢。
var heartbeatEvery = 5 * time.Second

// serveSession 把浏览器 WebSocket 上的玩家帧转到本机玩家口，连接保持到任一侧断开。
// 第一帧必须是 login，之后心跳和其它命令都走这一条。
func serveSession(w http.ResponseWriter, r *http.Request, gameAddr string) {
	ws, err := upgrade(w, r)
	if err != nil {
		return
	}
	tcp, err := net.DialTimeout("tcp", gameAddr, 3*time.Second)
	if err != nil {
		slog.Info("session dial", "err", err)
		_ = ws.Close()
		return
	}
	slog.Info("session", "remote", r.RemoteAddr, "game", gameAddr)
	var writeMu sync.Mutex
	writeTCP := func(cmd uint16, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return packet.Write(tcp, cmd, data)
	}
	done := make(chan struct{})
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			close(done)
			_ = tcp.Close()
			_ = ws.Close()
		})
	}
	if heartbeatEvery > 0 {
		go func() {
			tick := time.NewTicker(heartbeatEvery)
			defer tick.Stop()
			for {
				select {
				case <-done:
					return
				case <-tick.C:
					if err := writeTCP(client.HeartbeatID, nil); err != nil {
						closeBoth()
						return
					}
				}
			}
		}()
	}
	go func() {
		defer closeBoth()
		for {
			cmd, data, err := packet.Read(tcp)
			if err != nil {
				if !packet.IsGone(err) && !errors.Is(err, io.EOF) {
					slog.Info("session read", "err", err)
				}
				return
			}
			var buf bytes.Buffer
			if err := packet.Write(&buf, cmd, data); err != nil {
				slog.Info("session frame", "err", err)
				return
			}
			if err := ws.WriteBinary(buf.Bytes()); err != nil {
				return
			}
		}
	}()
	defer closeBoth()
	for {
		op, payload, err := ws.Read()
		if err != nil {
			return
		}
		switch op {
		case opBin:
			if err := forwardFrame(writeTCP, payload); err != nil {
				slog.Info("session forward", "err", err)
				return
			}
		case opPing:
			if err := ws.WritePong(payload); err != nil {
				return
			}
		case opClose:
			return
		default:
			return
		}
	}
}

// forwardFrame 只转发完整的一帧。半包写进玩家口会把这条连接的后续字节错位。
func forwardFrame(write func(uint16, []byte) error, b []byte) error {
	rd := bytes.NewReader(b)
	cmd, data, err := packet.Read(rd)
	if err != nil || rd.Len() != 0 {
		return errors.New("web: 坏帧")
	}
	return write(cmd, data)
}
