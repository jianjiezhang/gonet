package launcher

import (
	"log/slog"

	"gonet"
)

// Name 是本进程 launcher 的别名。Bind 之前为空。
var Name string

// Bind 按 harbor 分配的 nodeid 确定本进程的 launcher 别名。
func Bind(nodeID uint64) {
	Name = gonet.ServiceAlias("launcher", nodeID)
}

type pendingSpawn struct {
	reply gonet.Envelope
	pid   uint64
	name  string
}

// Actor 是进程内第一个服务：后续负责拉起、登记其它 actor。
type Actor struct {
	gonet.ActorContext
	pending map[uint64]*pendingSpawn // init session → newservice 等待回复
}

func New() *Actor {
	return &Actor{pending: make(map[uint64]*pendingSpawn)}
}

func (a *Actor) Init() error {
	slog.Info("service started", "service", "launcher", "pid", a.Self(), "name", Name)
	return a.RegisterCmds()
}

func (a *Actor) Term() {
	slog.Info("service stopped", "service", "launcher", "pid", a.Self(), "name", Name)
}

func (a *Actor) onPing(e gonet.Envelope) {
	slog.Info("pong", "pid", a.Self())
	e.Reply("pong")
}

func (a *Actor) onNewService(e gonet.Envelope) {
	m, ok := e.Msg.(*newServiceMsg)
	if !ok || m.Impl == nil {
		e.Reply(ServiceResult{Err: gonet.ErrNilActor})
		return
	}
	pid, sess, err := gonet.SpawnAsync(m.Impl, gonet.SpawnOptions{
		Name:     m.ServiceName,
		InitFrom: a.Self(),
		// InitTimeout<=0：只等 Init，不因超时失败。newservice 的 Call 超时由调用方自己定。
	})
	if err != nil {
		slog.Error("newservice", "name", m.ServiceName, "err", err)
		e.Reply(ServiceResult{Err: err})
		return
	}
	if sess == 0 {
		e.Reply(ServiceResult{PID: pid})
		return
	}
	a.pending[sess] = &pendingSpawn{reply: e, pid: pid, name: m.ServiceName}
}

func (a *Actor) onCallResponse(e gonet.Envelope) {
	m, ok := e.Msg.(*gonet.CallResponse)
	if !ok {
		return
	}
	p := a.pending[m.Session]
	delete(a.pending, m.Session)
	if p == nil {
		return
	}
	if m.Err != nil {
		slog.Error("newservice init", "name", p.name, "pid", p.pid, "err", m.Err)
		p.reply.Reply(ServiceResult{PID: p.pid, Err: m.Err})
		return
	}
	slog.Info("newservice", "pid", p.pid, "name", p.name)
	p.reply.Reply(ServiceResult{PID: p.pid})
}
