package idgen

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"gonet"
)

func TestAllocKindsStayApart(t *testing.T) {
	ctx := start(t)
	role1 := alloc(t, ctx, "role")
	guild1 := alloc(t, ctx, "guild")
	role2 := alloc(t, ctx, "role")
	if role1.Err != "" || guild1.Err != "" || role2.Err != "" {
		t.Fatalf("alloc: %+v %+v %+v", role1, guild1, role2)
	}
	if role1.ID != "1" || guild1.ID != "1" || role2.ID != "2" {
		t.Fatalf("ids role=%s guild=%s role2=%s", role1.ID, guild1.ID, role2.ID)
	}
	if view := alloc(t, ctx, ""); view.Err != "种类无效" {
		t.Fatalf("empty: %+v", view)
	}
	if view := alloc(t, ctx, "has space"); view.Err != "种类无效" {
		t.Fatalf("space: %+v", view)
	}
	q := query(t, ctx, "role")
	if q.Err != "" || q.ID != "2" {
		t.Fatalf("query: %+v", q)
	}
	if q := query(t, ctx, "room"); q.Err != "" || q.ID != "0" {
		t.Fatalf("unused: %+v", q)
	}
	list := callList(t, ctx)
	if list.Err != "" || len(list.Items) != 2 || list.Items[0].Kind != "guild" || list.Items[0].ID != "1" || list.Items[1].Kind != "role" || list.Items[1].ID != "2" {
		t.Fatalf("list: %+v", list)
	}
}

func TestSeqRestartContinues(t *testing.T) {
	seq := &memSeq{m: map[string]uint64{}}
	UseSeq(seq)
	ctx := start(t)
	if view := alloc(t, ctx, "room"); view.Err != "" || view.ID != "1" {
		t.Fatalf("first: %+v", view)
	}
	gonet.Stop()
	ctx = start(t)
	if view := alloc(t, ctx, "room"); view.Err != "" || view.ID != "2" {
		t.Fatalf("restart: %+v", view)
	}
	if got := seq.snapshot()["room"]; got != 2 {
		t.Fatalf("saved %d", got)
	}
}

func TestQueuedAllocsDoNotShare(t *testing.T) {
	seq := &blockSeq{memSeq: memSeq{m: map[string]uint64{}}, entered: make(chan struct{}), release: make(chan struct{})}
	UseSeq(seq)
	ctx := start(t)
	first := make(chan *Reply, 1)
	second := make(chan *Reply, 1)
	go func() { first <- alloc(t, ctx, "room") }()
	select {
	case <-seq.entered:
	case <-ctx.Done():
		t.Fatal("save did not start")
	}
	go func() { second <- alloc(t, ctx, "room") }()
	time.Sleep(30 * time.Millisecond)
	close(seq.release)
	a := <-first
	b := <-second
	if a.Err != "" || b.Err != "" || a.ID == b.ID || (a.ID != "1" && a.ID != "2") || (b.ID != "1" && b.ID != "2") {
		t.Fatalf("queued: %+v %+v", a, b)
	}
}

func TestSeqExhausted(t *testing.T) {
	UseSeq(&memSeq{m: map[string]uint64{"role": math.MaxUint64}})
	ctx := start(t)
	if view := alloc(t, ctx, "role"); view.Err != "编号已用尽" {
		t.Fatalf("exhausted: %+v", view)
	}
	if view := alloc(t, ctx, "guild"); view.Err != "" || view.ID != "1" {
		t.Fatalf("other kind: %+v", view)
	}
}

func start(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(func() {
		cancel()
		gonet.Stop()
		UseSeq(nil)
	})
	pid, err := gonet.SpawnNamed(New(), Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := gonet.WaitInit(ctx, pid); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func alloc(t *testing.T, ctx context.Context, kind string) *Reply {
	t.Helper()
	msg := &AllocMsg{Kind: kind}
	msg.SetCmd(CmdAlloc)
	return callReply(t, ctx, msg)
}

func query(t *testing.T, ctx context.Context, kind string) *Reply {
	t.Helper()
	msg := &QueryMsg{Kind: kind}
	msg.SetCmd(CmdQuery)
	return callReply(t, ctx, msg)
}

func callReply(t *testing.T, ctx context.Context, msg gonet.MessageInterface) *Reply {
	t.Helper()
	v, err := gonet.SuspendCallName(ctx, Name, msg)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := v.(*Reply)
	if view == nil {
		t.Fatalf("reply %T", v)
	}
	return view
}

func callList(t *testing.T, ctx context.Context) *ListReply {
	t.Helper()
	msg := &ListMsg{}
	msg.SetCmd(CmdList)
	v, err := gonet.SuspendCallName(ctx, Name, msg)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := v.(*ListReply)
	if view == nil {
		t.Fatalf("list %T", v)
	}
	return view
}

type blockSeq struct {
	memSeq
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockSeq) Save(ctx context.Context, counters map[string]uint64) error {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.memSeq.Save(ctx, counters)
}

type memSeq struct {
	mu sync.Mutex
	m  map[string]uint64
}

func (s *memSeq) Load(context.Context) (map[string]uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]uint64, len(s.m))
	for k, n := range s.m {
		out[k] = n
	}
	return out, nil
}

func (s *memSeq) Save(_ context.Context, counters map[string]uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = make(map[string]uint64, len(counters))
	for k, n := range counters {
		s.m[k] = n
	}
	return nil
}

func (s *memSeq) snapshot() map[string]uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]uint64, len(s.m))
	for k, n := range s.m {
		out[k] = n
	}
	return out
}
