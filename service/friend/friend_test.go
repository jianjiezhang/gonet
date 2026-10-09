package friend

import (
	"context"
	"testing"
	"time"

	"game/store"

	"gonet"
)

func TestApplyAgreeDelete(t *testing.T) {
	mem := store.NewMemory()
	store.Set(mem)
	t.Cleanup(func() { store.Set(nil) })
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

	if view := call(t, ctx, &ApplyMsg{From: "a", To: "a"}); view.Err != "不能加自己" {
		t.Fatalf("self: %+v", view)
	}
	if view := call(t, ctx, &ApplyMsg{From: "a", To: "b", FromName: "A"}); view.Err != "" {
		t.Fatalf("apply: %+v", view)
	}
	if view := call(t, ctx, &ApplyMsg{From: "a", To: "b"}); view.Err != "已经申请过" {
		t.Fatalf("dup: %+v", view)
	}
	if view := call(t, ctx, &ApplyMsg{From: "b", To: "a"}); view.Err != "对方已申请你" {
		t.Fatalf("reverse: %+v", view)
	}

	list := call(t, ctx, &ListMsg{Self: "b"})
	if len(list.Incoming) != 1 || list.Incoming[0].RoleID != "a" || len(list.Friends) != 0 {
		t.Fatalf("incoming: %+v", list)
	}
	out := call(t, ctx, &ListMsg{Self: "a"})
	if len(out.Outgoing) != 1 || out.Outgoing[0].RoleID != "b" {
		t.Fatalf("outgoing: %+v", out)
	}

	if view := call(t, ctx, decide(CmdAgree, "b", "a")); view.Err != "" {
		t.Fatalf("agree: %+v", view)
	}
	both := call(t, ctx, &ListMsg{Self: "a"})
	if len(both.Friends) != 1 || both.Friends[0] != "b" || len(both.Outgoing) != 0 {
		t.Fatalf("friends: %+v", both)
	}
	if view := call(t, ctx, &DeleteMsg{Self: "a", Target: "b"}); view.Err != "" {
		t.Fatalf("delete: %+v", view)
	}
	if view := call(t, ctx, &ListMsg{Self: "b"}); len(view.Friends) != 0 {
		t.Fatalf("after delete: %+v", view)
	}
	if view := call(t, ctx, &DeleteMsg{Self: "a", Target: "b"}); view.Err != "不是好友" {
		t.Fatalf("delete missing: %+v", view)
	}
}

func TestRejectAndCap(t *testing.T) {
	store.Set(store.NewMemory())
	t.Cleanup(func() { store.Set(nil) })
	old := friendLimit
	friendLimit = 1
	t.Cleanup(func() { friendLimit = old })
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

	if view := call(t, ctx, decide(CmdReject, "b", "a")); view.Err != "没有这条申请" {
		t.Fatalf("reject missing: %+v", view)
	}
	_ = call(t, ctx, &ApplyMsg{From: "a", To: "b"})
	if view := call(t, ctx, decide(CmdReject, "b", "a")); view.Err != "" {
		t.Fatalf("reject: %+v", view)
	}
	if view := call(t, ctx, &ListMsg{Self: "b"}); len(view.Incoming) != 0 {
		t.Fatalf("rejected still incoming: %+v", view)
	}

	_ = call(t, ctx, &ApplyMsg{From: "a", To: "b"})
	_ = call(t, ctx, decide(CmdAgree, "b", "a"))
	if view := call(t, ctx, &ApplyMsg{From: "a", To: "c"}); view.Err != "好友已满" {
		t.Fatalf("cap apply: %+v", view)
	}
	_ = call(t, ctx, &ApplyMsg{From: "c", To: "b"})
	if view := call(t, ctx, decide(CmdAgree, "b", "c")); view.Err != "好友已满" {
		t.Fatalf("cap agree: %+v", view)
	}
}

func TestReload(t *testing.T) {
	mem := store.NewMemory()
	store.Set(mem)
	t.Cleanup(func() { store.Set(nil) })
	defer gonet.Stop()
	pid, err := gonet.SpawnNamed(New(), Name)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := gonet.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}
	if view := call(t, ctx, &ApplyMsg{From: "a", To: "b"}); view.Err != "" {
		t.Fatal(view.Err)
	}
	if view := call(t, ctx, decide(CmdAgree, "b", "a")); view.Err != "" {
		t.Fatal(view.Err)
	}
	if err := gonet.StopActor(pid); err != nil {
		t.Fatal(err)
	}
	pid, err = gonet.SpawnNamed(New(), Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := gonet.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}
	view := call(t, ctx, &ListMsg{Self: "a"})
	if len(view.Friends) != 1 || view.Friends[0] != "b" {
		t.Fatalf("reloaded: %+v", view)
	}
}

func decide(cmd, self, from string) *DecideMsg {
	m := &DecideMsg{Self: self, From: from, SelfName: self}
	m.SetCmd(cmd)
	return m
}

func call(t *testing.T, ctx context.Context, msg gonet.MessageInterface) *View {
	t.Helper()
	switch m := msg.(type) {
	case *ApplyMsg:
		m.SetCmd(CmdApply)
	case *DecideMsg:
		if m.Command() == "" {
			t.Fatal("decide 缺少 cmd")
		}
	case *DeleteMsg:
		m.SetCmd(CmdDelete)
	case *ListMsg:
		m.SetCmd(CmdList)
	default:
		t.Fatalf("call %T", msg)
	}
	v, err := gonet.SuspendCallName(ctx, Name, msg)
	if err != nil {
		t.Fatal(err)
	}
	view, ok := v.(*View)
	if !ok {
		t.Fatalf("view %T", v)
	}
	return view
}
