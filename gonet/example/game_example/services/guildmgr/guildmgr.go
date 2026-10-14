package guildmgr

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"game/services/internal/actormsg"
	"game/services/internal/names"
	"game/store"

	"gonet"
	"gonet/components/mysql"
	"gonet/services/launcher"
)

const (
	dbTimeout    = 5 * time.Second
	spawnTimeout = 5 * time.Second

	// rankLeader 必须与 guild.RankLeader 相同。目录不引用 guild 包。
	rankLeader = 1
)

// Name 是全服公会目录的别名。全服只有一个，不加 nodeid。
const Name = names.Guildmgr

// SpawnGuild 由进程入口装上。目录拉起公会 actor，公会 actor 再 Call 目录，两边不互相引用。
var SpawnGuild func() gonet.ActorContextInterface

func guildAlias(id string) string { return names.Guild(id) }

type jobMsg struct {
	gonet.BaseMessage
	ID  uint64 `json:"-"`
	Err string `json:"-"`
}

type meta struct {
	name, notice, leader string
	pid                  uint64
}

// Actor 管公会目录：创建、一人一公会、拉起和停掉公会 actor。不管成员细节。
type Actor struct {
	gonet.ActorContext
	guilds  map[string]meta
	byName  map[string]string
	byRole  map[string]string
	nextID  uint64
	jobs    map[uint64]func(string)
	nextJob uint64
	saveWG  sync.WaitGroup
}

func New() *Actor {
	return &Actor{
		guilds: make(map[string]meta),
		byName: make(map[string]string),
		byRole: make(map[string]string),
		jobs:   make(map[uint64]func(string)),
		nextID: 1,
	}
}

func (a *Actor) Init() error {
	slog.Info("service started", "service", "guildmgr", "pid", a.Self(), "name", a.SelfName())
	if err := a.load(); err != nil {
		return err
	}
	if err := a.RegisterCmds(); err != nil {
		return err
	}
	for id := range a.guilds {
		a.spawn(id, gonet.Envelope{}, false)
	}
	return nil
}

func (a *Actor) Term() {
	a.saveWG.Wait()
	for _, meta := range a.guilds {
		if meta.pid != 0 {
			_ = gonet.StopActor(meta.pid)
		}
	}
	launcher.DropWaits(a.Self(), gonet.ErrDead)
	slog.Info("service stopped", "service", "guildmgr", "pid", a.Self(), "name", a.SelfName())
}

func (a *Actor) RegisterCmds() error {
	if err := gonet.RegisterCmd(a, actormsg.CmdGuildmgrCreate, func() gonet.MessageInterface { return &actormsg.GuildmgrCreateMsg{} }, a.onCreate); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdGuildmgrQuery, func() gonet.MessageInterface { return &actormsg.GuildmgrQueryMsg{} }, a.onQuery); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdGuildmgrCheck, func() gonet.MessageInterface { return &actormsg.GuildmgrIndexMsg{} }, a.onCheck); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdGuildmgrClaim, func() gonet.MessageInterface { return &actormsg.GuildmgrIndexMsg{} }, a.onClaim); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdGuildmgrRelease, func() gonet.MessageInterface { return &actormsg.GuildmgrIndexMsg{} }, a.onRelease); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdGuildmgrDrop, func() gonet.MessageInterface { return &actormsg.GuildmgrIndexMsg{} }, a.onDrop); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdGuildmgrJob, func() gonet.MessageInterface { return &jobMsg{} }, a.onJob); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdGuildmgrGuilds, func() gonet.MessageInterface { return &actormsg.GuildmgrCatalogMsg{} }, a.onCatalog); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdGuildmgrGuildID, func() gonet.MessageInterface { return &actormsg.GuildmgrCatalogMsg{} }, a.onCatalog); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, actormsg.CmdGuildmgrByName, func() gonet.MessageInterface { return &actormsg.GuildmgrCatalogMsg{} }, a.onCatalog); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, gonet.CmdResponse, func() gonet.MessageInterface { return &gonet.CallResponse{} }, a.onCallResponse)
}

