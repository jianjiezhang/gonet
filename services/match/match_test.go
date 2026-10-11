package match_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"game/services/internal/actormsg"
	"game/services/internal/names"
	"game/services/match"
	"game/services/room"

	"gonet"
	"gonet/services/launcher"
)

func TestMatchLobby(t *testing.T) {
	ctx := start(t)
	if view := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "a", Mode: -1, Capacity: 2}); view.Err != "模式无效" {
		t.Fatalf("mode: %+v", view)
	}
	if view := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "a", Mode: 1, Capacity: 0}); view.Err != "人数无效" {
		t.Fatalf("cap: %+v", view)
	}
	created := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "a", Mode: 1, Capacity: 2})
	if created.Err != "" || created.RoomID == "" || created.Phase != actormsg.PhaseMatchWait || len(created.Seats) != 1 || created.Seats[0] != "a" {
		t.Fatalf("create: %+v", created)
	}
	if view := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "a", Mode: 1, Capacity: 2}); view.Err != "已经在房间" {
		t.Fatalf("dup: %+v", view)
	}
	if view := call(t, ctx, &actormsg.MatchJoinMsg{RoleID: "b", RoomID: "missing"}); view.Err != "房间不存在" {
		t.Fatalf("missing: %+v", view)
	}
	if view := call(t, ctx, &actormsg.MatchJoinMsg{RoleID: "b", RoomID: created.RoomID}); view.Err != "" || len(view.Seats) != 2 {
		t.Fatalf("join: %+v", view)
	}
	if view := call(t, ctx, &actormsg.MatchJoinMsg{RoleID: "c", RoomID: created.RoomID}); view.Err != "房间已满" {
		t.Fatalf("full: %+v", view)
	}
	if view := call(t, ctx, &actormsg.MatchStartMsg{RoleID: "nope"}); view.Err != "不在房间" {
		t.Fatalf("start outsider: %+v", view)
	}
	if view := call(t, ctx, &actormsg.MatchLeaveMsg{RoleID: "b"}); view.Err != "" {
		t.Fatalf("leave: %+v", view)
	}
	if view := call(t, ctx, &actormsg.MatchJoinMsg{RoleID: "c", RoomID: created.RoomID}); view.Err != "" || view.Seats[1] != "c" {
		t.Fatalf("rejoin: %+v", view)
	}
	if view := call(t, ctx, &actormsg.MatchLeaveMsg{RoleID: "a"}); view.Err != "" {
		t.Fatalf("leave a: %+v", view)
	}
	if view := call(t, ctx, &actormsg.MatchLeaveMsg{RoleID: "c"}); view.Err != "" {
		t.Fatalf("leave c: %+v", view)
	}
	if view := call(t, ctx, &actormsg.MatchJoinMsg{RoleID: "a", RoomID: created.RoomID}); view.Err != "房间不存在" {
		t.Fatalf("dropped: %+v", view)
	}
}

func TestMatchStartAndSettle(t *testing.T) {
	ctx := start(t)
	one := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "a", Mode: 1, Capacity: 1})
	two := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "b", Mode: 2, Capacity: 2})
	if one.Err != "" || two.Err != "" || one.RoomID == two.RoomID {
		t.Fatalf("create: %+v %+v", one, two)
	}
	if view := call(t, ctx, &actormsg.MatchStartMsg{RoleID: "b"}); view.Err != "人还没满" {
		t.Fatalf("not full: %+v", view)
	}
	if _, err := gonet.Query(names.Room(two.RoomID)); !errors.Is(err, gonet.ErrUnknownAlias) {
		t.Fatalf("scene exists early: %v", err)
	}
	if view := call(t, ctx, &actormsg.MatchJoinMsg{RoleID: "c", RoomID: two.RoomID}); view.Err != "" {
		t.Fatalf("join c: %+v", view)
	}
	started := call(t, ctx, &actormsg.MatchStartMsg{RoleID: "a"})
	if started.Err != "" || started.Phase != actormsg.PhaseMatchPlay || started.RoomID != one.RoomID {
		t.Fatalf("start one: %+v", started)
	}
	other := call(t, ctx, &actormsg.MatchStartMsg{RoleID: "b"})
	if other.Err != "" || other.Phase != actormsg.PhaseMatchPlay || other.Mode != 2 || len(other.Seats) != 2 {
		t.Fatalf("start two: %+v", other)
	}
	pid1, err := gonet.Query(names.Room(one.RoomID))
	if err != nil {
		t.Fatal(err)
	}
	pid2, err := gonet.Query(names.Room(two.RoomID))
	if err != nil {
		t.Fatal(err)
	}
	if pid1 == pid2 {
		t.Fatalf("same scene %d", pid1)
	}
	if view := call(t, ctx, &actormsg.MatchStartMsg{RoleID: "a"}); view.Err != "已经开场" {
		t.Fatalf("again: %+v", view)
	}
	if view := callRoom(t, ctx, two.RoomID, &actormsg.RoomLeaveMsg{RoleID: "c"}); view.Err != "" {
		t.Fatalf("scene leave: %+v", view)
	}
	if view := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "c", Mode: 1, Capacity: 1}); view.Err != "" {
		t.Fatalf("c free: %+v", view)
	}
	if view := callRoom(t, ctx, one.RoomID, &actormsg.RoomSettleMsg{RoleID: "a"}); view.Err != "" {
		t.Fatalf("settle one: %+v", view)
	}
	waitGone(t, names.Room(one.RoomID))
	if view := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "a", Mode: 1, Capacity: 1}); view.Err != "" {
		t.Fatalf("a free: %+v", view)
	}
	if view := callRoom(t, ctx, two.RoomID, &actormsg.RoomSettleMsg{RoleID: "b"}); view.Err != "" {
		t.Fatalf("settle two: %+v", view)
	}
	waitGone(t, names.Room(two.RoomID))
	if _, err := gonet.Query(match.Name); err != nil {
		t.Fatal(err)
	}
}

