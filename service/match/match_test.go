package match_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"game/service/match"
	"game/service/room"

	"gonet"
	"gonet/services/launcher"
)

func TestMatchLobby(t *testing.T) {
	ctx := start(t)
	if view := call(t, ctx, &match.CreateMsg{RoleID: "a", Mode: -1, Capacity: 2}); view.Err != "模式无效" {
		t.Fatalf("mode: %+v", view)
	}
	if view := call(t, ctx, &match.CreateMsg{RoleID: "a", Mode: 1, Capacity: 0}); view.Err != "人数无效" {
		t.Fatalf("cap: %+v", view)
	}
	created := call(t, ctx, &match.CreateMsg{RoleID: "a", Mode: 1, Capacity: 2})
	if created.Err != "" || created.RoomID == "" || created.Phase != match.PhaseWait || len(created.Seats) != 1 || created.Seats[0] != "a" {
		t.Fatalf("create: %+v", created)
	}
	if view := call(t, ctx, &match.CreateMsg{RoleID: "a", Mode: 1, Capacity: 2}); view.Err != "已经在房间" {
		t.Fatalf("dup: %+v", view)
	}
	if view := call(t, ctx, &match.JoinMsg{RoleID: "b", RoomID: "missing"}); view.Err != "房间不存在" {
		t.Fatalf("missing: %+v", view)
	}
	if view := call(t, ctx, &match.JoinMsg{RoleID: "b", RoomID: created.RoomID}); view.Err != "" || len(view.Seats) != 2 {
		t.Fatalf("join: %+v", view)
	}
	if view := call(t, ctx, &match.JoinMsg{RoleID: "c", RoomID: created.RoomID}); view.Err != "房间已满" {
		t.Fatalf("full: %+v", view)
	}
	if view := call(t, ctx, &match.StartMsg{RoleID: "nope"}); view.Err != "不在房间" {
		t.Fatalf("start outsider: %+v", view)
	}
	if view := call(t, ctx, &match.LeaveMsg{RoleID: "b"}); view.Err != "" {
		t.Fatalf("leave: %+v", view)
	}
	if view := call(t, ctx, &match.JoinMsg{RoleID: "c", RoomID: created.RoomID}); view.Err != "" || view.Seats[1] != "c" {
		t.Fatalf("rejoin: %+v", view)
	}
	if view := call(t, ctx, &match.LeaveMsg{RoleID: "a"}); view.Err != "" {
		t.Fatalf("leave a: %+v", view)
	}
	if view := call(t, ctx, &match.LeaveMsg{RoleID: "c"}); view.Err != "" {
		t.Fatalf("leave c: %+v", view)
	}
	if view := call(t, ctx, &match.JoinMsg{RoleID: "a", RoomID: created.RoomID}); view.Err != "房间不存在" {
		t.Fatalf("dropped: %+v", view)
	}
}

