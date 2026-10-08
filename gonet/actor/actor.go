package actor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	statusStarting int32 = 0
	statusRunning  int32 = 1
	statusStopping int32 = 2
	statusDead     int32 = 3

	// SlowDispatchThreshold 超过该耗时记一次 slow_dispatch。
	SlowDispatchThreshold = 100 * time.Millisecond

	cmdInit = ".init"
)

// actor 是内核里一个 Actor 的运行记录。
type actor struct {
	pid          uint64
	alias        string // 仅在 registry 锁内读写
	impl         ActorContextInterface
	mailbox      chan Envelope
	quit         chan struct{}
	done         chan struct{}
	status       atomic.Int32
	stopOnce     sync.Once
	lastCmd      atomic.Value // string
	lastDispatch atomic.Int64
	pendingMu    sync.Mutex
	pending      map[uint64]uint64 // session → timer id
	initWait     chan struct{}     // Init 结束后 close
	initErr      error             // Init 结果；仅在 initWait 关闭后读
	initOnce     sync.Once
	loopID       atomic.Uint64 // mailbox 协程的 goid；SuspendCall 自己用

	sendFull     atomic.Uint64
	sendDead     atomic.Uint64
	callTimeouts atomic.Uint64
	mailboxHigh  atomic.Uint64
	slowDispatch atomic.Uint64
	replyFail    atomic.Uint64
}

// Envelope 是本进程 mailbox 里的一条消息，不是网上的包。
// FromName 是发送方别名。本进程 Reply 优先用 From；From 为 0 时按别名送回，别名不在本进程就交给 harbor。
// memory 为真时 Msg 是原指针（SendMemory），Dispatch 不解包。
type Envelope struct {
	Self     uint64
	From     uint64
	FromName string
	Msg      MessageInterface
	memory   bool
	reply    chan any
	session  uint64
}

type initMsg struct {
	BaseMessage
}

func (m *initMsg) Command() string { return cmdInit }

func isInitMsg(msg MessageInterface) bool {
	_, ok := msg.(*initMsg)
	return ok
}

// Reply 回复当前 Call；Send 触发的消息上调用则忽略。
// 本进程 SuspendCall 走 reply；异步 Call 优先按 From pid，否则按 FromName。
func (e Envelope) Reply(v any) {
	if e.reply != nil {
		select {
		case e.reply <- v:
		default:
		}
		return
	}
	if e.session == 0 {
		return
	}
	if e.From != 0 {
		deliverCallResponse(e.From, e.session, v, nil)
		return
	}
	deliverCallResponseName(e.FromName, e.session, v, nil)
}

func (a *actor) runInit() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("gonet: Init panic: %v", r)
		}
	}()
	if a.impl == nil {
		return ErrNilActor
	}
	return a.impl.Init()
}

func (a *actor) signalInit(err error) {
	a.initOnce.Do(func() {
		a.initErr = err
		close(a.initWait)
	})
}

func (a *actor) handleInit(e Envelope) {
	if a.status.Load() != statusStarting {
		failEnvelopeCall(e, ErrNotReady)
		return
	}
	err := a.runInit()
	if err != nil {
		a.signalInit(err)
		failEnvelopeCall(e, err)
		a.abort()
		return
	}
	select {
	case <-a.quit:
		a.signalInit(ErrDead)
		failEnvelopeCall(e, ErrDead)
		a.abort()
		return
	default:
	}
	a.status.Store(statusRunning)
	a.signalInit(nil)
	if e.isCall() {
		e.Reply(a.pid)
	}
}

func (a *actor) abort() {
	if a.status.Load() == statusDead {
		return
	}
	starting := a.status.Load() == statusStarting
	a.status.Store(statusDead)
	a.requestStop()
	a.rejectMailboxCalls()
	if starting {
		a.signalInit(ErrDead)
	}
	if a.impl != nil {
		func() {
			defer func() { recover() }()
			a.impl.Term()
		}()
	}
	actorRegistry.remove(a.pid)
	close(a.done)
}

func (a *actor) requestStop() {
	a.stopOnce.Do(func() {
		close(a.quit)
	})
}

func (a *actor) term() {
	if a.status.Load() == statusDead {
		return
	}
	starting := a.status.Load() == statusStarting
	a.status.Store(statusStopping)
	a.requestStop()
	a.rejectMailboxCalls()
	if starting {
		a.signalInit(ErrDead)
	}
	if a.impl != nil {
		func() {
			defer func() { recover() }()
			a.impl.Term()
		}()
	}
	a.status.Store(statusDead)
	actorRegistry.remove(a.pid)
	close(a.done)
}