func TestMatchDuoStart(t *testing.T) {
	ctx := start(t)
	created := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "a", Mode: room.ModeDuo, Capacity: 2})
	if created.Err != "" {
		t.Fatal(created.Err)
	}
	if view := call(t, ctx, &actormsg.MatchJoinMsg{RoleID: "b", RoomID: created.RoomID}); view.Err != "" {
		t.Fatal(view.Err)
	}
	started := call(t, ctx, &actormsg.MatchStartMsg{RoleID: "a"})
	if started.Err != "" || started.Phase != actormsg.PhaseMatchPlay || started.Mode != room.ModeDuo {
		t.Fatalf("start: %+v", started)
	}
	if _, err := gonet.Query(names.Room(created.RoomID)); err != nil {
		t.Fatal(err)
	}
	if view := callRoom(t, ctx, created.RoomID, &actormsg.RoomSettleMsg{RoleID: "a"}); view.Err != "" {
		t.Fatalf("settle: %+v", view)
	}
	waitGone(t, names.Room(created.RoomID))
	if view := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "a", Mode: room.ModeDuo, Capacity: 2}); view.Err != "" {
		t.Fatalf("reuse: %+v", view)
	}
}

func TestMatchDuoDead(t *testing.T) {
	ctx := start(t)
	created := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "a", Mode: room.ModeDuo, Capacity: 2})
	if created.Err != "" {
		t.Fatal(created.Err)
	}
	if view := call(t, ctx, &actormsg.MatchJoinMsg{RoleID: "b", RoomID: created.RoomID}); view.Err != "" {
		t.Fatal(view.Err)
	}
	if view := call(t, ctx, &actormsg.MatchStartMsg{RoleID: "a"}); view.Err != "" {
		t.Fatal(view.Err)
	}
	if view := callRoom(t, ctx, created.RoomID, &actormsg.RoomDeadMsg{RoleID: "a", Frame: 1_000_000}); view.Err != "帧号无效" {
		t.Fatalf("future: %+v", view)
	}
	var accepted bool
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		view := callRoom(t, ctx, created.RoomID, &actormsg.RoomDeadMsg{RoleID: "a", Frame: 1})
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
	if _, err := gonet.Query(names.Room(created.RoomID)); err != nil {
		t.Fatal(err)
	}
	if view := callRoom(t, ctx, created.RoomID, &actormsg.RoomDeadMsg{RoleID: "b", Frame: 1}); view.Err != "" {
		t.Fatalf("second: %+v", view)
	}
	waitGone(t, names.Room(created.RoomID))
}

func TestMatchSceneMissing(t *testing.T) {
	ctx := start(t)
	old := match.SpawnRoom
	match.SpawnRoom = nil
	t.Cleanup(func() { match.SpawnRoom = old })
	created := call(t, ctx, &actormsg.MatchCreateMsg{RoleID: "a", Mode: 1, Capacity: 1})
	if created.Err != "" {
		t.Fatal(created.Err)
	}
	if view := call(t, ctx, &actormsg.MatchStartMsg{RoleID: "a"}); view.Err != "玩法场景还没接上" {
		t.Fatalf("start: %+v", view)
	}
	if _, err := gonet.Query(names.Room(created.RoomID)); !errors.Is(err, gonet.ErrUnknownAlias) {
		t.Fatalf("scene: %v", err)
	}
	match.SpawnRoom = old
	if view := call(t, ctx, &actormsg.MatchStartMsg{RoleID: "a"}); view.Err != "" || view.Phase != actormsg.PhaseMatchPlay {
		t.Fatalf("retry: %+v", view)
	}
}

func start(t *testing.T) context.Context {
	t.Helper()
	launcher.Bind(1)
	match.Bind(1)
	match.SpawnRoom = func(id string, mode, capacity int, seats []string) gonet.ActorContextInterface {
		return room.New(id, mode, capacity, seats)
	}
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

func call(t *testing.T, ctx context.Context, msg gonet.MessageInterface) *actormsg.MatchView {
	t.Helper()
	switch m := msg.(type) {
	case *actormsg.MatchCreateMsg:
		m.SetCmd(actormsg.CmdMatchCreate)
	case *actormsg.MatchJoinMsg:
		m.SetCmd(actormsg.CmdMatchJoin)
	case *actormsg.MatchLeaveMsg:
		m.SetCmd(actormsg.CmdMatchLeave)
	case *actormsg.MatchStartMsg:
		m.SetCmd(actormsg.CmdMatchStart)
	default:
		t.Fatalf("msg %T", msg)
	}
	v, err := gonet.SuspendCallName(ctx, match.Name, msg)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := v.(*actormsg.MatchView)
	if view == nil {
		t.Fatalf("reply %T", v)
	}
	return view
}

func callRoom(t *testing.T, ctx context.Context, id string, msg gonet.MessageInterface) *actormsg.RoomReply {
	t.Helper()
	switch m := msg.(type) {
	case *actormsg.RoomSettleMsg:
		m.SetCmd(actormsg.CmdRoomSettle)
	case *actormsg.RoomLeaveMsg:
		m.SetCmd(actormsg.CmdRoomLeave)
	case *actormsg.RoomDeadMsg:
		m.SetCmd(actormsg.CmdRoomDead)
	default:
		t.Fatalf("room %T", msg)
	}
	v, err := gonet.SuspendCallName(ctx, names.Room(id), msg)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := v.(*actormsg.RoomReply)
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
