package harbormgr

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net"
	"sync"
	"time"

	"gonet/actor"
	"gonet/services/harbormgr/proto"
)

// Name 是本进程 harbormgr 的别名。UseNode 之前为空，之后直接是 .harbormgr_节点编号。
var Name string

var (
	sweepEvery = 5 * time.Second
	aliveFor   = 15 * time.Second

	mu      sync.Mutex
	running uint64
	nodeSeq NodeSeq
)

// NodeSeq 记住已经发出的最大 nodeid。harbormgr 重启后先读它，新节点只能领下一个。
// Load 在没有记录时返回 0。Save 要在把这个编号发给 harbor 之前完成。
type NodeSeq interface {
	Load(ctx context.Context) (uint64, error)
	Save(ctx context.Context, nodeID uint64) error
}

// UseNodeSeq 在 Start 之前装上编号记录。不装时编号只留在内存里，重启从 0 再计。
func UseNodeSeq(seq NodeSeq) {
	mu.Lock()
	nodeSeq = seq
	mu.Unlock()
}

// nodeBook 是一个节点的当前地址、别名、最后存活时间，以及正在使用它的连接。
// token 为 0 表示没有活连接，记录仍保留到存活时间结束。
type nodeBook struct {
	addr  string
	names map[string]struct{}
	seen  time.Time
	token uint64
}

// Actor 是唯一的 harbormgr，维护别名到节点的通讯录。harbor 通过 TCP 连到监听地址。
type Actor struct {
	actor.ActorContext
	listen    string
	bound     string
	stop      chan struct{}
	stopOnce  sync.Once
	ln        net.Listener
	nextNode  uint64
	seq       NodeSeq
	seqQueue  []seqJob
	seqBusy   bool
	seqSaving uint64
	byNode    map[uint64]*nodeBook
	byName    map[string]uint64
	byAddr    map[string]uint64
}

// seqJob 是一次要占编号的地址登记。node 为 0 表示新领编号，否则是认回这个编号。
type seqJob struct {
	e    actor.Envelope
	m    *RegisterAddr
	node uint64
}

// TestingTTL 把扫表间隔和存活时间改成测试用的短时间，并返回恢复函数。
func TestingTTL(alive, sweep time.Duration) func() {
	prevAlive, prevSweep := aliveFor, sweepEvery
	aliveFor, sweepEvery = alive, sweep
	return func() {
		aliveFor, sweepEvery = prevAlive, prevSweep
	}
}

// Start 监听 addr 并拉起 harbormgr，等到 Init 完成。端口为 0 时返回实际地址。
// 别名要等本进程 harbor 拿到 nodeid 后，由 UseNode 写成 .harbormgr_节点编号。
func Start(ctx context.Context, addr string) (string, error) {
	if ctx == nil {
		return "", actor.ErrNilContext
	}
	if addr == "" {
		return "", errors.New("harbormgr: 监听地址不能为空")
	}
	Name = ""
	mu.Lock()
	seq := nodeSeq
	mu.Unlock()
	var next uint64
	if seq != nil {
		n, err := seq.Load(ctx)
		if err != nil {
			return "", err
		}
		next = n
	}
	mu.Lock()
	defer mu.Unlock()
	if running != 0 {
		return "", errors.New("harbormgr: 已经启动")
	}
	a := &Actor{listen: addr, stop: make(chan struct{}), nextNode: next, seq: seq}
	pid, err := actor.Spawn(a)
	if err != nil {
		return "", err
	}
	if err := actor.WaitInit(ctx, pid); err != nil {
		_ = actor.StopActor(pid)
		return "", err
	}
	running = pid
	return a.bound, nil
}

// UseNode 直接挂上 .harbormgr_节点编号。nodeID 是本进程 harbor 分配到的编号。
func UseNode(nodeID uint64) error {
	if nodeID == 0 {
		return errors.New("harbormgr: 节点编号不能为 0")
	}
	mu.Lock()
	pid := running
	mu.Unlock()
	if pid == 0 {
		return errors.New("harbormgr: 尚未启动")
	}
	name := actor.ServiceAlias("harbormgr", nodeID)
	if err := actor.Rebind(pid, name); err != nil {
		return err
	}
	Name = name
	return nil
}

