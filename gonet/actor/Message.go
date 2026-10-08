package actor

import (
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"sync"
)

var (
	ErrInvalidCmd    = errors.New("gonet: 命令名不能为空")
	ErrMsgRegistered = errors.New("gonet: 消息类型已注册")
	ErrUnknownCmd    = errors.New("gonet: 未知命令")
	ErrNilFactory    = errors.New("gonet: 消息工厂不能为 nil")
	ErrNilHandler    = errors.New("gonet: handler 不能为 nil")
)

// MessageInterface 是 Send/Call 的消息正文。业务结构体嵌入 BaseMessage 即可。
type MessageInterface interface {
	gonetMsg()
}

// MsgFactory 按 cmd 构造空的业务消息实例，供反序列化写入。
type MsgFactory func() MessageInterface

var (
	msgMu    sync.RWMutex
	msgTypes = map[string]MsgFactory{}
)

// RegisterMsg 把 cmd 登记到消息类型表。已存在则覆盖为新工厂；类型变了会打日志。
func RegisterMsg(cmd string, factory MsgFactory) error {
	if cmd == "" {
		return ErrInvalidCmd
	}
	if factory == nil {
		return ErrNilFactory
	}
	msgMu.Lock()
	defer msgMu.Unlock()
	if old, ok := msgTypes[cmd]; ok {
		ot, nt := factoryType(old), factoryType(factory)
		if ot != nt {
			slog.Warn("gonet: cmd 工厂已替换", "cmd", cmd, "old", ot, "new", nt)
		}
	}
	msgTypes[cmd] = factory
	return nil
}

func factoryType(f MsgFactory) string {
	if f == nil {
		return "<nil>"
	}
	v := f()
	if v == nil {
		return "<nil>"
	}
	return reflect.TypeOf(v).String()
}

func lookupMsg(cmd string) MsgFactory {
	msgMu.RLock()
	f := msgTypes[cmd]
	msgMu.RUnlock()
	return f
}

// BaseMessage 协议头：Cmd 索引类型表和 handler 表，Data 是 JSON 正文。
type BaseMessage struct {
	Cmd  string `json:"cmd"`
	Data string `json:"data"`
}

func (BaseMessage) gonetMsg() {}

func (m *BaseMessage) SetCmd(name string) {
	if m == nil {
		return
	}
	m.Cmd = name
}

func (m BaseMessage) Command() string {
	return m.Cmd
}

func (m *BaseMessage) SetData(data string) {
	if m == nil {
		return
	}
	m.Data = data
}

func (m BaseMessage) Payload() string {
	return m.Data
}

// Pack 把 payload 编成 JSON，放入 BaseMessage.Data。系统命令（`.` 前缀）不能 Pack。
func Pack(cmd string, payload any) (*BaseMessage, error) {
	if cmd == "" {
		return nil, ErrInvalidCmd
	}
	if IsSystemCmd(cmd) {
		return nil, ErrSystemCmd
	}
	data := ""
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		data = string(b)
	}
	return &BaseMessage{Cmd: cmd, Data: data}, nil
}

// Unpack 用 cmd 查类型表，把 JSON Data 填进新实例。
func Unpack(cmd, data string) (MessageInterface, error) {
	if cmd == "" {
		return nil, ErrInvalidCmd
	}
	f := lookupMsg(cmd)
	if f == nil {
		return nil, ErrUnknownCmd
	}
	msg := f()
	if msg == nil {
		return nil, ErrNilFactory
	}
	if data != "" {
		if err := json.Unmarshal([]byte(data), msg); err != nil {
			return nil, err
		}
	}
	if s, ok := msg.(interface{ SetCmd(string) }); ok {
		s.SetCmd(cmd)
	}
	return msg, nil
}

// prepareOutgoing 把业务消息收成 *BaseMessage（Data 为 JSON）。
// memory 为真、已经是 *BaseMessage、或 `.` 系统命令时保持原值。
func prepareOutgoing(msg MessageInterface, memory bool) (MessageInterface, error) {
	if msg == nil {
		return nil, ErrNilMessage
	}
	if memory {
		return msg, nil
	}
	switch msg.(type) {
	case *BaseMessage, BaseMessage:
		return msg, nil
	}
	cmd := ""
	if c, ok := msg.(interface{ Command() string }); ok {
		cmd = c.Command()
	}
	if IsSystemCmd(cmd) {
		return msg, nil
	}
	if cmd == "" {
		return nil, ErrInvalidCmd
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return &BaseMessage{Cmd: cmd, Data: string(b)}, nil
}

func decodeIncoming(msg MessageInterface) (cmd string, out MessageInterface, err error) {
	if msg == nil {
		return "", nil, ErrNilMessage
	}
	cmd = ""
	if c, ok := msg.(interface{ Command() string }); ok {
		cmd = c.Command()
	}
	if cmd == "" {
		return "", nil, ErrInvalidCmd
	}
	data := ""
	if p, ok := msg.(interface{ Payload() string }); ok {
		data = p.Payload()
	}
	switch msg.(type) {
	case *BaseMessage, BaseMessage:
		if IsSystemCmd(cmd) {
			return "", nil, ErrSystemCmd
		}
		out, err = Unpack(cmd, data)
		return cmd, out, err
	default:
		return cmd, msg, nil
	}
}
