package gonet_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"gonet"
)

func waitInit(t *testing.T, pid uint64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := gonet.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}
}

func mustSpawn(t *testing.T, impl gonet.ActorContextInterface) uint64 {
	t.Helper()
	pid, err := gonet.Spawn(impl)
	if err != nil {
		t.Fatal(err)
	}
	waitInit(t, pid)
	return pid
}

func mustSpawnNamed(t *testing.T, impl gonet.ActorContextInterface, name string) uint64 {
	t.Helper()
	pid, err := gonet.SpawnNamed(impl, name)
	if err != nil {
		t.Fatal(err)
	}
	waitInit(t, pid)
	return pid
}

func mustSpawnWith(t *testing.T, impl gonet.ActorContextInterface, opt gonet.SpawnOptions) uint64 {
	t.Helper()
	pid, err := gonet.SpawnWith(impl, opt)
	if err != nil {
		t.Fatal(err)
	}
	waitInit(t, pid)
	return pid
}

func TestSpawnNil(t *testing.T) {
	_, err := gonet.Spawn(nil)
	if !errors.Is(err, gonet.ErrNilActor) {
		t.Fatalf("got %v", err)
	}
}

func TestSpawnInitError(t *testing.T) {
	pid, err := gonet.Spawn(&failInitActor{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = gonet.WaitInit(ctx, pid)
	if err == nil || err.Error() != "init failed" {
		t.Fatalf("got %v", err)
	}
	if err := gonet.SendMemory(pid, pingMsg{}); !errors.Is(err, gonet.ErrDead) {
		t.Fatalf("Send: %v", err)
	}
}

func TestNotReadyBeforeInit(t *testing.T) {
	pid, err := gonet.Spawn(&slowInitActor{d: 80 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err := gonet.SendMemory(pid, pingMsg{}); !errors.Is(err, gonet.ErrNotReady) {
		t.Fatalf("got %v", err)
	}
	waitInit(t, pid)
	defer gonet.StopActor(pid)
	if err := gonet.SendMemory(pid, pingMsg{}); err != nil {
		t.Fatal(err)
	}
}

func TestCallAndStopActor(t *testing.T) {
	a := &echoActor{}
	pid := mustSpawn(t, a)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v, err := gonet.SuspendCallMemory(ctx, pid, pingMsg{})
	if err != nil {
		t.Fatal(err)
	}
	if v != "pong" {
		t.Fatalf("got %v", v)
	}
	if err := gonet.StopActor(pid); err != nil {
		t.Fatal(err)
	}
	if !a.term.Load() {
		t.Fatal("Term not called")
	}
	if err := gonet.SendMemory(pid, pingMsg{}); !errors.Is(err, gonet.ErrDead) {
		t.Fatalf("Send after stop: %v", err)
	}
	if err := gonet.StopActor(pid); !errors.Is(err, gonet.ErrDead) {
		t.Fatalf("StopActor twice: %v", err)
	}
}

func TestEnvelopeSelf(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v, err := gonet.SuspendCallMemory(ctx, pid, getNMsg{})
	if err != nil {
		t.Fatal(err)
	}
	if v != pid {
		t.Fatalf("Self=%v want pid=%d", v, pid)
	}
}

func TestCallTimeout(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)

	started := make(chan struct{})
	release := make(chan struct{})
	if err := gonet.SendMemory(pid, holdMsg{started: started, release: release}); err != nil {
		t.Fatal(err)
	}
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := gonet.SuspendCallMemory(ctx, pid, pingMsg{})
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

func TestMailboxFull(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)

	started := make(chan struct{})
	release := make(chan struct{})
	if err := gonet.SendMemory(pid, holdMsg{started: started, release: release}); err != nil {
		t.Fatal(err)
	}
	<-started

	var full error
	for i := 0; i < 2000; i++ {
		err := gonet.SendMemory(pid, pingMsg{})
		if errors.Is(err, gonet.ErrMailboxFull) {
			full = err
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	close(release)
	if full == nil {
		t.Fatal("expected ErrMailboxFull")
	}
}

func TestDispatchPanicKeepsActor(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)

	if err := gonet.SendMemory(pid, boomMsg{}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v, err := gonet.SuspendCallMemory(ctx, pid, pingMsg{})
	if err != nil {
		t.Fatal(err)
	}
	if v != "pong" {
		t.Fatalf("actor should survive panic, got %v", v)
	}
}

func TestStopAll(t *testing.T) {
	pid1 := mustSpawn(t, &echoActor{})
	pid2 := mustSpawn(t, &echoActor{})
	gonet.Stop()
	if err := gonet.SendMemory(pid1, pingMsg{}); !errors.Is(err, gonet.ErrDead) {
		t.Fatalf("pid1: %v", err)
	}
	if err := gonet.SendMemory(pid2, pingMsg{}); !errors.Is(err, gonet.ErrDead) {
		t.Fatalf("pid2: %v", err)
	}
}

func TestStopClosesDebug(t *testing.T) {
	addr, err := gonet.ListenDebug("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gonet.Stop()
	_, err = net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err == nil {
		t.Fatal("debug still listening")
	}
}

func TestFromExternalIsZero(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v, err := gonet.SuspendCallMemory(ctx, pid, askFromMsg{})
	if err != nil {
		t.Fatal(err)
	}
	if v != uint64(0) {
		t.Fatalf("From=%v want 0", v)
	}
}

func TestContextSendSetsFrom(t *testing.T) {
	dst := &fromSink{got: make(chan uint64, 2)}
	dpid := mustSpawn(t, dst)
	defer gonet.StopActor(dpid)
	src := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(src)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := gonet.SuspendCallMemory(ctx, src, sendPeerMsg{to: dpid}); err != nil {
		t.Fatal(err)
	}
	select {
	case from := <-dst.got:
		if from != src {
			t.Fatalf("Send From=%d want %d", from, src)
		}
	case <-time.After(time.Second):
		t.Fatal("no Send")
	}

	if err := gonet.Register(dpid, "sink"); err != nil {
		t.Fatal(err)
	}
	if _, err := gonet.SuspendCallMemory(ctx, src, sendPeerMsg{name: "sink"}); err != nil {
		t.Fatal(err)
	}
	select {
	case from := <-dst.got:
		if from != src {
			t.Fatalf("SendName From=%d want %d", from, src)
		}
	case <-time.After(time.Second):
		t.Fatal("no SendName")
	}
}

func TestFromPeer(t *testing.T) {
	src := mustSpawn(t, &echoActor{})
	dst := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(src)
	defer gonet.StopActor(dst)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v, err := gonet.SuspendCallMemory(ctx, src, toPeerMsg{to: dst})
	if err != nil {
		t.Fatal(err)
	}
	if v != src {
		t.Fatalf("From=%v want src=%d", v, src)
	}
}

func TestSuspendCallSelf(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v, err := gonet.SuspendCallMemory(ctx, pid, suspendSelfMsg{})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := v.(error)
	if !ok || !errors.Is(got, gonet.ErrSuspendSelf) {
		t.Fatalf("got %v (%T)", v, v)
	}
}

func ctx1s(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), time.Second)
}

func TestSpawnNamedAndCallName(t *testing.T) {
	pid := mustSpawnNamed(t, &echoActor{}, "echo")
	defer gonet.StopActor(pid)

	got, err := gonet.Query("echo")
	if err != nil || got != pid {
		t.Fatalf("Query: pid=%d err=%v want %d", got, err, pid)
	}

	ctx, cancel := ctx1s(t)
	defer cancel()
	v, err := gonet.SuspendCallMemoryName(ctx, "echo", pingMsg{})
	if err != nil {
		t.Fatal(err)
	}
	if v != "pong" {
		t.Fatalf("got %v", v)
	}
}

func TestSpawnNamedEmptyAndTaken(t *testing.T) {
	if _, err := gonet.SpawnNamed(&echoActor{}, ""); !errors.Is(err, gonet.ErrInvalidAlias) {
		t.Fatalf("empty: %v", err)
	}
	pid := mustSpawnNamed(t, &echoActor{}, "taken")
	defer gonet.StopActor(pid)
	if _, err := gonet.SpawnNamed(&echoActor{}, "taken"); !errors.Is(err, gonet.ErrAliasTaken) {
		t.Fatalf("taken: %v", err)
	}
}

func TestRegisterOnceAndExternal(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)

	if err := gonet.Register(pid, ""); !errors.Is(err, gonet.ErrInvalidAlias) {
		t.Fatalf("empty: %v", err)
	}
	if err := gonet.Register(pid, "svc"); err != nil {
		t.Fatal(err)
	}
	if err := gonet.Register(pid, "svc2"); !errors.Is(err, gonet.ErrAliasBound) {
		t.Fatalf("second: %v", err)
	}
	pid2 := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid2)
	if err := gonet.Register(pid2, "svc"); !errors.Is(err, gonet.ErrAliasTaken) {
		t.Fatalf("taken: %v", err)
	}
}

func TestUnregisterReuse(t *testing.T) {
	pid := mustSpawnNamed(t, &echoActor{}, "reuse")
	defer gonet.StopActor(pid)

	if err := gonet.Unregister("reuse"); err != nil {
		t.Fatal(err)
	}
	if _, err := gonet.Query("reuse"); !errors.Is(err, gonet.ErrUnknownAlias) {
		t.Fatalf("query after unreg: %v", err)
	}
	if err := gonet.Register(pid, "reuse"); err != nil {
		t.Fatal(err)
	}
}

func TestDeathDropsAlias(t *testing.T) {
	pid := mustSpawnNamed(t, &echoActor{}, "dead")
	if err := gonet.StopActor(pid); err != nil {
		t.Fatal(err)
	}
	if _, err := gonet.Query("dead"); !errors.Is(err, gonet.ErrUnknownAlias) {
		t.Fatalf("query: %v", err)
	}
	if err := gonet.SendMemoryName("dead", pingMsg{}); !errors.Is(err, gonet.ErrUnknownAlias) {
		t.Fatalf("send: %v", err)
	}
	pid2 := mustSpawnNamed(t, &echoActor{}, "dead")
	gonet.StopActor(pid2)
}

func TestInitRegister(t *testing.T) {
	a := &initRegActor{name: "inited"}
	pid := mustSpawn(t, a)
	defer gonet.StopActor(pid)
	if a.reg != nil {
		t.Fatal(a.reg)
	}
	got, err := gonet.Query("inited")
	if err != nil || got != pid {
		t.Fatalf("Query: %d %v", got, err)
	}
}

func TestInitFailAfterRegisterDropsAlias(t *testing.T) {
	pid, err := gonet.Spawn(&failAfterRegActor{name: "tmp"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = gonet.WaitInit(ctx, pid)
	if err == nil || err.Error() != "init failed" {
		t.Fatalf("got %v", err)
	}
	if _, err := gonet.Query("tmp"); !errors.Is(err, gonet.ErrUnknownAlias) {
		t.Fatalf("alias leaked: %v", err)
	}
	pid2 := mustSpawnNamed(t, &echoActor{}, "tmp")
	gonet.StopActor(pid2)
}

func TestSpawnNamedThenInitRegisterFails(t *testing.T) {
	a := &initRegActor{name: "dup"}
	pid, err := gonet.SpawnNamed(a, "dup")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = gonet.WaitInit(ctx, pid)
	if !errors.Is(err, gonet.ErrAliasBound) {
		t.Fatalf("got %v", err)
	}
	if _, err := gonet.Query("dup"); !errors.Is(err, gonet.ErrUnknownAlias) {
		t.Fatalf("alias leaked: %v", err)
	}
}

func TestEnvelopeRegisterAndCallName(t *testing.T) {
	src := mustSpawn(t, &echoActor{})
	dst := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(src)
	defer gonet.StopActor(dst)

	ctx, cancel := ctx1s(t)
	defer cancel()
	v, err := gonet.SuspendCallMemory(ctx, dst, registerMsg{name: "peer"})
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Fatalf("register: %v", v)
	}

	from, err := gonet.SuspendCallMemory(ctx, src, toPeerNameMsg{name: "peer"})
	if err != nil {
		t.Fatal(err)
	}
	if from != src {
		t.Fatalf("From=%v want src=%d", from, src)
	}

	v, err = gonet.SuspendCallMemory(ctx, dst, unregisterMsg{})
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Fatalf("unregister: %v", v)
	}
	if err := gonet.SendMemoryName("peer", pingMsg{}); !errors.Is(err, gonet.ErrUnknownAlias) {
		t.Fatalf("send: %v", err)
	}
}

func TestQueryUnknownAndRegisterDead(t *testing.T) {
	if _, err := gonet.Query(""); !errors.Is(err, gonet.ErrInvalidAlias) {
		t.Fatalf("empty query: %v", err)
	}
	if _, err := gonet.Query("nope"); !errors.Is(err, gonet.ErrUnknownAlias) {
		t.Fatalf("unknown: %v", err)
	}
	if err := gonet.Register(999999, "x"); !errors.Is(err, gonet.ErrDead) {
		t.Fatalf("dead: %v", err)
	}
}

func waitCount(t *testing.T, pid uint64, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	got := -1
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		v, err := gonet.SuspendCallMemory(ctx, pid, getCountMsg{})
		cancel()
		if err == nil {
			got = v.(int)
			if got == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("count=%d want %d", got, want)
}

func TestTimeoutFires(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)

	msg := pingMsg{}
	msg.SetCmd("ping")
	if _, err := gonet.Timeout(pid, 10*time.Millisecond, msg); err != nil {
		t.Fatal(err)
	}
	waitCount(t, pid, 1)
}

func TestStopTimer(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)

	id, err := gonet.Timeout(pid, 80*time.Millisecond, pingMsg{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gonet.StopTimer(id); err != nil {
		t.Fatal(err)
	}
	if err := gonet.StopTimer(id); err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	ctx, cancel := ctx1s(t)
	defer cancel()
	v, err := gonet.SuspendCallMemory(ctx, pid, getCountMsg{})
	if err != nil {
		t.Fatal(err)
	}
	if v.(int) != 0 {
		t.Fatalf("count=%v", v)
	}
}

func TestStopActorDropsTimers(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	if _, err := gonet.Timeout(pid, 50*time.Millisecond, pingMsg{}); err != nil {
		t.Fatal(err)
	}
	if err := gonet.StopActor(pid); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if err := gonet.SendMemory(pid, pingMsg{}); !errors.Is(err, gonet.ErrDead) {
		t.Fatalf("Send: %v", err)
	}
	if gonet.Monitor().Timers != 0 {
		snap := gonet.Monitor()
		t.Fatalf("timers leaked: %+v", snap)
	}
}

func TestTimeoutDeadAndNil(t *testing.T) {
	if _, err := gonet.Timeout(1, time.Millisecond, pingMsg{}); !errors.Is(err, gonet.ErrDead) {
		t.Fatalf("dead: %v", err)
	}
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)
	if _, err := gonet.Timeout(pid, time.Millisecond, nil); !errors.Is(err, gonet.ErrNilMessage) {
		t.Fatalf("nil: %v", err)
	}
}

func TestEnvelopeTimeout(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)

	ctx, cancel := ctx1s(t)
	defer cancel()
	v, err := gonet.SuspendCallMemory(ctx, pid, scheduleMsg{d: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Fatalf("schedule: %v", v)
	}
	waitCount(t, pid, 1)
}

func TestMonitor(t *testing.T) {
	pid := mustSpawnNamed(t, &echoActor{}, "mon")
	defer gonet.StopActor(pid)

	id, err := gonet.Timeout(pid, time.Second, pingMsg{})
	if err != nil {
		t.Fatal(err)
	}
	defer gonet.StopTimer(id)

	msg := pingMsg{}
	msg.SetCmd("ping")
	ctx, cancel := ctx1s(t)
	defer cancel()
	if _, err := gonet.SuspendCallMemory(ctx, pid, msg); err != nil {
		t.Fatal(err)
	}

	snap := gonet.Monitor()
	if snap.Actors < 1 || snap.Timers < 1 {
		t.Fatalf("snap=%+v", snap)
	}
	found := false
	for _, s := range snap.List {
		if s.PID != pid {
			continue
		}
		found = true
		if s.Alias != "mon" || s.Timers < 1 || s.MailboxCap != 1024 {
			t.Fatalf("stat=%+v", s)
		}
		if s.LastCmd != "ping" {
			t.Fatalf("LastCmd=%q", s.LastCmd)
		}
		if s.LastDispatch < 0 {
			t.Fatalf("LastDispatch=%v", s.LastDispatch)
		}
	}
	if !found {
		t.Fatalf("pid %d missing: %+v", pid, snap.List)
	}
}

func TestExitFromDispatch(t *testing.T) {
	a := &echoActor{}
	pid := mustSpawn(t, a)
	ctx, cancel := ctx1s(t)
	defer cancel()
	v, err := gonet.SuspendCallMemory(ctx, pid, exitMsg{})
	if err != nil {
		t.Fatal(err)
	}
	if v != "bye" {
		t.Fatalf("got %v", v)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := gonet.SendMemory(pid, pingMsg{}); errors.Is(err, gonet.ErrDead) {
			if !a.term.Load() {
				t.Fatal("Term not called")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("actor still alive")
}

func TestSpawnMailboxSize(t *testing.T) {
	pid := mustSpawnWith(t, &echoActor{}, gonet.SpawnOptions{Mailbox: 2})
	defer gonet.StopActor(pid)

	started := make(chan struct{})
	release := make(chan struct{})
	if err := gonet.SendMemory(pid, holdMsg{started: started, release: release}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := gonet.SendMemory(pid, pingMsg{}); err != nil {
		t.Fatal(err)
	}
	if err := gonet.SendMemory(pid, pingMsg{}); err != nil {
		t.Fatal(err)
	}
	if err := gonet.SendMemory(pid, pingMsg{}); !errors.Is(err, gonet.ErrMailboxFull) {
		t.Fatalf("got %v", err)
	}
	close(release)
}

func TestSendDeadCounted(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	if err := gonet.StopActor(pid); err != nil {
		t.Fatal(err)
	}
	before := gonet.Monitor().SendDead
	if err := gonet.SendMemory(pid, pingMsg{}); !errors.Is(err, gonet.ErrDead) {
		t.Fatalf("got %v", err)
	}
	if gonet.Monitor().SendDead < before+1 {
		t.Fatalf("SendDead not counted")
	}
}

func TestCmdFactoryReplaced(t *testing.T) {
	pid := mustSpawn(t, &swapHost{})
	defer gonet.StopActor(pid)

	msg, err := gonet.Pack("swap", struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := ctx1s(t)
	defer cancel()
	v, err := gonet.SuspendCall(ctx, pid, msg)
	if err != nil {
		t.Fatal(err)
	}
	if v != "*gonet_test.typeB" {
		t.Fatalf("got %v", v)
	}
}

func TestDebugKill(t *testing.T) {
	a := &echoActor{}
	pid := mustSpawn(t, a)
	addr, err := gonet.ListenDebug("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer gonet.StopDebug()

	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := fmt.Fprintf(c, "kill %d\n", pid); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := gonet.SendMemory(pid, pingMsg{}); errors.Is(err, gonet.ErrDead) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("kill did not stop actor")
}

func TestCallRequiresCaller(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)
	if _, err := gonet.CallMemory(0, pid, time.Second, pingMsg{}); !errors.Is(err, gonet.ErrNoCaller) {
		t.Fatalf("got %v", err)
	}
}

func TestCallNoTimeout(t *testing.T) {
	dst := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(dst)
	src := &asyncCaller{}
	spid := mustSpawn(t, src)
	defer gonet.StopActor(spid)

	started := make(chan struct{})
	release := make(chan struct{})
	if err := gonet.SendMemory(dst, holdMsg{started: started, release: release}); err != nil {
		t.Fatal(err)
	}
	<-started

	sess, err := gonet.CallMemory(spid, dst, 0, pingMsg{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case cr := <-src.got:
		t.Fatalf("unexpected CallResponse %+v", cr)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case cr := <-src.got:
		if cr.Session != sess || cr.Err != nil {
			t.Fatalf("got %+v", cr)
		}
	case <-time.After(time.Second):
		t.Fatal("no CallResponse")
	}
}

func TestAsyncCallTimeout(t *testing.T) {
	dst := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(dst)
	src := &asyncCaller{}
	spid := mustSpawn(t, src)
	defer gonet.StopActor(spid)

	started := make(chan struct{})
	release := make(chan struct{})
	if err := gonet.SendMemory(dst, holdMsg{started: started, release: release}); err != nil {
		t.Fatal(err)
	}
	<-started

	sess, err := gonet.CallMemory(spid, dst, 30*time.Millisecond, pingMsg{})
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	select {
	case cr := <-src.got:
		if cr.Session != sess {
			t.Fatalf("session=%d want %d", cr.Session, sess)
		}
		if !errors.Is(cr.Err, gonet.ErrCallTimeout) {
			t.Fatalf("err=%v", cr.Err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("no CallResponse")
	}
	close(release)
}

func TestAsyncCallReply(t *testing.T) {
	dst := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(dst)
	src := &asyncCaller{}
	spid := mustSpawn(t, src)
	defer gonet.StopActor(spid)

	sess, err := gonet.CallMemory(spid, dst, time.Second, pingMsg{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case cr := <-src.got:
		if cr.Session != sess || cr.Err != nil || cr.Value != "pong" {
			t.Fatalf("got %+v", cr)
		}
	case <-time.After(time.Second):
		t.Fatal("no CallResponse")
	}
}

func TestDispatchPanicFailsCall(t *testing.T) {
	dst := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(dst)
	src := &asyncCaller{}
	spid := mustSpawn(t, src)
	defer gonet.StopActor(spid)

	sess, err := gonet.CallMemory(spid, dst, time.Second, boomMsg{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case cr := <-src.got:
		if cr.Session != sess || !errors.Is(cr.Err, gonet.ErrDispatchPanic) {
			t.Fatalf("got %+v", cr)
		}
	case <-time.After(time.Second):
		t.Fatal("no CallResponse")
	}
}

func TestInitTimeout(t *testing.T) {
	pid, err := gonet.Spawn(&slowInitActor{d: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := gonet.WaitInit(ctx, pid); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	_ = gonet.StopActor(pid)
}

func TestInitFromCallTimeout(t *testing.T) {
	src := &asyncCaller{}
	spid := mustSpawn(t, src)
	defer gonet.StopActor(spid)
	const alias = "init-timeout-keep"
	pid, sess, err := gonet.SpawnAsync(&slowInitActor{d: 200 * time.Millisecond}, gonet.SpawnOptions{
		Name:        alias,
		InitFrom:    spid,
		InitTimeout: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sess == 0 {
		t.Fatal("expected init session")
	}
	select {
	case cr := <-src.got:
		if !errors.Is(cr.Err, gonet.ErrCallTimeout) {
			t.Fatalf("err=%v", cr.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("no CallResponse")
	}
	got, err := gonet.Query(alias)
	if err != nil || got != pid {
		t.Fatalf("actor should still exist: pid=%d err=%v", got, err)
	}
	if err := gonet.SendMemory(pid, pingMsg{}); !errors.Is(err, gonet.ErrNotReady) {
		t.Fatalf("Send during Init: %v", err)
	}
	waitInit(t, pid)
	defer gonet.StopActor(pid)
	if err := gonet.SendMemory(pid, pingMsg{}); err != nil {
		t.Fatal(err)
	}
}

func TestInitFromNoTimeout(t *testing.T) {
	src := &asyncCaller{}
	spid := mustSpawn(t, src)
	defer gonet.StopActor(spid)
	pid, sess, err := gonet.SpawnAsync(&slowInitActor{d: 80 * time.Millisecond}, gonet.SpawnOptions{
		InitFrom: spid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sess == 0 {
		t.Fatal("expected init session")
	}
	select {
	case cr := <-src.got:
		if cr.Err != nil {
			t.Fatalf("err=%v", cr.Err)
		}
		v, ok := cr.Value.(uint64)
		if !ok || v != pid {
			t.Fatalf("value=%v", cr.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("no CallResponse")
	}
	defer gonet.StopActor(pid)
}

func TestStopActorWaitTimeout(t *testing.T) {
	pid := mustSpawn(t, &slowTermActor{d: 200 * time.Millisecond})
	err := gonet.StopActorWait(pid, 20*time.Millisecond)
	if !errors.Is(err, gonet.ErrStopTimeout) {
		t.Fatalf("got %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := gonet.SendMemory(pid, pingMsg{}); errors.Is(err, gonet.ErrDead) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("actor still alive")
}

func TestPackSystemCmd(t *testing.T) {
	if _, err := gonet.Pack(gonet.CmdResponse, struct{}{}); !errors.Is(err, gonet.ErrSystemCmd) {
		t.Fatalf("got %v", err)
	}
	if !gonet.IsSystemCmd(".response") || gonet.IsSystemCmd("ping") {
		t.Fatal("IsSystemCmd")
	}
}

func TestSystemCmdFromBaseMessage(t *testing.T) {
	a := &bareHost{}
	pid := mustSpawn(t, a)
	defer gonet.StopActor(pid)
	msg := &gonet.BaseMessage{Cmd: gonet.CmdResponse, Data: "{}"}
	if err := gonet.Send(pid, msg); err != nil {
		t.Fatal(err)
	}
	ping, err := gonet.Pack("ping", struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := ctx1s(t)
	defer cancel()
	v, err := gonet.SuspendCall(ctx, pid, ping)
	if err != nil || v != "pong" {
		t.Fatalf("actor broken: %v %v", v, err)
	}
}

func TestDefaultResponseNoHandler(t *testing.T) {
	dst := mustSpawn(t, &bareHost{})
	defer gonet.StopActor(dst)
	src := mustSpawn(t, &bareHost{})
	defer gonet.StopActor(src)
	ping, err := gonet.Pack("ping", struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gonet.Call(src, dst, 50*time.Millisecond, ping); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	ctx, cancel := ctx1s(t)
	defer cancel()
	if v, err := gonet.SuspendCall(ctx, src, ping); err != nil || v != "pong" {
		t.Fatalf("src broken: %v %v", v, err)
	}
}

func TestMonitorPerActorFull(t *testing.T) {
	pid := mustSpawnWith(t, &echoActor{}, gonet.SpawnOptions{Mailbox: 2})
	defer gonet.StopActor(pid)
	started := make(chan struct{})
	release := make(chan struct{})
	if err := gonet.SendMemory(pid, holdMsg{started: started, release: release}); err != nil {
		t.Fatal(err)
	}
	<-started
	_ = gonet.SendMemory(pid, pingMsg{})
	_ = gonet.SendMemory(pid, pingMsg{})
	if err := gonet.SendMemory(pid, pingMsg{}); !errors.Is(err, gonet.ErrMailboxFull) {
		t.Fatalf("got %v", err)
	}
	snap := gonet.Monitor()
	found := false
	for _, s := range snap.List {
		if s.PID != pid {
			continue
		}
		found = true
		if s.SendFull < 1 || s.MailboxHigh < 1 {
			t.Fatalf("stat=%+v", s)
		}
	}
	if !found {
		t.Fatal("pid missing")
	}
	if snap.SendFull < 1 {
		t.Fatalf("global SendFull=%d", snap.SendFull)
	}
	close(release)
}

func TestAsyncCallTimeoutCounted(t *testing.T) {
	dst := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(dst)
	src := &asyncCaller{}
	spid := mustSpawn(t, src)
	defer gonet.StopActor(spid)

	started := make(chan struct{})
	release := make(chan struct{})
	if err := gonet.SendMemory(dst, holdMsg{started: started, release: release}); err != nil {
		t.Fatal(err)
	}
	<-started
	before := gonet.Monitor().CallTimeouts
	if _, err := gonet.CallMemory(spid, dst, 20*time.Millisecond, pingMsg{}); err != nil {
		close(release)
		t.Fatal(err)
	}
	select {
	case cr := <-src.got:
		if !errors.Is(cr.Err, gonet.ErrCallTimeout) {
			close(release)
			t.Fatalf("err=%v", cr.Err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("no response")
	}
	close(release)
	snap := gonet.Monitor()
	if snap.CallTimeouts < before+1 {
		t.Fatalf("CallTimeouts=%d", snap.CallTimeouts)
	}
	for _, s := range snap.List {
		if s.PID == spid && s.CallTimeouts < 1 {
			t.Fatalf("src call_to=%d", s.CallTimeouts)
		}
	}
}

func TestMonitorDuringStopNoRace(t *testing.T) {
	pid := mustSpawn(t, &slowTermActor{d: 50 * time.Millisecond})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = gonet.StopActor(pid)
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		_ = gonet.Monitor()
		select {
		case <-done:
			return
		default:
			time.Sleep(time.Millisecond)
		}
	}
	<-done
}

type greetMsg struct {
	gonet.BaseMessage
	Name string `json:"name"`
}

type greetActor struct {
	gonet.ActorContext
}

func (a *greetActor) Init() error {
	return gonet.RegisterCmd(a, "greet", func() gonet.MessageInterface { return &greetMsg{} }, func(e gonet.Envelope) {
		m, _ := e.Msg.(*greetMsg)
		if m == nil {
			e.Reply("")
			return
		}
		e.Reply(m.Name)
	})
}

func TestSendJSONRoundTrip(t *testing.T) {
	pid := mustSpawn(t, &greetActor{})
	defer gonet.StopActor(pid)
	msg := &greetMsg{Name: "ada"}
	msg.SetCmd("greet")
	ctx, cancel := ctx1s(t)
	defer cancel()
	v, err := gonet.SuspendCall(ctx, pid, msg)
	if err != nil {
		t.Fatal(err)
	}
	if v != "ada" {
		t.Fatalf("got %#v", v)
	}
}

func TestSendRejectsEmptyCmdAndChannel(t *testing.T) {
	pid := mustSpawn(t, &echoActor{})
	defer gonet.StopActor(pid)
	if err := gonet.Send(pid, pingMsg{}); !errors.Is(err, gonet.ErrInvalidCmd) {
		t.Fatalf("empty cmd: %v", err)
	}
	bad := struct {
		gonet.BaseMessage
		Ch chan struct{} `json:"ch"`
	}{Ch: make(chan struct{})}
	bad.SetCmd("bad")
	if err := gonet.Send(pid, bad); err == nil {
		t.Fatal("channel should not serialize")
	}
}
