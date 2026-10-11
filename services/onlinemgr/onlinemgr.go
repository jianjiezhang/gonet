package onlinemgr

import (
	"log/slog"

	"game/services/internal/actormsg"
	"game/services/internal/names"

	"gonet"
)

// Name 是全服在线表的别名。全服只有一个，不加 nodeid。
const Name = names.Onlinemgr

type online struct {
	alias string
	pid   uint64
}

// Actor 维护全服在线玩家，并按名单把消息转给对应 role。
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
	if err := gonet.RegisterCmd(a, actormsg.CmdOnlineLogin, func() gonet.MessageInterface { return &actormsg.OnlineLoginMsg{} }, a.onLogin); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdOnlineLogout, func() gonet.MessageInterface { return &actormsg.OnlineLogoutMsg{} }, a.onLogout); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdOnlineQuery, func() gonet.MessageInterface { return &actormsg.OnlineQueryMsg{} }, a.onQuery); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdOnlineQueryBatch, func() gonet.MessageInterface { return &actormsg.OnlineQueryBatchMsg{} }, a.onQueryBatch); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdOnlineBroadcastAll, func() gonet.MessageInterface { return &actormsg.OnlineBroadcastAllMsg{} }, a.onBroadcastAll); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, actormsg.CmdOnlineBroadcastSome, func() gonet.MessageInterface { return &actormsg.OnlineBroadcastSomeMsg{} }, a.onBroadcastSome)
}

func (a *Actor) onLogin(e gonet.Envelope) {
	m, ok := e.Msg.(*actormsg.OnlineLoginMsg)
	if !ok || m.RoleID == "" || m.Alias == "" || m.PID == 0 {
		return
	}
	a.online[m.RoleID] = online{alias: m.Alias, pid: m.PID}
	slog.Info("online", "roleid", m.RoleID, "pid", m.PID, "n", len(a.online))
}

func (a *Actor) onLogout(e gonet.Envelope) {
	m, ok := e.Msg.(*actormsg.OnlineLogoutMsg)
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
	m, _ := e.Msg.(*actormsg.OnlineQueryMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(false)
		return
	}
	_, ok := a.online[m.RoleID]
	e.Reply(ok)
}

func (a *Actor) onQueryBatch(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.OnlineQueryBatchMsg)
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
	e.Reply(onlineBatch(actormsg.OnlineQueryBatchReply{Online: online}))
}

func (a *Actor) onBroadcastAll(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.OnlineBroadcastAllMsg)
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
	m, _ := e.Msg.(*actormsg.OnlineBroadcastSomeMsg)
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
		msg := &actormsg.OnlinePushMsg{Cmd: cmd, Data: data}
		msg.SetCmd(actormsg.CmdOnlinePush)
		if err := gonet.SendName(ent.alias, msg); err != nil {
			slog.Warn("online broadcast", "roleid", id, "cmd", cmd, "err", err)
			continue
		}
		n++
	}
	return n
}

func onlineBatch(v actormsg.OnlineQueryBatchReply) *actormsg.OnlineQueryBatchReply {
	v.SetCmd(actormsg.CmdOnlineQueryBatchReply)
	return &v
}
