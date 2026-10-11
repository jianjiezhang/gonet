package harbormgr

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"gonet/actor"

	"gonet/services/internal/actormsg"
	"gonet/services/internal/harborproto"
)

func TestHello(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := Start(ctx, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer Stop()
	if _, err := Start(ctx, "127.0.0.1:0"); err == nil {
		t.Fatal("second start")
	}

	msg, err := actor.Pack(actormsg.CmdKey(actormsg.HarbormgrCmdHello), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	v, err := actor.SuspendCall(ctx, running, msg)
	if err != nil {
		t.Fatal(err)
	}
	if v != "hello harbormgr" {
		t.Fatalf("got %v", v)
	}
}

type client struct {
	actor.ActorContext
	ch chan actormsg.HarbormgrResult
}

func (c *client) Init() error {
	return c.RegisterCmd(actormsg.CmdKey(actormsg.HarbormgrCmdResult), func() actor.MessageInterface { return &actormsg.HarbormgrResult{} }, c.onResult)
}

func (c *client) onResult(e actor.Envelope) {
	m, ok := e.Msg.(*actormsg.HarbormgrResult)
	if !ok {
		return
	}
	c.ch <- *m
}

func (c *client) ask(t *testing.T, msg actor.MessageInterface) view {
	t.Helper()
	if err := c.Send(running, msg); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-c.ch:
		return open(t, r)
	case <-time.After(time.Second):
		t.Fatal("no result")
	}
	return view{}
}

type view struct {
	Cmd   uint16
	ID    uint64
	Node  uint64
	Fresh bool
	OK    bool
	Err   string
	Addr  string
	Name  string
}

func open(t *testing.T, r actormsg.HarbormgrResult) view {
	t.Helper()
	body, ok := r.Env.Body()
	if !ok {
		t.Fatalf("bad result %+v", r.Env)
	}
	v := view{Cmd: r.Env.Cmd}
	switch m := body.(type) {
	case *harborproto.RegisterAddrResult:
		v.ID, v.Node, v.Fresh, v.OK, v.Err, v.Addr = m.ID, m.NodeID, m.Fresh, m.OK, m.Err, m.Addr
	case *harborproto.RegisterNameResult:
		v.ID, v.Node, v.OK, v.Err, v.Addr, v.Name = m.ID, m.NodeID, m.OK, m.Err, m.Addr, m.Name
	case *harborproto.HeartbeatResult:
		v.ID, v.Node, v.OK, v.Err, v.Addr = m.ID, m.NodeID, m.OK, m.Err, m.Addr
	case *harborproto.QueryAddrResult:
		v.ID, v.Node, v.OK, v.Err, v.Addr, v.Name = m.ID, m.NodeID, m.OK, m.Err, m.Addr, m.Name
	case *harborproto.UnregisterNameResult:
		v.ID, v.Node, v.OK, v.Err, v.Addr, v.Name = m.ID, m.NodeID, m.OK, m.Err, m.Addr, m.Name
	case *harborproto.UnregisterAddrResult:
		v.ID, v.Node, v.OK, v.Err, v.Addr = m.ID, m.NodeID, m.OK, m.Err, m.Addr
	default:
		t.Fatalf("unknown result %T", body)
	}
	return v
}

func startPair(t *testing.T) *client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := Start(ctx, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Stop)
	c := &client{ch: make(chan actormsg.HarbormgrResult, 4)}
	pid, err := actor.Spawn(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = actor.StopActor(pid) })
	if err := actor.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}
	return c
}

func regAddr(id, node uint64, addr string) *actormsg.HarbormgrRegisterAddr {
	m := &actormsg.HarbormgrRegisterAddr{RegisterAddr: harborproto.RegisterAddr{ID: id, NodeID: node, Addr: addr}}
	m.SetCmd(actormsg.CmdKey(actormsg.HarbormgrCmdRegisterAddr))
	return m
}

func regName(id, node uint64, addr, name string) *actormsg.HarbormgrRegisterName {
	m := &actormsg.HarbormgrRegisterName{RegisterName: harborproto.RegisterName{ID: id, NodeID: node, Addr: addr, Name: name}}
	m.SetCmd(actormsg.CmdKey(actormsg.HarbormgrCmdRegisterName))
	return m
}

func beat(id, node uint64) *actormsg.HarbormgrHeartbeat {
	m := &actormsg.HarbormgrHeartbeat{Heartbeat: harborproto.Heartbeat{ID: id, NodeID: node}}
	m.SetCmd(actormsg.CmdKey(actormsg.HarbormgrCmdHeartbeat))
	return m
}

func query(id uint64, name string) *actormsg.HarbormgrQueryAddr {
	m := &actormsg.HarbormgrQueryAddr{QueryAddr: harborproto.QueryAddr{ID: id, Name: name}}
	m.SetCmd(actormsg.CmdKey(actormsg.HarbormgrCmdQueryAddr))
	return m
}

func unregName(id, node uint64, addr, name string) *actormsg.HarbormgrUnregisterName {
	m := &actormsg.HarbormgrUnregisterName{UnregisterName: harborproto.UnregisterName{ID: id, NodeID: node, Addr: addr, Name: name}}
	m.SetCmd(actormsg.CmdKey(actormsg.HarbormgrCmdUnregisterName))
	return m
}

