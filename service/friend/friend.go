package friend

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"game/store"

	"gonet"
)

const dbTimeout = 5 * time.Second

// Name 是全服好友服务的别名。全服只有一个，不加 nodeid。
const Name = ".friend"

var errNoStore = errors.New("friend: store 未初始化")

// Actor 持有全服好友关系和未处理申请。
type Actor struct {
	gonet.ActorContext
	friends map[string]map[string]struct{}
	inbox   map[string]map[string]int64
	outbox  map[string]map[string]int64
	saveWG  sync.WaitGroup
}

func New() *Actor {
	return &Actor{
		friends: make(map[string]map[string]struct{}),
		inbox:   make(map[string]map[string]int64),
		outbox:  make(map[string]map[string]int64),
	}
}

func (a *Actor) Init() error {
	slog.Info("service started", "service", "friend", "pid", a.Self(), "name", a.SelfName())
	if err := a.load(); err != nil {
		return err
	}
	return a.RegisterCmds()
}

func (a *Actor) Term() {
	a.saveWG.Wait()
	slog.Info("service stopped", "service", "friend", "pid", a.Self(), "name", a.SelfName())
}

func (a *Actor) RegisterCmds() error {
	if err := gonet.RegisterCmd(a, CmdApply, func() gonet.MessageInterface { return &ApplyMsg{} }, a.onApply); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdAgree, func() gonet.MessageInterface { return &DecideMsg{} }, a.onAgree); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdReject, func() gonet.MessageInterface { return &DecideMsg{} }, a.onReject); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdDelete, func() gonet.MessageInterface { return &DeleteMsg{} }, a.onDelete); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, CmdList, func() gonet.MessageInterface { return &ListMsg{} }, a.onList)
}

func (a *Actor) load() error {
	db := store.Get()
	if db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	edges, err := db.LoadFriends(ctx)
	if err != nil {
		return err
	}
	reqs, err := db.LoadFriendRequests(ctx)
	if err != nil {
		return err
	}
	for _, e := range edges {
		a.link(e.RoleID, e.FriendID)
	}
	for _, req := range reqs {
		a.putReq(req.FromID, req.ToID, req.Time)
	}
	return nil
}

func (a *Actor) onApply(e gonet.Envelope) {
	m, _ := e.Msg.(*ApplyMsg)
	if m == nil {
		e.Reply(&View{Err: "参数无效"})
		return
	}
	if err := a.apply(m.From, m.To); err != nil {
		e.Reply(&View{Err: err.Error()})
		return
	}
	a.notify(m.To, KindApply, m.From, m.FromName, a.outbox[m.From][m.To])
	e.Reply(&View{})
}

func (a *Actor) onAgree(e gonet.Envelope) {
	m, _ := e.Msg.(*DecideMsg)
	if m == nil {
		e.Reply(&View{Err: "参数无效"})
		return
	}
	if err := a.agree(m.Self, m.From); err != nil {
		e.Reply(&View{Err: err.Error()})
		return
	}
	a.notify(m.From, KindAgree, m.Self, m.SelfName, 0)
	e.Reply(&View{})
}

func (a *Actor) onReject(e gonet.Envelope) {
	m, _ := e.Msg.(*DecideMsg)
	if m == nil {
		e.Reply(&View{Err: "参数无效"})
		return
	}
	if err := a.reject(m.Self, m.From); err != nil {
		e.Reply(&View{Err: err.Error()})
		return
	}
	a.notify(m.From, KindReject, m.Self, m.SelfName, 0)
	e.Reply(&View{})
}

func (a *Actor) onDelete(e gonet.Envelope) {
	m, _ := e.Msg.(*DeleteMsg)
	if m == nil {
		e.Reply(&View{Err: "参数无效"})
		return
	}
	if err := a.remove(m.Self, m.Target); err != nil {
		e.Reply(&View{Err: err.Error()})
		return
	}
	a.notify(m.Target, KindDelete, m.Self, m.SelfName, 0)
	e.Reply(&View{})
}

func (a *Actor) onList(e gonet.Envelope) {
	m, _ := e.Msg.(*ListMsg)
	if m == nil || m.Self == "" {
		e.Reply(&View{Err: "参数无效"})
		return
	}
	e.Reply(a.view(m.Self))
}

func (a *Actor) apply(from, to string) error {
	if from == "" || to == "" {
		return errors.New("参数无效")
	}
	if from == to {
		return errors.New("不能加自己")
	}
	if a.linked(from, to) {
		return errors.New("已经是好友")
	}
	if _, ok := a.outbox[from][to]; ok {
		return errors.New("已经申请过")
	}
	if _, ok := a.inbox[from][to]; ok {
		return errors.New("对方已申请你")
	}
	if a.count(from) >= friendLimit {
		return errors.New("好友已满")
	}
	now := time.Now().Unix()
	a.putReq(from, to, now)
	a.async(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		db := store.Get()
		if db == nil {
			return errNoStore
		}
		return db.AddFriendRequest(ctx, store.FriendRequest{FromID: from, ToID: to, Time: now})
	})
	return nil
}

