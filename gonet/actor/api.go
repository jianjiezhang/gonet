package actor

import (
	"errors"
	"sync/atomic"
	"time"
)

const defaultMailboxSize = 1024

var (
	ErrNilActor      = errors.New("gonet: ActorContextInterface 不能为 nil")
	ErrDead          = errors.New("gonet: actor 不存在或已结束")
	ErrMailboxFull   = errors.New("gonet: mailbox 已满")
	ErrNilContext    = errors.New("gonet: context 不能为 nil")
	ErrNilMessage    = errors.New("gonet: MessageInterface 不能为 nil")
	ErrInvalidAlias  = errors.New("gonet: 别名不能为空")
	ErrAliasTaken    = errors.New("gonet: 别名已被占用")
	ErrAliasBound    = errors.New("gonet: actor 已有别名")
	ErrUnknownAlias  = errors.New("gonet: 别名不存在")
	ErrRemoteMemory  = errors.New("gonet: 跨服消息不能带进程内指针")
	ErrRemoteDown    = errors.New("gonet: 跨服节点不可达")
	ErrRemoteTimeout = errors.New("gonet: 跨服发送超时")

	sendFull     atomic.Uint64
	sendDead     atomic.Uint64
	callTimeouts atomic.Uint64
	slowDispatch atomic.Uint64
	replyFail    atomic.Uint64
)

// Stats 是进程级投递 / Call 观测计数。
type Stats struct {
	SendFull     uint64
	SendDead     uint64
	CallTimeouts uint64
	SlowDispatch uint64
	ReplyFail    uint64
}

func SnapshotStats() Stats {
	return Stats{
		SendFull:     sendFull.Load(),
		SendDead:     sendDead.Load(),
		CallTimeouts: callTimeouts.Load(),
		SlowDispatch: slowDispatch.Load(),
		ReplyFail:    replyFail.Load(),
	}
}

func SendStats() (full, dead uint64) {
	return sendFull.Load(), sendDead.Load()
}

func noteSendDead(a *actor) {
	sendDead.Add(1)
	if a != nil {
		a.obs.sendDead.Add(1)
	}
}

func noteSendFull(a *actor) {
	sendFull.Add(1)
	if a != nil {
		a.obs.sendFull.Add(1)
	}
}

func noteCallTimeout(a *actor) {
	callTimeouts.Add(1)
	if a != nil {
		a.obs.callTimeouts.Add(1)
	}
}

func noteSlowDispatch() { slowDispatch.Add(1) }

func noteReplyFail(a *actor) {
	replyFail.Add(1)
	if a != nil {
		a.obs.replyFail.Add(1)
	}
}

// IsSystemCmd 内核保留命令（`.` 前缀），不能 Pack，也不能从外部 BaseMessage 注入。
func IsSystemCmd(cmd string) bool {
	return len(cmd) > 0 && cmd[0] == '.'
}

// SpawnOptions 创建参数。Mailbox<=0 时用 1024；Name 非空则占用该别名。
// InitFrom!=0 时，Init 作为对该调用方的异步 Call，结果进其 .response。
// InitTimeout>0 才会超时失败；<=0 表示一直等到 Init 结束，不发超时。
type SpawnOptions struct {
	Name        string
	Mailbox     int
	InitFrom    uint64
	InitTimeout time.Duration
}

func Spawn(impl ActorContextInterface) (uint64, error) {
	pid, _, err := spawn(impl, SpawnOptions{})
	return pid, err
}

func SpawnNamed(impl ActorContextInterface, name string) (uint64, error) {
	if name == "" {
		return 0, ErrInvalidAlias
	}
	pid, _, err := spawn(impl, SpawnOptions{Name: name})
	return pid, err
}

func SpawnWith(impl ActorContextInterface, opt SpawnOptions) (uint64, error) {
	pid, _, err := spawn(impl, opt)
	return pid, err
}

// SpawnAsync 创建 actor 并立刻返回。Init 在 actor 自己的 mailbox 里跑。
// 若 InitFrom!=0，返回 initSession，调用方用 CmdResponse 收 Init 结果。
func SpawnAsync(impl ActorContextInterface, opt SpawnOptions) (pid, initSession uint64, err error) {
	return spawn(impl, opt)
}

