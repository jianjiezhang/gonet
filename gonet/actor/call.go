package actor

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// CmdResponse 是异步 Call 的回复命令。业务用 RegisterCmd 接收。
	CmdResponse    = ".response"
	cmdCallTimeout = ".call.timeout"
)

var (
	ErrNoCaller     = errors.New("gonet: 异步 Call 必须有调用方 actor")
	ErrRemoteCaller = errors.New("gonet: 跨服 Call 的调用方要有别名")
	ErrCallTimeout  = errors.New("gonet: Call 超时")
	nextCallSession atomic.Uint64
)

func init() {
	_ = RegisterMsg(CmdResponse, func() MessageInterface { return &CallResponse{} })
}

// CallResponse 投进调用方 mailbox。Value 是对方 Reply 的值；Err 是超时或对方已死。
type CallResponse struct {
	BaseMessage
	Session uint64
	Value   any
	Err     error
}

func (m *CallResponse) Command() string {
	if m == nil {
		return CmdResponse
	}
	if m.Cmd != "" {
		return m.Cmd
	}
	return CmdResponse
}

type callTimeoutMsg struct {
	BaseMessage
	Session uint64
}

func (m *callTimeoutMsg) Command() string { return cmdCallTimeout }

// callSet 记下还没结束的异步 Call。session 映射到 timer id，0 表示没有超时。
type callSet struct {
	mu      sync.Mutex
	pending map[uint64]uint64
}

func (s *callSet) track(session, timerID uint64) {
	s.mu.Lock()
	if s.pending == nil {
		s.pending = make(map[uint64]uint64)
	}
	s.pending[session] = timerID
	s.mu.Unlock()
}

func (s *callSet) take(session uint64) bool {
	tid, ok := s.detach(session)
	if ok {
		stopScheduled(tid)
	}
	return ok
}

func (s *callSet) detach(session uint64) (tid uint64, ok bool) {
	s.mu.Lock()
	tid, ok = s.pending[session]
	if ok {
		delete(s.pending, session)
	}
	s.mu.Unlock()
	return tid, ok
}

func (s *callSet) count() int {
	s.mu.Lock()
	n := len(s.pending)
	s.mu.Unlock()
	return n
}

// bindTimer 给已经 track 的 session 补上 timer。session 已结束时返回 false。
func (s *callSet) bindTimer(session, timerID uint64) bool {
	s.mu.Lock()
	_, ok := s.pending[session]
	if ok {
		s.pending[session] = timerID
	}
	s.mu.Unlock()
	return ok
}

func (a *actor) applyCallTimeout(e *Envelope) bool {
	t, ok := e.Msg.(*callTimeoutMsg)
	if !ok {
		return true
	}
	if !a.calls.take(t.Session) {
		return false
	}
	noteCallTimeout(a)
	cr := &CallResponse{Session: t.Session, Err: ErrCallTimeout}
	cr.SetCmd(CmdResponse)
	e.Msg = cr
	return true
}

// callFail 仅内核用于 SuspendCall：把错误从 reply 通道传回。
type callFail struct{ err error }

func failEnvelopeCall(e Envelope, err error) {
	if e.reply != nil {
		select {
		case e.reply <- callFail{err: err}:
		default:
		}
		return
	}
	replyCallFailure(e, err)
}

func (a *actor) rejectMailboxCalls() {
	for {
		select {
		case e := <-a.mailbox:
			failAsyncCall(e, ErrDead)
		default:
			return
		}
	}
}

func failAsyncCall(e Envelope, err error) {
	replyCallFailure(e, err)
}

func replyCallFailure(e Envelope, err error) {
	if e.session == 0 {
		return
	}
	if e.From != 0 {
		deliverCallResponse(e.From, e.session, nil, err)
		return
	}
	deliverCallResponseName(e.FromName, e.session, nil, err)
}

func deliverCallResponse(to, session uint64, v any, err error) {
	a := actorRegistry.get(to)
	if a == nil {
		return
	}
	tid, ok := a.calls.detach(session)
	if !ok {
		return
	}
	stopScheduled(tid)
	msg := &CallResponse{Session: session, Value: v, Err: err}
	msg.SetCmd(CmdResponse)
	if postErr := a.post(Envelope{Msg: msg, memory: true}); postErr != nil {
		if !errors.Is(postErr, ErrDead) {
			noteReplyFail(a)
			slog.Error("gonet: Call 回复投递失败", "to", to, "session", session, "err", postErr)
		}
	}
}

