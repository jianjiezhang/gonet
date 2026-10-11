package monitor

import (
	"bufio"
	"errors"
	"fmt"
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

func (a *Actor) listen() error {
	if a.addr == "" {
		return errors.New("monitor: debug 地址为空")
	}
	ln, err := net.Listen("tcp", a.addr)
	if err != nil {
		return err
	}
	a.debug.mu.Lock()
	a.debug.ln = ln
	a.debug.conns = make(map[net.Conn]struct{})
	a.debug.mu.Unlock()
	a.bound = ln.Addr().String()
	go a.acceptLoop(ln)
	return nil
}

func (a *Actor) closeListen() {
	a.debug.mu.Lock()
	ln := a.debug.ln
	a.debug.ln = nil
	conns := a.debug.conns
	a.debug.conns = nil
	a.debug.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	for c := range conns {
		_ = c.Close()
	}
}

func (a *Actor) acceptLoop(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		a.debug.mu.Lock()
		if a.debug.ln != ln {
			a.debug.mu.Unlock()
			_ = c.Close()
			return
		}
		a.debug.conns[c] = struct{}{}
		a.debug.mu.Unlock()
		go a.serve(c)
	}
}

func (a *Actor) dropConn(c net.Conn) {
	a.debug.mu.Lock()
	delete(a.debug.conns, c)
	a.debug.mu.Unlock()
	_ = c.Close()
}

// serve 在 debug 协程里直接读花名册。不把命令投进本 actor 的 mailbox。
func (a *Actor) serve(c net.Conn) {
	defer a.dropConn(c)
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
