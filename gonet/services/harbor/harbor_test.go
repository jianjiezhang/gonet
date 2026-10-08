package harbor

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"gonet/actor"
	"gonet/services/harbormgr"
	"gonet/services/harbormgr/proto"
)

func TestHello(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := Start(ctx, "", "127.0.0.1:1"); err == nil {
		t.Fatal("empty addr")
	}
	if _, err := Start(ctx, "127.0.0.1:1", ""); err == nil {
		t.Fatal("empty mgr")
	}
	if _, err := Start(ctx, "127.0.0.1:0", "127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	defer Stop()
	if _, err := Start(ctx, "127.0.0.1:0", "127.0.0.1:1"); err == nil {
		t.Fatal("second start")
	}

	msg, err := actor.Pack(CmdHello, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	v, err := actor.SuspendCall(ctx, running, msg)
	if err != nil {
		t.Fatal(err)
	}
	if v != "hello harbor" {
		t.Fatalf("got %v", v)
	}
}

type client struct {
	actor.ActorContext
	harbor chan Result
	mgr    chan harbormgr.Result
}

func (c *client) Init() error {
	if err := c.RegisterCmd(CmdResult, func() actor.MessageInterface { return &Result{} }, c.onHarbor); err != nil {
		return err
	}
	return c.RegisterCmd(harbormgr.CmdKey(harbormgr.CmdResult), func() actor.MessageInterface { return &harbormgr.Result{} }, c.onMgr)
}

func (c *client) onHarbor(e actor.Envelope) {
	m, ok := e.Msg.(*Result)
	if !ok {
		return
	}
	c.harbor <- *m
}

func (c *client) onMgr(e actor.Envelope) {
	m, ok := e.Msg.(*harbormgr.Result)
	if !ok {
		return
	}
	c.mgr <- *m
}

func (c *client) ask(t *testing.T, msg actor.MessageInterface) Result {
	t.Helper()
	var err error
	if Name == "" {
		err = c.Send(running, msg)
	} else {
		err = c.SendName(Name, msg)
	}
	if err != nil {
		t.Fatal(err)
	}
	return waitHarbor(t, c.harbor)
}

func (c *client) askMgr(t *testing.T, msg actor.MessageInterface) mgrView {
	t.Helper()
	if err := c.SendName(harbormgr.Name, msg); err != nil {
		t.Fatal(err)
	}
	return openMgr(t, waitMgr(t, c.mgr))
}

func waitHarbor(t *testing.T, ch <-chan Result) Result {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(time.Second):
		t.Fatal("no harbor result")
	}
	return Result{}
}

func waitMgr(t *testing.T, ch <-chan harbormgr.Result) harbormgr.Result {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(time.Second):
		t.Fatal("no harbormgr result")
	}
	return harbormgr.Result{}
}

type mgrView struct {
	ID    uint64
	Node  uint64
	Fresh bool
	OK    bool
	Err   string
	Addr  string
	Name  string
}

func openMgr(t *testing.T, r harbormgr.Result) mgrView {
	t.Helper()
	body, ok := r.Env.Body()
	if !ok {
		t.Fatalf("bad result %+v", r.Env)
	}
	var v mgrView
	switch m := body.(type) {
	case *proto.RegisterAddrResult:
		v = mgrView{ID: m.ID, Node: m.NodeID, Fresh: m.Fresh, OK: m.OK, Err: m.Err, Addr: m.Addr}
	case *proto.RegisterNameResult:
		v = mgrView{ID: m.ID, Node: m.NodeID, OK: m.OK, Err: m.Err, Addr: m.Addr, Name: m.Name}
	case *proto.QueryAddrResult:
		v = mgrView{ID: m.ID, Node: m.NodeID, OK: m.OK, Err: m.Err, Addr: m.Addr, Name: m.Name}
	case *proto.UnregisterAddrResult:
		v = mgrView{ID: m.ID, Node: m.NodeID, OK: m.OK, Err: m.Err, Addr: m.Addr}
	default:
		t.Fatalf("unknown result %T", body)
	}
	return v
}

