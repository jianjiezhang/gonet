package guild

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"game/service/guildmgr"
	"game/store"

	"gonet"
)

const (
	dbTimeout   = 5 * time.Second
	callTimeout = 3 * time.Second

	// RankLeader 是会长，RankMember 是成员。存在成员数据里，不是消息。
	RankLeader = 1
	RankMember = 2
)

func init() {
	guildmgr.SpawnGuild = func() gonet.ActorContextInterface { return New() }
}

// Actor 是一个公会。成员和申请只放在这里。
type Actor struct {
	gonet.ActorContext
	id        string
	name      string
	notice    string
	leader    string
	members   map[string]Member
	applies   map[string]int64
	waits     map[uint64]wait
	saveWG    sync.WaitGroup
	disbanded bool
}

type wait struct {
	reply  gonet.Envelope
	op     string
	roleID string
	target string
	name   string
}

func New() *Actor {
	return &Actor{
		members: make(map[string]Member),
		applies: make(map[string]int64),
		waits:   make(map[uint64]wait),
	}
}

func (a *Actor) Init() error {
	a.id = ID(a.SelfName())
	if a.id == "" {
		return errors.New("guild: 缺少公会 id")
	}
	if err := a.load(); err != nil {
		return err
	}
	slog.Info("service started", "service", "guild", "pid", a.Self(), "name", a.SelfName(), "guild", a.id)
	return a.RegisterCmds()
}

func (a *Actor) Term() {
	a.saveWG.Wait()
	if a.disbanded {
		a.deleteRow()
	}
	slog.Info("service stopped", "service", "guild", "pid", a.Self(), "name", a.SelfName())
}

func (a *Actor) RegisterCmds() error {
	if err := gonet.RegisterCmd(a, CmdApply, func() gonet.MessageInterface { return &OpMsg{} }, a.onApply); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdAgree, func() gonet.MessageInterface { return &OpMsg{} }, a.onAgree); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdReject, func() gonet.MessageInterface { return &OpMsg{} }, a.onReject); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdLeave, func() gonet.MessageInterface { return &OpMsg{} }, a.onLeave); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdKick, func() gonet.MessageInterface { return &OpMsg{} }, a.onKick); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdList, func() gonet.MessageInterface { return &OpMsg{} }, a.onList); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdDisband, func() gonet.MessageInterface { return &OpMsg{} }, a.onDisband); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, gonet.CmdResponse, func() gonet.MessageInterface { return &gonet.CallResponse{} }, a.onCallResponse)
}

