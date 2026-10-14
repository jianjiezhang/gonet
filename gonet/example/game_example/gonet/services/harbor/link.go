package harbor

import (
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"gonet/actor"

	"gonet/services/internal/actormsg"
	"gonet/services/internal/harborproto"
)

var errDown = errors.New("harbor: harbormgr 未连接")

// endpoint 是到 harbormgr 的一条 TCP 连接。写操作串行。
type endpoint struct {
	mu   sync.Mutex
	conn net.Conn
}

func (e *endpoint) WriteFrame(f harborproto.Frame) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.conn == nil {
		return errDown
	}
	_ = e.conn.SetWriteDeadline(time.Now().Add(time.Second))
	return harborproto.Write(e.conn, f)
}

func (e *endpoint) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.conn != nil {
		_ = e.conn.Close()
		e.conn = nil
	}
}

type linkMsg struct {
	actor.BaseMessage
	up  bool
	ep  *endpoint
	out chan harborproto.Frame
}

func (a *Actor) connLoop() {
	pid := a.Self()
	dialer := net.Dialer{Timeout: time.Second}
	for {
		if a.stopped() {
			return
		}
		conn, err := dialer.DialContext(a.dialBase, "tcp", a.mgrAddr)
		if err != nil {
			if a.stopped() {
				return
			}
			slog.Warn("harbor: 连接 harbormgr 失败", "addr", a.mgrAddr, "err", err)
			select {
			case <-a.stop:
				return
			case <-a.dialBase.Done():
				return
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}
		a.serveConn(pid, conn)
		if a.stopped() {
			return
		}
		select {
		case <-a.stop:
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (a *Actor) stopped() bool {
	select {
	case <-a.stop:
		return true
	default:
		return false
	}
}

func (a *Actor) serveConn(pid uint64, conn net.Conn) {
	ep := &endpoint{conn: conn}
	out := make(chan harborproto.Frame, 64)
	quit := make(chan struct{})
	defer close(quit)
	defer ep.Close()
	defer postLink(pid, false, ep, nil)

	postLink(pid, true, ep, out)

	reads := make(chan harborproto.Frame, 8)
	go func() {
		defer close(reads)
		for {
			f, err := harborproto.Read(conn)
			if err != nil {
				return
			}
			select {
			case reads <- f:
			case <-quit:
				return
			case <-a.stop:
				return
			}
		}
	}()

	for {
		select {
		case <-a.stop:
			return
		case f, ok := <-reads:
			if !ok {
				return
			}
			postResult(pid, f)
		case f := <-out:
			if err := ep.WriteFrame(f); err != nil {
				return
			}
		}
	}
}

func (a *Actor) onLink(e actor.Envelope) {
	m, ok := e.Msg.(*linkMsg)
	if !ok {
		return
	}
	if !m.up {
		if a.ep != m.ep {
			return
		}
		slog.Warn("harbor: 与 harbormgr 断开", "addr", a.mgrAddr)
		a.requeuePending()
		a.addrOK = false
		a.resyncing = false
		a.resync = nil
		a.ep = nil
		a.out = nil
		return
	}
	a.ep = m.ep
	a.out = m.out
	a.resyncing = true
	slog.Info("harbor linked", "mgr", a.mgrAddr, "addr", a.addr)
	asks := a.addrAsks
	a.addrAsks = nil
	a.sendRegisterAddr(asks)
	a.flushQueued()
}

func (a *Actor) requeuePending() {
	for id, p := range a.pending {
		delete(a.pending, id)
		if p == nil {
			continue
		}
		switch p.req {
		case harborproto.CmdRegisterName:
			delete(a.publishing, p.name)
			a.waiting[p.name] = append(a.waiting[p.name], p.askers...)
		case harborproto.CmdUnregisterName:
			a.dropping[p.name] = append(a.dropping[p.name], p.askers...)
		case harborproto.CmdQueryAddr:
			for _, ask := range p.askers {
				a.enqueueQuery(ask, p.name, true)
			}
		case harborproto.CmdRegisterAddr:
			a.addrAsks = append(a.addrAsks, p.askers...)
		}
	}
}

func (a *Actor) unregisterRemote() {
	if a.ep == nil || a.nodeID == 0 {
		return
	}
	id := a.allocID()
	f, err := harborproto.Pack(harborproto.CmdUnregisterAddr, &harborproto.UnregisterAddr{ID: id, NodeID: a.nodeID, Addr: a.addr})
	if err != nil {
		slog.Warn("harbor: 下线地址失败", "addr", a.addr, "err", err)
		return
	}
	err = a.ep.WriteFrame(f)
	if err != nil {
		slog.Warn("harbor: 下线地址失败", "addr", a.addr, "err", err)
	}
}

func (a *Actor) halt() {
	a.failAcks("", "down")
	if a.dialCancel != nil {
		a.dialCancel()
	}
	a.stopOnce.Do(func() { close(a.stop) })
	if a.ep != nil {
		a.ep.Close()
		a.ep = nil
	}
	a.out = nil
	if a.peerLn != nil {
		_ = a.peerLn.Close()
		a.peerLn = nil
	}
	for addr, p := range a.peers {
		delete(a.peers, addr)
		if p != nil && p.ep != nil {
			p.ep.Close()
		}
	}
}

func (a *Actor) sendMgr(cmd uint16, payload any) error {
	if a.out == nil {
		return errDown
	}
	f, err := harborproto.Pack(cmd, payload)
	if err != nil {
		return err
	}
	select {
	case a.out <- f:
		return nil
	default:
		slog.Warn("harbor: 连接队列已满")
		return errDown
	}
}

func postLink(pid uint64, up bool, ep *endpoint, out chan harborproto.Frame) {
	msg := &linkMsg{up: up, ep: ep, out: out}
	msg.SetCmd(actormsg.CmdHarborLink)
	post(pid, msg)
}

func postResult(pid uint64, f harborproto.Frame) {
	body, ok := f.Body()
	if !ok {
		return
	}
	res, ok := body.(*harborproto.Result)
	if !ok {
		return
	}
	msg := &actormsg.HarborMgrResult{Env: *res}
	msg.SetCmd(actormsg.CmdHarborMgrResult)
	post(pid, msg)
}

func post(pid uint64, msg actor.MessageInterface) {
	for {
		err := actor.SendMemory(pid, msg)
		if err == nil || errors.Is(err, actor.ErrDead) || errors.Is(err, actor.ErrMailboxFull) {
			return
		}
		if !errors.Is(err, actor.ErrNotReady) {
			slog.Warn("harbor: 投递连接事件失败", "err", err)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