func (a *Actor) onCatalog(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.GuildmgrCatalogMsg)
	if m == nil {
		e.Reply(guildmgrCatalog(actormsg.GuildmgrCatalogReply{Err: "参数无效"}))
		return
	}
	switch m.Command() {
	case actormsg.CmdGuildmgrGuilds:
		e.Reply(guildmgrCatalog(actormsg.GuildmgrCatalogReply{Cards: a.allCards()}))
	case actormsg.CmdGuildmgrGuildID:
		if m.GuildID == "" {
			e.Reply(guildmgrCatalog(actormsg.GuildmgrCatalogReply{Err: "参数无效"}))
			return
		}
		card, ok := a.oneCard(m.GuildID)
		if !ok {
			e.Reply(guildmgrCatalog(actormsg.GuildmgrCatalogReply{Err: "公会不存在"}))
			return
		}
		e.Reply(guildmgrCatalog(actormsg.GuildmgrCatalogReply{Cards: []actormsg.GuildmgrCard{card}}))
	case actormsg.CmdGuildmgrByName:
		name := trimGuildName(m.Name)
		if name == "" {
			e.Reply(guildmgrCatalog(actormsg.GuildmgrCatalogReply{Err: "参数无效"}))
			return
		}
		id := a.byName[name]
		card, ok := a.oneCard(id)
		if !ok {
			e.Reply(guildmgrCatalog(actormsg.GuildmgrCatalogReply{Err: "公会不存在"}))
			return
		}
		e.Reply(guildmgrCatalog(actormsg.GuildmgrCatalogReply{Cards: []actormsg.GuildmgrCard{card}}))
	default:
		e.Reply(guildmgrCatalog(actormsg.GuildmgrCatalogReply{Err: "参数无效"}))
	}
}

func (a *Actor) allCards() []actormsg.GuildmgrCard {
	counts := a.memberCounts()
	ids := make([]string, 0, len(a.guilds))
	for id := range a.guilds {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		ai, _ := strconv.ParseUint(ids[i], 10, 64)
		aj, _ := strconv.ParseUint(ids[j], 10, 64)
		if ai != aj {
			return ai < aj
		}
		return ids[i] < ids[j]
	})
	out := make([]actormsg.GuildmgrCard, 0, len(ids))
	for _, id := range ids {
		out = append(out, a.card(id, counts))
	}
	return out
}

func (a *Actor) oneCard(id string) (actormsg.GuildmgrCard, bool) {
	if id == "" {
		return actormsg.GuildmgrCard{}, false
	}
	if _, ok := a.guilds[id]; !ok {
		return actormsg.GuildmgrCard{}, false
	}
	return a.card(id, a.memberCounts()), true
}

func (a *Actor) card(id string, counts map[string]int) actormsg.GuildmgrCard {
	meta := a.guilds[id]
	return actormsg.GuildmgrCard{GuildID: id, Name: meta.name, Notice: meta.notice, Leader: meta.leader, Members: counts[id]}
}

func (a *Actor) memberCounts() map[string]int {
	counts := make(map[string]int)
	for _, id := range a.byRole {
		counts[id]++
	}
	return counts
}

func (a *Actor) load() error {
	db := store.Get()
	if db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	rows, err := db.LoadGuilds(ctx)
	if err != nil {
		return err
	}
	members, err := db.LoadGuildMembers(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		a.guilds[row.ID] = meta{name: row.Name, notice: row.Notice, leader: row.Leader}
		a.byName[row.Name] = row.ID
		if n, err := strconv.ParseUint(row.ID, 10, 64); err == nil && n >= a.nextID {
			a.nextID = n + 1
		}
	}
	for _, m := range members {
		if _, ok := a.guilds[m.GuildID]; ok {
			a.byRole[m.RoleID] = m.GuildID
		}
	}
	return nil
}

func (a *Actor) onCallResponse(e gonet.Envelope) {
	m, _ := e.Msg.(*gonet.CallResponse)
	if launcher.DispatchResponse(m) {
		return
	}
}

