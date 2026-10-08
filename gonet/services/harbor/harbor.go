package harbor

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"gonet/actor"
	"gonet/services/harbormgr"
	"gonet/services/harbormgr/proto"
)

// Name 是本进程 harbor 的别名。地址登记拿到 nodeid 之前为空，之后直接是 .harbor_节点编号。
var Name string

var (
	readyCh   chan struct{}
	readyOnce sync.Once
	readyID   uint64
)

var (
	beatEvery      = 5 * time.Second
	requestTimeout = 3 * time.Second
	expireEvery    = time.Second
	maxQueued      = 1024
	maxWaiting     = 1024
	maxDropping    = 1024
	maxPending     = 64

	mu      sync.Mutex
	running uint64
)

// asker 是一条还在等 harbormgr 结果的本进程请求。
type asker struct {
	from     uint64
	fromName string
	id       uint64
	deadline time.Time
	done     chan Result
}

// inflight 是一条已经发给 harbormgr 的请求。
type inflight struct {
	askers []asker
	name   string
	req    uint16
}

// Actor 是本节点的 harbor。addr 是登记到通讯录的节点地址，mgrAddr 是 harbormgr 的 TCP 地址。
type Actor struct {
	actor.ActorContext
	addr        string
	mgrAddr     string
	nodeID      uint64
	addrOK      bool
	nextID      uint64
	stop        chan struct{}
	stopOnce    sync.Once
	dialBase    context.Context
	dialCancel  context.CancelFunc
	ep          *endpoint
	out         chan proto.Frame
	names       map[string]struct{}
	remoteName  map[string]remoteDest
	peers       map[string]*peer
	forwardHold map[string][]peerSend
	replyHold   map[string][]pendingReply
	acks        map[uint64]*ackWait
	peerLn      net.Listener
	idMu        sync.Mutex
	selfNode    uint64
	selfAddr    string
	revive      map[string]struct{}
	waiting     map[string][]asker
	dropping    map[string][]asker
	publishing  map[string]uint64
	cancel      map[string][]asker
	pending     map[uint64]*inflight
	queued      []pendingQuery
	addrAsks    []asker
	resyncing   bool
	resync      []string
}

type pendingQuery struct {
	ask  asker
	name string
}

// Start 拉起 harbor，并等到 Init 完成。addr 是本节点要监听并登记的地址，端口为 0 时由系统分配。返回实际监听地址。mgrAddr 是 harbormgr 的地址。
// 节点编号要等地址登记成功，用 WaitReady 取。
func Start(ctx context.Context, addr, mgrAddr string) (string, error) {
	if ctx == nil {
		return "", actor.ErrNilContext
	}
	if addr == "" {
		return "", errors.New("harbor: 监听地址不能为空")
	}
	if mgrAddr == "" {
		return "", errors.New("harbor: harbormgr 地址不能为空")
	}
	Name = ""
	readyCh = make(chan struct{})
	readyOnce = sync.Once{}
	readyID = 0
	mu.Lock()
	defer mu.Unlock()
	if running != 0 {
		return "", errors.New("harbor: 已经启动")
	}
	dialBase, dialCancel := context.WithCancel(context.Background())
	a := &Actor{
		addr:       addr,
		mgrAddr:    mgrAddr,
		stop:       make(chan struct{}),
		dialBase:   dialBase,
		dialCancel: dialCancel,
	}
	pid, err := actor.Spawn(a)
	if err != nil {
		dialCancel()
		return "", err
	}
	if err := actor.WaitInit(ctx, pid); err != nil {
		_ = actor.StopActor(pid)
		return "", err
	}
	running = pid
	actor.SetRemote(actor.RemoteHooks{
		Publish:     publishCluster,
		Unpublish:   unpublishCluster,
		Forward:     forwardCluster,
		ForwardAck:  forwardClusterAck,
		ForwardCall: forwardClusterCall,
		ReplyCall:   replyCluster,
	})
	return a.addr, nil
}