func unregAddr(id, node uint64, addr string) *actormsg.HarbormgrUnregisterAddr {
	m := &actormsg.HarbormgrUnregisterAddr{UnregisterAddr: harborproto.UnregisterAddr{ID: id, NodeID: node, Addr: addr}}
	m.SetCmd(actormsg.CmdKey(actormsg.HarbormgrCmdUnregisterAddr))
	return m
}

func TestDirectory(t *testing.T) {
	c := startPair(t)

	r := c.ask(t, regName(1, 1, "127.0.0.1:1", "@a"))
	if r.OK || r.Err != actormsg.ErrHarbormgrUnknown || r.ID != 1 {
		t.Fatalf("name before addr: %+v", r)
	}
	r = c.ask(t, regAddr(2, 0, "127.0.0.1:1"))
	if !r.OK || !r.Fresh || r.Node == 0 || r.Addr != "127.0.0.1:1" {
		t.Fatalf("register addr: %+v", r)
	}
	node := r.Node
	r = c.ask(t, regAddr(3, 0, "127.0.0.1:1"))
	if r.OK || r.Err != actormsg.ErrHarbormgrTaken {
		t.Fatalf("second server takes addr: %+v", r)
	}
	r = c.ask(t, regAddr(4, node, "127.0.0.1:1"))
	if !r.OK || r.Fresh || r.Node != node {
		t.Fatalf("same node refresh: %+v", r)
	}
	r = c.ask(t, regName(5, node, "127.0.0.1:1", "@a"))
	if !r.OK || r.Name != "@a" {
		t.Fatalf("register name: %+v", r)
	}
	r = c.ask(t, regName(6, node, "127.0.0.1:1", "@a"))
	if !r.OK {
		t.Fatalf("register name again: %+v", r)
	}

	r = c.ask(t, regAddr(7, 0, "127.0.0.1:2"))
	if !r.OK || !r.Fresh || r.Node == node {
		t.Fatalf("register second addr: %+v", r)
	}
	other := r.Node
	r = c.ask(t, regName(8, other, "127.0.0.1:2", "@a"))
	if r.ID != 8 || r.OK || r.Err != actormsg.ErrHarbormgrTaken || r.Addr != "127.0.0.1:1" {
		t.Fatalf("taken: %+v", r)
	}
	r = c.ask(t, query(9, "@a"))
	if !r.OK || r.Addr != "127.0.0.1:1" || r.Name != "@a" || r.Node != node {
		t.Fatalf("query: %+v", r)
	}

	r = c.ask(t, unregName(10, other, "127.0.0.1:2", "@a"))
	if r.OK || r.Err != actormsg.ErrHarbormgrUnknown {
		t.Fatalf("unregister wrong node: %+v", r)
	}
	r = c.ask(t, unregName(11, node, "127.0.0.1:1", "@a"))
	if !r.OK {
		t.Fatalf("unregister name: %+v", r)
	}
	r = c.ask(t, regName(12, other, "127.0.0.1:2", "@a"))
	if !r.OK || r.Addr != "127.0.0.1:2" {
		t.Fatalf("register after unregister: %+v", r)
	}

	r = c.ask(t, beat(13, 99))
	if r.OK || r.Err != actormsg.ErrHarbormgrUnknown {
		t.Fatalf("heartbeat unknown: %+v", r)
	}
	r = c.ask(t, query(14, "@missing"))
	if r.OK || r.Err != actormsg.ErrHarbormgrUnknown {
		t.Fatalf("query missing: %+v", r)
	}

	r = c.ask(t, unregAddr(15, other, "127.0.0.1:2"))
	if !r.OK {
		t.Fatalf("unregister addr: %+v", r)
	}
	r = c.ask(t, query(16, "@a"))
	if r.OK || r.Err != actormsg.ErrHarbormgrUnknown {
		t.Fatalf("query after addr gone: %+v", r)
	}
}

