package role

import (
	"context"
	"log/slog"
	"time"

	"game/service/friend"
	"game/service/onlinemgr"
	"game/store"
	"protocol/client"

	"gonet"
)

const friendCallTimeout = 3 * time.Second

type friendWait struct {
	link uint64
	op   string
}

type listBuild struct {
	link     uint64
	view     *friend.View
	profiles map[string]store.RoleRow
	online   map[string]bool
	left     int
}

func (a *Actor) onFriendList(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	a.callFriend(client.FriendList, &friend.ListMsg{Self: RoleID(a.SelfName())})
}

func (a *Actor) onFriendApply(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	target := friendTarget(e.Msg)
	self := RoleID(a.SelfName())
	if target == "" {
		a.clientErr(client.FriendApply, "参数无效")
		return
	}
	if target == self {
		a.clientErr(client.FriendApply, "不能加自己")
		return
	}
	link := a.link
	name := ""
	if a.data != nil && a.data.Base != nil {
		name = a.data.Base.Name()
	}
	pid := a.Self()
	go func() {
		ok, err := HasRole(target)
		msg := &friendReadyMsg{Link: link, Target: target, Name: name, OK: ok, Err: err}
		msg.SetCmd(cmdFriendReady)
		if err := gonet.SendMemory(pid, msg); err != nil {
			slog.Warn("friend exists", "roleid", self, "err", err)
		}
	}()
}

func (a *Actor) onFriendReady(e gonet.Envelope) {
	m, ok := e.Msg.(*friendReadyMsg)
	if !ok || m.Link != a.link {
		return
	}
	if m.Err != nil {
		a.clientErr(client.FriendApply, m.Err.Error())
		return
	}
	if !m.OK {
		a.clientErr(client.FriendApply, "玩家不存在")
		return
	}
	a.callFriend(client.FriendApply, &friend.ApplyMsg{From: RoleID(a.SelfName()), To: m.Target, FromName: m.Name})
}

func (a *Actor) onFriendAgree(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	a.callFriend(client.FriendAgree, &friend.DecideMsg{
		Self: RoleID(a.SelfName()), From: friendTarget(e.Msg), SelfName: a.selfName(),
	})
}

func (a *Actor) onFriendReject(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	a.callFriend(client.FriendReject, &friend.DecideMsg{
		Self: RoleID(a.SelfName()), From: friendTarget(e.Msg), SelfName: a.selfName(),
	})
}

func (a *Actor) onFriendDelete(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	a.callFriend(client.FriendDelete, &friend.DeleteMsg{
		Self: RoleID(a.SelfName()), Target: friendTarget(e.Msg), SelfName: a.selfName(),
	})
}

func (a *Actor) onFriendNotify(e gonet.Envelope) {
	m, ok := e.Msg.(*friend.NotifyMsg)
	if !ok || a.conn == nil {
		return
	}
	a.clientFriendNotify(m.Kind, m.RoleID, m.Name, m.Time)
}

func (a *Actor) onCallResponse(e gonet.Envelope) {
	m, ok := e.Msg.(*gonet.CallResponse)
	if !ok || m == nil {
		return
	}
	if a.takeRoomResponse(m) {
		return
	}
	if a.takeGuildResponse(m) {
		return
	}
	if id, pending := a.onlineWaits[m.Session]; pending {
		delete(a.onlineWaits, m.Session)
		a.finishOnline(id, m)
		return
	}
	wait, pending := a.friendWaits[m.Session]
	if !pending {
		return
	}
	delete(a.friendWaits, m.Session)
	if wait.link != a.link {
		return
	}
	if m.Err != nil {
		a.clientErr(wait.op, m.Err.Error())
		return
	}
	view, _ := m.Value.(*friend.View)
	if view == nil {
		a.clientErr(wait.op, "好友服务没有回复")
		return
	}
	if wait.op == client.FriendList {
		a.startFriendList(wait.link, view)
		return
	}
	if view.Err != "" {
		a.clientErr(wait.op, view.Err)
		return
	}
	a.clientOK(wait.op)
}

func (a *Actor) onFriendProfiles(e gonet.Envelope) {
	m, ok := e.Msg.(*friendProfilesMsg)
	if !ok {
		return
	}
	b := a.listBuilds[m.ID]
	if b == nil {
		return
	}
	b.profiles = m.Rows
	a.noteListPart(m.ID)
}