func (a *actor) recv() (e Envelope, ok bool) {
	select {
	case e = <-a.mailbox:
		return e, true
	case <-a.quit:
		a.term()
		return Envelope{}, false
	}
}

func (a *actor) dispatch(e Envelope) {
	if a.impl != nil {
		a.impl.Dispatch(e)
	}
}

func (a *actor) handle(e Envelope) {
	e.Self = a.pid
	if a.status.Load() == statusStarting {
		if isInitMsg(e.Msg) {
			a.handleInit(e)
			return
		}
		failEnvelopeCall(e, ErrNotReady)
		return
	}
	if !a.applyCallTimeout(&e) {
		return
	}
	cmd := messageCmd(e.Msg)
	start := time.Now()
	defer func() {
		d := time.Since(start)
		a.lastDispatch.Store(int64(d))
		if cmd != "" {
			a.lastCmd.Store(cmd)
		}
		if d >= SlowDispatchThreshold {
			a.slowDispatch.Add(1)
			noteSlowDispatch()
		}
		if r := recover(); r != nil {
			slog.Error("gonet: dispatch panic", "pid", a.pid, "msg", e.Msg, "err", r)
			failEnvelopeCall(e, ErrDispatchPanic)
		}
	}()
	a.dispatch(e)
}

func messageCmd(msg MessageInterface) string {
	if msg == nil {
		return ""
	}
	if c, ok := msg.(interface{ Command() string }); ok {
		return c.Command()
	}
	return ""
}

func (e Envelope) isCall() bool {
	return e.reply != nil || e.session != 0
}

func (a *actor) noteMailboxLen() {
	n := uint64(len(a.mailbox))
	for {
		old := a.mailboxHigh.Load()
		if n <= old {
			return
		}
		if a.mailboxHigh.CompareAndSwap(old, n) {
			return
		}
	}
}

func (a *actor) post(env Envelope) error {
	if a == nil {
		noteSendDead(nil)
		return ErrDead
	}
	st := a.status.Load()
	if st == statusDead {
		noteSendDead(a)
		return ErrDead
	}
	if st != statusRunning && !isInitMsg(env.Msg) {
		return ErrNotReady
	}
	select {
	case <-a.quit:
		// 已请求停机但 Term 未完成：还在表里，不是 ErrDead。
		if a.status.Load() == statusDead {
			noteSendDead(a)
			return ErrDead
		}
		return ErrNotReady
	default:
	}
	select {
	case a.mailbox <- env:
		a.noteMailboxLen()
		return nil
	default:
		select {
		case <-a.quit:
			if a.status.Load() == statusDead {
				noteSendDead(a)
				return ErrDead
			}
			return ErrNotReady
		default:
			noteSendFull(a)
			return ErrMailboxFull
		}
	}
}

func (a *actor) run() {
	a.loopID.Store(goid())
	defer a.loopID.Store(0)
	defer bindLogPID(a.pid)()
	for {
		e, ok := a.recv()
		if !ok {
			return
		}
		a.handle(e)
	}
}

func goid() uint64 {
	var buf [32]byte
	n := runtime.Stack(buf[:], false)
	const p = "goroutine "
	if n < len(p)+1 {
		return 0
	}
	var id uint64
	for i := len(p); i < n; i++ {
		c := buf[i]
		if c < '0' || c > '9' {
			return id
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}

func (a *actor) pendingCount() int {
	a.pendingMu.Lock()
	n := len(a.pending)
	a.pendingMu.Unlock()
	return n
}

var (
	ErrDispatchPanic = errors.New("gonet: Dispatch panic")
	ErrStopTimeout   = errors.New("gonet: 停机超时")
	ErrSystemCmd     = errors.New("gonet: 系统命令不能 Pack/从外部注入")
	ErrNotReady      = errors.New("gonet: actor 尚未 Init 完成")
)

// WaitInit 等到 pid 的 Init 完成。成功返回 nil；Init 失败返回该错误。
func WaitInit(ctx context.Context, pid uint64) error {
	if ctx == nil {
		return ErrNilContext
	}
	a := actorRegistry.get(pid)
	if a == nil {
		return ErrDead
	}
	select {
	case <-a.initWait:
		return a.initErr
	case <-ctx.Done():
		return ctx.Err()
	case <-a.done:
		select {
		case <-a.initWait:
			return a.initErr
		default:
			return ErrDead
		}
	}
}
