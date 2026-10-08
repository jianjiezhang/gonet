package actor

import (
	"log/slog"
	"sync"
	"time"
)

// ActorContextInterface 是用户 Actor 必须实现的生命周期方法。
// attach 未导出，业务通过嵌入 ActorContext 获得，不能自己实现。
type ActorContextInterface interface {
	attach(*actor)
	Init() error
	Dispatch(Envelope)
	Term()
}

// Handler 是某条 cmd 在本 actor 上的处理函数。
type Handler func(Envelope)

// ActorContext 是业务侧上下文。嵌入后即可用 Self / SelfName / Register，
// 无需再查 actorTable。
type ActorContext struct {
	a        *actor
	mu       sync.Mutex
	handlers map[string]Handler
}

func (c *ActorContext) attach(a *actor) {
	c.a = a
}

// RegisterCmd 同时登记：全局 cmd→消息类型，以及本 actor 的 cmd→处理函数。
func (c *ActorContext) RegisterCmd(cmd string, factory func() MessageInterface, h Handler) error {
	if c == nil {
		return ErrNilActor
	}
	if h == nil {
		return ErrNilHandler
	}
	if err := RegisterMsg(cmd, factory); err != nil {
		return err
	}
	c.bindHandler(cmd, h)
	return nil
}

func (c *ActorContext) bindHandler(cmd string, h Handler) {
	if c == nil || cmd == "" || h == nil {
		return
	}
	c.mu.Lock()
	if c.handlers == nil {
		c.handlers = make(map[string]Handler)
	}
	c.handlers[cmd] = h
	c.mu.Unlock()
}

func decodeDispatch(e Envelope) (string, MessageInterface, error) {
	if e.memory {
		cmd := ""
		if c, ok := e.Msg.(interface{ Command() string }); ok {
			cmd = c.Command()
		}
		if cmd == "" {
			return "", nil, ErrInvalidCmd
		}
		return cmd, e.Msg, nil
	}
	return decodeIncoming(e.Msg)
}

func (c *ActorContext) lookupHandler(cmd string) Handler {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	h := c.handlers[cmd]
	c.mu.Unlock()
	return h
}

// Init 默认空实现。业务覆盖时不必再调基类。
func (c *ActorContext) Init() error { return nil }

// Dispatch 用 cmd 解出业务结构体，再调本 actor 登记的 handler。
// memory 消息保持原指针。CmdResponse 未登记 handler 时默认丢弃（不打未知命令）。
func (c *ActorContext) Dispatch(e Envelope) {
	cmd, msg, err := decodeDispatch(e)
	if err != nil {
		slog.Warn("gonet: 解码消息失败", "pid", c.Self(), "err", err)
		return
	}
	e.Msg = msg
	h := c.lookupHandler(cmd)
	if h == nil {
		if cmd == CmdResponse {
			return
		}
		slog.Warn("gonet: 未知命令", "pid", c.Self(), "cmd", cmd)
		return
	}
	h(e)
}

// Term 默认空实现。
func (c *ActorContext) Term() {}

// Self 返回自身 pid；尚未挂接时为 0。
func (c *ActorContext) Self() uint64 {
	if c == nil || c.a == nil {
		return 0
	}
	return c.a.pid
}

// SelfName 返回当前别名；未注册或未挂接时为空串。
func (c *ActorContext) SelfName() string {
	if c == nil || c.a == nil {
		return ""
	}
	return actorRegistry.aliasOf(c.a.pid)
}

// Register 给自身绑定别名。
func (c *ActorContext) Register(name string) error {
	return Register(c.Self(), name)
}

// Unregister 摘掉自身当前别名。
func (c *ActorContext) Unregister() error {
	if c == nil || c.a == nil {
		return ErrDead
	}
	name := actorRegistry.aliasOf(c.a.pid)
	if err := actorRegistry.unbindPID(c.a.pid); err != nil {
		return err
	}
	return unpublishName(name)
}

func (c *ActorContext) Send(to uint64, msg MessageInterface) error {
	if c == nil || c.a == nil {
		return ErrNilActor
	}
	return sendFrom(c.Self(), to, msg, false)
}

func (c *ActorContext) SendName(name string, msg MessageInterface) error {
	if c == nil || c.a == nil {
		return ErrNilActor
	}
	return sendNameFrom(c.Self(), name, msg, false)
}

func (c *ActorContext) SendNameAck(name string, msg MessageInterface) error {
	if c == nil || c.a == nil {
		return ErrNilActor
	}
	return sendNameAckFrom(c.Self(), name, msg)
}

func (c *ActorContext) SendMemory(to uint64, msg MessageInterface) error {
	if c == nil || c.a == nil {
		return ErrNilActor
	}
	return sendFrom(c.Self(), to, msg, true)
}

func (c *ActorContext) SendMemoryName(name string, msg MessageInterface) error {
	if c == nil || c.a == nil {
		return ErrNilActor
	}
	return sendNameFrom(c.Self(), name, msg, true)
}

func (c *ActorContext) Timeout(d time.Duration, msg MessageInterface) (uint64, error) {
	if c == nil {
		return 0, ErrNilActor
	}
	return scheduleTimeout(c.Self(), d, msg)
}

func (c *ActorContext) Exit() {
	if c == nil {
		return
	}
	_ = Kill(c.Self())
}