// Stop 停掉 harbormgr。未启动时直接返回。
func Stop() {
	mu.Lock()
	pid := running
	running = 0
	mu.Unlock()
	if pid == 0 {
		return
	}
	_ = actor.StopActor(pid)
}

func (a *Actor) Init() error {
	a.byNode = make(map[uint64]*nodeBook)
	a.byName = make(map[string]uint64)
	a.byAddr = make(map[string]uint64)
	if err := a.RegisterCmds(); err != nil {
		return err
	}
	if err := a.armSweep(); err != nil {
		return err
	}
	if err := a.openListen(a.listen); err != nil {
		return err
	}
	slog.Info("harbormgr started", "pid", a.Self(), "addr", a.bound)
	return nil
}

func (a *Actor) Term() {
	a.closeListen()
	slog.Info("harbormgr stopped", "pid", a.Self())
}

func (a *Actor) onHello(e actor.Envelope) {
	e.Reply("hello harbormgr")
}

func (a *Actor) onRegisterAddr(e actor.Envelope) {
	m, ok := e.Msg.(*RegisterAddr)
	if !ok {
		return
	}
	if m.Addr == "" {
		a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{Status: done(m.ID, ErrInvalid)})
		return
	}
	if m.NodeID == 0 {
		a.allocNode(e, m)
		return
	}
	book := a.byNode[m.NodeID]
	if book == nil {
		a.reclaimExpired(e, m)
		return
	}
	if book.token != 0 && book.token != m.token {
		a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{Status: done(m.ID, ErrTaken), NodeID: m.NodeID, Addr: book.addr})
		return
	}
	if owner, exists := a.byAddr[m.Addr]; exists && owner != m.NodeID {
		a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{Status: done(m.ID, ErrTaken), NodeID: m.NodeID, Addr: m.Addr})
		return
	}
	if book.addr != m.Addr {
		delete(a.byAddr, book.addr)
		book.addr = m.Addr
		a.byAddr[m.Addr] = m.NodeID
		slog.Info("addr update", "node", m.NodeID, "addr", m.Addr)
	}
	book.token = m.token
	book.seen = time.Now()
	a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{Status: done(m.ID, ""), NodeID: m.NodeID, Addr: book.addr})
}

// reclaimExpired 在目录里没有这条记录时，按 harbor 报上来的原编号重建。
// 编号大于已发出的最大值时，先把最大值记下来再回复，避免新节点领走这个编号。
// 地址已被别的节点占用则失败。
func (a *Actor) reclaimExpired(e actor.Envelope, m *RegisterAddr) {
	if m.NodeID == 0 {
		a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{Status: done(m.ID, ErrUnknown), NodeID: m.NodeID, Addr: m.Addr})
		return
	}
	if owner, exists := a.byAddr[m.Addr]; exists && owner != m.NodeID {
		a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{Status: done(m.ID, ErrTaken), NodeID: m.NodeID, Addr: m.Addr})
		return
	}
	a.enqueueSeq(seqJob{e: e, m: m, node: m.NodeID})
}

func (a *Actor) allocNode(e actor.Envelope, m *RegisterAddr) {
	if owner, exists := a.byAddr[m.Addr]; exists {
		book := a.byNode[owner]
		// 旧 TCP 已断开且这次是新连接：删掉过期记录，按 harbor 当前状态重新登记。
		// 进程内调用 token 为 0，活连接 token 非 0，这两种仍视为地址被占用。
		if book != nil && book.token == 0 && m.token != 0 {
			slog.Info("node replaced", "node", owner, "addr", book.addr)
			a.dropNode(owner, book)
		} else {
			if book == nil {
				delete(a.byAddr, m.Addr)
			} else {
				a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{Status: done(m.ID, ErrTaken), Addr: m.Addr})
				return
			}
		}
	}
	a.enqueueSeq(seqJob{e: e, m: m})
}

func (a *Actor) enqueueSeq(job seqJob) {
	a.seqQueue = append(a.seqQueue, job)
	a.pumpSeq()
}

