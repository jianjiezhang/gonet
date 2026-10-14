package monitor

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"gonet/actor"

	"gonet/services/internal/actormsg"
)

// Name 是本进程 monitor 的别名。Bind 之前为空。
var Name string

// Bind 按 harbor 分配的 nodeid 确定本进程的 monitor 别名。
func Bind(nodeID uint64) {
	Name = actor.ServiceAlias("monitor", nodeID)
}

// Actor 持有本节点 debug 口。花名册由 SnapshotNow 直接读，本 actor 不 Call 任何人。
type Actor struct {
	actor.ActorContext
	addr  string
	bound string
	debug debugServer
}

func New(addr string) *Actor {
	return &Actor{addr: addr}
}

var (
	gate    sync.Mutex
	running uint64
)

// Start 拉起 monitor actor 并等到 debug 口开始监听。已在监听则失败。
func Start(ctx context.Context, addr string) (string, error) {
	if ctx == nil {
		return "", actor.ErrNilContext
	}
	gate.Lock()
	defer gate.Unlock()
	if running != 0 && actor.Alive(running) {
		return "", errors.New("monitor: debug 已在监听")
	}
	running = 0
	a := New(addr)
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

// Stop 停掉 Start 拉起的 monitor。未由 Start 拉起时直接返回。
func Stop() error {
	gate.Lock()
	pid := running
	running = 0
	gate.Unlock()
	if pid == 0 {
		return nil
	}
	err := actor.StopActor(pid)
	if errors.Is(err, actor.ErrDead) {
		return nil
	}
	return err
}

func (a *Actor) Init() error {
	if err := a.RegisterCmds(); err != nil {
		return err
	}
	if err := a.listen(); err != nil {
		return err
	}
	slog.Info("monitor started", "pid", a.Self(), "name", a.SelfName(), "addr", a.bound)
	return nil
}

func (a *Actor) Term() {
	a.closeListen()
	slog.Info("monitor stopped", "pid", a.Self(), "name", a.SelfName())
}

func (a *Actor) RegisterCmds() error {
	return a.RegisterCmd(actormsg.CmdMonitor, func() actor.MessageInterface { return &actormsg.MonitorMsg{} }, a.onMonitor)
}

// onMonitor 读花名册后 Reply。不向其它 actor 发 Call。
func (a *Actor) onMonitor(e actor.Envelope) {
	e.Reply(SnapshotNow())
}