func (a *Actor) load() error {
	db := store.Get()
	if db == nil {
		return errors.New("guild: store 未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	row, members, applies, err := db.LoadGuild(ctx, a.id)
	if err != nil {
		return err
	}
	a.name = row.Name
	a.notice = row.Notice
	a.leader = row.Leader
	for _, m := range members {
		a.members[m.RoleID] = Member{RoleID: m.RoleID, Rank: m.Rank, Time: m.Time}
	}
	for _, apply := range applies {
		a.applies[apply.RoleID] = apply.Time
	}
	return nil
}

func (a *Actor) onApply(e gonet.Envelope) {
	m, _ := e.Msg.(*OpMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	if _, ok := a.members[m.RoleID]; ok {
		e.Reply(&Reply{Err: "已经在公会"})
		return
	}
	if _, ok := a.applies[m.RoleID]; ok {
		e.Reply(&Reply{Err: "已经申请过"})
		return
	}
	a.callMgr(e, guildmgr.CmdCheck, m.RoleID, "", 0, 0, "apply", m.RoleID, m.Name)
}

func (a *Actor) onAgree(e gonet.Envelope) {
	m, _ := e.Msg.(*OpMsg)
	if m == nil || m.RoleID == "" || m.Target == "" {
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	if m.RoleID != a.leader {
		e.Reply(&Reply{Err: "不是会长"})
		return
	}
	tm, ok := a.applies[m.Target]
	if !ok {
		e.Reply(&Reply{Err: "没有这条申请"})
		return
	}
	a.callMgr(e, guildmgr.CmdClaim, m.Target, m.Name, RankMember, tm, "agree", m.RoleID, m.Name)
}

func (a *Actor) onReject(e gonet.Envelope) {
	m, _ := e.Msg.(*OpMsg)
	if m == nil || m.RoleID == "" || m.Target == "" {
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	if m.RoleID != a.leader {
		e.Reply(&Reply{Err: "不是会长"})
		return
	}
	if _, ok := a.applies[m.Target]; !ok {
		e.Reply(&Reply{Err: "没有这条申请"})
		return
	}
	delete(a.applies, m.Target)
	a.async(func() error { return storeApplyDel(a.id, m.Target) })
	a.notify(m.Target, CmdReject, m.RoleID, m.Name, time.Now().Unix())
	e.Reply(&Reply{GuildID: a.id})
}

func (a *Actor) onLeave(e gonet.Envelope) {
	m, _ := e.Msg.(*OpMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	if _, ok := a.members[m.RoleID]; !ok {
		e.Reply(&Reply{Err: "不是成员"})
		return
	}
	if m.RoleID == a.leader {
		e.Reply(&Reply{Err: "会长不能退出"})
		return
	}
	a.callMgr(e, guildmgr.CmdRelease, m.RoleID, "", 0, 0, "leave", m.RoleID, m.Name)
}

func (a *Actor) onKick(e gonet.Envelope) {
	m, _ := e.Msg.(*OpMsg)
	if m == nil || m.RoleID == "" || m.Target == "" {
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	if m.RoleID != a.leader {
		e.Reply(&Reply{Err: "不是会长"})
		return
	}
	if m.Target == a.leader {
		e.Reply(&Reply{Err: "不能踢自己"})
		return
	}
	if _, ok := a.members[m.Target]; !ok {
		e.Reply(&Reply{Err: "不是成员"})
		return
	}
	a.callMgr(e, guildmgr.CmdRelease, m.Target, "", 0, 0, "kick", m.RoleID, m.Name)
}

func (a *Actor) onList(e gonet.Envelope) {
	m, _ := e.Msg.(*OpMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	if _, ok := a.members[m.RoleID]; !ok {
		e.Reply(&Reply{Err: "不是成员"})
		return
	}
	e.Reply(a.view(m.RoleID == a.leader))
}

func (a *Actor) onDisband(e gonet.Envelope) {
	m, _ := e.Msg.(*OpMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(&Reply{Err: "参数无效"})
		return
	}
	if m.RoleID != a.leader {
		e.Reply(&Reply{Err: "不是会长"})
		return
	}
	a.callMgr(e, guildmgr.CmdDrop, "", "", 0, 0, "disband", m.RoleID, m.Name)
}

func (a *Actor) callMgr(e gonet.Envelope, cmd, roleID, name string, rank int, tm int64, op, self, selfName string) {
	if guildmgr.Name == "" {
		e.Reply(&Reply{Err: "公会服务还没启动"})
		return
	}
	msg := &guildmgr.IndexMsg{GuildID: a.id, RoleID: roleID, Rank: rank, Time: tm}
	msg.SetCmd(cmd)
	sess, err := a.CallName(guildmgr.Name, callTimeout, msg)
	if err != nil {
		e.Reply(&Reply{Err: err.Error()})
		return
	}
	a.waits[sess] = wait{reply: e, op: op, roleID: roleID, target: self, name: selfName}
	if name != "" {
		w := a.waits[sess]
		w.name = name
		a.waits[sess] = w
	}
}

func (a *Actor) onCallResponse(e gonet.Envelope) {
	m, _ := e.Msg.(*gonet.CallResponse)
	if m == nil {
		return
	}
	w, ok := a.waits[m.Session]
	if !ok {
		return
	}
	delete(a.waits, m.Session)
	if m.Err != nil {
		w.reply.Reply(&Reply{Err: m.Err.Error()})
		return
	}
	view, _ := m.Value.(*guildmgr.Reply)
	if view == nil {
		w.reply.Reply(&Reply{Err: "公会目录没有回复"})
		return
	}
	if view.Err != "" {
		w.reply.Reply(&Reply{Err: view.Err})
		return
	}
	switch w.op {
	case "apply":
		now := time.Now().Unix()
		a.applies[w.roleID] = now
		roleID := w.roleID
		a.async(func() error {
			return storeApplyAdd(store.GuildApply{GuildID: a.id, RoleID: roleID, Time: now})
		})
		a.notify(a.leader, CmdApply, w.roleID, w.name, now)
		w.reply.Reply(&Reply{GuildID: a.id})
	case "agree":
		a.members[w.roleID] = Member{RoleID: w.roleID, Rank: RankMember, Time: a.applies[w.roleID]}
		delete(a.applies, w.roleID)
		roleID := w.roleID
		a.async(func() error { return storeApplyDel(a.id, roleID) })
		a.notify(w.roleID, CmdAgree, a.leader, w.name, time.Now().Unix())
		w.reply.Reply(&Reply{GuildID: a.id})
	case "leave":
		delete(a.members, w.roleID)
		a.notify(a.leader, CmdLeave, w.roleID, w.name, time.Now().Unix())
		w.reply.Reply(&Reply{GuildID: a.id})
	case "kick":
		delete(a.members, w.roleID)
		a.notify(w.roleID, CmdKick, a.leader, w.name, time.Now().Unix())
		w.reply.Reply(&Reply{GuildID: a.id})
	case "disband":
		for roleID := range a.members {
			if roleID == w.target {
				continue
			}
			a.notify(roleID, CmdDisband, a.id, a.name, time.Now().Unix())
		}
		w.reply.Reply(&Reply{GuildID: a.id})
		a.disbanded = true
		a.Exit()
	default:
		w.reply.Reply(&Reply{Err: "参数无效"})
	}
}

func (a *Actor) view(withApply bool) *Reply {
	out := &Reply{
		GuildID: a.id,
		Name:    a.name,
		Notice:  a.notice,
		Leader:  a.leader,
		Members: make([]Member, 0, len(a.members)),
	}
	for _, m := range a.members {
		out.Members = append(out.Members, m)
	}
	sort.Slice(out.Members, func(i, j int) bool { return out.Members[i].RoleID < out.Members[j].RoleID })
	if withApply {
		out.Applies = make([]Apply, 0, len(a.applies))
		for roleID, tm := range a.applies {
			out.Applies = append(out.Applies, Apply{RoleID: roleID, Time: tm})
		}
		sort.Slice(out.Applies, func(i, j int) bool { return out.Applies[i].RoleID < out.Applies[j].RoleID })
	}
	return out
}

func (a *Actor) notify(roleID, kind, who, name string, tm int64) {
	alias := roleAlias(roleID)
	if alias == "" || a.disbanded && kind != CmdDisband {
		return
	}
	msg := &NotifyMsg{Kind: kind, GuildID: a.id, RoleID: who, Name: name, Time: tm}
	msg.SetCmd(CmdNotify)
	if err := gonet.SendName(alias, msg); err != nil {
		if errors.Is(err, gonet.ErrUnknownAlias) || errors.Is(err, gonet.ErrDead) {
			return
		}
		slog.Warn("guild notify", "roleid", roleID, "kind", kind, "err", err)
	}
}

func (a *Actor) async(fn func() error) {
	if a.disbanded {
		return
	}
	a.saveWG.Add(1)
	go func() {
		defer a.saveWG.Done()
		if err := fn(); err != nil {
			slog.Error("guild save", "id", a.id, "err", err)
		}
	}()
}

func (a *Actor) deleteRow() {
	db := store.Get()
	if db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if err := db.DeleteGuild(ctx, a.id); err != nil {
		slog.Error("guild delete", "id", a.id, "err", err)
	}
}

func storeApplyAdd(row store.GuildApply) error {
	db := store.Get()
	if db == nil {
		return errors.New("guild: store 未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	return db.AddGuildApply(ctx, row)
}

func storeApplyDel(guildID, roleID string) error {
	db := store.Get()
	if db == nil {
		return errors.New("guild: store 未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	return db.RemoveGuildApply(ctx, guildID, roleID)
}