func (a *Actor) onQuery(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.GuildmgrQueryMsg)
	if m == nil || m.RoleID == "" {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "参数无效"}))
		return
	}
	id := a.byRole[m.RoleID]
	if id == "" {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{}))
		return
	}
	meta := a.guilds[id]
	e.Reply(guildmgrReply(actormsg.GuildmgrReply{GuildID: id, Name: meta.name, Notice: meta.notice, Leader: meta.leader}))
}

func (a *Actor) onCheck(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.GuildmgrIndexMsg)
	if m == nil || m.RoleID == "" || m.GuildID == "" {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "参数无效"}))
		return
	}
	if _, ok := a.guilds[m.GuildID]; !ok {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "公会不存在"}))
		return
	}
	if gid, ok := a.byRole[m.RoleID]; ok && gid != m.GuildID {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "已经在公会"}))
		return
	}
	e.Reply(guildmgrReply(actormsg.GuildmgrReply{GuildID: m.GuildID}))
}

func (a *Actor) onCreate(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.GuildmgrCreateMsg)
	if m == nil {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "参数无效"}))
		return
	}
	name := trimGuildName(m.Name)
	if name == "" || m.RoleID == "" || utf8.RuneCountInString(name) > 64 || utf8.RuneCountInString(m.Notice) > 255 {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "参数无效"}))
		return
	}
	if _, ok := a.byRole[m.RoleID]; ok {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "已经在公会"}))
		return
	}
	if _, ok := a.byName[name]; ok {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "公会名已占用"}))
		return
	}
	id := strconv.FormatUint(a.nextID, 10)
	a.nextID++
	now := time.Now().Unix()
	a.guilds[id] = meta{name: name, notice: m.Notice, leader: m.RoleID}
	a.byName[name] = id
	a.byRole[m.RoleID] = id
	row := store.GuildRow{ID: id, Name: name, Leader: m.RoleID, Notice: m.Notice, Created: now}
	leader := store.GuildMember{RoleID: m.RoleID, GuildID: id, Rank: rankLeader, Time: now}
	a.later(func(err string) {
		if err != "" {
			a.unreserve(id, name, m.RoleID)
			e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: err}))
			return
		}
		a.spawn(id, e, true)
	}, func() error {
		db := store.Get()
		if db == nil {
			return errors.New("公会存储还没准备好")
		}
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		err := db.CreateGuild(ctx, row, leader)
		if err == nil {
			return nil
		}
		if errors.Is(err, store.ErrExists) || mysql.IsDuplicate(err) {
			return errors.New("公会名已占用")
		}
		return err
	})
}

func (a *Actor) onClaim(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.GuildmgrIndexMsg)
	if m == nil || m.RoleID == "" || m.GuildID == "" {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "参数无效"}))
		return
	}
	if _, ok := a.guilds[m.GuildID]; !ok {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "公会不存在"}))
		return
	}
	if gid, ok := a.byRole[m.RoleID]; ok && gid != m.GuildID {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "已经在公会"}))
		return
	}
	if a.byRole[m.RoleID] == m.GuildID {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{GuildID: m.GuildID}))
		return
	}
	member := store.GuildMember{RoleID: m.RoleID, GuildID: m.GuildID, Rank: m.Rank, Time: m.Time}
	a.later(func(err string) {
		if err != "" {
			e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: err}))
			return
		}
		a.byRole[m.RoleID] = m.GuildID
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{GuildID: m.GuildID}))
	}, func() error {
		db := store.Get()
		if db == nil {
			return errors.New("公会存储还没准备好")
		}
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		err := db.AddGuildMember(ctx, member)
		if errors.Is(err, store.ErrExists) || mysql.IsDuplicate(err) {
			return errors.New("已经在公会")
		}
		return err
	})
}

func (a *Actor) onRelease(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.GuildmgrIndexMsg)
	if m == nil || m.RoleID == "" || m.GuildID == "" {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "参数无效"}))
		return
	}
	if a.byRole[m.RoleID] != m.GuildID {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "不是成员"}))
		return
	}
	a.later(func(err string) {
		if err != "" {
			e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: err}))
			return
		}
		delete(a.byRole, m.RoleID)
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{GuildID: m.GuildID}))
	}, func() error {
		db := store.Get()
		if db == nil {
			return errors.New("公会存储还没准备好")
		}
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		return db.RemoveGuildMember(ctx, m.RoleID)
	})
}

