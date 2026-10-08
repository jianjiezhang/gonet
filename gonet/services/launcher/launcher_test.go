package launcher

import (
	"context"
	"testing"
	"time"

	"gonet"
)

func TestPingPong(t *testing.T) {
	Bind(1)
	pid, err := gonet.SpawnNamed(New(), Name)
	if err != nil {
		t.Fatal(err)
	}
	defer gonet.StopActor(pid)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := gonet.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}

	msg, err := gonet.Pack(cmdPing, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	v, err := gonet.SuspendCallName(ctx, Name, msg)
	if err != nil {
		t.Fatal(err)
	}
	if v != "pong" {
		t.Fatalf("got %v", v)
	}
}

type dummyActor struct {
	gonet.ActorContext
}

func TestNewService(t *testing.T) {
	Bind(1)
	pid, err := gonet.SpawnNamed(New(), Name)
	if err != nil {
		t.Fatal(err)
	}
	defer gonet.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := gonet.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}

	child, err := WaitService(ctx, &dummyActor{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if child == 0 || child == pid {
		t.Fatalf("child pid=%d launcher=%d", child, pid)
	}

	named, err := WaitService(ctx, &dummyActor{}, ".dummy")
	if err != nil {
		t.Fatal(err)
	}
	got, err := gonet.Query(".dummy")
	if err != nil {
		t.Fatal(err)
	}
	if got != named {
		t.Fatalf("Query=.dummy got %d want %d", got, named)
	}

	if _, err := WaitService(ctx, nil, ""); err != gonet.ErrNilActor {
		t.Fatalf("nil impl: %v", err)
	}
	if err := NewService(pid, &dummyActor{}, "", time.Second, nil); err != ErrNilCallback {
		t.Fatalf("nil callback: %v", err)
	}
}