// Call 把请求放入 to 的 mailbox，立刻返回 session。error 只表示这次投递：
// nil 已入队；ErrDead / ErrMailboxFull / ErrNoCaller 等表示没发出去，不会再有 CallResponse。
// d>0 排超时；d<=0 只等 Reply，不发 ErrCallTimeout。
// 入队成功后的超时或对方死亡，走调用方 mailbox 里的 CallResponse.Err。
func Call(from, to uint64, d time.Duration, msg MessageInterface) (uint64, error) {
	return callFromAsync(from, to, d, msg, false)
}

// CallMemory 异步 Call，消息保持原指针，不序列化。
func CallMemory(from, to uint64, d time.Duration, msg MessageInterface) (uint64, error) {
	return callFromAsync(from, to, d, msg, true)
}

// CallName 按别名异步 Call。名不存在返回 ErrUnknownAlias，此时不会有 CallResponse。
// 跨服会等到对方 mailbox 收下才返回；收不了则取消 session 并返回错误。对方 Reply 之后才有 CallResponse。
func CallName(from uint64, name string, d time.Duration, msg MessageInterface) (uint64, error) {
	return callNameFromAsync(from, name, d, msg, false)
}

// CallMemoryName 按别名做 CallMemory。
func CallMemoryName(from uint64, name string, d time.Duration, msg MessageInterface) (uint64, error) {
	return callNameFromAsync(from, name, d, msg, true)
}

func callNameFromAsync(from uint64, name string, d time.Duration, msg MessageInterface, memory bool) (uint64, error) {
	if name == "" {
		return 0, ErrInvalidAlias
	}
	if a := actorRegistry.getByAlias(name); a != nil {
		return callOnAsync(from, a, d, msg, memory)
	}
	if memory {
		if !callReady() {
			return 0, ErrUnknownAlias
		}
		return 0, ErrRemoteMemory
	}
	if !callReady() {
		return 0, ErrUnknownAlias
	}
	if from == 0 {
		return 0, ErrNoCaller
	}
	if msg == nil {
		return 0, ErrNilMessage
	}
	caller := actorRegistry.get(from)
	if caller == nil {
		return 0, ErrDead
	}
	fromName := aliasOf(from)
	if fromName == "" {
		return 0, ErrRemoteCaller
	}
	out, err := prepareOutgoing(msg, false)
	if err != nil {
		return 0, err
	}
	packed, err := asBase(out)
	if err != nil {
		return 0, err
	}
	session, err := trackCaller(caller, from, 0)
	if err != nil {
		return 0, err
	}
	if err := forwardCallName(fromName, name, packed, session); err != nil {
		caller.calls.take(session)
		return 0, err
	}
	if err := armCallTimeout(caller, from, session, d); err != nil {
		caller.calls.take(session)
		return 0, err
	}
	return session, nil
}

func asBase(msg MessageInterface) (*BaseMessage, error) {
	switch m := msg.(type) {
	case *BaseMessage:
		return m, nil
	case BaseMessage:
		return &m, nil
	default:
		return nil, ErrUnknownAlias
	}
}

func callFromAsync(from, to uint64, d time.Duration, msg MessageInterface, memory bool) (uint64, error) {
	a := actorRegistry.get(to)
	if a == nil {
		if msg == nil {
			return 0, ErrNilMessage
		}
		if from == 0 {
			return 0, ErrNoCaller
		}
		return 0, ErrDead
	}
	return callOnAsync(from, a, d, msg, memory)
}

func callOnAsync(from uint64, dest *actor, d time.Duration, msg MessageInterface, memory bool) (uint64, error) {
	if from == 0 {
		return 0, ErrNoCaller
	}
	if msg == nil {
		return 0, ErrNilMessage
	}
	caller := actorRegistry.get(from)
	if caller == nil {
		return 0, ErrDead
	}
	out, err := prepareOutgoing(msg, memory)
	if err != nil {
		return 0, err
	}
	session, err := trackCaller(caller, from, d)
	if err != nil {
		return 0, err
	}
	env := Envelope{From: from, FromName: aliasOf(from), Msg: out, memory: memory, session: session}
	if err := dest.post(env); err != nil {
		caller.calls.take(session)
		return 0, err
	}
	return session, nil
}

