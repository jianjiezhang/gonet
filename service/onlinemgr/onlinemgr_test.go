package onlinemgr

import (
	"context"
	"testing"
	"time"

	"gonet"
)

type catch struct {
	gonet.ActorContext
	got chan PushMsg
}

func (c *catch) Init() error {
	return gonet.RegisterCmd(c, CmdPush, func() gonet.MessageInterface { return &PushMsg{} }, c.onPush)
}

func (c *catch) onPush(e gonet.Envelope) {
	m, ok := e.Msg.(*PushMsg)
	if !ok {
		return
	}
	c.got <- *m
}

func TestOnlineListAndBroadcast(t *testing.T) {
	Bind(1)
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

	a := &catch{got: make(chan PushMsg, 4)}
	b := &catch{got: make(chan PushMsg, 4)}
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

	send(t, &LoginMsg{RoleID: "a", Alias: "role/a", PID: 11})
	send(t, &LoginMsg{RoleID: "b", Alias: "role/b", PID: 22})
	if !query(t, ctx, "a") || !query(t, ctx, "b") || query(t, ctx, "c") {
		t.Fatal("online set")
	}
	batch := &QueryBatchMsg{RoleIDs: []string{"a", "c", "b"}}
	batch.SetCmd(CmdQueryBatch)
	bv, err := gonet.SuspendCallName(ctx, Name, batch)
	if err != nil {
		t.Fatal(err)
	}
	rep, ok := bv.(QueryBatchReply)
	if !ok || !rep.Online["a"] || !rep.Online["b"] || rep.Online["c"] {
		t.Fatalf("batch %#v", bv)
	}

	broadcast(t, ctx, &BroadcastAllMsg{Cmd: "notice", Data: `{"text":"all"}`})
	waitPush(t, a.got, "notice", `{"text":"all"}`)
	waitPush(t, b.got, "notice", `{"text":"all"}`)

	broadcast(t, ctx, &BroadcastSomeMsg{RoleIDs: []string{"a", "c", "a"}, Cmd: "notice", Data: `{"text":"one"}`})
	waitPush(t, a.got, "notice", `{"text":"one"}`)
	select {
	case m := <-b.got:
		t.Fatalf("b got %s %s", m.Cmd, m.Data)
	default:
	}

	send(t, &LoginMsg{RoleID: "a", Alias: "role/a", PID: 33})
	send(t, &LogoutMsg{RoleID: "a", PID: 11})
	if !query(t, ctx, "a") {
		t.Fatal("old logout removed newer login")
	}
	send(t, &LogoutMsg{RoleID: "a", PID: 33})
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
	case *LoginMsg:
		m.SetCmd(CmdLogin)
	case *LogoutMsg:
		m.SetCmd(CmdLogout)
	default:
		t.Fatalf("send %T", msg)
	}
	if err := gonet.SendName(Name, msg); err != nil {
		t.Fatal(err)
	}
}

func query(t *testing.T, ctx context.Context, roleID string) bool {
	t.Helper()
	msg := &QueryMsg{RoleID: roleID}
	msg.SetCmd(CmdQuery)
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
	case *BroadcastAllMsg:
		m.SetCmd(CmdBroadcastAll)
	case *BroadcastSomeMsg:
		m.SetCmd(CmdBroadcastSome)
	default:
		t.Fatalf("broadcast %T", msg)
	}
	if _, err := gonet.SuspendCallName(ctx, Name, msg); err != nil {
		t.Fatal(err)
	}
}

func waitPush(t *testing.T, ch <-chan PushMsg, cmd, data string) {
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