func (a *Actor) onDrop(e gonet.Envelope) {
	m, _ := e.Msg.(*actormsg.GuildmgrIndexMsg)
	if m == nil || m.GuildID == "" {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "参数无效"}))
		return
	}
	meta, ok := a.guilds[m.GuildID]
	if !ok {
		e.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "公会不存在"}))
		return
	}
	for roleID, gid := range a.byRole {
		if gid == m.GuildID {
			delete(a.byRole, roleID)
		}
	}
	delete(a.byName, meta.name)
	delete(a.guilds, m.GuildID)
	a.deleteGuild(m.GuildID)
	e.Reply(guildmgrReply(actormsg.GuildmgrReply{GuildID: m.GuildID}))
}

func (a *Actor) deleteGuild(id string) {
	a.later(func(string) {}, func() error {
		db := store.Get()
		if db == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		return db.DeleteGuild(ctx, id)
	})
}

func (a *Actor) later(done func(string), fn func() error) {
	a.nextJob++
	id := a.nextJob
	a.jobs[id] = done
	pid := a.Self()
	a.saveWG.Add(1)
	go func() {
		defer a.saveWG.Done()
		errText := ""
		if err := fn(); err != nil {
			errText = err.Error()
		}
		msg := &jobMsg{ID: id, Err: errText}
		msg.SetCmd(actormsg.CmdGuildmgrJob)
		if err := gonet.SendMemory(pid, msg); err != nil {
			slog.Error("guildmgr job", "err", err)
		}
	}()
}

func (a *Actor) onJob(e gonet.Envelope) {
	m, _ := e.Msg.(*jobMsg)
	if m == nil {
		return
	}
	done := a.jobs[m.ID]
	delete(a.jobs, m.ID)
	if done != nil {
		done(m.Err)
	}
}

func (a *Actor) unreserve(id, name, roleID string) {
	delete(a.guilds, id)
	delete(a.byName, name)
	if a.byRole[roleID] == id {
		delete(a.byRole, roleID)
	}
}

func (a *Actor) spawn(id string, reply gonet.Envelope, wait bool) {
	if SpawnGuild == nil {
		if wait {
			reply.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "公会服务还没接上"}))
		}
		return
	}
	err := launcher.NewService(a.Self(), SpawnGuild(), guildAlias(id), spawnTimeout, func(res launcher.ServiceResult) {
		meta, ok := a.guilds[id]
		if !ok {
			if res.PID != 0 {
				go gonet.StopActor(res.PID)
			}
			if wait {
				reply.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: "公会不存在"}))
			}
			return
		}
		if res.Err != nil {
			if wait {
				a.unreserve(id, meta.name, meta.leader)
				a.deleteGuild(id)
				reply.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: res.Err.Error()}))
			} else {
				slog.Error("spawn guild", "id", id, "err", res.Err)
			}
			return
		}
		meta.pid = res.PID
		a.guilds[id] = meta
		if wait {
			reply.Reply(guildmgrReply(actormsg.GuildmgrReply{GuildID: id, Name: meta.name, Leader: meta.leader, Notice: meta.notice}))
		}
	})
	if err != nil && wait {
		reply.Reply(guildmgrReply(actormsg.GuildmgrReply{Err: err.Error()}))
	}
}

func trimGuildName(name string) string {
	for len(name) > 0 && (name[0] == ' ' || name[0] == '\t') {
		name = name[1:]
	}
	for len(name) > 0 && (name[len(name)-1] == ' ' || name[len(name)-1] == '\t') {
		name = name[:len(name)-1]
	}
	return name
}

func guildmgrCatalog(v actormsg.GuildmgrCatalogReply) *actormsg.GuildmgrCatalogReply {
	v.SetCmd(actormsg.CmdGuildmgrCatalogReply)
	return &v
}

func guildmgrReply(v actormsg.GuildmgrReply) *actormsg.GuildmgrReply {
	v.SetCmd(actormsg.CmdGuildmgrReply)
	return &v
}
