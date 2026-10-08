package gonet

import (
	"context"
	"sync/atomic"
	"time"

	"gonet/actor"
	"gonet/monitor"
	"gonet/timer"
)

func init() {
	actor.InstallLogPrefix()
}

type ActorContextInterface = actor.ActorContextInterface
type ActorContext = actor.ActorContext
type Envelope = actor.Envelope
type MessageInterface = actor.MessageInterface
type BaseMessage = actor.BaseMessage
type Handler = actor.Handler
type TimerID = timer.ID
type Snapshot = monitor.Snapshot
type ActorStat = monitor.ActorStat
type SpawnOptions = actor.SpawnOptions
type CallResponse = actor.CallResponse
type Stats = actor.Stats

var (
	ErrNilActor      = actor.ErrNilActor
	ErrDead          = actor.ErrDead
	ErrMailboxFull   = actor.ErrMailboxFull
	ErrNilContext    = actor.ErrNilContext
	ErrNilMessage    = actor.ErrNilMessage
	ErrInvalidAlias  = actor.ErrInvalidAlias
	ErrAliasTaken    = actor.ErrAliasTaken
	ErrAliasBound    = actor.ErrAliasBound
	ErrUnknownAlias  = actor.ErrUnknownAlias
	ErrRemoteMemory  = actor.ErrRemoteMemory
	ErrRemoteDown    = actor.ErrRemoteDown
	ErrRemoteTimeout = actor.ErrRemoteTimeout
	ErrInvalidCmd    = actor.ErrInvalidCmd
	ErrMsgRegistered = actor.ErrMsgRegistered
	ErrUnknownCmd    = actor.ErrUnknownCmd
	ErrNilFactory    = actor.ErrNilFactory
	ErrNilHandler    = actor.ErrNilHandler
	ErrNoCaller      = actor.ErrNoCaller
	ErrRemoteCaller  = actor.ErrRemoteCaller
	ErrCallTimeout   = actor.ErrCallTimeout
	ErrSuspendSelf   = actor.ErrSuspendSelf
	ErrDispatchPanic = actor.ErrDispatchPanic
	ErrStopTimeout   = actor.ErrStopTimeout
	ErrSystemCmd     = actor.ErrSystemCmd
	ErrNotReady      = actor.ErrNotReady
)

const (
	CmdResponse           = actor.CmdResponse
	SlowDispatchThreshold = actor.SlowDispatchThreshold
)

func Spawn(impl ActorContextInterface) (uint64, error) {
	return actor.Spawn(impl)
}

func SpawnNamed(impl ActorContextInterface, name string) (uint64, error) {
	return actor.SpawnNamed(impl, name)
}

func SpawnWith(impl ActorContextInterface, opt SpawnOptions) (uint64, error) {
	return actor.SpawnWith(impl, opt)
}

// SpawnAsync 立刻返回 pid；Init 在 mailbox 里跑。InitFrom!=0 时返回 initSession。
func SpawnAsync(impl ActorContextInterface, opt SpawnOptions) (pid, initSession uint64, err error) {
	return actor.SpawnAsync(impl, opt)
}

func WaitInit(ctx context.Context, pid uint64) error {
	return actor.WaitInit(ctx, pid)
}

func StopActor(pid uint64) error {
	return actor.StopActor(pid)
}

func StopActorWait(pid uint64, d time.Duration) error {
	return actor.StopActorWait(pid, d)
}

func Kill(pid uint64) error {
	return actor.Kill(pid)
}

func Send(pid uint64, msg MessageInterface) error {
	return actor.Send(pid, msg)
}

// SendMemory 把原指针放进对方 mailbox，不序列化。只在本进程使用。
func SendMemory(pid uint64, msg MessageInterface) error {
	return actor.SendMemory(pid, msg)
}

func SendName(name string, msg MessageInterface) error {
	return actor.SendName(name, msg)
}

// SendNameAck 等到目标 mailbox 接受消息后才返回。跨服失败时 error 就是这次发送的结果。
func SendNameAck(name string, msg MessageInterface) error {
	return actor.SendNameAck(name, msg)
}

