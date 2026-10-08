package harbor

import (
	"log/slog"
	"net"
	"time"

	"gonet/actor"
	"gonet/services/harbormgr/proto"
)

// cmdPeerHello 只出现在 harbor 与 harbor 的连接上。两边都报自己的节点编号和登记地址。
const (
	cmdPeerHello uint16 = 1
	cmdPeerSend  uint16 = 2
	cmdPeerAck   uint16 = 3
	cmdPeerReply uint16 = 4
)

type peerHello struct {
	NodeID uint64 `json:"node"`
	Addr   string `json:"addr"`
}

// remoteDest 是通讯录里另一个节点的别名落到哪条地址。
type remoteDest struct {
	addr string
	node uint64
}

// peer 是到另一个节点登记地址的一条连接。dialing 时 ep 还是空的。
type peer struct {
	addr    string
	node    uint64
	ep      *endpoint
	out     chan proto.Frame
	dialing bool
	hold    []proto.Frame
}

type peerEvent struct {
	actor.BaseMessage
	ok     bool
	dialed bool
	addr   string
	node   uint64
	ep     *endpoint
	out    chan proto.Frame
	stub   *peer
	data   bool
	frame  proto.Frame
}

func (a *Actor) publishSelf() {
	a.idMu.Lock()
	a.selfNode = a.nodeID
	a.selfAddr = a.addr
	a.idMu.Unlock()
}

func (a *Actor) snapshot() (uint64, string) {
	a.idMu.Lock()
	defer a.idMu.Unlock()
	return a.selfNode, a.selfAddr
}

func (a *Actor) bind(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	if a.peerLn != nil {
		_ = a.peerLn.Close()
	}
	a.peerLn = ln
	a.addr = ln.Addr().String()
	a.publishSelf()
	go a.acceptPeers(ln)
	return nil
}

func (a *Actor) acceptPeers(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go a.handshakeIn(conn)
	}
}