// pumpSeq 按队列占用编号。新编号和抬高水位都要先写入 NodeSeq，再建立记录。
func (a *Actor) pumpSeq() {
	for !a.seqBusy && len(a.seqQueue) > 0 {
		job := a.seqQueue[0]
		n, save := a.seqTarget(job)
		if n == 0 {
			a.seqQueue = a.seqQueue[1:]
			slog.Error("harbormgr: 节点编号已用尽")
			a.reply(job.e, CmdRegisterAddr, &proto.RegisterAddrResult{Status: done(job.m.ID, ErrInvalid), Addr: job.m.Addr})
			continue
		}
		if !save || a.seq == nil {
			a.seqQueue = a.seqQueue[1:]
			if save {
				a.nextNode = n
			}
			a.commitSeq(job, n)
			continue
		}
		a.seqBusy = true
		a.seqSaving = n
		go a.persistSeq(a.Self(), a.seq, n, a.stop)
		return
	}
}

func (a *Actor) seqTarget(job seqJob) (uint64, bool) {
	if job.node == 0 {
		return a.allocID(), true
	}
	if job.node > a.nextNode {
		return job.node, true
	}
	return job.node, false
}

// allocID 取下一个编号。到 uint64 最大值后从 1 继续，并跳过目录里还在的编号。
func (a *Actor) allocID() uint64 {
	start := uint64(1)
	if a.nextNode < math.MaxUint64 {
		start = a.nextNode + 1
	}
	id := start
	for {
		if id != 0 && a.byNode[id] == nil {
			return id
		}
		if id == math.MaxUint64 {
			id = 1
		} else {
			id++
		}
		if id == start {
			return 0
		}
	}
}

func (a *Actor) persistSeq(pid uint64, seq NodeSeq, n uint64, stop <-chan struct{}) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := seq.Save(ctx, n)
		cancel()
		if err == nil {
			msg := &seqSaved{node: n, ok: true}
			msg.SetCmd(CmdKey(CmdSeq))
			if err := actor.SendMemory(pid, msg); err != nil {
				select {
				case <-stop:
					return
				case <-time.After(200 * time.Millisecond):
				}
				continue
			}
			return
		}
		slog.Warn("harbormgr: 保存节点编号失败", "node", n, "err", err)
		select {
		case <-stop:
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (a *Actor) onSeqSaved(e actor.Envelope) {
	m, ok := e.Msg.(*seqSaved)
	if !ok || !m.ok || !a.seqBusy || m.node != a.seqSaving || len(a.seqQueue) == 0 {
		return
	}
	a.seqBusy = false
	job := a.seqQueue[0]
	a.seqQueue = a.seqQueue[1:]
	a.nextNode = m.node
	a.commitSeq(job, m.node)
	a.pumpSeq()
}

func (a *Actor) commitSeq(job seqJob, id uint64) {
	if job.node == 0 {
		a.commitNew(job.e, job.m, id)
		return
	}
	a.commitReclaim(job.e, job.m)
}

func (a *Actor) commitNew(e actor.Envelope, m *RegisterAddr, id uint64) {
	if owner, exists := a.byAddr[m.Addr]; exists {
		a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{Status: done(m.ID, ErrTaken), Addr: m.Addr, NodeID: owner})
		return
	}
	book := &nodeBook{
		addr:  m.Addr,
		names: make(map[string]struct{}),
		seen:  time.Now(),
		token: m.token,
	}
	a.byNode[id] = book
	a.byAddr[m.Addr] = id
	slog.Info("node register", "node", id, "addr", m.Addr)
	a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{
		Status: done(m.ID, ""),
		NodeID: id,
		Addr:   m.Addr,
		Fresh:  true,
	})
}

func (a *Actor) commitReclaim(e actor.Envelope, m *RegisterAddr) {
	if book := a.byNode[m.NodeID]; book != nil {
		a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{Status: done(m.ID, ErrTaken), NodeID: m.NodeID, Addr: book.addr})
		return
	}
	if owner, exists := a.byAddr[m.Addr]; exists && owner != m.NodeID {
		a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{Status: done(m.ID, ErrTaken), NodeID: m.NodeID, Addr: m.Addr})
		return
	}
	book := &nodeBook{
		addr:  m.Addr,
		names: make(map[string]struct{}),
		seen:  time.Now(),
		token: m.token,
	}
	a.byNode[m.NodeID] = book
	a.byAddr[m.Addr] = m.NodeID
	slog.Info("node reclaim", "node", m.NodeID, "addr", m.Addr)
	a.reply(e, CmdRegisterAddr, &proto.RegisterAddrResult{
		Status: done(m.ID, ""),
		NodeID: m.NodeID,
		Addr:   m.Addr,
		Fresh:  true,
	})
}

