package guildmgr_test

import (
	"context"
	"testing"
	"time"

	"game/service/guild"
	"game/service/guildmgr"
	"game/store"

	"gonet"
	"gonet/services/launcher"
)

func TestGuildLife(t *testing.T) {
	mem := store.NewMemory()
	store.Set(mem)
	t.Cleanup(func() {
		store.Set(nil)
		gonet.Stop()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start(t, ctx)

	created := callMgr(t, ctx, &guildmgr.CreateMsg{RoleID: "a", Name: "  alpha ", Notice: "hi"})
	if created.Err != "" || created.GuildID == "" || created.Name != "alpha" {
		t.Fatalf("create: %+v", created)
	}
	if view := callMgr(t, ctx, &guildmgr.CreateMsg{RoleID: "b", Name: "alpha"}); view.Err != "公会名已占用" {
		t.Fatalf("dup name: %+v", view)
	}
	if view := callMgr(t, ctx, &guildmgr.CreateMsg{RoleID: "a", Name: "beta"}); view.Err != "已经在公会" {
		t.Fatalf("dup role: %+v", view)
	}

	list := callGuild(t, ctx, created.GuildID, guild.CmdList, &guild.OpMsg{RoleID: "a"})
	if list.Err != "" || list.Leader != "a" || list.Notice != "hi" || len(list.Members) != 1 || list.Members[0].Rank != guild.RankLeader {
		t.Fatalf("list: %+v", list)
	}
	if view := callGuild(t, ctx, created.GuildID, guild.CmdApply, &guild.OpMsg{RoleID: "b", Name: "B"}); view.Err != "" {
		t.Fatalf("apply: %+v", view)
	}
	list = callGuild(t, ctx, created.GuildID, guild.CmdList, &guild.OpMsg{RoleID: "a"})
	if len(list.Applies) != 1 || list.Applies[0].RoleID != "b" {
		t.Fatalf("applies: %+v", list)
	}
	if view := callGuild(t, ctx, created.GuildID, guild.CmdAgree, &guild.OpMsg{RoleID: "a", Target: "b", Name: "A"}); view.Err != "" {
		t.Fatalf("agree: %+v", view)
	}
	list = callGuild(t, ctx, created.GuildID, guild.CmdList, &guild.OpMsg{RoleID: "b"})
	if len(list.Members) != 2 || len(list.Applies) != 0 {
		t.Fatalf("members: %+v", list)
	}
	if view := callGuild(t, ctx, created.GuildID, guild.CmdLeave, &guild.OpMsg{RoleID: "a"}); view.Err != "会长不能退出" {
		t.Fatalf("leader leave: %+v", view)
	}

	beta := callMgr(t, ctx, &guildmgr.CreateMsg{RoleID: "c", Name: "beta"})
	if beta.Err != "" {
		t.Fatal(beta.Err)
	}
	cards := callCatalog(t, ctx, guildmgr.CmdGuilds, &guildmgr.CatalogMsg{})
	if cards.Err != "" || len(cards.Cards) != 2 || cards.Cards[0].Name != "alpha" || cards.Cards[0].Members != 2 || cards.Cards[1].Name != "beta" || cards.Cards[1].Members != 1 {
		t.Fatalf("guilds: %+v", cards)
	}
	one := callCatalog(t, ctx, guildmgr.CmdGuildID, &guildmgr.CatalogMsg{GuildID: created.GuildID})
	if one.Err != "" || len(one.Cards) != 1 || one.Cards[0].Leader != "a" || one.Cards[0].Notice != "hi" {
		t.Fatalf("by id: %+v", one)
	}
	if view := callCatalog(t, ctx, guildmgr.CmdGuildID, &guildmgr.CatalogMsg{GuildID: "missing"}); view.Err != "公会不存在" {
		t.Fatalf("missing id: %+v", view)
	}
	named := callCatalog(t, ctx, guildmgr.CmdByName, &guildmgr.CatalogMsg{Name: " beta "})
	if named.Err != "" || len(named.Cards) != 1 || named.Cards[0].GuildID != beta.GuildID {
		t.Fatalf("by name: %+v", named)
	}
	if view := callCatalog(t, ctx, guildmgr.CmdByName, &guildmgr.CatalogMsg{Name: "nope"}); view.Err != "公会不存在" {
		t.Fatalf("missing name: %+v", view)
	}
	if view := callGuild(t, ctx, beta.GuildID, guild.CmdApply, &guild.OpMsg{RoleID: "b", Name: "B"}); view.Err != "已经在公会" {
		t.Fatalf("second guild: %+v", view)
	}
	if view := callGuild(t, ctx, created.GuildID, guild.CmdKick, &guild.OpMsg{RoleID: "a", Target: "b", Name: "A"}); view.Err != "" {
		t.Fatalf("kick: %+v", view)
	}
	if view := callMgr(t, ctx, &guildmgr.QueryMsg{RoleID: "b"}); view.GuildID != "" {
		t.Fatalf("kicked still in: %+v", view)
	}
	if view := callGuild(t, ctx, created.GuildID, guild.CmdDisband, &guild.OpMsg{RoleID: "a", Name: "A"}); view.Err != "" {
		t.Fatalf("disband: %+v", view)
	}
	if view := callMgr(t, ctx, &guildmgr.QueryMsg{RoleID: "a"}); view.GuildID != "" || view.Err != "" {
		t.Fatalf("after disband: %+v", view)
	}
}

func TestGuildReload(t *testing.T) {
	mem := store.NewMemory()
	store.Set(mem)
	t.Cleanup(func() {
		store.Set(nil)
		gonet.Stop()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgrPID := start(t, ctx)
	created := callMgr(t, ctx, &guildmgr.CreateMsg{RoleID: "a", Name: "alpha"})
	if created.Err != "" {
		t.Fatal(created.Err)
	}
	if view := callGuild(t, ctx, created.GuildID, guild.CmdApply, &guild.OpMsg{RoleID: "b", Name: "B"}); view.Err != "" {
		t.Fatal(view.Err)
	}
	if err := gonet.StopActor(mgrPID); err != nil {
		t.Fatal(err)
	}
	if _, err := launcher.WaitService(ctx, guildmgr.New(), guildmgr.Name); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		list, err := tryGuild(ctx, created.GuildID, &guild.OpMsg{RoleID: "a"})
		if err == nil && list.Err == "" && len(list.Applies) == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("reload list=%+v err=%v", list, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func start(t *testing.T, ctx context.Context) uint64 {
	t.Helper()
	launcher.Bind(1)
	pid, err := gonet.SpawnNamed(launcher.New(), launcher.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := gonet.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}
	mgrPID, err := launcher.WaitService(ctx, guildmgr.New(), guildmgr.Name)
	if err != nil {
		t.Fatal(err)
	}
	return mgrPID
}

func callMgr(t *testing.T, ctx context.Context, msg gonet.MessageInterface) *guildmgr.Reply {
	t.Helper()
	switch m := msg.(type) {
	case *guildmgr.CreateMsg:
		m.SetCmd(guildmgr.CmdCreate)
	case *guildmgr.QueryMsg:
		m.SetCmd(guildmgr.CmdQuery)
	default:
		t.Fatalf("mgr %T", msg)
	}
	v, err := gonet.SuspendCallName(ctx, guildmgr.Name, msg)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := v.(*guildmgr.Reply)
	if view == nil {
		t.Fatalf("reply %T", v)
	}
	return view
}

func callCatalog(t *testing.T, ctx context.Context, cmd string, msg *guildmgr.CatalogMsg) *guildmgr.CatalogReply {
	t.Helper()
	msg.SetCmd(cmd)
	v, err := gonet.SuspendCallName(ctx, guildmgr.Name, msg)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := v.(*guildmgr.CatalogReply)
	if view == nil {
		t.Fatalf("catalog %T", v)
	}
	return view
}

func callGuild(t *testing.T, ctx context.Context, id, cmd string, msg *guild.OpMsg) *guild.Reply {
	t.Helper()
	msg.SetCmd(cmd)
	view, err := tryGuild(ctx, id, msg)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func tryGuild(ctx context.Context, id string, msg *guild.OpMsg) (*guild.Reply, error) {
	if msg.Command() == "" {
		msg.SetCmd(guild.CmdList)
	}
	v, err := gonet.SuspendCallName(ctx, guild.Alias(id), msg)
	if err != nil {
		return nil, err
	}
	view, _ := v.(*guild.Reply)
	if view == nil {
		return nil, errReply
	}
	return view, nil
}

var errReply = errString("reply")

type errString string

func (e errString) Error() string { return string(e) }