func trackCaller(caller *actor, from uint64, d time.Duration) (uint64, error) {
	session := nextCallSession.Add(1)
	var tid uint64
	if d > 0 {
		timeoutMsg := &callTimeoutMsg{Session: session}
		timeoutMsg.SetCmd(cmdCallTimeout)
		var err error
		tid, err = scheduleTimeout(from, d, timeoutMsg)
		if err != nil {
			return 0, err
		}
	}
	caller.calls.track(session, tid)
	return session, nil
}

func armCallTimeout(caller *actor, from, session uint64, d time.Duration) error {
	if d <= 0 || caller == nil {
		return nil
	}
	timeoutMsg := &callTimeoutMsg{Session: session}
	timeoutMsg.SetCmd(cmdCallTimeout)
	tid, err := scheduleTimeout(from, d, timeoutMsg)
	if err != nil {
		return err
	}
	if !caller.calls.bindTimer(session, tid) {
		stopScheduled(tid)
	}
	return nil
}

// AllocSession 分配进程内唯一的 Call session。
func AllocSession() uint64 {
	return nextCallSession.Add(1)
}

// TrackSession 让 pid 接收这次 session 的 Reply，不排超时。
func TrackSession(pid, session uint64) error {
	a := actorRegistry.get(pid)
	if a == nil {
		return ErrDead
	}
	a.calls.track(session, 0)
	return nil
}

// UntrackSession 取消 TrackSession。
func UntrackSession(pid, session uint64) {
	if a := actorRegistry.get(pid); a != nil {
		a.calls.take(session)
	}
}

// FinishCall 把跨服回复送进调用方 mailbox。session 已结束时不再投递。
// 阻塞 Call 在等这条 session 时，直接唤醒等待的协程。
func FinishCall(pid, session uint64, v any, err error) {
	if session == 0 {
		return
	}
	if finishSuspend(session, v, err) {
		return
	}
	deliverCallResponse(pid, session, v, err)
}

// ResolveRemote 把阻塞 Call 的结果写回 SuspendCall 的通道。
func ResolveRemote(ch chan any, v any, err error) {
	if ch == nil {
		return
	}
	var out any = v
	if err != nil {
		out = callFail{err: err}
	}
	select {
	case ch <- out:
	default:
	}
}

// Post 把消息投进本进程 actor。session 非 0 时对方 Reply 回到 from。
func Post(from, to uint64, fromName string, msg MessageInterface, session uint64, memory bool) error {
	if msg == nil {
		return ErrNilMessage
	}
	a := actorRegistry.get(to)
	if a == nil {
		noteSendDead(nil)
		return ErrDead
	}
	out, err := prepareOutgoing(msg, memory)
	if err != nil {
		return err
	}
	return a.post(Envelope{From: from, FromName: fromName, Msg: out, memory: memory, session: session})
}

func deliverCallResponseName(name string, session uint64, v any, err error) {
	if name == "" || session == 0 {
		return
	}
	pid, qerr := actorRegistry.query(name)
	if qerr == nil {
		deliverCallResponse(pid, session, v, err)
		return
	}
	if replyReady() {
		replyRemote(name, session, v, err)
		return
	}
	noteReplyFail(nil)
	slog.Error("gonet: Call 回复找不到调用方", "name", name, "session", session, "err", qerr)
}

// SuspendCall 阻塞当前协程直到 Reply、超时或对方死亡。
// 只给邮箱外面的协程用。邮箱协程里用 Call，对自己也一样。
func SuspendCall(ctx context.Context, pid uint64, msg MessageInterface) (any, error) {
	return suspendFrom(ctx, 0, pid, msg, false)
}

// SuspendCallMemory 阻塞 Call，消息保持原指针。不要在 Dispatch 里用。
func SuspendCallMemory(ctx context.Context, pid uint64, msg MessageInterface) (any, error) {
	return suspendFrom(ctx, 0, pid, msg, true)
}

// SuspendCallName 按别名的阻塞 Call。不要在 Dispatch 里用。
func SuspendCallName(ctx context.Context, name string, msg MessageInterface) (any, error) {
	return suspendNameFrom(ctx, 0, name, msg, false)
}

// SuspendCallMemoryName 按别名做 SuspendCallMemory。
func SuspendCallMemoryName(ctx context.Context, name string, msg MessageInterface) (any, error) {
	return suspendNameFrom(ctx, 0, name, msg, true)
}

