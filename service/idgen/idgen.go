package idgen

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"
	"unicode"

	"gonet"
)

// Name 是全服编号服务的别名。全服只有一个，不加 nodeid。只在 center 上启动。
const Name = ".idgen"

const saveTimeout = time.Second

// Seq 记住每种编号已经发出的最大值。重启后先读它，新号只能领下一个。
// Load 在没有记录时返回空表。Save 要在把这个号回复给调用方之前完成。
type Seq interface {
	Load(ctx context.Context) (map[string]uint64, error)
	Save(ctx context.Context, counters map[string]uint64) error
}

var (
	seqMu sync.RWMutex
	seq   Seq
)

// UseSeq 在启动之前装上编号记录。不装时编号只留在内存里，重启从 0 再计。
func UseSeq(s Seq) {
	seqMu.Lock()
	seq = s
	seqMu.Unlock()
}

func currentSeq() Seq {
	seqMu.RLock()
	defer seqMu.RUnlock()
	return seq
}

type allocJob struct {
	e    gonet.Envelope
	kind string
	n    uint64
	snap map[string]uint64
}

// Actor 按种类各自发号。种类名由调用方传入，这里不写死有哪些业务。
type Actor struct {
	gonet.ActorContext
	cur   map[string]uint64
	seq   Seq
	queue []allocJob
	busy  bool
	stop  chan struct{}
}

func New() *Actor {
	return &Actor{
		cur:  make(map[string]uint64),
		stop: make(chan struct{}),
	}
}

func (a *Actor) Init() error {
	a.seq = currentSeq()
	if a.seq != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		loaded, err := a.seq.Load(ctx)
		cancel()
		if err != nil {
			return err
		}
		for kind, n := range loaded {
			if validKind(kind) && n > 0 {
				a.cur[kind] = n
			}
		}
	}
	slog.Info("service started", "service", "idgen", "pid", a.Self(), "name", a.SelfName(), "kinds", len(a.cur))
	return a.RegisterCmds()
}

func (a *Actor) Term() {
	close(a.stop)
	slog.Info("service stopped", "service", "idgen", "pid", a.Self(), "name", a.SelfName())
}

func (a *Actor) RegisterCmds() error {
	if err := gonet.RegisterCmd(a, CmdAlloc, func() gonet.MessageInterface { return &AllocMsg{} }, a.onAlloc); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdQuery, func() gonet.MessageInterface { return &QueryMsg{} }, a.onQuery); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, CmdList, func() gonet.MessageInterface { return &ListMsg{} }, a.onList); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, cmdSaved, func() gonet.MessageInterface { return &savedMsg{} }, a.onSaved)
}

func (a *Actor) onAlloc(e gonet.Envelope) {
	m, _ := e.Msg.(*AllocMsg)
	if m == nil || !validKind(m.Kind) {
		e.Reply(&Reply{Err: "种类无效"})
		return
	}
	cur := a.high(m.Kind)
	if cur == math.MaxUint64 {
		e.Reply(&Reply{Kind: m.Kind, Err: "编号已用尽"})
		return
	}
	n := cur + 1
	if a.seq == nil {
		a.cur[m.Kind] = n
		e.Reply(&Reply{Kind: m.Kind, ID: formatID(n)})
		return
	}
	snap := a.snapshot()
	snap[m.Kind] = n
	a.queue = append(a.queue, allocJob{e: e, kind: m.Kind, n: n, snap: snap})
	a.pump()
}

func (a *Actor) onQuery(e gonet.Envelope) {
	m, _ := e.Msg.(*QueryMsg)
	if m == nil || !validKind(m.Kind) {
		e.Reply(&Reply{Err: "种类无效"})
		return
	}
	e.Reply(&Reply{Kind: m.Kind, ID: formatID(a.cur[m.Kind])})
}

func (a *Actor) onList(e gonet.Envelope) {
	kinds := make([]string, 0, len(a.cur))
	for kind := range a.cur {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	items := make([]Item, 0, len(kinds))
	for _, kind := range kinds {
		items = append(items, Item{Kind: kind, ID: formatID(a.cur[kind])})
	}
	e.Reply(&ListReply{Items: items})
}

func (a *Actor) pump() {
	if a.busy || len(a.queue) == 0 || a.seq == nil {
		return
	}
	job := a.queue[0]
	a.busy = true
	go a.persist(a.Self(), a.seq, job.snap, job.n, a.stop)
}

func (a *Actor) persist(pid uint64, seq Seq, snap map[string]uint64, n uint64, stop <-chan struct{}) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		err := seq.Save(ctx, snap)
		cancel()
		if err == nil {
			msg := &savedMsg{N: n}
			msg.SetCmd(cmdSaved)
			if err := gonet.SendMemory(pid, msg); err != nil {
				select {
				case <-stop:
					return
				case <-time.After(200 * time.Millisecond):
				}
				continue
			}
			return
		}
		slog.Warn("idgen save", "err", err)
		select {
		case <-stop:
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (a *Actor) onSaved(e gonet.Envelope) {
	m, _ := e.Msg.(*savedMsg)
	if m == nil || !a.busy || len(a.queue) == 0 || m.N != a.queue[0].n {
		return
	}
	job := a.queue[0]
	a.queue = a.queue[1:]
	a.busy = false
	a.cur[job.kind] = job.n
	job.e.Reply(&Reply{Kind: job.kind, ID: formatID(job.n)})
	a.pump()
}

// high 含已经排队、尚未写入的号，避免两笔分配算出同一个号。
func (a *Actor) high(kind string) uint64 {
	n := a.cur[kind]
	for _, job := range a.queue {
		if job.kind == kind && job.n > n {
			n = job.n
		}
	}
	return n
}

func (a *Actor) snapshot() map[string]uint64 {
	out := make(map[string]uint64, len(a.cur)+len(a.queue)+1)
	for kind, n := range a.cur {
		out[kind] = n
	}
	for _, job := range a.queue {
		if job.n > out[job.kind] {
			out[job.kind] = job.n
		}
	}
	return out
}

func formatID(n uint64) string {
	return strconv.FormatUint(n, 10)
}

// validKind 限制种类名，避免空名和写进存储键时失控。
func validKind(kind string) bool {
	if kind == "" || len(kind) > 32 {
		return false
	}
	for _, r := range kind {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
