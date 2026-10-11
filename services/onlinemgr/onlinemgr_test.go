package onlinemgr

import (
	"context"
	"testing"
	"time"

	"game/services/internal/actormsg"

	"gonet"
)

type catch struct {
	gonet.ActorContext
	got chan actormsg.OnlinePushMsg
}

func (c *catch) Init() error {
	return gonet.RegisterCmd(c, actormsg.CmdOnlinePush, func() gonet.MessageInterface { return &actormsg.OnlinePushMsg{} }, c.onPush)
}

func (c *catch) onPush(e gonet.Envelope) {
	m, ok := e.Msg.(*actormsg.OnlinePushMsg)
	if !ok {
		return
	}
	c.got <- *m
}

func TestOnlineListAndBroadcast(t *testing.T) {
	pid, err := gonet.SpawnNamed(New(), Name)
	if err != nil {
		t.Fatal(err)
	}
	defer gonet.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := gonet.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}

	a := &catch{got: make(chan actormsg.OnlinePushMsg, 4)}
	b := &catch{got: make(chan actormsg.OnlinePushMsg, 4)}
	apid, err := gonet.SpawnNamed(a, "role/a")
	if err != nil {
		t.Fatal(err)
	}
	bpid, err := gonet.SpawnNamed(b, "role/b")
	if err != nil {
		t.Fatal(err)
	}
	if err := gonet.WaitInit(ctx, apid); err != nil {
		t.Fatal(err)
	}
	if err := gonet.WaitInit(ctx, bpid); err != nil {
		t.Fatal(err)
	}

	send(t, &actormsg.OnlineLoginMsg{RoleID: "a", Alias: "role/a", PID: 11})
	send(t, &actormsg.OnlineLoginMsg{RoleID: "b", Alias: "role/b", PID: 22})
	if !query(t, ctx, "a") || !query(t, ctx, "b") || query(t, ctx, "c") {
		t.Fatal("online set")
	}
	batch := &actormsg.OnlineQueryBatchMsg{RoleIDs: []string{"a", "c", "b"}}
	batch.SetCmd(actormsg.CmdOnlineQueryBatch)
	bv, err := gonet.SuspendCallName(ctx, Name, batch)
	if err != nil {
		t.Fatal(err)
	}
	rep, ok := bv.(*actormsg.OnlineQueryBatchReply)
	if !ok || !rep.Online["a"] || !rep.Online["b"] || rep.Online["c"] {
		t.Fatalf("batch %#v", bv)
	}

	broadcast(t, ctx, &actormsg.OnlineBroadcastAllMsg{Cmd: "notice", Data: `{"text":"all"}`})
	waitPush(t, a.got, "notice", `{"text":"all"}`)
	waitPush(t, b.got, "notice", `{"text":"all"}`)

	broadcast(t, ctx, &actormsg.OnlineBroadcastSomeMsg{RoleIDs: []string{"a", "c", "a"}, Cmd: "notice", Data: `{"text":"one"}`})
	waitPush(t, a.got, "notice", `{"text":"one"}`)
	select {
	case m := <-b.got:
		t.Fatalf("b got %s %s", m.Cmd, m.Data)
	default:
	}

	send(t, &actormsg.OnlineLoginMsg{RoleID: "a", Alias: "role/a", PID: 33})
	send(t, &actormsg.OnlineLogoutMsg{RoleID: "a", PID: 11})
	if !query(t, ctx, "a") {
		t.Fatal("old logout removed newer login")
	}
	send(t, &actormsg.OnlineLogoutMsg{RoleID: "a", PID: 33})
	if query(t, ctx, "a") || !query(t, ctx, "b") {
		t.Fatal("after logout")
	}
}

func send(t *testing.T, msg interface {
	SetCmd(string)
	gonet.MessageInterface
}) {
	t.Helper()
	switch m := msg.(type) {
	case *actormsg.OnlineLoginMsg:
		m.SetCmd(actormsg.CmdOnlineLogin)
	case *actormsg.OnlineLogoutMsg:
		m.SetCmd(actormsg.CmdOnlineLogout)
	default:
		t.Fatalf("send %T", msg)
	}
	if err := gonet.SendName(Name, msg); err != nil {
		t.Fatal(err)
	}
}

func query(t *testing.T, ctx context.Context, roleID string) bool {
	t.Helper()
	msg := &actormsg.OnlineQueryMsg{RoleID: roleID}
	msg.SetCmd(actormsg.CmdOnlineQuery)
	v, err := gonet.SuspendCallName(ctx, Name, msg)
	if err != nil {
		t.Fatal(err)
	}
	on, ok := v.(bool)
	if !ok {
		t.Fatalf("query %T", v)
	}
	return on
}

func broadcast(t *testing.T, ctx context.Context, msg gonet.MessageInterface) {
	t.Helper()
	switch m := msg.(type) {
	case *actormsg.OnlineBroadcastAllMsg:
		m.SetCmd(actormsg.CmdOnlineBroadcastAll)
	case *actormsg.OnlineBroadcastSomeMsg:
		m.SetCmd(actormsg.CmdOnlineBroadcastSome)
	default:
		t.Fatalf("broadcast %T", msg)
	}
	if _, err := gonet.SuspendCallName(ctx, Name, msg); err != nil {
		t.Fatal(err)
	}
}

func waitPush(t *testing.T, ch <-chan actormsg.OnlinePushMsg, cmd, data string) {
	t.Helper()
	select {
	case m := <-ch:
		if m.Cmd != cmd || m.Data != data {
			t.Fatalf("push %s %s", m.Cmd, m.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("no push")
	}
}
