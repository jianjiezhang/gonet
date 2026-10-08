package role

import (
	"context"
	"log/slog"
	"time"

	"game/service/guild"
	"game/service/guildmgr"
	"game/service/onlinemgr"
	"game/store"
	"protocol/client"

	"gonet"
)

const guildCallTimeout = 3 * time.Second

type guildWait struct {
	link   uint64
	step   string
	op     string
	wantID string
	target string
}

const (
	guildStepQuery = "query"
	guildStepAct   = "act"
)

func (a *Actor) onGuilds(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	a.callGuild(guildStepAct, client.Guilds, "", "", &guildmgr.CatalogMsg{})
}

func (a *Actor) onGuildID(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	m, _ := e.Msg.(*guildIDMsg)
	if m == nil || m.GuildID == "" {
		a.guildErr(client.GuildID, "参数无效")
		return
	}
	a.callGuild(guildStepAct, client.GuildID, "", "", &guildmgr.CatalogMsg{GuildID: m.GuildID})
}

func (a *Actor) onGuildName(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	m, _ := e.Msg.(*guildNameMsg)
	if m == nil || m.Name == "" {
		a.guildErr(client.GuildName, "参数无效")
		return
	}
	a.callGuild(guildStepAct, client.GuildName, "", "", &guildmgr.CatalogMsg{Name: m.Name})
}

func (a *Actor) onGuildCreate(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	m, _ := e.Msg.(*guildCreateMsg)
	name, notice := "", ""
	if m != nil {
		name, notice = m.Name, m.Notice
	}
	a.callGuild(guildStepAct, client.GuildCreate, "", "", &guildmgr.CreateMsg{
		RoleID: RoleID(a.SelfName()), Name: name, Notice: notice,
	})
}

func (a *Actor) onGuildList(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	a.callGuild(guildStepQuery, client.GuildList, "", "", &guildmgr.QueryMsg{RoleID: RoleID(a.SelfName())})
}

func (a *Actor) onGuildApply(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	m, _ := e.Msg.(*guildApplyMsg)
	if m == nil || m.GuildID == "" {
		a.guildErr(client.GuildApply, "参数无效")
		return
	}
	a.callGuild(guildStepQuery, client.GuildApply, m.GuildID, "", &guildmgr.QueryMsg{RoleID: RoleID(a.SelfName())})
}

func (a *Actor) onGuildAgree(e gonet.Envelope)  { a.guildTarget(e, client.GuildAgree) }
func (a *Actor) onGuildReject(e gonet.Envelope) { a.guildTarget(e, client.GuildReject) }
func (a *Actor) onGuildKick(e gonet.Envelope)   { a.guildTarget(e, client.GuildKick) }

func (a *Actor) onGuildLeave(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	a.callGuild(guildStepQuery, client.GuildLeave, "", "", &guildmgr.QueryMsg{RoleID: RoleID(a.SelfName())})
}

func (a *Actor) onGuildDisband(e gonet.Envelope) {
	if !a.sameLink(e.Msg) {
		return
	}
	a.callGuild(guildStepQuery, client.GuildDisband, "", "", &guildmgr.QueryMsg{RoleID: RoleID(a.SelfName())})
}

func (a *Actor) guildTarget(e gonet.Envelope, op string) {
	if !a.sameLink(e.Msg) {
		return
	}
	m, _ := e.Msg.(*guildTargetMsg)
	if m == nil || m.RoleID == "" {
		a.guildErr(op, "参数无效")
		return
	}
	a.callGuild(guildStepQuery, op, "", m.RoleID, &guildmgr.QueryMsg{RoleID: RoleID(a.SelfName())})
}

func (a *Actor) onGuildNotify(e gonet.Envelope) {
	m, ok := e.Msg.(*guild.NotifyMsg)
	if !ok || a.conn == nil {
		return
	}
	_ = a.write(client.NewGuildNotify(m.Kind, m.GuildID, m.RoleID, m.Name, m.Time))
}

func (a *Actor) callGuild(step, op, wantID, target string, msg gonet.MessageInterface) {
	if guildmgr.Name == "" && step == guildStepQuery {
		a.guildErr(op, "公会服务还没启动")
		return
	}
	switch m := msg.(type) {
	case *guildmgr.CreateMsg:
		m.SetCmd(guildmgr.CmdCreate)
	case *guildmgr.QueryMsg:
		m.SetCmd(guildmgr.CmdQuery)
	case *guildmgr.CatalogMsg:
		switch op {
		case client.Guilds:
			m.SetCmd(guildmgr.CmdGuilds)
		case client.GuildID:
			m.SetCmd(guildmgr.CmdGuildID)
		case client.GuildName:
			m.SetCmd(guildmgr.CmdByName)
		default:
			a.guildErr(op, "参数无效")
			return
		}
	case *guild.OpMsg:
		if m.Command() == "" {
			a.guildErr(op, "参数无效")
			return
		}
	default:
		a.guildErr(op, "参数无效")
		return
	}
	name := guildmgr.Name
	if _, ok := msg.(*guild.OpMsg); ok {
		name = guild.Alias(wantID)
	}
	if name == "" {
		a.guildErr(op, "公会服务还没启动")
		return
	}
	sess, err := a.CallName(name, guildCallTimeout, msg)
	if err != nil {
		a.guildErr(op, err.Error())
		return
	}
	if a.guildWaits == nil {
		a.guildWaits = make(map[uint64]guildWait)
	}
	a.guildWaits[sess] = guildWait{link: a.link, step: step, op: op, wantID: wantID, target: target}
}

