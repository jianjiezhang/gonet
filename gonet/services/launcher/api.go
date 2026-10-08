package launcher

import (
	"context"
	"errors"
	"sync"
	"time"

	"gonet"
)

// ServiceResult 是 newservice 的结果，异步时交给 NewService 的 callback。
type ServiceResult struct {
	PID uint64
	Err error
}

var ErrNilCallback = errors.New("launcher: callback 不能为空")

type spawnWait struct {
	from uint64
	cb   func(ServiceResult)
}

var (
	spawnMu    sync.Mutex
	spawnWaits = map[uint64]spawnWait{}
)

// NewService 向 .launcher 发异步 Call。创建完成（或超时/失败）后在调用方 mailbox 里跑 callback。
func NewService(from uint64, impl gonet.ActorContextInterface, serviceName string, timeout time.Duration, callback func(ServiceResult)) error {
	if impl == nil {
		return gonet.ErrNilActor
	}
	if callback == nil {
		return ErrNilCallback
	}
	msg := &newServiceMsg{
		BaseMessage: gonet.BaseMessage{Cmd: cmdNewService},
		Impl:        impl,
		ServiceName: serviceName,
	}
	sess, err := gonet.CallMemoryName(from, Name, timeout, msg)
	if err != nil {
		return err
	}
	spawnMu.Lock()
	spawnWaits[sess] = spawnWait{from: from, cb: callback}
	spawnMu.Unlock()
	return nil
}

// DispatchResponse 若这条 .response 是 NewService 的回复则调用 callback 并返回 true。
// 在调用方的 CmdResponse handler 里最先调。callback 跑在本 mailbox。
func DispatchResponse(m *gonet.CallResponse) bool {
	if m == nil {
		return false
	}
	spawnMu.Lock()
	w, ok := spawnWaits[m.Session]
	if ok {
		delete(spawnWaits, m.Session)
	}
	spawnMu.Unlock()
	if !ok {
		return false
	}
	w.cb(serviceResult(m))
	return true
}

// DropWaits 丢掉 from 尚未完成的 NewService 等待，并以 err 回调（Term 用）。
func DropWaits(from uint64, err error) {
	var cbs []func(ServiceResult)
	spawnMu.Lock()
	for sess, w := range spawnWaits {
		if w.from == from {
			delete(spawnWaits, sess)
			cbs = append(cbs, w.cb)
		}
	}
	spawnMu.Unlock()
	res := ServiceResult{Err: err}
	for _, cb := range cbs {
		cb(res)
	}
}

func serviceResult(m *gonet.CallResponse) ServiceResult {
	if m.Err != nil {
		return ServiceResult{Err: m.Err}
	}
	res, ok := m.Value.(ServiceResult)
	if !ok {
		return ServiceResult{Err: gonet.ErrNilMessage}
	}
	return res
}

// WaitService 阻塞等 launcher 创建完成。给 main / 测试用，不要在 Dispatch 里调。
func WaitService(ctx context.Context, impl gonet.ActorContextInterface, serviceName string) (uint64, error) {
	if impl == nil {
		return 0, gonet.ErrNilActor
	}
	msg := &newServiceMsg{
		BaseMessage: gonet.BaseMessage{Cmd: cmdNewService},
		Impl:        impl,
		ServiceName: serviceName,
	}
	v, err := gonet.SuspendCallMemoryName(ctx, Name, msg)
	if err != nil {
		return 0, err
	}
	r, ok := v.(ServiceResult)
	if !ok {
		return 0, gonet.ErrNilMessage
	}
	return r.PID, r.Err
}