func TestMatchStartAndSettle(t *testing.T) {
	ctx := start(t)
	one := call(t, ctx, &match.CreateMsg{RoleID: "a", Mode: 1, Capacity: 1})
	two := call(t, ctx, &match.CreateMsg{RoleID: "b", Mode: 2, Capacity: 2})
	if one.Err != "" || two.Err != "" || one.RoomID == two.RoomID {
		t.Fatalf("create: %+v %+v", one, two)
	}
	if view := call(t, ctx, &match.StartMsg{RoleID: "b"}); view.Err != "人还没满" {
		t.Fatalf("not full: %+v", view)
	}
	if _, err := gonet.Query(room.Alias(two.RoomID)); !errors.Is(err, gonet.ErrUnknownAlias) {
		t.Fatalf("scene exists early: %v", err)
	}
	if view := call(t, ctx, &match.JoinMsg{RoleID: "c", RoomID: two.RoomID}); view.Err != "" {
		t.Fatalf("join c: %+v", view)
	}
	started := call(t, ctx, &match.StartMsg{RoleID: "a"})
	if started.Err != "" || started.Phase != match.PhasePlay || started.RoomID != one.RoomID {
		t.Fatalf("start one: %+v", started)
	}
	other := call(t, ctx, &match.StartMsg{RoleID: "b"})
	if other.Err != "" || other.Phase != match.PhasePlay || other.Mode != 2 || len(other.Seats) != 2 {
		t.Fatalf("start two: %+v", other)
	}
	pid1, err := gonet.Query(room.Alias(one.RoomID))
	if err != nil {
		t.Fatal(err)
	}
	pid2, err := gonet.Query(room.Alias(two.RoomID))
	if err != nil {
		t.Fatal(err)
	}
	if pid1 == pid2 {
		t.Fatalf("same scene %d", pid1)
	}
	if view := call(t, ctx, &match.StartMsg{RoleID: "a"}); view.Err != "已经开场" {
		t.Fatalf("again: %+v", view)
	}
	if view := callRoom(t, ctx, two.RoomID, &room.LeaveMsg{RoleID: "c"}); view.Err != "" {
		t.Fatalf("scene leave: %+v", view)
	}
	if view := call(t, ctx, &match.CreateMsg{RoleID: "c", Mode: 1, Capacity: 1}); view.Err != "" {
		t.Fatalf("c free: %+v", view)
	}
	if view := callRoom(t, ctx, one.RoomID, &room.SettleMsg{RoleID: "a"}); view.Err != "" {
		t.Fatalf("settle one: %+v", view)
	}
	waitGone(t, room.Alias(one.RoomID))
	if view := call(t, ctx, &match.CreateMsg{RoleID: "a", Mode: 1, Capacity: 1}); view.Err != "" {
		t.Fatalf("a free: %+v", view)
	}
	if view := callRoom(t, ctx, two.RoomID, &room.SettleMsg{RoleID: "b"}); view.Err != "" {
		t.Fatalf("settle two: %+v", view)
	}
	waitGone(t, room.Alias(two.RoomID))
	if _, err := gonet.Query(match.Name); err != nil {
		t.Fatal(err)
	}
}

func TestMatchDuoStart(t *testing.T) {
	ctx := start(t)
	created := call(t, ctx, &match.CreateMsg{RoleID: "a", Mode: room.ModeDuo, Capacity: 2})
	if created.Err != "" {
		t.Fatal(created.Err)
	}
	if view := call(t, ctx, &match.JoinMsg{RoleID: "b", RoomID: created.RoomID}); view.Err != "" {
		t.Fatal(view.Err)
	}
	started := call(t, ctx, &match.StartMsg{RoleID: "a"})
	if started.Err != "" || started.Phase != match.PhasePlay || started.Mode != room.ModeDuo {
		t.Fatalf("start: %+v", started)
	}
	if _, err := gonet.Query(room.Alias(created.RoomID)); err != nil {
		t.Fatal(err)
	}
	if view := callRoom(t, ctx, created.RoomID, &room.SettleMsg{RoleID: "a"}); view.Err != "" {
		t.Fatalf("settle: %+v", view)
	}
	waitGone(t, room.Alias(created.RoomID))
	if view := call(t, ctx, &match.CreateMsg{RoleID: "a", Mode: room.ModeDuo, Capacity: 2}); view.Err != "" {
		t.Fatalf("reuse: %+v", view)
	}
}

