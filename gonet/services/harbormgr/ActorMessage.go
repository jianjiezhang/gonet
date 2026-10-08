package harbormgr

import (
	"strconv"

	"gonet/actor"
	"gonet/services/harbormgr/proto"
)

const (
	CmdRegisterAddr   = proto.CmdRegisterAddr
	CmdRegisterName   = proto.CmdRegisterName
	CmdHeartbeat      = proto.CmdHeartbeat
	CmdQueryAddr      = proto.CmdQueryAddr
	CmdUnregisterName = proto.CmdUnregisterName
	CmdUnregisterAddr = proto.CmdUnregisterAddr
	CmdResult         = proto.CmdResult

	// 100 起只在本进程 mailbox 里用，不会写进 TCP 帧。
	CmdHello  uint16 = 100
	CmdSweep  uint16 = 101
	CmdUnbind uint16 = 102
	CmdSeq    uint16 = 103
)

const (
	ErrInvalid = proto.ErrInvalid
	ErrUnknown = proto.ErrUnknown
	ErrTaken   = proto.ErrTaken
)

// CmdKey 把命令号变成 mailbox 的登记键。内核的命令表是字符串。
func CmdKey(cmd uint16) string {
	return strconv.FormatUint(uint64(cmd), 10)
}

// fromConn 是这条 TCP 请求带进 mailbox 的连接编号和回复通道。
// 进程内直接 Send 时两者都是零值。
type fromConn struct {
	token uint64
	back  chan Result
}

func (c *fromConn) takeBack() chan Result {
	if c == nil {
		return nil
	}
	back := c.back
	c.back = nil
	return back
}

// RegisterAddr 是协议 RegisterAddr 进入 mailbox 后的消息。
type RegisterAddr struct {
	actor.BaseMessage
	proto.RegisterAddr
	fromConn
}

// RegisterName 是协议 RegisterName 进入 mailbox 后的消息。
type RegisterName struct {
	actor.BaseMessage
	proto.RegisterName
	fromConn
}

// Heartbeat 是协议 Heartbeat 进入 mailbox 后的消息。
type Heartbeat struct {
	actor.BaseMessage
	proto.Heartbeat
	fromConn
}

// QueryAddr 是协议 QueryAddr 进入 mailbox 后的消息。
type QueryAddr struct {
	actor.BaseMessage
	proto.QueryAddr
	fromConn
}

// UnregisterName 是协议 UnregisterName 进入 mailbox 后的消息。
type UnregisterName struct {
	actor.BaseMessage
	proto.UnregisterName
	fromConn
}

// UnregisterAddr 是协议 UnregisterAddr 进入 mailbox 后的消息。
type UnregisterAddr struct {
	actor.BaseMessage
	proto.UnregisterAddr
	fromConn
}

// Result 是协议回复进入 mailbox 后的消息。Env 是命令号 + 正文，命令号决定正文的结果结构。
type Result struct {
	actor.BaseMessage
	Env proto.Result
}

type sweepMsg struct {
	actor.BaseMessage
}

type helloMsg struct {
	actor.BaseMessage
}

// unbindMsg 表示这条连接已经断开。目录记录保留到存活时间结束。
type unbindMsg struct {
	actor.BaseMessage
	token uint64
}

// seqSaved 是节点编号已经写入 NodeSeq 之后的回执。
type seqSaved struct {
	actor.BaseMessage
	node uint64
	ok   bool
}

func (a *Actor) RegisterCmds() error {
	if err := a.RegisterCmd(CmdKey(CmdHello), func() actor.MessageInterface { return &helloMsg{} }, a.onHello); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdKey(CmdRegisterAddr), func() actor.MessageInterface { return &RegisterAddr{} }, a.onRegisterAddr); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdKey(CmdRegisterName), func() actor.MessageInterface { return &RegisterName{} }, a.onRegisterName); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdKey(CmdHeartbeat), func() actor.MessageInterface { return &Heartbeat{} }, a.onHeartbeat); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdKey(CmdQueryAddr), func() actor.MessageInterface { return &QueryAddr{} }, a.onQueryAddr); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdKey(CmdUnregisterName), func() actor.MessageInterface { return &UnregisterName{} }, a.onUnregisterName); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdKey(CmdUnregisterAddr), func() actor.MessageInterface { return &UnregisterAddr{} }, a.onUnregisterAddr); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdKey(CmdSweep), func() actor.MessageInterface { return &sweepMsg{} }, a.onSweep); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdKey(CmdUnbind), func() actor.MessageInterface { return &unbindMsg{} }, a.onUnbind); err != nil {
		return err
	}
	return a.RegisterCmd(CmdKey(CmdSeq), func() actor.MessageInterface { return &seqSaved{} }, a.onSeqSaved)
}
