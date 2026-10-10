package actor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	//身份+主要结构
	pid     uint64
	alias   string // 仅在 registry 锁内读写
	impl    ActorContextInterface
	mailbox chan Envelope

	life  lifecycle   // 生命周期管理
	calls callSet     // 未完成的Call mu + session 表
	obs   observation // 监控相关+性能指标
}

// lifecycle 管停机信号、状态和 Init 完成。
type lifecycle struct {
	quit     chan struct{}
	done     chan struct{}
	status   atomic.Int32
	stopOnce sync.Once
	initWait chan struct{} // Init 结束后 close，等待一个actor执行第一条消息：initmsg
	initErr  error         // Init 结果；仅在 initWait 关闭后读
	initOnce sync.Once
}

func newLifecycle() lifecycle {
	l := lifecycle{
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
		initWait: make(chan struct{}),
	}
	l.status.Store(statusStarting)
	return l
}

func (l *lifecycle) signalInit(err error) {
	l.initOnce.Do(func() {
		l.initErr = err
		close(l.initWait)
	})
}

func (l *lifecycle) requestStop() {
	l.stopOnce.Do(func() {
		close(l.quit)
	})
}

// observation 是单个 actor 的监控计数。
type observation struct {
	lastCmd      atomic.Value // string
	lastDispatch atomic.Int64
	sendFull     atomic.Uint64
	sendDead     atomic.Uint64
	callTimeouts atomic.Uint64
	mailboxHigh  atomic.Uint64
	slowDispatch atomic.Uint64
	replyFail    atomic.Uint64
}

func (o *observation) noteDispatch(cmd string, d time.Duration) {
	o.lastDispatch.Store(int64(d))
	if cmd != "" {
		o.lastCmd.Store(cmd)
	}
	if d >= SlowDispatchThreshold {
		o.slowDispatch.Add(1)
		noteSlowDispatch()
	}
}

func (o *observation) noteMailbox(n uint64) {
	for {
		old := o.mailboxHigh.Load()
		if n <= old {
			return
		}
		if o.mailboxHigh.CompareAndSwap(old, n) {
			return
		}
	}
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

func (a *actor) handleInit(e Envelope) {
	if a.life.status.Load() != statusStarting {
		failEnvelopeCall(e, ErrNotReady)
		return
	}
	err := a.runInit()
	if err != nil {
		a.life.signalInit(err)
		failEnvelopeCall(e, err)
		a.abort()
		return
	}
	a.life.status.Store(statusRunning)
	a.life.signalInit(nil)
	if e.isCall() {
		e.Reply(a.pid)
	}
}

func (a *actor) abort() {
	if a.life.status.Load() == statusDead {
		return
	}
	starting := a.life.status.Load() == statusStarting
	a.life.status.Store(statusDead)
	a.life.requestStop()
	a.rejectMailboxCalls()
	if starting {
		a.life.signalInit(ErrDead)
	}
	if a.impl != nil {
		func() {
			defer func() { recover() }()
			a.impl.Term()
		}()
	}
	actorRegistry.remove(a.pid)
	close(a.life.done)
}

func (a *actor) term() {
	if a.life.status.Load() == statusDead {
		return
	}
	starting := a.life.status.Load() == statusStarting
	a.life.status.Store(statusStopping)
	a.life.requestStop()
	a.rejectMailboxCalls()
	if starting {
		a.life.signalInit(ErrDead)
	}
	if a.impl != nil {
		func() {
			defer func() { recover() }()
			a.impl.Term()
		}()
	}
	a.life.status.Store(statusDead)
	actorRegistry.remove(a.pid)
	close(a.life.done)
}

func (a *actor) recv() (e Envelope, ok bool) {
	select {
	case <-a.life.quit:
		a.term()
		return Envelope{}, false
	default:
	}
	select {
	case e = <-a.mailbox:
		return e, true
	case <-a.life.quit:
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
	if a.life.status.Load() == statusStarting {
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
		a.obs.noteDispatch(cmd, time.Since(start))
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
	a.obs.noteMailbox(uint64(len(a.mailbox)))
}

func (a *actor) post(env Envelope) error {
	if a == nil {
		noteSendDead(nil)
		return ErrDead
	}
	st := a.life.status.Load()
	if st == statusDead {
		noteSendDead(a)
		return ErrDead
	}
	if st != statusRunning && !isInitMsg(env.Msg) {
		return ErrNotReady
	}
	select {
	case <-a.life.quit:
		// 已请求停机但 Term 未完成：还在表里，不是 ErrDead。
		if a.life.status.Load() == statusDead {
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
		case <-a.life.quit:
			if a.life.status.Load() == statusDead {
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
	defer bindLogPID(a.pid)()
	for {
		e, ok := a.recv()
		if !ok {
			return
		}
		a.handle(e)
	}
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
	case <-a.life.initWait:
		return a.life.initErr
	case <-ctx.Done():
		return ctx.Err()
	case <-a.life.done:
		select {
		case <-a.life.initWait:
			return a.life.initErr
		default:
			return ErrDead
		}
	}
}