func (a *Actor) onRegisterName(e actor.Envelope) {
	m, ok := e.Msg.(*RegisterName)
	if !ok {
		return
	}
	if m.NodeID == 0 || m.Addr == "" || m.Name == "" {
		a.reply(e, CmdRegisterName, &proto.RegisterNameResult{Status: done(m.ID, ErrInvalid), NodeID: m.NodeID, Addr: m.Addr, Name: m.Name})
		return
	}
	book, errText := a.owned(m.NodeID, m.token)
	if errText != "" {
		addr := m.Addr
		if book != nil {
			addr = book.addr
		}
		a.reply(e, CmdRegisterName, &proto.RegisterNameResult{Status: done(m.ID, errText), NodeID: m.NodeID, Addr: addr, Name: m.Name})
		return
	}
	if book.addr != m.Addr {
		a.reply(e, CmdRegisterName, &proto.RegisterNameResult{Status: done(m.ID, ErrUnknown), NodeID: m.NodeID, Addr: book.addr, Name: m.Name})
		return
	}
	if cur, exists := a.byName[m.Name]; exists && cur != m.NodeID {
		addr := ""
		if owner := a.byNode[cur]; owner != nil {
			addr = owner.addr
		}
		a.reply(e, CmdRegisterName, &proto.RegisterNameResult{Status: done(m.ID, ErrTaken), NodeID: m.NodeID, Addr: addr, Name: m.Name})
		return
	}
	a.byName[m.Name] = m.NodeID
	book.names[m.Name] = struct{}{}
	book.seen = time.Now()
	slog.Info("name register", "name", m.Name, "node", m.NodeID, "addr", m.Addr)
	a.reply(e, CmdRegisterName, &proto.RegisterNameResult{Status: done(m.ID, ""), NodeID: m.NodeID, Addr: m.Addr, Name: m.Name})
}

func (a *Actor) onHeartbeat(e actor.Envelope) {
	m, ok := e.Msg.(*Heartbeat)
	if !ok {
		return
	}
	if m.NodeID == 0 {
		a.reply(e, CmdHeartbeat, &proto.HeartbeatResult{Status: done(m.ID, ErrInvalid)})
		return
	}
	book, errText := a.owned(m.NodeID, m.token)
	if errText != "" {
		addr := m.Addr
		if book != nil {
			addr = book.addr
		}
		a.reply(e, CmdHeartbeat, &proto.HeartbeatResult{Status: done(m.ID, errText), NodeID: m.NodeID, Addr: addr})
		return
	}
	book.seen = time.Now()
	a.reply(e, CmdHeartbeat, &proto.HeartbeatResult{Status: done(m.ID, ""), NodeID: m.NodeID, Addr: book.addr})
}

func (a *Actor) onQueryAddr(e actor.Envelope) {
	m, ok := e.Msg.(*QueryAddr)
	if !ok {
		return
	}
	if m.Name == "" {
		a.reply(e, CmdQueryAddr, &proto.QueryAddrResult{Status: done(m.ID, ErrInvalid)})
		return
	}
	node, exists := a.byName[m.Name]
	book := a.byNode[node]
	if !exists || book == nil {
		a.reply(e, CmdQueryAddr, &proto.QueryAddrResult{Status: done(m.ID, ErrUnknown), Name: m.Name})
		return
	}
	a.reply(e, CmdQueryAddr, &proto.QueryAddrResult{Status: done(m.ID, ""), NodeID: node, Addr: book.addr, Name: m.Name})
}

func (a *Actor) onUnregisterName(e actor.Envelope) {
	m, ok := e.Msg.(*UnregisterName)
	if !ok {
		return
	}
	if m.NodeID == 0 || m.Addr == "" || m.Name == "" {
		a.reply(e, CmdUnregisterName, &proto.UnregisterNameResult{Status: done(m.ID, ErrInvalid), NodeID: m.NodeID, Addr: m.Addr, Name: m.Name})
		return
	}
	book, errText := a.owned(m.NodeID, m.token)
	if errText != "" {
		addr := m.Addr
		if book != nil {
			addr = book.addr
		}
		a.reply(e, CmdUnregisterName, &proto.UnregisterNameResult{Status: done(m.ID, errText), NodeID: m.NodeID, Addr: addr, Name: m.Name})
		return
	}
	if a.byName[m.Name] != m.NodeID {
		a.reply(e, CmdUnregisterName, &proto.UnregisterNameResult{Status: done(m.ID, ErrUnknown), NodeID: m.NodeID, Addr: book.addr, Name: m.Name})
		return
	}
	delete(a.byName, m.Name)
	delete(book.names, m.Name)
	book.seen = time.Now()
	slog.Info("name unregister", "name", m.Name, "node", m.NodeID)
	a.reply(e, CmdUnregisterName, &proto.UnregisterNameResult{Status: done(m.ID, ""), NodeID: m.NodeID, Addr: book.addr, Name: m.Name})
}

