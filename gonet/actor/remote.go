package actor

import (
	"fmt"
	"log/slog"
	"sync"
)

// ServiceAlias 是本服服务的跨服别名，形式为 .名字_节点编号。编号是 harbormgr 分配的 nodeid。
// nodeid 还没有时返回空字符串，不生成 _0 这种占位别名。
func ServiceAlias(service string, nodeID uint64) string {
	if service == "" || nodeID == 0 {
		return ""
	}
	return fmt.Sprintf(".%s_%d", service, nodeID)
}

// RemoteHooks 由 harbor 在启动时装上。没装时，别名只在本进程内有效。
type RemoteHooks struct {
	Publish     func(name string) error
	Unpublish   func(name string) error
	Forward     func(fromName, name string, msg *BaseMessage) error
	ForwardAck  func(fromName, name string, msg *BaseMessage) error
	ForwardCall func(fromName, name string, msg *BaseMessage, session uint64) error
	ReplyCall   func(name string, session uint64, v any, err error)
}

var (
	remoteMu sync.RWMutex
	remote   RemoteHooks
)

// SetRemote 装上跨服登记和转发。harbor 停止时调用 ClearRemote。
func SetRemote(h RemoteHooks) {
	remoteMu.Lock()
	remote = h
	remoteMu.Unlock()
}

// ClearRemote 卸掉跨服钩子。之后找不到的名字按本进程别名不存在处理。
func ClearRemote() {
	remoteMu.Lock()
	remote = RemoteHooks{}
	remoteMu.Unlock()
}

func publishName(name string) error {
	if name == "" {
		return nil
	}
	remoteMu.RLock()
	fn := remote.Publish
	remoteMu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn(name)
}

func unpublishName(name string) error {
	if name == "" {
		return nil
	}
	remoteMu.RLock()
	fn := remote.Unpublish
	remoteMu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn(name)
}

func forwardName(fromName, name string, msg *BaseMessage) error {
	remoteMu.RLock()
	fn := remote.Forward
	remoteMu.RUnlock()
	if fn == nil {
		return ErrUnknownAlias
	}
	return fn(fromName, name, msg)
}

func forwardAckName(fromName, name string, msg *BaseMessage) error {
	remoteMu.RLock()
	fn := remote.ForwardAck
	remoteMu.RUnlock()
	if fn == nil {
		return ErrUnknownAlias
	}
	return fn(fromName, name, msg)
}

func ackReady() bool {
	remoteMu.RLock()
	ok := remote.ForwardAck != nil
	remoteMu.RUnlock()
	return ok
}

func callReady() bool {
	remoteMu.RLock()
	ok := remote.ForwardCall != nil
	remoteMu.RUnlock()
	return ok
}

func forwardCallName(fromName, name string, msg *BaseMessage, session uint64) error {
	remoteMu.RLock()
	fn := remote.ForwardCall
	remoteMu.RUnlock()
	if fn == nil {
		return ErrUnknownAlias
	}
	return fn(fromName, name, msg, session)
}

func replyReady() bool {
	remoteMu.RLock()
	ok := remote.ReplyCall != nil
	remoteMu.RUnlock()
	return ok
}

func replyRemote(name string, session uint64, v any, err error) {
	if name == "" || session == 0 {
		return
	}
	remoteMu.RLock()
	fn := remote.ReplyCall
	remoteMu.RUnlock()
	if fn == nil {
		slog.Error("gonet: 跨服 Call 回复没有通路", "name", name, "session", session)
		return
	}
	fn(name, session, v, err)
}

func remoteReady() bool {
	remoteMu.RLock()
	ok := remote.Forward != nil
	remoteMu.RUnlock()
	return ok
}