func startBoth(t *testing.T, addr string) (*client, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	mgrAddr, err := harbormgr.Start(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(harbormgr.Stop)
	bound, err := Start(ctx, addr, mgrAddr)
	if err != nil {
		t.Fatal(err)
	}
	nodeID, err := WaitReady(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := harbormgr.UseNode(nodeID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Stop)
	c := &client{
		harbor: make(chan Result, 4),
		mgr:    make(chan harbormgr.Result, 4),
	}
	pid, err := actor.Spawn(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = actor.StopActor(pid) })
	if err := actor.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}
	return c, bound
}

func regName(id uint64, name string) *RegisterName {
	m := &RegisterName{ID: id, Name: name}
	m.SetCmd(CmdRegisterName)
	return m
}

func unregName(id uint64, name string) *UnregisterName {
	m := &UnregisterName{ID: id, Name: name}
	m.SetCmd(CmdUnregisterName)
	return m
}

func query(id uint64, name string) *QueryAddr {
	m := &QueryAddr{ID: id, Name: name}
	m.SetCmd(CmdQueryAddr)
	return m
}

func mgrRegAddr(id uint64, addr string) *harbormgr.RegisterAddr {
	m := &harbormgr.RegisterAddr{RegisterAddr: proto.RegisterAddr{ID: id, Addr: addr}}
	m.SetCmd(harbormgr.CmdKey(harbormgr.CmdRegisterAddr))
	return m
}

func mgrRegName(id, node uint64, addr, name string) *harbormgr.RegisterName {
	m := &harbormgr.RegisterName{RegisterName: proto.RegisterName{ID: id, NodeID: node, Addr: addr, Name: name}}
	m.SetCmd(harbormgr.CmdKey(harbormgr.CmdRegisterName))
	return m
}

func mgrQuery(id uint64, name string) *harbormgr.QueryAddr {
	m := &harbormgr.QueryAddr{QueryAddr: proto.QueryAddr{ID: id, Name: name}}
	m.SetCmd(harbormgr.CmdKey(harbormgr.CmdQueryAddr))
	return m
}

func TestDirectory(t *testing.T) {
	c, addr := startBoth(t, "127.0.0.1:0")

	r := c.ask(t, regName(1, ""))
	if r.OK || r.Err != harbormgr.ErrInvalid {
		t.Fatalf("empty name: %+v", r)
	}
	r = c.ask(t, regName(2, "@a"))
	if !r.OK || r.ID != 2 || r.Addr != addr || r.Name != "@a" || r.Req != CmdRegisterName {
		t.Fatalf("register: %+v", r)
	}
	r = c.ask(t, regName(3, "@a"))
	if r.OK || r.Err != harbormgr.ErrTaken {
		t.Fatalf("register again: %+v", r)
	}
	r = c.ask(t, query(4, "@a"))
	if !r.OK || r.Addr != addr || r.Name != "@a" || r.Req != CmdQueryAddr {
		t.Fatalf("query: %+v", r)
	}
	r = c.ask(t, unregName(5, "@a"))
	if !r.OK || r.Req != CmdUnregisterName {
		t.Fatalf("unregister: %+v", r)
	}
	r = c.ask(t, query(6, "@a"))
	if r.OK || r.Err != harbormgr.ErrUnknown {
		t.Fatalf("query after unregister: %+v", r)
	}
	r = c.ask(t, unregName(7, "@missing"))
	if r.OK || r.Err != harbormgr.ErrUnknown {
		t.Fatalf("unregister missing: %+v", r)
	}
}

func TestNameTaken(t *testing.T) {
	c, _ := startBoth(t, "127.0.0.1:0")
	other := c.askMgr(t, mgrRegAddr(1, "127.0.0.1:2"))
	if !other.OK {
		t.Fatalf("other addr: %+v", other)
	}
	if r := c.askMgr(t, mgrRegName(2, other.Node, "127.0.0.1:2", "@a")); !r.OK {
		t.Fatalf("other name: %+v", r)
	}
	r := c.ask(t, regName(3, "@a"))
	if r.OK || r.Err != harbormgr.ErrTaken || r.Addr != "127.0.0.1:2" {
		t.Fatalf("taken: %+v", r)
	}
}

