package onlinemgr

import (
	"log/slog"

	"gonet"
)

// Name 是本进程 onlinemgr 的别名。Bind 之前为空。
var Name string

// Bind 按 harbor 分配的 nodeid 确定本进程的 onlinemgr 别名。
func Bind(nodeID uint64) {
	Name = gonet.ServiceAlias("onlinemgr", nodeID)
}

type online struct {
	alias string
	pid   uint64
}

// Actor 维护本节点在线玩家，并按名单把消息转给对应 role。
type Actor struct {
	gonet.ActorContext
	online map[string]online
}

func New() *Actor {
	return &Actor{online: make(map[string]online)}
}

func (a *Actor) Init() error {
	slog.Info("service started", "service", "onlinemgr", "pid", a.Self(), "name", a.SelfName())
	return a.RegisterCmds()
}

func (a *Actor) Term() {
	slog.Info("service stopped", "service", "onlinemgr", "pid", a.Self(), "name", a.SelfName(), "online", len(a.online))
}

func (a *Actor) RegisterCmds() error {
	if err := gonet.RegisterCmd(a, CmdLogin, func() gonet.MessageInterface { return &LoginMsg{} }, a.onLogin); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdLogout, func() gonet.MessageInterface { return &LogoutMsg{} }, a.onLogout); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdQuery, func() gonet.MessageInterface { return &QueryMsg{} }, a.onQuery); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdQueryBatch, func() gonet.MessageInterface { return &QueryBatchMsg{} }, a.onQueryBatch); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdBroadcastAll, func() gonet.MessageInterface { return &BroadcastAllMsg{} }, a.onBroadcastAll); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, CmdBroadcastSome, func() gonet.MessageInterface { return &BroadcastSomeMsg{} }, a.onBroadcastSome)
}

func (a *Actor) onLogin(e gonet.Envelope) {
	m, ok := e.Msg.(*LoginMsg)
	if !ok || m.RoleID == "" || m.Alias == "" || m.PID == 0 {
		return
	}
	a.online[m.RoleID] = online{alias: m.Alias, pid: m.PID}
	slog.Info("online", "roleid", m.RoleID, "pid", m.PID, "n", len(a.online))
}

func (a *Actor) onLogout(e gonet.Envelope) {
	m, ok := e.Msg.(*LogoutMsg)
	if !ok || m.RoleID == "" || m.PID == 0 {
		return
	}
	cur, exists := a.online[m.RoleID]
	if !exists || cur.pid != m.PID {
		return
	}
	delete(a.online, m.RoleID)
	slog.Info("offline", "roleid", m.RoleID, "pid", m.PID, "n", len(a.online))
}

func (a *Actor) onQuery(e gonet.Envelope) {
	m, _ := e.Msg.(*QueryMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(false)
		return
	}
	_, ok := a.online[m.RoleID]
	e.Reply(ok)
}

func (a *Actor) onQueryBatch(e gonet.Envelope) {
	m, _ := e.Msg.(*QueryBatchMsg)
	online := map[string]bool{}
	if m != nil {
		for _, id := range m.RoleIDs {
			if id == "" {
				continue
			}
			_, ok := a.online[id]
			online[id] = ok
		}
	}
	e.Reply(QueryBatchReply{Online: online})
}

func (a *Actor) onBroadcastAll(e gonet.Envelope) {
	m, _ := e.Msg.(*BroadcastAllMsg)
	if m == nil {
		e.Reply(0)
		return
	}
	ids := make([]string, 0, len(a.online))
	for id := range a.online {
		ids = append(ids, id)
	}
	e.Reply(a.deliver(ids, m.Cmd, m.Data))
}

func (a *Actor) onBroadcastSome(e gonet.Envelope) {
	m, _ := e.Msg.(*BroadcastSomeMsg)
	if m == nil {
		e.Reply(0)
		return
	}
	e.Reply(a.deliver(m.RoleIDs, m.Cmd, m.Data))
}

func (a *Actor) deliver(ids []string, cmd, data string) int {
	if cmd == "" {
		return 0
	}
	seen := make(map[string]struct{}, len(ids))
	n := 0
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ent, ok := a.online[id]
		if !ok || ent.alias == "" {
			continue
		}
		msg := &PushMsg{Cmd: cmd, Data: data}
		msg.SetCmd(CmdPush)
		if err := gonet.SendName(ent.alias, msg); err != nil {
			slog.Warn("online broadcast", "roleid", id, "cmd", cmd, "err", err)
			continue
		}
		n++
	}
	return n
}