func suspendNameFrom(ctx context.Context, from uint64, name string, msg MessageInterface, memory bool) (any, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if msg == nil {
		return nil, ErrNilMessage
	}
	if name == "" {
		return nil, ErrInvalidAlias
	}
	if a := actorRegistry.getByAlias(name); a != nil {
		return suspendOn(ctx, from, a, msg, memory)
	}
	if memory {
		if !callReady() {
			return nil, ErrUnknownAlias
		}
		return nil, ErrRemoteMemory
	}
	if !callReady() {
		return nil, ErrUnknownAlias
	}
	out, err := prepareOutgoing(msg, false)
	if err != nil {
		return nil, err
	}
	packed, err := asBase(out)
	if err != nil {
		return nil, err
	}
	return suspendRemote(ctx, from, name, packed)
}

type suspendResult struct {
	v   any
	err error
}

var remoteSuspend sync.Map

func suspendRemote(ctx context.Context, from uint64, name string, msg *BaseMessage) (any, error) {
	fromName := aliasOf(from)
	if fromName == "" {
		return nil, ErrRemoteCaller
	}
	session := nextCallSession.Add(1)
	ch := make(chan suspendResult, 1)
	remoteSuspend.Store(session, ch)
	if err := forwardCallName(fromName, name, msg, session); err != nil {
		remoteSuspend.Delete(session)
		return nil, err
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		return r.v, nil
	case <-ctx.Done():
		select {
		case r := <-ch:
			if r.err != nil {
				return nil, r.err
			}
			return r.v, nil
		default:
			remoteSuspend.Delete(session)
			return nil, ctx.Err()
		}
	}
}

func finishSuspend(session uint64, v any, err error) bool {
	ch, ok := remoteSuspend.LoadAndDelete(session)
	if !ok {
		return false
	}
	select {
	case ch.(chan suspendResult) <- suspendResult{v: v, err: err}:
	default:
	}
	return true
}

func suspendFrom(ctx context.Context, from, to uint64, msg MessageInterface, memory bool) (any, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if msg == nil {
		return nil, ErrNilMessage
	}
	a := actorRegistry.get(to)
	if a == nil {
		return nil, ErrDead
	}
	return suspendOn(ctx, from, a, msg, memory)
}

func suspendOn(ctx context.Context, from uint64, a *actor, msg MessageInterface, memory bool) (any, error) {
	out, err := prepareOutgoing(msg, memory)
	if err != nil {
		return nil, err
	}
	reply := make(chan any, 1)
	env := Envelope{From: from, FromName: aliasOf(from), Msg: out, memory: memory, reply: reply}
	if err := a.post(env); err != nil {
		return nil, err
	}
	select {
	case v := <-reply:
		if f, ok := v.(callFail); ok {
			return nil, f.err
		}
		return v, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.life.done:
		select {
		case v := <-reply:
			if f, ok := v.(callFail); ok {
				return nil, f.err
			}
			return v, nil
		default:
			return nil, ErrDead
		}
	}
}

func (e Envelope) Call(to uint64, d time.Duration, msg MessageInterface) (uint64, error) {
	return Call(e.Self, to, d, msg)
}

func (e Envelope) CallName(name string, d time.Duration, msg MessageInterface) (uint64, error) {
	return CallName(e.Self, name, d, msg)
}

func (e Envelope) CallMemory(to uint64, d time.Duration, msg MessageInterface) (uint64, error) {
	return CallMemory(e.Self, to, d, msg)
}

func (e Envelope) CallMemoryName(name string, d time.Duration, msg MessageInterface) (uint64, error) {
	return CallMemoryName(e.Self, name, d, msg)
}

func (c *ActorContext) Call(to uint64, d time.Duration, msg MessageInterface) (uint64, error) {
	if c == nil {
		return 0, ErrNilActor
	}
	return Call(c.Self(), to, d, msg)
}

func (c *ActorContext) CallName(name string, d time.Duration, msg MessageInterface) (uint64, error) {
	if c == nil {
		return 0, ErrNilActor
	}
	return CallName(c.Self(), name, d, msg)
}

func (c *ActorContext) CallMemory(to uint64, d time.Duration, msg MessageInterface) (uint64, error) {
	if c == nil {
		return 0, ErrNilActor
	}
	return CallMemory(c.Self(), to, d, msg)
}

func (c *ActorContext) CallMemoryName(name string, d time.Duration, msg MessageInterface) (uint64, error) {
	if c == nil {
		return 0, ErrNilActor
	}
	return CallMemoryName(c.Self(), name, d, msg)
}