func TestMatchDuoDead(t *testing.T) {
	ctx := start(t)
	created := call(t, ctx, &match.CreateMsg{RoleID: "a", Mode: room.ModeDuo, Capacity: 2})
	if created.Err != "" {
		t.Fatal(created.Err)
	}
	if view := call(t, ctx, &match.JoinMsg{RoleID: "b", RoomID: created.RoomID}); view.Err != "" {
		t.Fatal(view.Err)
	}
	if view := call(t, ctx, &match.StartMsg{RoleID: "a"}); view.Err != "" {
		t.Fatal(view.Err)
	}
	if view := callRoom(t, ctx, created.RoomID, &room.DeadMsg{RoleID: "a", Frame: 1_000_000}); view.Err != "帧号无效" {
		t.Fatalf("future: %+v", view)
	}
	var accepted bool
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		view := callRoom(t, ctx, created.RoomID, &room.DeadMsg{RoleID: "a", Frame: 1})
		if view.Err == "" {
			accepted = true
			break
		}
		if view.Err != "帧号无效" {
			t.Fatalf("frame 1: %+v", view)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !accepted {
		t.Fatal("frame 1 was not accepted")
	}
	if _, err := gonet.Query(room.Alias(created.RoomID)); err != nil {
		t.Fatal(err)
	}
	if view := callRoom(t, ctx, created.RoomID, &room.DeadMsg{RoleID: "b", Frame: 1}); view.Err != "" {
		t.Fatalf("second: %+v", view)
	}
	waitGone(t, room.Alias(created.RoomID))
}

func TestMatchSceneMissing(t *testing.T) {
	ctx := start(t)
	old := match.SpawnRoom
	match.SpawnRoom = nil
	t.Cleanup(func() { match.SpawnRoom = old })
	created := call(t, ctx, &match.CreateMsg{RoleID: "a", Mode: 1, Capacity: 1})
	if created.Err != "" {
		t.Fatal(created.Err)
	}
	if view := call(t, ctx, &match.StartMsg{RoleID: "a"}); view.Err != "玩法场景还没接上" {
		t.Fatalf("start: %+v", view)
	}
	if _, err := gonet.Query(room.Alias(created.RoomID)); !errors.Is(err, gonet.ErrUnknownAlias) {
		t.Fatalf("scene: %v", err)
	}
	match.SpawnRoom = old
	if view := call(t, ctx, &match.StartMsg{RoleID: "a"}); view.Err != "" || view.Phase != match.PhasePlay {
		t.Fatalf("retry: %+v", view)
	}
}

func start(t *testing.T) context.Context {
	t.Helper()
	launcher.Bind(1)
	match.Bind(1)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	t.Cleanup(func() {
		cancel()
		gonet.Stop()
	})
	pid, err := gonet.SpawnNamed(launcher.New(), launcher.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := gonet.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}
	if _, err := launcher.WaitService(ctx, match.New(), match.Name); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func call(t *testing.T, ctx context.Context, msg gonet.MessageInterface) *match.View {
	t.Helper()
	switch m := msg.(type) {
	case *match.CreateMsg:
		m.SetCmd(match.CmdCreate)
	case *match.JoinMsg:
		m.SetCmd(match.CmdJoin)
	case *match.LeaveMsg:
		m.SetCmd(match.CmdLeave)
	case *match.StartMsg:
		m.SetCmd(match.CmdStart)
	default:
		t.Fatalf("msg %T", msg)
	}
	v, err := gonet.SuspendCallName(ctx, match.Name, msg)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := v.(*match.View)
	if view == nil {
		t.Fatalf("reply %T", v)
	}
	return view
}

func callRoom(t *testing.T, ctx context.Context, id string, msg gonet.MessageInterface) *room.Reply {
	t.Helper()
	switch m := msg.(type) {
	case *room.SettleMsg:
		m.SetCmd(room.CmdSettle)
	case *room.LeaveMsg:
		m.SetCmd(room.CmdLeave)
	case *room.DeadMsg:
		m.SetCmd(room.CmdDead)
	default:
		t.Fatalf("room %T", msg)
	}
	v, err := gonet.SuspendCallName(ctx, room.Alias(id), msg)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := v.(*room.Reply)
	if view == nil {
		t.Fatalf("room reply %T", v)
	}
	return view
}

func waitGone(t *testing.T, name string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := gonet.Query(name)
		if errors.Is(err, gonet.ErrUnknownAlias) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s still up", name)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
