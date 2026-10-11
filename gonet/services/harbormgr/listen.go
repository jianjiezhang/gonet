package harbormgr

import (
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"gonet/actor"

	"gonet/services/internal/actormsg"
	"gonet/services/internal/harborproto"
)

var connSeq uint64

func (a *Actor) openListen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	a.ln = ln
	a.bound = ln.Addr().String()
	go a.acceptLoop(ln)
	slog.Info("harbormgr listen", "pid", a.Self(), "addr", a.bound)
	return nil
}

func (a *Actor) acceptLoop(ln net.Listener) {
	go func() {
		<-a.stop
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go serveConn(a.Self(), conn)
	}
}

func (a *Actor) closeListen() {
	a.stopOnce.Do(func() { close(a.stop) })
	if a.ln != nil {
		_ = a.ln.Close()
		a.ln = nil
	}
}

func serveConn(pid uint64, conn net.Conn) {
	token := atomic.AddUint64(&connSeq, 1)
	defer conn.Close()
	defer func() {
		msg := &actormsg.HarbormgrUnbind{Token: token}
		msg.SetCmd(actormsg.CmdKey(actormsg.HarbormgrCmdUnbind))
		_ = actor.SendMemory(pid, msg)
	}()
	for {
		f, err := harborproto.Read(conn)
		if err != nil {
			return
		}
		back := make(chan actormsg.HarbormgrResult, 1)
		msg, ok := frameToMsg(f, token, back)
		if !ok {
			if err := writeInvalid(conn, f.Cmd); err != nil {
				return
			}
			continue
		}
		if err := actor.SendMemory(pid, msg); err != nil {
			slog.Warn("harbormgr: 投递请求失败", "err", err)
			return
		}
		select {
		case resp := <-back:
			if err := harborproto.Write(conn, resultFrame(resp)); err != nil {
				return
			}
		case <-time.After(3 * time.Second):
			slog.Warn("harbormgr: 处理超时")
			return
		}
	}
}

func frameToMsg(f harborproto.Frame, token uint64, back chan actormsg.HarbormgrResult) (actor.MessageInterface, bool) {
	body, ok := f.Body()
	if !ok {
		return nil, false
	}
	wire := actormsg.HarbormgrFromConn{Token: token, Back: back}
	var msg actor.MessageInterface
	switch m := body.(type) {
	case *harborproto.RegisterAddr:
		out := &actormsg.HarbormgrRegisterAddr{RegisterAddr: *m, HarbormgrFromConn: wire}
		out.SetCmd(actormsg.CmdKey(f.Cmd))
		msg = out
	case *harborproto.RegisterName:
		out := &actormsg.HarbormgrRegisterName{RegisterName: *m, HarbormgrFromConn: wire}
		out.SetCmd(actormsg.CmdKey(f.Cmd))
		msg = out
	case *harborproto.Heartbeat:
		out := &actormsg.HarbormgrHeartbeat{Heartbeat: *m, HarbormgrFromConn: wire}
		out.SetCmd(actormsg.CmdKey(f.Cmd))
		msg = out
	case *harborproto.QueryAddr:
		out := &actormsg.HarbormgrQueryAddr{QueryAddr: *m, HarbormgrFromConn: wire}
		out.SetCmd(actormsg.CmdKey(f.Cmd))
		msg = out
	case *harborproto.UnregisterName:
		out := &actormsg.HarbormgrUnregisterName{UnregisterName: *m, HarbormgrFromConn: wire}
		out.SetCmd(actormsg.CmdKey(f.Cmd))
		msg = out
	case *harborproto.UnregisterAddr:
		out := &actormsg.HarbormgrUnregisterAddr{UnregisterAddr: *m, HarbormgrFromConn: wire}
		out.SetCmd(actormsg.CmdKey(f.Cmd))
		msg = out
	default:
		return nil, false
	}
	return msg, true
}

func resultFrame(m actormsg.HarbormgrResult) harborproto.Frame {
	f, err := harborproto.Pack(harborproto.CmdResult, &m.Env)
	if err != nil {
		return harborproto.Frame{Cmd: harborproto.CmdResult}
	}
	return f
}

func writeInvalid(conn net.Conn, cmd uint16) error {
	res, err := harborproto.NewResult(cmd, &harborproto.Status{Err: harborproto.ErrInvalid})
	if err != nil {
		res = harborproto.Result{Cmd: cmd}
	}
	f, err := harborproto.Pack(harborproto.CmdResult, &res)
	if err != nil {
		return err
	}
	return harborproto.Write(conn, f)
}