// SendMemoryName 按别名做 SendMemory。
func SendMemoryName(name string, msg MessageInterface) error {
	return actor.SendMemoryName(name, msg)
}

func Call(from, to uint64, d time.Duration, msg MessageInterface) (uint64, error) {
	return actor.Call(from, to, d, msg)
}

// CallMemory 异步 Call，消息保持原指针，不序列化。
func CallMemory(from, to uint64, d time.Duration, msg MessageInterface) (uint64, error) {
	return actor.CallMemory(from, to, d, msg)
}

func CallName(from uint64, name string, d time.Duration, msg MessageInterface) (uint64, error) {
	return actor.CallName(from, name, d, msg)
}

// CallMemoryName 按别名做 CallMemory。
func CallMemoryName(from uint64, name string, d time.Duration, msg MessageInterface) (uint64, error) {
	return actor.CallMemoryName(from, name, d, msg)
}

func SuspendCall(ctx context.Context, pid uint64, msg MessageInterface) (any, error) {
	return actor.SuspendCall(ctx, pid, msg)
}

// SuspendCallMemory 阻塞 Call，消息保持原指针。不要在 Dispatch 里用。
func SuspendCallMemory(ctx context.Context, pid uint64, msg MessageInterface) (any, error) {
	return actor.SuspendCallMemory(ctx, pid, msg)
}

func SuspendCallName(ctx context.Context, name string, msg MessageInterface) (any, error) {
	return actor.SuspendCallName(ctx, name, msg)
}

// SuspendCallMemoryName 按别名做 SuspendCallMemory。
func SuspendCallMemoryName(ctx context.Context, name string, msg MessageInterface) (any, error) {
	return actor.SuspendCallMemoryName(ctx, name, msg)
}

func Register(pid uint64, name string) error {
	return actor.Register(pid, name)
}

func Unregister(name string) error {
	return actor.Unregister(name)
}

func Query(name string) (uint64, error) {
	return actor.Query(name)
}

func IsSystemCmd(cmd string) bool {
	return actor.IsSystemCmd(cmd)
}

// ServiceAlias 是本服服务的跨服别名：.名字_节点编号。编号是 harbormgr 分配的 nodeid。
func ServiceAlias(service string, nodeID uint64) string {
	return actor.ServiceAlias(service, nodeID)
}

var clusterNode atomic.Uint64

// SetClusterNode 记下本进程 harbor 分配到的 nodeid。服务别名用这个编号。
func SetClusterNode(id uint64) {
	clusterNode.Store(id)
}

// ClusterNode 返回本进程的 nodeid。还没登记成功时为 0。
func ClusterNode() uint64 {
	return clusterNode.Load()
}

func Stop() {
	_ = monitor.StopListen()
	actor.Stop()
}

// StopWait 停掉 debug 后停止所有 actor。d<=0 一直等；超时返回尚未结束的 pid。
func StopWait(d time.Duration) []uint64 {
	_ = monitor.StopListen()
	return actor.StopWait(d)
}

func RegisterCmd(host interface {
	RegisterCmd(cmd string, factory func() MessageInterface, h Handler) error
}, cmd string, factory func() MessageInterface, h Handler) error {
	if host == nil {
		return ErrNilActor
	}
	return host.RegisterCmd(cmd, factory, h)
}

func Pack(cmd string, payload any) (*BaseMessage, error) {
	return actor.Pack(cmd, payload)
}

// EnterService 让当前协程上的日志带上服务名。mailbox 协程在 run 里已经带上别名。
// 旁路协程（玩家 readLoop、等登录）退出前要调用返回的函数。
func EnterService(name string) func() {
	return actor.EnterService(name)
}

func Unpack(cmd, data string) (MessageInterface, error) {
	return actor.Unpack(cmd, data)
}

func Timeout(pid uint64, d time.Duration, msg MessageInterface) (TimerID, error) {
	id, err := actor.Timeout(pid, d, msg)
	return TimerID(id), err
}

func StopTimer(id TimerID) error {
	return timer.Stop(id)
}

func Monitor() Snapshot {
	return monitor.SnapshotNow()
}

func ListenDebug(addr string) (string, error) {
	return monitor.Listen(addr)
}

func StopDebug() error {
	return monitor.StopListen()
}