func (a *Actor) agree(self, from string) error {
	if self == "" || from == "" || self == from {
		return errors.New("参数无效")
	}
	if _, ok := a.inbox[self][from]; !ok {
		return errors.New("没有这条申请")
	}
	if a.linked(self, from) {
		a.dropReq(from, self)
		return errors.New("已经是好友")
	}
	if a.count(self) >= friendLimit || a.count(from) >= friendLimit {
		return errors.New("好友已满")
	}
	a.link(self, from)
	a.link(from, self)
	a.dropReq(from, self)
	a.async(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		db := store.Get()
		if db == nil {
			return errNoStore
		}
		return db.AcceptFriend(ctx, self, from)
	})
	return nil
}

func (a *Actor) reject(self, from string) error {
	if self == "" || from == "" {
		return errors.New("参数无效")
	}
	if _, ok := a.inbox[self][from]; !ok {
		return errors.New("没有这条申请")
	}
	a.dropReq(from, self)
	a.async(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		db := store.Get()
		if db == nil {
			return errNoStore
		}
		return db.RemoveFriendRequest(ctx, from, self)
	})
	return nil
}

func (a *Actor) remove(self, target string) error {
	if self == "" || target == "" {
		return errors.New("参数无效")
	}
	if !a.linked(self, target) {
		return errors.New("不是好友")
	}
	a.unlink(self, target)
	a.unlink(target, self)
	a.async(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		db := store.Get()
		if db == nil {
			return errNoStore
		}
		return db.RemoveFriends(ctx, self, target)
	})
	return nil
}

func (a *Actor) view(self string) *View {
	v := &View{Friends: a.friendIDs(self)}
	for id, tm := range a.inbox[self] {
		v.Incoming = append(v.Incoming, Req{RoleID: id, Time: tm})
	}
	for id, tm := range a.outbox[self] {
		v.Outgoing = append(v.Outgoing, Req{RoleID: id, Time: tm})
	}
	sort.Slice(v.Incoming, func(i, j int) bool { return v.Incoming[i].RoleID < v.Incoming[j].RoleID })
	sort.Slice(v.Outgoing, func(i, j int) bool { return v.Outgoing[i].RoleID < v.Outgoing[j].RoleID })
	return v
}

func (a *Actor) notify(roleID, kind, who, name string, tm int64) {
	alias := RoleAlias(roleID)
	if alias == "" {
		return
	}
	msg := &NotifyMsg{Kind: kind, RoleID: who, Name: name, Time: tm}
	msg.SetCmd(CmdNotify)
	if err := gonet.SendName(alias, msg); err != nil {
		if errors.Is(err, gonet.ErrUnknownAlias) || errors.Is(err, gonet.ErrDead) {
			return
		}
		slog.Warn("friend notify", "roleid", roleID, "kind", kind, "err", err)
	}
}

func (a *Actor) async(fn func() error) {
	a.saveWG.Add(1)
	go func() {
		defer a.saveWG.Done()
		if err := fn(); err != nil {
			slog.Error("friend save", "err", err)
		}
	}()
}

func (a *Actor) count(id string) int { return len(a.friends[id]) }

func (a *Actor) linked(aID, bID string) bool {
	_, ok := a.friends[aID][bID]
	return ok
}

func (a *Actor) link(aID, bID string) {
	set := a.friends[aID]
	if set == nil {
		set = make(map[string]struct{})
		a.friends[aID] = set
	}
	set[bID] = struct{}{}
}

func (a *Actor) unlink(aID, bID string) {
	delete(a.friends[aID], bID)
	if len(a.friends[aID]) == 0 {
		delete(a.friends, aID)
	}
}

func (a *Actor) putReq(from, to string, tm int64) {
	in := a.inbox[to]
	if in == nil {
		in = make(map[string]int64)
		a.inbox[to] = in
	}
	in[from] = tm
	out := a.outbox[from]
	if out == nil {
		out = make(map[string]int64)
		a.outbox[from] = out
	}
	out[to] = tm
}

func (a *Actor) dropReq(from, to string) {
	delete(a.inbox[to], from)
	if len(a.inbox[to]) == 0 {
		delete(a.inbox, to)
	}
	delete(a.outbox[from], to)
	if len(a.outbox[from]) == 0 {
		delete(a.outbox, from)
	}
}

func (a *Actor) friendIDs(id string) []string {
	set := a.friends[id]
	out := make([]string, 0, len(set))
	for friendID := range set {
		out = append(out, friendID)
	}
	sort.Strings(out)
	return out
}
