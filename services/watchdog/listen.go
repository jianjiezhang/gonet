package watchdog

import (
	"errors"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"gonet"
)

const (
	loginTimeout = 5 * time.Second
	maxPending   = 1024
)

var (
	listenMu      sync.Mutex
	listener      net.Listener
	pendingLogin  atomic.Int64
	errListenBusy = errors.New("watchdog: 已经在 listen")
)

// StartAccept 在独立协程里 Listen/Accept，新连接用消息交给 .watchdog，不占用 mailbox。
func StartAccept(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	listenMu.Lock()
	if listener != nil {
		listenMu.Unlock()
		ln.Close()
		return errListenBusy
	}
	listener = ln
	listenMu.Unlock()

	go acceptLoop(ln)
	slog.Info("accept loop started", "addr", addr)
	return nil
}

func StopAccept() error {
	listenMu.Lock()
	ln := listener
	listener = nil
	listenMu.Unlock()
	if ln == nil {
		return nil
	}
	return ln.Close()
}

func acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Info("accept loop stopped", "err", err)
			return
		}
		if !tryHoldPending() {
			slog.Warn("pending login full, drop", "remote", conn.RemoteAddr(), "max", maxPending)
			conn.Close()
			continue
		}
		_ = conn.SetReadDeadline(time.Now().Add(loginTimeout))
		msg := &socketOpenMsg{
			BaseMessage: gonet.BaseMessage{Cmd: cmdSocketOpen},
			Conn:        conn,
		}
		if err := gonet.SendMemoryName(Name, msg); err != nil {
			slog.Error("notify watchdog open", "err", err)
			releasePending()
			conn.Close()
		}
	}
}

func tryHoldPending() bool {
	for {
		n := pendingLogin.Load()
		if n >= maxPending {
			return false
		}
		if pendingLogin.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

func releasePending() {
	pendingLogin.Add(-1)
}