func spawn(impl ActorContextInterface, opt SpawnOptions) (uint64, uint64, error) {
	if impl == nil {
		return 0, 0, ErrNilActor
	}
	size := opt.Mailbox
	if size <= 0 {
		size = defaultMailboxSize
	}
	a := &actor{
		impl:    impl,
		mailbox: make(chan Envelope, size),
		life:    newLifecycle(),
	}
	if err := actorRegistry.add(a, opt.Name); err != nil {
		return 0, 0, err
	}
	if err := publishName(opt.Name); err != nil {
		actorRegistry.remove(a.pid)
		close(a.life.done)
		return 0, 0, err
	}
	a.impl.attach(a)

	init := &initMsg{}
	init.SetCmd(cmdInit)
	env := Envelope{Msg: init}
	var initSession uint64
	if opt.InitFrom != 0 {
		caller := actorRegistry.get(opt.InitFrom)
		if caller == nil {
			actorRegistry.remove(a.pid)
			close(a.life.done)
			return 0, 0, ErrDead
		}
		initSession = nextCallSession.Add(1)
		var tid uint64
		if opt.InitTimeout > 0 {
			timeoutMsg := &callTimeoutMsg{Session: initSession}
			timeoutMsg.SetCmd(cmdCallTimeout)
			var err error
			tid, err = scheduleTimeout(opt.InitFrom, opt.InitTimeout, timeoutMsg)
			if err != nil {
				actorRegistry.remove(a.pid)
				close(a.life.done)
				return 0, 0, err
			}
		}
		caller.calls.track(initSession, tid)
		env.From = opt.InitFrom
		env.session = initSession
	}
	// 入表后、开跑前塞进 init，保证是第一条；此时 status=starting，只允许 init。
	if err := a.post(env); err != nil {
		if initSession != 0 {
			if caller := actorRegistry.get(opt.InitFrom); caller != nil {
				caller.calls.take(initSession)
			}
		}
		actorRegistry.remove(a.pid)
		close(a.life.done)
		return 0, 0, err
	}
	go a.run()
	return a.pid, initSession, nil
}

// Rebind 换成新别名，只改本进程表，不向通讯录登记。
func Rebind(pid uint64, name string) error {
	if name == "" {
		return ErrInvalidAlias
	}
	return actorRegistry.rebind(pid, name)
}

func Register(pid uint64, name string) error {
	if name == "" {
		return ErrInvalidAlias
	}
	if err := actorRegistry.bind(pid, name); err != nil {
		return err
	}
	if err := publishName(name); err != nil {
		_ = actorRegistry.unbindName(name)
		_ = unpublishName(name)
		return err
	}
	return nil
}

func Unregister(name string) error {
	if name == "" {
		return ErrInvalidAlias
	}
	if err := actorRegistry.unbindName(name); err != nil {
		return err
	}
	return unpublishName(name)
}

func Query(name string) (uint64, error) {
	if name == "" {
		return 0, ErrInvalidAlias
	}
	return actorRegistry.query(name)
}

func StopActor(pid uint64) error {
	return StopActorWait(pid, 0)
}

// StopActorWait 请求停机并等待 Term。d<=0 表示一直等；超时返回 ErrStopTimeout（仍在停）。
func StopActorWait(pid uint64, d time.Duration) error {
	a := actorRegistry.get(pid)
	if a == nil {
		return ErrDead
	}
	a.life.requestStop()
	if d <= 0 {
		<-a.life.done
		return nil
	}
	select {
	case <-a.life.done:
		return nil
	case <-time.After(d):
		return ErrStopTimeout
	}
}

func Kill(pid uint64) error {
	a := actorRegistry.get(pid)
	if a == nil {
		return ErrDead
	}
	a.life.requestStop()
	return nil
}

func Send(pid uint64, msg MessageInterface) error {
	return sendFrom(0, pid, msg, false)
}

// SendMemory 把原指针放进对方 mailbox，不序列化。只在本进程使用。
func SendMemory(pid uint64, msg MessageInterface) error {
	return sendFrom(0, pid, msg, true)
}

func SendName(name string, msg MessageInterface) error {
	return sendNameFrom(0, name, msg, false)
}

// SendMemoryName 按别名做 SendMemory。
func SendMemoryName(name string, msg MessageInterface) error {
	return sendNameFrom(0, name, msg, true)
}

func sendFrom(from, to uint64, msg MessageInterface, memory bool) error {
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
	return a.post(Envelope{From: from, FromName: aliasOf(from), Msg: out, memory: memory})
}

func sendNameFrom(from uint64, name string, msg MessageInterface, memory bool) error {
	if msg == nil {
		return ErrNilMessage
	}
	if name == "" {
		return ErrInvalidAlias
	}
	if a := actorRegistry.getByAlias(name); a != nil {
		out, err := prepareOutgoing(msg, memory)
		if err != nil {
			return err
		}
		return a.post(Envelope{From: from, FromName: aliasOf(from), Msg: out, memory: memory})
	}
	if !remoteReady() {
		return ErrUnknownAlias
	}
	if memory {
		return ErrRemoteMemory
	}
	out, err := prepareOutgoing(msg, false)
	if err != nil {
		return err
	}
	var packed *BaseMessage
	switch m := out.(type) {
	case *BaseMessage:
		packed = m
	case BaseMessage:
		packed = &m
	default:
		return ErrUnknownAlias
	}
	return forwardName(aliasOf(from), name, packed)
}