func (a *Actor) handshakeIn(conn net.Conn) {
	node, addr := a.snapshot()
	if node == 0 || addr == "" {
		_ = conn.Close()
		return
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	remote, err := exchangeHello(conn, node, addr)
	if err != nil || remote.Addr == "" || remote.Addr == addr || remote.NodeID == 0 || remote.NodeID == node {
		_ = conn.Close()
		return
	}
	_ = conn.SetDeadline(time.Time{})
	a.servePeer(a.Self(), conn, remote.Addr, false, nil, remote.NodeID)
}

func (a *Actor) ensurePeer(addr string, node uint64) {
	if addr == "" || addr == a.addr || node == 0 || node == a.nodeID || a.nodeID == 0 {
		return
	}
	if _, ok := a.peers[addr]; ok {
		return
	}
	stub := &peer{addr: addr, node: node, dialing: true}
	a.peers[addr] = stub
	go a.dialPeer(a.Self(), stub)
}

func (a *Actor) dialPeer(pid uint64, stub *peer) {
	node, addr := a.snapshot()
	if node == 0 || addr == "" {
		postPeer(pid, &peerEvent{addr: stub.addr, stub: stub})
		return
	}
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.Dial("tcp", stub.addr)
	if err != nil {
		slog.Warn("harbor: 连接节点失败", "addr", stub.addr, "err", err)
		postPeer(pid, &peerEvent{addr: stub.addr, stub: stub})
		return
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	remote, err := exchangeHello(conn, node, addr)
	if err != nil || remote.Addr != stub.addr || remote.NodeID == 0 || remote.NodeID == node || remote.NodeID != stub.node {
		_ = conn.Close()
		postPeer(pid, &peerEvent{addr: stub.addr, stub: stub})
		return
	}
	_ = conn.SetDeadline(time.Time{})
	a.servePeer(pid, conn, remote.Addr, true, stub, remote.NodeID)
}

func (a *Actor) servePeer(pid uint64, conn net.Conn, dirAddr string, dialed bool, stub *peer, remoteNode uint64) {
	ep := &endpoint{conn: conn}
	out := make(chan proto.Frame, 16)
	quit := make(chan struct{})
	defer close(quit)
	defer ep.Close()
	defer postPeer(pid, &peerEvent{addr: dirAddr, ep: ep})

	postPeer(pid, &peerEvent{
		ok:     true,
		dialed: dialed,
		addr:   dirAddr,
		node:   remoteNode,
		ep:     ep,
		out:    out,
		stub:   stub,
	})

	reads := make(chan proto.Frame, 8)
	go func() {
		defer close(reads)
		for {
			f, err := proto.Read(conn)
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
			postPeer(pid, &peerEvent{data: true, addr: dirAddr, ep: ep, out: out, frame: f})
		case f := <-out:
			if err := ep.WriteFrame(f); err != nil {
				return
			}
		}
	}
}

func (a *Actor) onPeer(e actor.Envelope) {
	m, ok := e.Msg.(*peerEvent)
	if !ok {
		return
	}
	if m.data {
		a.onPeerData(m)
		return
	}
	if !m.ok {
		a.onPeerDown(m)
		return
	}
	a.onPeerUp(m)
}

func (a *Actor) onPeerUp(m *peerEvent) {
	if m.ep == nil || m.addr == "" || m.addr == a.addr {
		if m.ep != nil {
			m.ep.Close()
		}
		return
	}
	cur := a.peers[m.addr]
	if m.stub != nil && cur != m.stub {
		if cur == nil || cur.ep == nil {
			m.ep.Close()
			return
		}
	}
	if cur != nil && cur.ep != nil && cur.ep != m.ep {
		if !keepDial(a.nodeID, m.node, m.dialed) {
			m.ep.Close()
			return
		}
		cur.ep.Close()
	}
	var held []proto.Frame
	if cur != nil {
		held = cur.hold
	}
	np := &peer{addr: m.addr, node: m.node, ep: m.ep, out: m.out}
	a.peers[m.addr] = np
	a.dropStale(m.addr, m.node)
	for _, f := range held {
		select {
		case np.out <- f:
		default:
			slog.Warn("harbor: 发往节点的队列已满", "addr", m.addr)
		}
	}
	slog.Info("harbor peer up", "addr", m.addr, "node", m.node, "dialed", m.dialed)
}

func (a *Actor) onPeerData(m *peerEvent) {
	cur := a.peers[m.addr]
	if cur == nil || cur.ep != m.ep {
		return
	}
	switch m.frame.Cmd {
	case cmdPeerAck:
		a.onPeerAck(m)
	case cmdPeerReply:
		a.onPeerReply(m)
	case cmdPeerSend:
		var body peerSend
		if err := m.frame.Decode(&body); err != nil || body.Dest == "" {
			return
		}
		a.onPeerSend(m, body)
	}
}

func (a *Actor) onPeerDown(m *peerEvent) {
	if m.ep == nil {
		if m.stub == nil || a.peers[m.addr] != m.stub {
			return
		}
		delete(a.peers, m.addr)
		a.dropRemoteAddr(m.addr)
		a.failAcks(m.addr, "down")
		return
	}
	cur := a.peers[m.addr]
	if cur == nil || cur.ep != m.ep {
		return
	}
	delete(a.peers, m.addr)
	a.failAcks(m.addr, "down")
	slog.Info("harbor peer down", "addr", m.addr)
}

func (a *Actor) noteRemote(name, addr string, node uint64) {
	if name == "" {
		return
	}
	if addr == "" || addr == a.addr || node == 0 || node == a.nodeID {
		a.forgetRemote(name)
		return
	}
	if old, ok := a.remoteName[name]; ok && old.addr != addr {
		a.remoteName[name] = remoteDest{addr: addr, node: node}
		a.maybeClose(old.addr)
	} else {
		a.remoteName[name] = remoteDest{addr: addr, node: node}
	}
	a.ensurePeer(addr, node)
}

func (a *Actor) forgetRemote(name string) {
	old, ok := a.remoteName[name]
	if !ok {
		return
	}
	delete(a.remoteName, name)
	a.maybeClose(old.addr)
}

// dropStale 丢掉仍指向这个地址、但节点编号已经对不上的别名缓存。
func (a *Actor) dropStale(addr string, node uint64) {
	for name, dest := range a.remoteName {
		if dest.addr == addr && dest.node != node {
			delete(a.remoteName, name)
		}
	}
}

func (a *Actor) dropRemoteAddr(addr string) {
	for name, dest := range a.remoteName {
		if dest.addr == addr {
			delete(a.remoteName, name)
		}
	}
}

func (a *Actor) maybeClose(addr string) {
	for _, dest := range a.remoteName {
		if dest.addr == addr {
			return
		}
	}
	p := a.peers[addr]
	if p == nil {
		return
	}
	delete(a.peers, addr)
	if p.ep != nil {
		p.ep.Close()
	}
}

// keepDial 在同一登记地址上只留一条连接。节点编号更小的一方拨号，留下它拨出的那条。
func keepDial(self, remote uint64, dialed bool) bool {
	if self == 0 || remote == 0 || self == remote {
		return false
	}
	if self < remote {
		return dialed
	}
	return !dialed
}

func postPeer(pid uint64, ev *peerEvent) {
	ev.SetCmd(cmdPeer)
	post(pid, ev)
}

func exchangeHello(conn net.Conn, node uint64, addr string) (peerHello, error) {
	if err := writePeerHello(conn, node, addr); err != nil {
		return peerHello{}, err
	}
	return readPeerHello(conn)
}

func writePeerHello(conn net.Conn, node uint64, addr string) error {
	f, err := proto.Pack(cmdPeerHello, &peerHello{NodeID: node, Addr: addr})
	if err != nil {
		return err
	}
	return proto.Write(conn, f)
}

func readPeerHello(conn net.Conn) (peerHello, error) {
	f, err := proto.Read(conn)
	if err != nil {
		return peerHello{}, err
	}
	if f.Cmd != cmdPeerHello {
		return peerHello{}, proto.ErrEmpty
	}
	var hello peerHello
	if err := f.Decode(&hello); err != nil {
		return peerHello{}, err
	}
	return hello, nil
}