func (a *Actor) takeGuildResponse(m *gonet.CallResponse) bool {
	w, ok := a.guildWaits[m.Session]
	if !ok {
		return false
	}
	delete(a.guildWaits, m.Session)
	if w.link != a.link {
		return true
	}
	if m.Err != nil {
		a.guildErr(w.op, m.Err.Error())
		return true
	}
	if w.op == client.Guilds || w.op == client.GuildID || w.op == client.GuildName {
		view, _ := m.Value.(*guildmgr.CatalogReply)
		if view == nil {
			a.guildErr(w.op, "公会没有回复")
			return true
		}
		if view.Err != "" {
			a.guildErr(w.op, view.Err)
			return true
		}
		a.paintGuildCards(w.link, w.op, view.Cards)
		return true
	}
	if w.op == client.GuildCreate || w.step == guildStepQuery {
		view, _ := m.Value.(*guildmgr.Reply)
		if view == nil {
			a.guildErr(w.op, "公会没有回复")
			return true
		}
		if view.Err != "" {
			a.guildErr(w.op, view.Err)
			return true
		}
		if w.op == client.GuildCreate {
			_ = a.write(client.NewGuildCreated(view.GuildID, view.Name))
			return true
		}
		a.afterGuildQuery(w, view.GuildID)
		return true
	}
	view, _ := m.Value.(*guild.Reply)
	if view == nil {
		a.guildErr(w.op, "公会没有回复")
		return true
	}
	if view.Err != "" {
		a.guildErr(w.op, view.Err)
		return true
	}
	if w.op == client.GuildList {
		a.paintGuildList(w.link, view)
		return true
	}
	_ = a.write(client.NewText(w.op, client.AckOK))
	return true
}

func (a *Actor) afterGuildQuery(w guildWait, guildID string) {
	switch w.op {
	case client.GuildList:
		if guildID == "" {
			a.guildErr(w.op, "还没有公会")
			return
		}
		msg := &guild.OpMsg{RoleID: RoleID(a.SelfName())}
		msg.SetCmd(guild.CmdList)
		a.callGuild(guildStepAct, w.op, guildID, "", msg)
	case client.GuildApply:
		if guildID != "" {
			a.guildErr(w.op, "已经在公会")
			return
		}
		msg := &guild.OpMsg{RoleID: RoleID(a.SelfName()), Name: a.selfName()}
		msg.SetCmd(guild.CmdApply)
		a.callGuild(guildStepAct, w.op, w.wantID, "", msg)
	case client.GuildLeave, client.GuildDisband:
		if guildID == "" {
			a.guildErr(w.op, "还没有公会")
			return
		}
		cmd := guild.CmdLeave
		if w.op == client.GuildDisband {
			cmd = guild.CmdDisband
		}
		msg := &guild.OpMsg{RoleID: RoleID(a.SelfName()), Name: a.selfName()}
		msg.SetCmd(cmd)
		a.callGuild(guildStepAct, w.op, guildID, "", msg)
	case client.GuildAgree, client.GuildReject, client.GuildKick:
		if guildID == "" {
			a.guildErr(w.op, "还没有公会")
			return
		}
		cmd := guild.CmdAgree
		if w.op == client.GuildReject {
			cmd = guild.CmdReject
		} else if w.op == client.GuildKick {
			cmd = guild.CmdKick
		}
		msg := &guild.OpMsg{RoleID: RoleID(a.SelfName()), Target: w.target, Name: a.selfName()}
		msg.SetCmd(cmd)
		a.callGuild(guildStepAct, w.op, guildID, w.target, msg)
	default:
		a.guildErr(w.op, "参数无效")
	}
}

