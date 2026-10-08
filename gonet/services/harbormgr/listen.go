package harbormgr

import (
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"gonet/actor"
	"gonet/services/harbormgr/proto"
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
		msg := &unbindMsg{token: token}
		msg.SetCmd(CmdKey(CmdUnbind))
		_ = actor.SendMemory(pid, msg)
	}()
	for {
		f, err := proto.Read(conn)
		if err != nil {
			return
		}
		back := make(chan Result, 1)
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
			if err := proto.Write(conn, resultFrame(resp)); err != nil {
				return
			}
		case <-time.After(3 * time.Second):
			slog.Warn("harbormgr: 处理超时")
			return
		}
	}
}

func frameToMsg(f proto.Frame, token uint64, back chan Result) (actor.MessageInterface, bool) {
	body, ok := f.Body()
	if !ok {
		return nil, false
	}
	wire := fromConn{token: token, back: back}
	var msg actor.MessageInterface
	switch m := body.(type) {
	case *proto.RegisterAddr:
		out := &RegisterAddr{RegisterAddr: *m, fromConn: wire}
		out.SetCmd(CmdKey(f.Cmd))
		msg = out
	case *proto.RegisterName:
		out := &RegisterName{RegisterName: *m, fromConn: wire}
		out.SetCmd(CmdKey(f.Cmd))
		msg = out
	case *proto.Heartbeat:
		out := &Heartbeat{Heartbeat: *m, fromConn: wire}
		out.SetCmd(CmdKey(f.Cmd))
		msg = out
	case *proto.QueryAddr:
		out := &QueryAddr{QueryAddr: *m, fromConn: wire}
		out.SetCmd(CmdKey(f.Cmd))
		msg = out
	case *proto.UnregisterName:
		out := &UnregisterName{UnregisterName: *m, fromConn: wire}
		out.SetCmd(CmdKey(f.Cmd))
		msg = out
	case *proto.UnregisterAddr:
		out := &UnregisterAddr{UnregisterAddr: *m, fromConn: wire}
		out.SetCmd(CmdKey(f.Cmd))
		msg = out
	default:
		return nil, false
	}
	return msg, true
}

func resultFrame(m Result) proto.Frame {
	f, err := proto.Pack(proto.CmdResult, &m.Env)
	if err != nil {
		return proto.Frame{Cmd: proto.CmdResult}
	}
	return f
}

func writeInvalid(conn net.Conn, cmd uint16) error {
	res, err := proto.NewResult(cmd, &proto.Status{Err: proto.ErrInvalid})
	if err != nil {
		res = proto.Result{Cmd: cmd}
	}
	f, err := proto.Pack(proto.CmdResult, &res)
	if err != nil {
		return err
	}
	return proto.Write(conn, f)
}
