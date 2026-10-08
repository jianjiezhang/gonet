package monitor

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"

	"gonet/actor"
)

type debugServer struct {
	mu    sync.Mutex
	ln    net.Listener
	conns map[net.Conn]struct{}
}

var debug debugServer

// Listen 在 addr 上接受 debug 连接（行协议）。已在监听则失败。
func Listen(addr string) (string, error) {
	if addr == "" {
		return "", fmt.Errorf("gonet: debug 地址为空")
	}
	debug.mu.Lock()
	defer debug.mu.Unlock()
	if debug.ln != nil {
		return "", fmt.Errorf("gonet: debug 已在监听")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	debug.ln = ln
	debug.conns = make(map[net.Conn]struct{})
	go acceptLoop(ln)
	bound := ln.Addr().String()
	slog.Info("gonet debug listen", "addr", bound)
	return bound, nil
}

// StopListen 关闭 debug 端口和已有连接。未监听则成功。
func StopListen() error {
	debug.mu.Lock()
	ln := debug.ln
	debug.ln = nil
	conns := debug.conns
	debug.conns = nil
	debug.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	for c := range conns {
		_ = c.Close()
	}
	return nil
}

func acceptLoop(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		debug.mu.Lock()
		if debug.ln != ln {
			debug.mu.Unlock()
			_ = c.Close()
			return
		}
		debug.conns[c] = struct{}{}
		debug.mu.Unlock()
		go serve(c)
	}
}

func dropConn(c net.Conn) {
	debug.mu.Lock()
	delete(debug.conns, c)
	debug.mu.Unlock()
	_ = c.Close()
}

func serve(c net.Conn) {
	defer dropConn(c)
	_, _ = fmt.Fprint(c, "gonet debug\n> ")
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			_, _ = fmt.Fprint(c, "> ")
			continue
		}
		fields := strings.Fields(line)
		cmd := strings.ToLower(fields[0])
		switch cmd {
		case "quit", "exit":
			_, _ = fmt.Fprint(c, "bye\n")
			return
		case "help":
			_, _ = fmt.Fprint(c, "monitor\nkill <pid>\nhelp\nquit\n> ")
		case "monitor":
			_, _ = fmt.Fprint(c, Format(SnapshotNow()))
			_, _ = fmt.Fprint(c, "> ")
		case "kill":
			if len(fields) < 2 {
				_, _ = fmt.Fprint(c, "usage: kill <pid>\n> ")
				continue
			}
			pid, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				_, _ = fmt.Fprintf(c, "invalid pid\n> ")
				continue
			}
			if err := actor.Kill(pid); err != nil {
				_, _ = fmt.Fprintf(c, "%v\n> ", err)
				continue
			}
			_, _ = fmt.Fprintf(c, "ok pid=%d\n> ", pid)
		default:
			_, _ = fmt.Fprintf(c, "unknown command: %s\n> ", cmd)
		}
	}
}