// SendNameAck 把消息送进目标 mailbox 后才返回。本地立刻得到 post 的结果。
// 跨服要等对方 harbor 确认；actor 不存在、连接失败或超时都会返回错误。
func SendNameAck(name string, msg MessageInterface) error {
	return sendNameAckFrom(0, name, msg)
}

func sendNameAckFrom(from uint64, name string, msg MessageInterface) error {
	if msg == nil {
		return ErrNilMessage
	}
	if name == "" {
		return ErrInvalidAlias
	}
	if a := actorRegistry.getByAlias(name); a != nil {
		out, err := prepareOutgoing(msg, false)
		if err != nil {
			return err
		}
		return a.post(Envelope{From: from, FromName: aliasOf(from), Msg: out})
	}
	if !ackReady() {
		return ErrUnknownAlias
	}
	out, err := prepareOutgoing(msg, false)
	if err != nil {
		return err
	}
	var packed *BaseMessage
	switch m := out.(type) {
	case *BaseMessage:
		packed = m
	case BaseMessage:
		packed = &m
	default:
		return ErrUnknownAlias
	}
	return forwardAckName(aliasOf(from), name, packed)
}

// DeliverLocal 把对端转发来的消息送进本进程别名。名字不在本地时直接失败，不再向外转发。
func DeliverLocal(name, fromName string, msg MessageInterface) error {
	if msg == nil {
		return ErrNilMessage
	}
	if name == "" {
		return ErrInvalidAlias
	}
	a := actorRegistry.getByAlias(name)
	if a == nil {
		return ErrUnknownAlias
	}
	out, err := prepareOutgoing(msg, false)
	if err != nil {
		return err
	}
	return a.post(Envelope{FromName: fromName, Msg: out})
}

// DeliverCall 把对端转发来的 Call 送进本进程别名。From 留空，Reply 按 fromName 送回。
func DeliverCall(name, fromName string, msg MessageInterface, session uint64) error {
	if msg == nil {
		return ErrNilMessage
	}
	if name == "" {
		return ErrInvalidAlias
	}
	if session == 0 {
		return DeliverLocal(name, fromName, msg)
	}
	a := actorRegistry.getByAlias(name)
	if a == nil {
		return ErrUnknownAlias
	}
	out, err := prepareOutgoing(msg, false)
	if err != nil {
		return err
	}
	return a.post(Envelope{FromName: fromName, Msg: out, session: session})
}

func aliasOf(pid uint64) string {
	if pid == 0 {
		return ""
	}
	return actorRegistry.aliasOf(pid)
}

func (e Envelope) Send(to uint64, msg MessageInterface) error {
	return sendFrom(e.Self, to, msg, false)
}

func (e Envelope) SendName(name string, msg MessageInterface) error {
	return sendNameFrom(e.Self, name, msg, false)
}

func (e Envelope) SendNameAck(name string, msg MessageInterface) error {
	return sendNameAckFrom(e.Self, name, msg)
}

func (e Envelope) SendMemory(to uint64, msg MessageInterface) error {
	return sendFrom(e.Self, to, msg, true)
}

func (e Envelope) SendMemoryName(name string, msg MessageInterface) error {
	return sendNameFrom(e.Self, name, msg, true)
}

func (e Envelope) Register(name string) error {
	return Register(e.Self, name)
}

func (e Envelope) Unregister() error {
	name := aliasOf(e.Self)
	if err := actorRegistry.unbindPID(e.Self); err != nil {
		return err
	}
	return unpublishName(name)
}

func (e Envelope) Timeout(d time.Duration, msg MessageInterface) (uint64, error) {
	return scheduleTimeout(e.Self, d, msg)
}

func (e Envelope) Exit() {
	_ = Kill(e.Self)
}

// Stop 停止所有 Actor，并一直等到 Term 完成。
func Stop() {
	_ = StopWait(0)
}

// StopWait 停止所有 Actor。d<=0 一直等；超时返回尚未 done 的 pid 列表（仍在停）。
func StopWait(d time.Duration) []uint64 {
	all := actorRegistry.list()
	for _, a := range all {
		a.life.requestStop()
	}
	if d <= 0 {
		for _, a := range all {
			<-a.life.done
		}
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	var stuck []uint64
	for i, a := range all {
		if len(stuck) > 0 {
			select {
			case <-a.life.done:
			default:
				stuck = append(stuck, a.pid)
			}
			continue
		}
		select {
		case <-a.life.done:
		case <-timer.C:
			stuck = append(stuck, a.pid)
			for _, b := range all[i+1:] {
				select {
				case <-b.life.done:
				default:
					stuck = append(stuck, b.pid)
				}
			}
			return stuck
		}
	}
	return stuck
}