func TestQueryRemote(t *testing.T) {
	c, _ := startBoth(t, "127.0.0.1:0")
	other := c.askMgr(t, mgrRegAddr(1, "127.0.0.1:2"))
	if !other.OK {
		t.Fatalf("addr: %+v", other)
	}
	if r := c.askMgr(t, mgrRegName(2, other.Node, "127.0.0.1:2", "@remote")); !r.OK {
		t.Fatalf("name: %+v", r)
	}
	r := c.ask(t, query(3, "@remote"))
	if !r.OK || r.Addr != "127.0.0.1:2" || r.Name != "@remote" {
		t.Fatalf("query remote: %+v", r)
	}
}

func TestStopUnregistersAddr(t *testing.T) {
	c, _ := startBoth(t, "127.0.0.1:0")
	if r := c.ask(t, regName(1, "@a")); !r.OK {
		t.Fatalf("register: %+v", r)
	}
	Stop()
	deadline := time.Now().Add(time.Second)
	for {
		r := c.askMgr(t, mgrQuery(2, "@a"))
		if !r.OK && r.Err == harbormgr.ErrUnknown {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("after stop: %+v", r)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHeartbeatReregister(t *testing.T) {
	prev := beatEvery
	beatEvery = 400 * time.Millisecond
	t.Cleanup(func() { beatEvery = prev })
	restore := harbormgr.TestingTTL(50*time.Millisecond, 10*time.Millisecond)
	t.Cleanup(restore)

	c, addr := startBoth(t, "127.0.0.1:0")
	if r := c.ask(t, regName(1, "@a")); !r.OK {
		t.Fatalf("register: %+v", r)
	}
	time.Sleep(120 * time.Millisecond)
	deadline := time.Now().Add(time.Second)
	missed := false
	for {
		r := c.askMgr(t, mgrQuery(3, "@a"))
		if !r.OK && r.Err == harbormgr.ErrUnknown {
			missed = true
		}
		if missed && r.OK && r.Addr == addr {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("not reregistered: %+v missed=%v", r, missed)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestQueryBusy(t *testing.T) {
	prev := maxQueued
	maxQueued = 0
	t.Cleanup(func() { maxQueued = prev })
	c := startLoose(t)
	r := c.ask(t, query(1, "@a"))
	if r.OK || r.Err != ErrBusy {
		t.Fatalf("busy: %+v", r)
	}
}

func TestQueryTimeout(t *testing.T) {
	prevTimeout, prevExpire := requestTimeout, expireEvery
	requestTimeout = 40 * time.Millisecond
	expireEvery = 10 * time.Millisecond
	t.Cleanup(func() {
		requestTimeout, expireEvery = prevTimeout, prevExpire
	})
	c := startLoose(t)
	r := c.ask(t, query(1, "@a"))
	if r.OK || r.Err != ErrTimeout {
		t.Fatalf("timeout: %+v", r)
	}
}

func TestPeerDial(t *testing.T) {
	c, self := startBoth(t, "127.0.0.1:0")
	if r := c.ask(t, regName(1, "@self")); !r.OK {
		t.Fatalf("register self: %+v", r)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	fake := ln.Addr().String()
	other := c.askMgr(t, mgrRegAddr(1, fake))
	if !other.OK {
		t.Fatalf("fake addr: %+v", other)
	}
	if r := c.askMgr(t, mgrRegName(2, other.Node, fake, "@peer")); !r.OK {
		t.Fatalf("fake name: %+v", r)
	}

	ready := make(chan net.Conn, 1)
	helloCh := make(chan peerHello, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		hello, err := readPeerHello(conn)
		if err != nil {
			errCh <- err
			return
		}
		if err := writePeerHello(conn, other.Node, fake); err != nil {
			errCh <- err
			return
		}
		_ = conn.SetDeadline(time.Time{})
		helloCh <- hello
		ready <- conn
	}()

	r := c.ask(t, query(3, "@peer"))
	if !r.OK || r.Addr != fake || r.Name != "@peer" {
		t.Fatalf("query: %+v", r)
	}
	var hello peerHello
	var outbound net.Conn
	select {
	case err := <-errCh:
		t.Fatal(err)
	case hello = <-helloCh:
		outbound = <-ready
	case <-time.After(2 * time.Second):
		t.Fatal("handshake timeout")
	}
	t.Cleanup(func() { _ = outbound.Close() })
	if hello.Addr != self || hello.NodeID == 0 {
		t.Fatalf("hello: %+v self=%s", hello, self)
	}

	again := make(chan struct{}, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
			again <- struct{}{}
		}
	}()
	r = c.ask(t, query(4, "@peer"))
	if !r.OK || r.Addr != fake {
		t.Fatalf("cached query: %+v", r)
	}
	select {
	case <-again:
		t.Fatal("second dial")
	case <-time.After(200 * time.Millisecond):
	}

	inbound, err := net.Dial("tcp", self)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = inbound.Close() })
	_ = inbound.SetDeadline(time.Now().Add(2 * time.Second))
	if err := writePeerHello(inbound, other.Node, fake); err != nil {
		t.Fatal(err)
	}
	if _, err := readPeerHello(inbound); err != nil {
		t.Fatal(err)
	}
	_ = inbound.SetDeadline(time.Time{})
	_ = inbound.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := proto.Read(inbound); err == nil {
		t.Fatal("inbound stayed open")
	}

	_ = outbound.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, err := proto.Read(outbound); err == nil {
		t.Fatal("unexpected frame")
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("outbound closed: %v", err)
	}
}

type idleActor struct{ actor.ActorContext }

type boxActor struct {
	actor.ActorContext
	got chan actor.Envelope
}

func (b *boxActor) Init() error {
	return b.RegisterCmd("ping", func() actor.MessageInterface { return &actor.BaseMessage{} }, b.onPing)
}

func (b *boxActor) onPing(e actor.Envelope) {
	select {
	case b.got <- e:
	default:
	}
}

func TestRegisterUnique(t *testing.T) {
	c, _ := startBoth(t, "127.0.0.1:0")
	other := c.askMgr(t, mgrRegAddr(1, "127.0.0.1:2"))
	if !other.OK {
		t.Fatalf("other: %+v", other)
	}
	if r := c.askMgr(t, mgrRegName(2, other.Node, "127.0.0.1:2", "@taken")); !r.OK {
		t.Fatalf("name: %+v", r)
	}
	pid, err := actor.Spawn(&idleActor{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = actor.StopActor(pid) })
	err = actor.Register(pid, "@taken")
	if !errors.Is(err, actor.ErrAliasTaken) {
		t.Fatalf("register taken: %v", err)
	}
	if _, err := actor.Query("@taken"); !errors.Is(err, actor.ErrUnknownAlias) {
		t.Fatalf("local alias left: %v", err)
	}

	box := &boxActor{got: make(chan actor.Envelope, 1)}
	boxPID, err := actor.SpawnNamed(box, "box")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = actor.StopActor(boxPID) })
	if _, err := actor.SpawnNamed(&boxActor{got: make(chan actor.Envelope, 1)}, "box"); !errors.Is(err, actor.ErrAliasTaken) {
		t.Fatalf("second box: %v", err)
	}
}

func TestSendLocalOrRemote(t *testing.T) {
	c, _ := startBoth(t, "127.0.0.1:0")
	box := &boxActor{got: make(chan actor.Envelope, 1)}
	pid, err := actor.SpawnNamed(box, "box")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = actor.StopActor(pid) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := actor.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}

	msg, err := actor.Pack("ping", map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if err := actor.SendName("box", msg); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-box.got:
		cmd := ""
		if c, ok := got.Msg.(interface{ Command() string }); ok {
			cmd = c.Command()
		}
		if cmd != "ping" {
			t.Fatalf("local cmd %s", cmd)
		}
	case <-time.After(time.Second):
		t.Fatal("local send")
	}
	if err := actor.SendMemoryName("@far", &actor.BaseMessage{}); !errors.Is(err, actor.ErrRemoteMemory) {
		t.Fatalf("memory: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	fake := ln.Addr().String()
	other := c.askMgr(t, mgrRegAddr(1, fake))
	if !other.OK {
		t.Fatalf("fake: %+v", other)
	}
	if r := c.askMgr(t, mgrRegName(2, other.Node, fake, "@far")); !r.OK {
		t.Fatalf("far: %+v", r)
	}
	gotFrame := make(chan peerSend, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := readPeerHello(conn); err != nil {
			errCh <- err
			return
		}
		if err := writePeerHello(conn, other.Node, fake); err != nil {
			errCh <- err
			return
		}
		for {
			f, err := proto.Read(conn)
			if err != nil {
				errCh <- err
				return
			}
			if f.Cmd != cmdPeerSend {
				continue
			}
			var body peerSend
			if err := f.Decode(&body); err != nil {
				errCh <- err
				return
			}
			gotFrame <- body
			return
		}
	}()
	far, err := actor.Pack("ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := actor.SendName("@far", far); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		t.Fatal(err)
	case body := <-gotFrame:
		if body.Dest != "@far" || body.Cmd != "ping" {
			t.Fatalf("forward: %+v", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("forward timeout")
	}
}

func TestSendNameAck(t *testing.T) {
	c, _ := startBoth(t, "127.0.0.1:0")
	msg, err := actor.Pack("ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := actor.SendNameAck(".missing", msg); !errors.Is(err, actor.ErrUnknownAlias) {
		t.Fatalf("local missing: %v", err)
	}
	if err := actor.SendNameAck("@missing", msg); !errors.Is(err, actor.ErrUnknownAlias) {
		t.Fatalf("remote missing: %v", err)
	}

	box := &boxActor{got: make(chan actor.Envelope, 1)}
	pid, err := actor.SpawnNamed(box, "ackbox")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = actor.StopActor(pid) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := actor.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}
	if err := actor.SendNameAck("ackbox", msg); err != nil {
		t.Fatal(err)
	}

	dead := c.askMgr(t, mgrRegAddr(1, "127.0.0.1:2"))
	if !dead.OK {
		t.Fatalf("dead addr: %+v", dead)
	}
	if r := c.askMgr(t, mgrRegName(2, dead.Node, "127.0.0.1:2", "@down")); !r.OK {
		t.Fatalf("down name: %+v", r)
	}
	if err := actor.SendNameAck("@down", msg); !errors.Is(err, actor.ErrRemoteDown) {
		t.Fatalf("down: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	fake := ln.Addr().String()
	other := c.askMgr(t, mgrRegAddr(3, fake))
	if !other.OK {
		t.Fatalf("fake: %+v", other)
	}
	if r := c.askMgr(t, mgrRegName(4, other.Node, fake, "@ack")); !r.OK {
		t.Fatalf("ack name: %+v", r)
	}
	errCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := readPeerHello(conn); err != nil {
			errCh <- err
			return
		}
		if err := writePeerHello(conn, other.Node, fake); err != nil {
			errCh <- err
			return
		}
		for {
			f, err := proto.Read(conn)
			if err != nil {
				errCh <- err
				return
			}
			if f.Cmd != cmdPeerSend {
				continue
			}
			var body peerSend
			if err := f.Decode(&body); err != nil {
				errCh <- err
				return
			}
			ack, err := proto.Pack(cmdPeerAck, &peerAck{ID: body.ID})
			if err != nil {
				errCh <- err
				return
			}
			errCh <- proto.Write(conn, ack)
			return
		}
	}()
	if err := actor.SendNameAck("@ack", msg); err != nil {
		select {
		case dialErr := <-errCh:
			t.Fatalf("ack: %v dial: %v", err, dialErr)
		default:
			t.Fatal(err)
		}
	}

	prevTimeout, prevExpire := requestTimeout, expireEvery
	requestTimeout = 30 * time.Millisecond
	expireEvery = 10 * time.Millisecond
	t.Cleanup(func() {
		requestTimeout, expireEvery = prevTimeout, prevExpire
	})
	silent, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = silent.Close() })
	silentAddr := silent.Addr().String()
	quiet := c.askMgr(t, mgrRegAddr(5, silentAddr))
	if !quiet.OK {
		t.Fatalf("silent addr: %+v", quiet)
	}
	if r := c.askMgr(t, mgrRegName(6, quiet.Node, silentAddr, "@silent")); !r.OK {
		t.Fatalf("silent name: %+v", r)
	}
	go func() {
		conn, err := silent.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := readPeerHello(conn); err != nil {
			return
		}
		_ = writePeerHello(conn, quiet.Node, silentAddr)
		for {
			f, err := proto.Read(conn)
			if err != nil {
				return
			}
			if f.Cmd == cmdPeerSend {
				continue
			}
		}
	}()
	if err := actor.SendNameAck("@silent", msg); !errors.Is(err, actor.ErrRemoteTimeout) {
		t.Fatalf("timeout: %v", err)
	}
}

type callActor struct {
	actor.ActorContext
	got chan *actor.CallResponse
}

func (c *callActor) Init() error {
	return c.RegisterCmd(actor.CmdResponse, func() actor.MessageInterface { return &actor.CallResponse{} }, c.onResponse)
}

func (c *callActor) onResponse(e actor.Envelope) {
	m, ok := e.Msg.(*actor.CallResponse)
	if !ok {
		return
	}
	select {
	case c.got <- m:
	default:
	}
}

func TestCallName(t *testing.T) {
	c, _ := startBoth(t, "127.0.0.1:0")
	caller := &callActor{got: make(chan *actor.CallResponse, 1)}
	const callerName = ".caller_1"
	pid, err := actor.SpawnNamed(caller, callerName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = actor.StopActor(pid) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := actor.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}
	msg, err := actor.Pack("ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := actor.CallName(pid, ".missing", time.Second, msg); !errors.Is(err, actor.ErrUnknownAlias) {
		t.Fatalf("local missing: %v", err)
	}
	if _, err := actor.CallMemoryName(pid, "@mem", time.Second, &actor.BaseMessage{}); !errors.Is(err, actor.ErrRemoteMemory) {
		t.Fatalf("memory: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	fake := ln.Addr().String()
	other := c.askMgr(t, mgrRegAddr(1, fake))
	if !other.OK {
		t.Fatalf("fake: %+v", other)
	}
	if r := c.askMgr(t, mgrRegName(2, other.Node, fake, "@call")); !r.OK {
		t.Fatalf("call name: %+v", r)
	}
	errCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := readPeerHello(conn); err != nil {
			errCh <- err
			return
		}
		if err := writePeerHello(conn, other.Node, fake); err != nil {
			errCh <- err
			return
		}
		for {
			f, err := proto.Read(conn)
			if err != nil {
				errCh <- err
				return
			}
			if f.Cmd != cmdPeerSend {
				continue
			}
			var body peerSend
			if err := f.Decode(&body); err != nil {
				errCh <- err
				return
			}
			if body.Session == 0 || body.From != callerName || body.Dest != "@call" {
				errCh <- errors.New("call frame")
				return
			}
			ack, err := proto.Pack(cmdPeerAck, &peerAck{ID: body.ID})
			if err != nil {
				errCh <- err
				return
			}
			if err := proto.Write(conn, ack); err != nil {
				errCh <- err
				return
			}
			reply, err := proto.Pack(cmdPeerReply, &peerReply{From: body.From, Session: body.Session, Value: `"ok"`})
			if err != nil {
				errCh <- err
				return
			}
			if err := proto.Write(conn, reply); err != nil {
				errCh <- err
				return
			}
			for {
				if _, err := proto.Read(conn); err != nil {
					return
				}
			}
		}
	}()
	sess, err := actor.CallName(pid, "@call", 2*time.Second, msg)
	if err != nil {
		select {
		case dialErr := <-errCh:
			t.Fatalf("call: %v dial: %v", err, dialErr)
		default:
			t.Fatal(err)
		}
	}
	select {
	case err := <-errCh:
		t.Fatal(err)
	case resp := <-caller.got:
		if resp.Session != sess || resp.Err != nil || resp.Value != "ok" {
			t.Fatalf("response: %+v", resp)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call response")
	}
}

type replyActor struct {
	actor.ActorContext
}

func (r *replyActor) Init() error {
	return r.RegisterCmd("ping", func() actor.MessageInterface { return &actor.BaseMessage{} }, func(e actor.Envelope) {
		e.Reply("pong")
	})
}

func TestCallReply(t *testing.T) {
	c, bound := startBoth(t, "127.0.0.1:0")
	pid, err := actor.SpawnNamed(&replyActor{}, "callee")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = actor.StopActor(pid) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := actor.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var conn net.Conn
	var local string
	for {
		c, err := net.Dial("tcp", bound)
		if err != nil {
			t.Fatal(err)
		}
		local = c.LocalAddr().String()
		_ = c.SetDeadline(time.Now().Add(time.Second))
		_, err = exchangeHello(c, 99, local)
		if err == nil {
			conn = c
			break
		}
		_ = c.Close()
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	const callerName = ".caller_1"
	reg := c.askMgr(t, mgrRegAddr(1, local))
	if !reg.OK {
		t.Fatalf("caller addr: %+v", reg)
	}
	if r := c.askMgr(t, mgrRegName(2, reg.Node, local, callerName)); !r.OK {
		t.Fatalf("caller name: %+v", r)
	}
	body := peerSend{Dest: "callee", Cmd: "ping", Session: 42, From: callerName}
	frame, err := proto.Pack(cmdPeerSend, &body)
	if err != nil {
		t.Fatal(err)
	}
	if err := proto.Write(conn, frame); err != nil {
		t.Fatal(err)
	}
	for {
		f, err := proto.Read(conn)
		if err != nil {
			t.Fatal(err)
		}
		if f.Cmd != cmdPeerReply {
			continue
		}
		var reply peerReply
		if err := f.Decode(&reply); err != nil {
			t.Fatal(err)
		}
		if reply.Session != 42 || reply.From != callerName || reply.Err != "" || reply.Value != `"pong"` {
			t.Fatalf("reply: %+v", reply)
		}
		return
	}
}

func startLoose(t *testing.T) *client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := Start(ctx, "127.0.0.1:0", "127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Stop)
	c := &client{
		harbor: make(chan Result, 4),
		mgr:    make(chan harbormgr.Result, 1),
	}
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

func TestPeerRejectsStaleNode(t *testing.T) {
	c, _ := startBoth(t, "127.0.0.1:0")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	fake := ln.Addr().String()
	other := c.askMgr(t, mgrRegAddr(1, fake))
	if !other.OK {
		t.Fatalf("fake: %+v", other)
	}
	if r := c.askMgr(t, mgrRegName(2, other.Node, fake, "@stale")); !r.OK {
		t.Fatalf("name: %+v", r)
	}
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- conn
	}()
	if r := c.ask(t, query(3, "@stale")); !r.OK || r.Addr != fake {
		t.Fatalf("query: %+v", r)
	}
	var conn net.Conn
	select {
	case conn = <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("no dial")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	hello, err := readPeerHello(conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := writePeerHello(conn, hello.NodeID+100, fake); err != nil {
		t.Fatal(err)
	}
	if _, err := proto.Read(conn); err == nil {
		t.Fatal("stale node stayed up")
	}
}