// WaitReady 等到 harbormgr 分配了 nodeid，并且本进程别名已写成 .harbor_节点编号。
func WaitReady(ctx context.Context) (uint64, error) {
	if ctx == nil {
		return 0, actor.ErrNilContext
	}
	ch := readyCh
	if ch == nil {
		return 0, errors.New("harbor: 尚未启动")
	}
	select {
	case <-ch:
		if readyID == 0 {
			return 0, errors.New("harbor: 节点编号尚未分配")
		}
		return readyID, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func markReady(id uint64) {
	readyOnce.Do(func() {
		readyID = id
		if readyCh != nil {
			close(readyCh)
		}
	})
}

// Publish 把已经在本进程的别名登记到 harbormgr。harbor 自己的别名在地址登记成功后自动登记。
func Publish(name string) error {
	if name == "" {
		return actor.ErrInvalidAlias
	}
	return publishCluster(name)
}

// Stop 停掉 harbor。未启动时直接返回。
func Stop() {
	mu.Lock()
	pid := running
	running = 0
	mu.Unlock()
	actor.ClearRemote()
	if pid == 0 {
		return
	}
	_ = actor.StopActor(pid)
}

func (a *Actor) Init() error {
	a.names = make(map[string]struct{})
	a.remoteName = make(map[string]remoteDest)
	a.peers = make(map[string]*peer)
	a.forwardHold = make(map[string][]peerSend)
	a.replyHold = make(map[string][]pendingReply)
	a.acks = make(map[uint64]*ackWait)
	a.revive = make(map[string]struct{})
	a.waiting = make(map[string][]asker)
	a.dropping = make(map[string][]asker)
	a.publishing = make(map[string]uint64)
	a.cancel = make(map[string][]asker)
	a.pending = make(map[uint64]*inflight)
	if err := a.RegisterCmds(); err != nil {
		return err
	}
	if err := a.armBeat(); err != nil {
		return err
	}
	if err := a.armExpire(); err != nil {
		return err
	}
	if err := a.bind(a.addr); err != nil {
		return err
	}
	go a.connLoop()
	slog.Info("harbor started", "pid", a.Self(), "addr", a.addr, "mgr", a.mgrAddr)
	return nil
}

func (a *Actor) Term() {
	a.unregisterRemote()
	a.halt()
	slog.Info("harbor stopped", "pid", a.Self())
}

func (a *Actor) onHello(e actor.Envelope) {
	e.Reply("hello harbor")
}

func (a *Actor) onRegisterName(e actor.Envelope) {
	m, ok := e.Msg.(*RegisterName)
	if !ok {
		return
	}
	ask := a.newAsk(e, m.ID)
	if m.Name == "" {
		a.notify(ask, CmdRegisterName, "", "", harbormgr.ErrInvalid)
		return
	}
	delete(a.dropping, m.Name)
	if _, ok := a.names[m.Name]; ok {
		a.notify(ask, CmdRegisterName, a.addr, m.Name, harbormgr.ErrTaken)
		return
	}
	if id, busy := a.publishing[m.Name]; busy {
		if p := a.pending[id]; p != nil {
			p.askers = append(p.askers, ask)
		}
		return
	}
	if !a.addrOK {
		if !a.holdWaiting(m.Name, ask) {
			a.notify(ask, CmdRegisterName, a.addr, m.Name, ErrBusy)
		}
		return
	}
	a.sendRegisterName(m.Name, []asker{ask})
}

func (a *Actor) onUnregisterName(e actor.Envelope) {
	m, ok := e.Msg.(*UnregisterName)
	if !ok {
		return
	}
	ask := a.newAsk(e, m.ID)
	if m.Name == "" {
		a.notify(ask, CmdUnregisterName, "", "", harbormgr.ErrInvalid)
		return
	}
	_, waiting := a.waiting[m.Name]
	if asks, ok := a.waiting[m.Name]; ok {
		delete(a.waiting, m.Name)
		for _, old := range asks {
			a.notify(old, CmdRegisterName, a.addr, m.Name, harbormgr.ErrUnknown)
		}
	}
	_, reviving := a.revive[m.Name]
	delete(a.revive, m.Name)
	if _, busy := a.publishing[m.Name]; busy {
		a.cancel[m.Name] = append(a.cancel[m.Name], ask)
		return
	}
	if _, ok := a.names[m.Name]; !ok {
		errText := harbormgr.ErrUnknown
		if waiting || reviving {
			errText = ""
		}
		a.notify(ask, CmdUnregisterName, a.addr, m.Name, errText)
		return
	}
	if !a.addrOK {
		if !a.holdDropping(m.Name, ask) {
			a.notify(ask, CmdUnregisterName, a.addr, m.Name, ErrBusy)
		}
		return
	}
	a.sendUnregisterName(m.Name, []asker{ask})
}

func (a *Actor) onQueryAddr(e actor.Envelope) {
	m, ok := e.Msg.(*QueryAddr)
	if !ok {
		return
	}
	ask := a.newAsk(e, m.ID)
	if m.Name == "" {
		a.notify(ask, CmdQueryAddr, "", "", harbormgr.ErrInvalid)
		return
	}
	if dest, ok := a.remoteName[m.Name]; ok {
		a.ensurePeer(dest.addr, dest.node)
		a.notify(ask, CmdQueryAddr, dest.addr, m.Name, "")
		return
	}
	if a.out == nil || len(a.pending) >= maxPending {
		a.enqueueQuery(ask, m.Name, false)
		return
	}
	a.sendQuery(m.Name, ask)
}

func (a *Actor) onSetAddr(e actor.Envelope) {
	m, ok := e.Msg.(*SetAddr)
	if !ok {
		return
	}
	ask := a.newAsk(e, m.ID)
	if m.Addr == "" {
		a.notify(ask, CmdSetAddr, "", "", harbormgr.ErrInvalid)
		return
	}
	if m.Addr != a.addr {
		if err := a.bind(m.Addr); err != nil {
			a.notify(ask, CmdSetAddr, "", "", harbormgr.ErrInvalid)
			return
		}
	}
	a.addrOK = false
	if a.out == nil {
		a.addrAsks = append(a.addrAsks, ask)
		return
	}
	a.sendRegisterAddr([]asker{ask})
}

func (a *Actor) sendQuery(name string, ask asker) {
	if len(a.pending) >= maxPending {
		a.enqueueQuery(ask, name, false)
		return
	}
	id := a.allocID()
	if err := a.sendMgr(proto.CmdQueryAddr, &proto.QueryAddr{ID: id, Name: name}); err != nil {
		a.enqueueQuery(ask, name, true)
		return
	}
	a.pending[id] = &inflight{askers: []asker{ask}, name: name, req: harbormgr.CmdQueryAddr}
}

func (a *Actor) enqueueQuery(ask asker, name string, force bool) {
	if a.expired(ask) {
		a.notify(ask, CmdQueryAddr, "", name, ErrTimeout)
		return
	}
	if !force && len(a.queued) >= maxQueued {
		a.notify(ask, CmdQueryAddr, "", name, ErrBusy)
		return
	}
	a.queued = append(a.queued, pendingQuery{ask: ask, name: name})
}

func (a *Actor) flushQueued() {
	queued := a.queued
	a.queued = nil
	for _, q := range queued {
		a.sendQuery(q.name, q.ask)
	}
}

func (a *Actor) onMgrResult(e actor.Envelope) {
	m, ok := e.Msg.(*harbormgr.Result)
	if !ok {
		return
	}
	body, ok := m.Env.Body()
	if !ok {
		slog.Warn("harbor: 未知回包", "cmd", m.Env.Cmd)
		return
	}
	id, ok := proto.IDOf(body)
	if !ok {
		return
	}
	p := a.pending[id]
	delete(a.pending, id)
	switch r := body.(type) {
	case *proto.RegisterAddrResult:
		a.onAddrReady(r, p)
	case *proto.HeartbeatResult:
		a.onBeatResult(r)
	case *proto.RegisterNameResult:
		a.onNameReady(r, p)
	case *proto.UnregisterNameResult:
		a.onNameGone(r, p)
	case *proto.QueryAddrResult:
		a.onQueryResult(r, p)
	case *proto.UnregisterAddrResult:
	default:
		slog.Warn("harbor: 未知回包", "cmd", m.Env.Cmd)
	}
}

func (a *Actor) onAddrReady(m *proto.RegisterAddrResult, p *inflight) {
	var asks []asker
	if p != nil {
		asks = p.askers
	}
	if !m.OK {
		a.addrOK = false
		if a.nodeID != 0 {
			a.giveUp(errText(m.Err))
			return
		}
		slog.Warn("harbor: 登记地址失败", "addr", a.addr, "err", m.Err)
		for _, ask := range asks {
			a.notify(ask, CmdSetAddr, a.addr, "", errText(m.Err))
		}
		return
	}
	a.nodeID = m.NodeID
	a.publishSelf()
	a.addrOK = true
	if m.Fresh {
		a.discardKept()
	}
	slog.Info("harbor addr ready", "node", a.nodeID, "addr", a.addr, "fresh", m.Fresh)
	for _, ask := range asks {
		a.notify(ask, CmdSetAddr, a.addr, "", "")
	}
	if a.resyncing {
		a.resyncing = false
		if !m.Fresh {
			a.queueResync()
		}
	}
	a.flushWaiting()
	a.flushDropping()
	a.flushResync()
	a.bindOwnName()
}

func (a *Actor) bindOwnName() {
	if a.nodeID == 0 {
		return
	}
	// 第一次拿到 nodeid 时直接挂上 .harbor_节点编号。之前没有别名。之后重连必须认回这个编号。
	if a.SelfName() == "" {
		want := actor.ServiceAlias("harbor", a.nodeID)
		if err := actor.Rebind(a.Self(), want); err != nil {
			slog.Error("harbor: 绑定节点别名失败", "name", want, "err", err)
			return
		}
		Name = want
		markReady(a.nodeID)
	}
	a.ensureOwnName()
}

func (a *Actor) ensureOwnName() {
	name := a.SelfName()
	if name == "" || !a.addrOK || a.nodeID == 0 {
		return
	}
	if _, ok := a.names[name]; ok {
		return
	}
	if _, busy := a.publishing[name]; busy {
		return
	}
	a.sendRegisterName(name, nil)
}

func (a *Actor) onBeatResult(m *proto.HeartbeatResult) {
	if m.OK {
		return
	}
	if m.Err == harbormgr.ErrTaken {
		a.giveUp("节点编号已被占用")
		return
	}
	if m.Err != harbormgr.ErrUnknown {
		return
	}
	slog.Warn("harbor: 心跳发现节点已下线", "node", a.nodeID, "addr", a.addr)
	a.addrOK = false
	a.sendRegisterAddr(nil)
}

// giveUp 在已经拿到 nodeid 之后无法再认回原编号时退出进程。换新编号会让本进程别名和目录对不上，只能重启。
func (a *Actor) giveUp(reason string) {
	slog.Error("harbor: 无法认回原节点编号，进程退出", "node", a.nodeID, "addr", a.addr, "reason", reason)
	os.Exit(1)
}

func (a *Actor) onNameReady(m *proto.RegisterNameResult, p *inflight) {
	defer a.flushResync()
	name := m.Name
	if name == "" && p != nil {
		name = p.name
	}
	delete(a.publishing, name)
	if asks, cancelled := a.cancel[name]; cancelled {
		delete(a.cancel, name)
		if p != nil {
			for _, ask := range p.askers {
				a.notify(ask, CmdRegisterName, a.addr, name, harbormgr.ErrUnknown)
			}
		}
		if m.OK {
			a.sendUnregisterName(name, asks)
			return
		}
		for _, ask := range asks {
			a.notify(ask, CmdUnregisterName, a.addr, name, "")
		}
		return
	}
	if !a.addrOK {
		if m.OK {
			if p != nil {
				a.waiting[name] = append(a.waiting[name], p.askers...)
			} else if _, ok := a.waiting[name]; !ok {
				a.waiting[name] = nil
			}
			return
		}
		if p != nil {
			for _, ask := range p.askers {
				a.notify(ask, CmdRegisterName, m.Addr, name, errText(m.Err))
			}
		}
		return
	}
	if !m.OK {
		slog.Warn("harbor: 登记别名失败", "name", name, "err", m.Err)
		if p != nil {
			for _, ask := range p.askers {
				a.notify(ask, CmdRegisterName, m.Addr, name, errText(m.Err))
			}
		}
		return
	}
	delete(a.revive, name)
	a.names[name] = struct{}{}
	slog.Info("harbor name ready", "name", name, "addr", a.addr)
	if p != nil {
		for _, ask := range p.askers {
			a.notify(ask, CmdRegisterName, a.addr, name, "")
		}
	}
}

func (a *Actor) onNameGone(m *proto.UnregisterNameResult, p *inflight) {
	name := m.Name
	if name == "" && p != nil {
		name = p.name
	}
	if !m.OK {
		if p != nil {
			for _, ask := range p.askers {
				a.notify(ask, CmdUnregisterName, a.addr, name, errText(m.Err))
			}
		}
		return
	}
	delete(a.names, name)
	delete(a.revive, name)
	slog.Info("harbor name gone", "name", name, "addr", a.addr)
	if p != nil {
		for _, ask := range p.askers {
			a.notify(ask, CmdUnregisterName, a.addr, name, "")
		}
	}
}

func (a *Actor) onQueryResult(m *proto.QueryAddrResult, p *inflight) {
	if p == nil {
		return
	}
	name := m.Name
	if name == "" {
		name = p.name
	}
	text := ""
	if !m.OK {
		text = errText(m.Err)
		a.forgetRemote(name)
		cause := "down"
		if m.Err == "" || m.Err == harbormgr.ErrUnknown {
			cause = "unknown"
		}
		a.dropForward(name, cause)
		a.dropReply(name)
	} else {
		a.noteRemote(name, m.Addr, m.NodeID)
		a.flushForward(name)
		a.flushReply(name)
	}
	for _, ask := range p.askers {
		a.notify(ask, CmdQueryAddr, m.Addr, name, text)
	}
}

func (a *Actor) onBeat(e actor.Envelope) {
	if _, ok := e.Msg.(*beatMsg); !ok {
		return
	}
	if !a.addrOK {
		a.sendRegisterAddr(nil)
	} else {
		a.flushWaiting()
		a.flushDropping()
		a.flushResync()
		a.sendHeartbeat()
	}
	a.flushQueued()
	if err := a.armBeat(); err != nil {
		slog.Error("harbor: 续心跳失败", "err", err)
	}
}

// queueResync 把本进程已经登记的别名再报一遍。只在重连认回原节点后调用。
func (a *Actor) queueResync() {
	a.resync = a.resync[:0]
	for name := range a.names {
		if _, drop := a.dropping[name]; drop {
			continue
		}
		if _, busy := a.publishing[name]; busy {
			continue
		}
		if _, wait := a.waiting[name]; wait {
			continue
		}
		a.resync = append(a.resync, name)
	}
}

func (a *Actor) flushResync() {
	if !a.addrOK || a.nodeID == 0 {
		return
	}
	left := a.resync
	a.resync = nil
	for _, name := range left {
		if _, drop := a.dropping[name]; drop {
			continue
		}
		if _, ok := a.names[name]; !ok {
			continue
		}
		if _, busy := a.publishing[name]; busy {
			continue
		}
		if _, wait := a.waiting[name]; wait {
			continue
		}
		if len(a.pending) >= maxPending {
			a.resync = append(a.resync, name)
			continue
		}
		a.sendRegisterName(name, nil)
	}
}

func (a *Actor) discardKept() {
	for name := range a.names {
		a.revive[name] = struct{}{}
	}
	a.names = make(map[string]struct{})
	for name, asks := range a.dropping {
		delete(a.dropping, name)
		delete(a.revive, name)
		for _, ask := range asks {
			a.notify(ask, CmdUnregisterName, a.addr, name, "")
		}
	}
}

func (a *Actor) flushWaiting() {
	if !a.addrOK {
		return
	}
	names := make([]string, 0, len(a.waiting))
	for name := range a.waiting {
		names = append(names, name)
	}
	for _, name := range names {
		if _, drop := a.dropping[name]; drop {
			continue
		}
		if _, busy := a.publishing[name]; busy {
			continue
		}
		askers := a.waiting[name]
		delete(a.waiting, name)
		a.sendRegisterName(name, askers)
	}
	for name := range a.revive {
		if _, drop := a.dropping[name]; drop {
			continue
		}
		if _, busy := a.publishing[name]; busy {
			continue
		}
		if _, ok := a.waiting[name]; ok {
			continue
		}
		a.sendRegisterName(name, nil)
	}
}

func (a *Actor) flushDropping() {
	if !a.addrOK {
		return
	}
	names := make([]string, 0, len(a.dropping))
	for name := range a.dropping {
		names = append(names, name)
	}
	for _, name := range names {
		if _, busy := a.publishing[name]; busy {
			continue
		}
		asks := a.dropping[name]
		delete(a.dropping, name)
		a.sendUnregisterName(name, asks)
	}
}

func (a *Actor) sendRegisterAddr(asks []asker) {
	if a.out == nil {
		a.addrAsks = append(a.addrAsks, asks...)
		return
	}
	for _, p := range a.pending {
		if p != nil && p.req == harbormgr.CmdRegisterAddr {
			p.askers = append(p.askers, asks...)
			return
		}
	}
	if len(a.pending) >= maxPending {
		a.addrAsks = append(a.addrAsks, asks...)
		return
	}
	id := a.allocID()
	if err := a.sendMgr(proto.CmdRegisterAddr, &proto.RegisterAddr{ID: id, NodeID: a.nodeID, Addr: a.addr}); err != nil {
		a.addrAsks = append(a.addrAsks, asks...)
		return
	}
	a.pending[id] = &inflight{askers: asks, req: harbormgr.CmdRegisterAddr}
}

func (a *Actor) sendRegisterName(name string, askers []asker) {
	if a.nodeID == 0 || len(a.pending) >= maxPending {
		a.waiting[name] = append(a.waiting[name], askers...)
		return
	}
	id := a.allocID()
	if err := a.sendMgr(proto.CmdRegisterName, &proto.RegisterName{ID: id, NodeID: a.nodeID, Addr: a.addr, Name: name}); err != nil {
		a.waiting[name] = append(a.waiting[name], askers...)
		return
	}
	a.publishing[name] = id
	a.pending[id] = &inflight{askers: askers, name: name, req: harbormgr.CmdRegisterName}
}

func (a *Actor) sendUnregisterName(name string, askers []asker) {
	if a.nodeID == 0 || len(a.pending) >= maxPending {
		a.dropping[name] = append(a.dropping[name], askers...)
		return
	}
	id := a.allocID()
	if err := a.sendMgr(proto.CmdUnregisterName, &proto.UnregisterName{ID: id, NodeID: a.nodeID, Addr: a.addr, Name: name}); err != nil {
		a.dropping[name] = append(a.dropping[name], askers...)
		return
	}
	a.pending[id] = &inflight{askers: askers, name: name, req: harbormgr.CmdUnregisterName}
}

func (a *Actor) sendHeartbeat() {
	if a.nodeID == 0 || len(a.pending) >= maxPending {
		return
	}
	id := a.allocID()
	if err := a.sendMgr(proto.CmdHeartbeat, &proto.Heartbeat{ID: id, NodeID: a.nodeID, Addr: a.addr}); err != nil {
		return
	}
	a.pending[id] = &inflight{req: harbormgr.CmdHeartbeat}
}

func (a *Actor) armBeat() error {
	msg := &beatMsg{}
	msg.SetCmd(cmdBeat)
	_, err := a.Timeout(beatEvery, msg)
	return err
}

func (a *Actor) armExpire() error {
	msg := &expireMsg{}
	msg.SetCmd(cmdExpire)
	_, err := a.Timeout(expireEvery, msg)
	return err
}

func (a *Actor) onExpire(e actor.Envelope) {
	if _, ok := e.Msg.(*expireMsg); !ok {
		return
	}
	a.dropExpired(time.Now())
	if err := a.armExpire(); err != nil {
		slog.Error("harbor: 续超时扫描失败", "err", err)
	}
}

func (a *Actor) dropExpired(now time.Time) {
	for name, asks := range a.waiting {
		keep, dead := splitAskers(asks, now)
		for _, ask := range dead {
			a.notify(ask, CmdRegisterName, a.addr, name, ErrTimeout)
		}
		if len(keep) == 0 {
			delete(a.waiting, name)
		} else {
			a.waiting[name] = keep
		}
	}
	for name, asks := range a.dropping {
		keep, dead := splitAskers(asks, now)
		for _, ask := range dead {
			a.notify(ask, CmdUnregisterName, a.addr, name, ErrTimeout)
		}
		if len(keep) == 0 {
			delete(a.dropping, name)
		} else {
			a.dropping[name] = keep
		}
	}
	kept := a.queued[:0]
	for _, q := range a.queued {
		if a.expired(q.ask) {
			a.notify(q.ask, CmdQueryAddr, "", q.name, ErrTimeout)
			continue
		}
		kept = append(kept, q)
	}
	a.queued = kept
	keepAddr := a.addrAsks[:0]
	for _, ask := range a.addrAsks {
		if a.expired(ask) {
			a.notify(ask, CmdSetAddr, a.addr, "", ErrTimeout)
			continue
		}
		keepAddr = append(keepAddr, ask)
	}
	a.addrAsks = keepAddr
	for id, w := range a.acks {
		if w == nil || !now.Before(w.deadline) {
			a.finishAck(id, "timeout")
		}
	}
	for id, p := range a.pending {
		if p == nil {
			delete(a.pending, id)
			continue
		}
		keep, dead := splitAskers(p.askers, now)
		req := CmdRegisterName
		switch p.req {
		case harbormgr.CmdUnregisterName:
			req = CmdUnregisterName
		case harbormgr.CmdQueryAddr:
			req = CmdQueryAddr
		case harbormgr.CmdRegisterAddr:
			req = CmdSetAddr
		}
		for _, ask := range dead {
			a.notify(ask, req, a.addr, p.name, ErrTimeout)
		}
		p.askers = keep
		if len(keep) > 0 || p.req == harbormgr.CmdRegisterAddr || p.req == harbormgr.CmdHeartbeat {
			continue
		}
		if len(dead) == 0 {
			continue
		}
		delete(a.pending, id)
		if p.req == harbormgr.CmdRegisterName {
			delete(a.publishing, p.name)
		}
	}
}

func (a *Actor) holdWaiting(name string, ask asker) bool {
	if _, ok := a.waiting[name]; !ok && len(a.waiting) >= maxWaiting {
		return false
	}
	a.waiting[name] = append(a.waiting[name], ask)
	return true
}

func (a *Actor) holdDropping(name string, ask asker) bool {
	if _, ok := a.dropping[name]; !ok && len(a.dropping) >= maxDropping {
		return false
	}
	a.dropping[name] = append(a.dropping[name], ask)
	return true
}

func (a *Actor) newAsk(e actor.Envelope, id uint64) asker {
	ask := asker{from: e.From, fromName: e.FromName, id: id, deadline: time.Now().Add(requestTimeout)}
	switch m := e.Msg.(type) {
	case *RegisterName:
		ask.done = m.done
	case *UnregisterName:
		ask.done = m.done
	}
	return ask
}

func (a *Actor) expired(ask asker) bool {
	return !ask.deadline.IsZero() && time.Now().After(ask.deadline)
}

func splitAskers(asks []asker, now time.Time) (keep, dead []asker) {
	for _, ask := range asks {
		if !ask.deadline.IsZero() && now.After(ask.deadline) {
			dead = append(dead, ask)
			continue
		}
		keep = append(keep, ask)
	}
	return keep, dead
}

func (a *Actor) allocID() uint64 {
	a.nextID++
	return a.nextID
}

func (a *Actor) notify(ask asker, req, addr, name, errText string) {
	msg := &Result{
		ID:   ask.id,
		Req:  req,
		OK:   errText == "",
		Err:  errText,
		Addr: addr,
		Name: name,
	}
	msg.SetCmd(CmdResult)
	if ask.done != nil {
		select {
		case ask.done <- *msg:
		default:
		}
		return
	}
	if ask.from == 0 && ask.fromName == "" {
		return
	}
	var err error
	switch {
	case ask.from != 0:
		err = a.Send(ask.from, msg)
	case ask.fromName != "":
		err = a.SendName(ask.fromName, msg)
	default:
		return
	}
	if err != nil {
		slog.Warn("harbor: 结果投递失败", "req", req, "err", err)
	}
}

func errText(err string) string {
	if err == "" {
		return harbormgr.ErrUnknown
	}
	return err
}