func TestAddrExpire(t *testing.T) {
	prevSweep, prevAlive := sweepEvery, aliveFor
	sweepEvery = 20 * time.Millisecond
	aliveFor = 50 * time.Millisecond
	t.Cleanup(func() {
		sweepEvery = prevSweep
		aliveFor = prevAlive
	})

	c := startPair(t)
	r := c.ask(t, regAddr(1, 0, "127.0.0.1:1"))
	if !r.OK {
		t.Fatalf("register: %+v", r)
	}
	node := r.Node
	r = c.ask(t, regName(2, node, "127.0.0.1:1", "@a"))
	if !r.OK {
		t.Fatalf("name: %+v", r)
	}
	time.Sleep(30 * time.Millisecond)
	r = c.ask(t, beat(3, node))
	if !r.OK {
		t.Fatalf("heartbeat: %+v", r)
	}
	r = c.ask(t, query(4, "@a"))
	if !r.OK || r.Addr != "127.0.0.1:1" {
		t.Fatalf("still alive: %+v", r)
	}
	time.Sleep(120 * time.Millisecond)
	r = c.ask(t, query(5, "@a"))
	if r.OK || r.Err != actormsg.ErrHarbormgrUnknown {
		t.Fatalf("expired: %+v", r)
	}
	r = c.ask(t, beat(6, node))
	if r.OK || r.Err != actormsg.ErrHarbormgrUnknown {
		t.Fatalf("heartbeat after expire: %+v", r)
	}
	r = c.ask(t, regAddr(7, node, "127.0.0.1:1"))
	if !r.OK || !r.Fresh || r.Node != node || r.Addr != "127.0.0.1:1" {
		t.Fatalf("reclaim after expire: %+v", r)
	}
	r = c.ask(t, query(8, "@a"))
	if r.OK || r.Err != actormsg.ErrHarbormgrUnknown {
		t.Fatalf("name dropped on expire: %+v", r)
	}
	r = c.ask(t, regAddr(9, node+10, "127.0.0.1:8"))
	if !r.OK || !r.Fresh || r.Node != node+10 || r.Addr != "127.0.0.1:8" {
		t.Fatalf("reclaim unseen id: %+v", r)
	}
	r = c.ask(t, regAddr(12, 0, "127.0.0.1:3"))
	if !r.OK || !r.Fresh || r.Node != node+11 {
		t.Fatalf("alloc after high reclaim: %+v", r)
	}
	time.Sleep(120 * time.Millisecond)
	r = c.ask(t, regAddr(10, 0, "127.0.0.1:1"))
	if !r.OK || !r.Fresh || r.Node == node {
		t.Fatalf("new node takes addr: %+v", r)
	}
	r = c.ask(t, regAddr(11, node, "127.0.0.1:1"))
	if r.OK || r.Err != actormsg.ErrHarbormgrTaken {
		t.Fatalf("old id addr held: %+v", r)
	}
}

func TestReclaimMovesAddr(t *testing.T) {
	c := startPair(t)
	r := c.ask(t, regAddr(1, 0, "127.0.0.1:1"))
	if !r.OK || !r.Fresh {
		t.Fatalf("register: %+v", r)
	}
	node := r.Node
	if r = c.ask(t, regName(2, node, "127.0.0.1:1", "@a")); !r.OK {
		t.Fatalf("name: %+v", r)
	}
	r = c.ask(t, regAddr(3, node, "127.0.0.1:9"))
	if !r.OK || r.Fresh || r.Node != node || r.Addr != "127.0.0.1:9" {
		t.Fatalf("move: %+v", r)
	}
	r = c.ask(t, query(4, "@a"))
	if !r.OK || r.Addr != "127.0.0.1:9" || r.Node != node {
		t.Fatalf("query after move: %+v", r)
	}
	r = c.ask(t, regAddr(5, 0, "127.0.0.1:1"))
	if !r.OK || !r.Fresh {
		t.Fatalf("old addr free: %+v", r)
	}
	r = c.ask(t, regAddr(6, 0, "127.0.0.1:9"))
	if r.OK || r.Err != actormsg.ErrHarbormgrTaken {
		t.Fatalf("new addr still held: %+v", r)
	}
}

type memSeq struct {
	mu sync.Mutex
	n  uint64
}

func (s *memSeq) Load(context.Context) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n, nil
}

func (s *memSeq) Save(_ context.Context, nodeID uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n = nodeID
	return nil
}

func TestNodeSeqRestart(t *testing.T) {
	seq := &memSeq{}
	UseNodeSeq(seq)
	t.Cleanup(func() { UseNodeSeq(nil) })

	c := startPair(t)
	r := c.ask(t, regAddr(1, 0, "127.0.0.1:1"))
	if !r.OK || r.Node != 1 {
		t.Fatalf("first: %+v", r)
	}
	Stop()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := Start(ctx, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Stop)
	r = c.ask(t, regAddr(2, 0, "127.0.0.1:2"))
	if !r.OK || r.Node != 2 {
		t.Fatalf("after restart: %+v", r)
	}
	r = c.ask(t, regAddr(3, 1, "127.0.0.1:1"))
	if !r.OK || !r.Fresh || r.Node != 1 {
		t.Fatalf("reclaim old: %+v", r)
	}
	r = c.ask(t, regAddr(4, 0, "127.0.0.1:3"))
	if !r.OK || r.Node != 3 {
		t.Fatalf("next: %+v", r)
	}
}

func TestNodeSeqWrap(t *testing.T) {
	seq := &memSeq{n: math.MaxUint64}
	UseNodeSeq(seq)
	t.Cleanup(func() { UseNodeSeq(nil) })

	c := startPair(t)
	r := c.ask(t, regAddr(1, 1, "127.0.0.1:1"))
	if !r.OK || r.Node != 1 {
		t.Fatalf("hold 1: %+v", r)
	}
	r = c.ask(t, regAddr(2, 0, "127.0.0.1:2"))
	if !r.OK || !r.Fresh || r.Node != 2 {
		t.Fatalf("wrap skip: %+v", r)
	}
	r = c.ask(t, regAddr(3, 0, "127.0.0.1:3"))
	if !r.OK || r.Node != 3 {
		t.Fatalf("next: %+v", r)
	}
}