func (a *Actor) paintGuildCards(link uint64, op string, cards []guildmgr.Card) {
	ids := make([]string, 0, len(cards))
	seen := map[string]struct{}{}
	for _, card := range cards {
		if card.Leader == "" {
			continue
		}
		if _, ok := seen[card.Leader]; ok {
			continue
		}
		seen[card.Leader] = struct{}{}
		ids = append(ids, card.Leader)
	}
	if len(ids) == 0 {
		a.writeGuildCards(link, op, cards, nil, nil)
		return
	}
	pid := a.Self()
	go func() {
		rows := loadFriendRows(ids)
		online := map[string]bool{}
		if onlinemgr.Name != "" {
			q := &onlinemgr.QueryBatchMsg{RoleIDs: ids}
			q.SetCmd(onlinemgr.CmdQueryBatch)
			ctx, cancel := context.WithTimeout(context.Background(), guildCallTimeout)
			v, err := gonet.SuspendCallName(ctx, onlinemgr.Name, q)
			cancel()
			if err == nil {
				if rep, ok := v.(onlinemgr.QueryBatchReply); ok {
					online = rep.Online
				}
			}
		}
		msg := &guildCardsMsg{Link: link, Op: op, Cards: cards, Rows: rows, Online: online}
		msg.SetCmd(cmdGuildCards)
		if err := gonet.SendMemory(pid, msg); err != nil {
			slog.Warn("guild cards", "err", err)
		}
	}()
}

func (a *Actor) onGuildCards(e gonet.Envelope) {
	m, ok := e.Msg.(*guildCardsMsg)
	if !ok || m.Link != a.link {
		return
	}
	a.writeGuildCards(m.Link, m.Op, m.Cards, m.Rows, m.Online)
}

func (a *Actor) writeGuildCards(link uint64, op string, cards []guildmgr.Card, rows map[string]store.RoleRow, online map[string]bool) {
	if link != a.link {
		return
	}
	briefs := make([]client.GuildBrief, 0, len(cards))
	for _, card := range cards {
		person := guildPerson(card.Leader, rows, online)
		briefs = append(briefs, client.GuildBrief{
			GuildID: card.GuildID, Name: card.Name, Notice: card.Notice,
			Leader: card.Leader, LeaderName: person.Name, Level: person.Level, Online: person.Online,
			Members: card.Members,
		})
	}
	if op == client.Guilds {
		_ = a.write(client.NewGuilds(briefs))
		return
	}
	if len(briefs) != 1 {
		a.guildErr(op, "公会不存在")
		return
	}
	_ = a.write(client.NewGuildBrief(op, briefs[0]))
}

func (a *Actor) paintGuildList(link uint64, view *guild.Reply) {
	ids := guildIDs(view)
	pid := a.Self()
	go func() {
		rows := loadFriendRows(ids)
		online := map[string]bool{}
		if onlinemgr.Name != "" && len(ids) > 0 {
			q := &onlinemgr.QueryBatchMsg{RoleIDs: ids}
			q.SetCmd(onlinemgr.CmdQueryBatch)
			ctx, cancel := context.WithTimeout(context.Background(), guildCallTimeout)
			v, err := gonet.SuspendCallName(ctx, onlinemgr.Name, q)
			cancel()
			if err == nil {
				if rep, ok := v.(onlinemgr.QueryBatchReply); ok {
					online = rep.Online
				}
			}
		}
		msg := &guildReadyMsg{Link: link, View: view, Rows: rows, Online: online}
		msg.SetCmd(cmdGuildReady)
		if err := gonet.SendMemory(pid, msg); err != nil {
			slog.Warn("guild list", "err", err)
		}
	}()
}

func (a *Actor) onGuildReady(e gonet.Envelope) {
	m, ok := e.Msg.(*guildReadyMsg)
	if !ok || m.Link != a.link || m.View == nil {
		return
	}
	members := make([]client.GuildMemberCard, 0, len(m.View.Members))
	for _, member := range m.View.Members {
		person := guildPerson(member.RoleID, m.Rows, m.Online)
		members = append(members, client.GuildMemberCard{
			RoleID: person.RoleID, Name: person.Name, Level: person.Level, Online: person.Online,
			Rank: member.Rank, Time: member.Time,
		})
	}
	applies := make([]client.GuildApplyCard, 0, len(m.View.Applies))
	for _, apply := range m.View.Applies {
		person := guildPerson(apply.RoleID, m.Rows, m.Online)
		applies = append(applies, client.GuildApplyCard{
			RoleID: person.RoleID, Name: person.Name, Level: person.Level, Online: person.Online, Time: apply.Time,
		})
	}
	_ = a.write(client.NewGuildList(m.View.GuildID, m.View.Name, m.View.Notice, m.View.Leader, members, applies))
}

func (a *Actor) guildErr(op, err string) {
	_ = a.write(client.NewErr(op, err))
}

func guildIDs(view *guild.Reply) []string {
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
	for _, m := range view.Members {
		add(m.RoleID)
	}
	for _, apply := range view.Applies {
		add(apply.RoleID)
	}
	return ids
}

func guildPerson(id string, rows map[string]store.RoleRow, online map[string]bool) client.Friend {
	row := rows[id]
	name := row.Name
	if name == "" {
		name = id
	}
	return client.Friend{RoleID: id, Name: name, Level: row.Level, Online: online[id]}
}