func (a *Actor) callFriend(op string, msg gonet.MessageInterface) {
	if friend.Name == "" {
		a.clientErr(op, "好友服务还没启动")
		return
	}
	switch m := msg.(type) {
	case *friend.ListMsg:
		m.SetCmd(friend.CmdList)
	case *friend.ApplyMsg:
		m.SetCmd(friend.CmdApply)
	case *friend.DecideMsg:
		if op == client.FriendReject {
			m.SetCmd(friend.CmdReject)
		} else {
			m.SetCmd(friend.CmdAgree)
		}
	case *friend.DeleteMsg:
		m.SetCmd(friend.CmdDelete)
	default:
		a.clientErr(op, "参数无效")
		return
	}
	sess, err := a.CallName(friend.Name, friendCallTimeout, msg)
	if err != nil {
		a.clientErr(op, err.Error())
		return
	}
	if a.friendWaits == nil {
		a.friendWaits = make(map[uint64]friendWait)
	}
	a.friendWaits[sess] = friendWait{link: a.link, op: op}
}

func (a *Actor) startFriendList(link uint64, view *friend.View) {
	if view.Err != "" {
		a.clientErr(client.FriendList, view.Err)
		return
	}
	ids := collectFriendIDs(view)
	if len(ids) == 0 {
		a.writeFriendList(link, view, nil, nil)
		return
	}
	if a.listBuilds == nil {
		a.listBuilds = make(map[uint64]*listBuild)
	}
	a.nextBuild++
	id := a.nextBuild
	b := &listBuild{link: link, view: view, left: 2}
	a.listBuilds[id] = b
	self := a.Self()
	go func() {
		msg := &friendProfilesMsg{ID: id, Rows: loadFriendRows(ids)}
		msg.SetCmd(cmdFriendProfiles)
		if err := gonet.SendMemory(self, msg); err != nil {
			slog.Warn("friend profiles", "err", err)
		}
	}()
	if onlinemgr.Name == "" {
		b.online = map[string]bool{}
		a.noteListPart(id)
		return
	}
	q := &onlinemgr.QueryBatchMsg{RoleIDs: ids}
	q.SetCmd(onlinemgr.CmdQueryBatch)
	sess, err := a.CallName(onlinemgr.Name, friendCallTimeout, q)
	if err != nil {
		b.online = map[string]bool{}
		a.noteListPart(id)
		return
	}
	if a.onlineWaits == nil {
		a.onlineWaits = make(map[uint64]uint64)
	}
	a.onlineWaits[sess] = id
}

func (a *Actor) finishOnline(id uint64, m *gonet.CallResponse) {
	b := a.listBuilds[id]
	if b == nil {
		return
	}
	if m.Err == nil {
		if rep, ok := m.Value.(onlinemgr.QueryBatchReply); ok {
			b.online = rep.Online
		}
	}
	if b.online == nil {
		b.online = map[string]bool{}
	}
	a.noteListPart(id)
}

func (a *Actor) noteListPart(id uint64) {
	b := a.listBuilds[id]
	if b == nil {
		return
	}
	b.left--
	if b.left > 0 {
		return
	}
	delete(a.listBuilds, id)
	if b.link != a.link {
		return
	}
	a.writeFriendList(b.link, b.view, b.profiles, b.online)
}

func (a *Actor) selfName() string {
	if a.data == nil || a.data.Base == nil {
		return RoleID(a.SelfName())
	}
	return a.data.Base.Name()
}

func friendTarget(msg gonet.MessageInterface) string {
	m, _ := msg.(*FriendTargetMsg)
	if m == nil {
		return ""
	}
	return m.RoleID
}

func collectFriendIDs(view *friend.View) []string {
	if view == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var ids []string
	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, id := range view.Friends {
		add(id)
	}
	for _, req := range view.Incoming {
		add(req.RoleID)
	}
	for _, req := range view.Outgoing {
		add(req.RoleID)
	}
	return ids
}

func loadFriendRows(ids []string) map[string]store.RoleRow {
	out := make(map[string]store.RoleRow, len(ids))
	db := store.Get()
	if db == nil {
		return out
	}
	for _, id := range ids {
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		row, err := db.LoadRole(ctx, id)
		cancel()
		if err != nil {
			continue
		}
		out[id] = row
	}
	return out
}