func (a *Actor) onUnregisterAddr(e actor.Envelope) {
	m, ok := e.Msg.(*UnregisterAddr)
	if !ok {
		return
	}
	if m.NodeID == 0 {
		a.reply(e, CmdUnregisterAddr, &proto.UnregisterAddrResult{Status: done(m.ID, ErrInvalid), Addr: m.Addr})
		return
	}
	book, errText := a.owned(m.NodeID, m.token)
	if errText != "" {
		addr := m.Addr
		if book != nil {
			addr = book.addr
		}
		a.reply(e, CmdUnregisterAddr, &proto.UnregisterAddrResult{Status: done(m.ID, errText), NodeID: m.NodeID, Addr: addr})
		return
	}
	a.dropNode(m.NodeID, book)
	slog.Info("node unregister", "node", m.NodeID, "addr", book.addr)
	a.reply(e, CmdUnregisterAddr, &proto.UnregisterAddrResult{Status: done(m.ID, ""), NodeID: m.NodeID, Addr: book.addr})
}

func (a *Actor) onSweep(e actor.Envelope) {
	if _, ok := e.Msg.(*sweepMsg); !ok {
		return
	}
	now := time.Now()
	for id, book := range a.byNode {
		if now.Sub(book.seen) <= aliveFor {
			continue
		}
		addr := book.addr
		a.dropNode(id, book)
		slog.Info("node expired", "node", id, "addr", addr)
	}
	if err := a.armSweep(); err != nil {
		slog.Error("harbormgr: 续扫失败", "err", err)
	}
}

func (a *Actor) onUnbind(e actor.Envelope) {
	m, ok := e.Msg.(*unbindMsg)
	if !ok || m.token == 0 {
		return
	}
	for _, book := range a.byNode {
		if book.token == m.token {
			book.token = 0
			return
		}
	}
}

// owned 确认这条请求来自该节点当前绑定的连接。进程内调用双方 token 都是 0。
func (a *Actor) owned(node, token uint64) (*nodeBook, string) {
	book := a.byNode[node]
	if book == nil {
		return nil, ErrUnknown
	}
	if book.token != token {
		return book, ErrTaken
	}
	return book, ""
}

func (a *Actor) dropNode(id uint64, book *nodeBook) {
	for name := range book.names {
		if a.byName[name] == id {
			delete(a.byName, name)
		}
	}
	delete(a.byAddr, book.addr)
	delete(a.byNode, id)
}

func (a *Actor) armSweep() error {
	msg := &sweepMsg{}
	msg.SetCmd(CmdKey(CmdSweep))
	_, err := a.Timeout(sweepEvery, msg)
	return err
}

func done(id uint64, errText string) proto.Status {
	return proto.Status{ID: id, OK: errText == "", Err: errText}
}

func (a *Actor) reply(e actor.Envelope, cmd uint16, payload any) {
	res, err := proto.NewResult(cmd, payload)
	if err != nil {
		slog.Warn("harbormgr: 回复编码失败", "cmd", cmd, "err", err)
		return
	}
	msg := &Result{Env: res}
	msg.SetCmd(CmdKey(CmdResult))
	if src, ok := e.Msg.(interface{ takeBack() chan Result }); ok {
		if back := src.takeBack(); back != nil {
			select {
			case back <- *msg:
			default:
			}
			return
		}
	}
	var sendErr error
	switch {
	case e.From != 0:
		sendErr = a.Send(e.From, msg)
	case e.FromName != "":
		sendErr = a.SendName(e.FromName, msg)
	default:
		slog.Warn("harbormgr: 结果没有接收方", "cmd", cmd)
		return
	}
	if sendErr != nil {
		slog.Warn("harbormgr: 结果投递失败", "cmd", cmd, "err", sendErr)
	}
}
