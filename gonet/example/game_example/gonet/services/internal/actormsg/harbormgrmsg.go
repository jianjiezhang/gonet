package actormsg

import (
	"strconv"

	"gonet/actor"

	"gonet/services/internal/harborproto"
)

const (
	HarbormgrCmdRegisterAddr   = harborproto.CmdRegisterAddr
	HarbormgrCmdRegisterName   = harborproto.CmdRegisterName
	HarbormgrCmdHeartbeat      = harborproto.CmdHeartbeat
	HarbormgrCmdQueryAddr      = harborproto.CmdQueryAddr
	HarbormgrCmdUnregisterName = harborproto.CmdUnregisterName
	HarbormgrCmdUnregisterAddr = harborproto.CmdUnregisterAddr
	HarbormgrCmdResult         = harborproto.CmdResult

	// 100 起只在本进程 mailbox 里用，不会写进 TCP 帧。
	HarbormgrCmdHello  uint16 = 100
	HarbormgrCmdSweep  uint16 = 101
	HarbormgrCmdUnbind uint16 = 102
	HarbormgrCmdSeq    uint16 = 103
)

const (
	ErrHarbormgrInvalid = harborproto.ErrInvalid
	ErrHarbormgrUnknown = harborproto.ErrUnknown
	ErrHarbormgrTaken   = harborproto.ErrTaken
)

// CmdKey 把命令号变成 mailbox 的登记键。内核的命令表是字符串。
func CmdKey(cmd uint16) string {
	return strconv.FormatUint(uint64(cmd), 10)
}

// HarbormgrFromConn 是这条 TCP 请求带进 mailbox 的连接编号和回复通道。
// 进程内直接 Send 时两者都是零值。
type HarbormgrFromConn struct {
	Token uint64               `json:"-"`
	Back  chan HarbormgrResult `json:"-"`
}

// TakeBack 取出回复通道，只取一次。
func (c *HarbormgrFromConn) TakeBack() chan HarbormgrResult {
	if c == nil {
		return nil
	}
	back := c.Back
	c.Back = nil
	return back
}

// HarbormgrRegisterAddr 是协议 RegisterAddr 进入 mailbox 后的消息。
type HarbormgrRegisterAddr struct {
	actor.BaseMessage
	harborproto.RegisterAddr
	HarbormgrFromConn
}

// HarbormgrRegisterName 是协议 RegisterName 进入 mailbox 后的消息。
type HarbormgrRegisterName struct {
	actor.BaseMessage
	harborproto.RegisterName
	HarbormgrFromConn
}

// HarbormgrHeartbeat 是协议 Heartbeat 进入 mailbox 后的消息。
type HarbormgrHeartbeat struct {
	actor.BaseMessage
	harborproto.Heartbeat
	HarbormgrFromConn
}

// HarbormgrQueryAddr 是协议 QueryAddr 进入 mailbox 后的消息。
type HarbormgrQueryAddr struct {
	actor.BaseMessage
	harborproto.QueryAddr
	HarbormgrFromConn
}

// HarbormgrUnregisterName 是协议 UnregisterName 进入 mailbox 后的消息。
type HarbormgrUnregisterName struct {
	actor.BaseMessage
	harborproto.UnregisterName
	HarbormgrFromConn
}

// HarbormgrUnregisterAddr 是协议 UnregisterAddr 进入 mailbox 后的消息。
type HarbormgrUnregisterAddr struct {
	actor.BaseMessage
	harborproto.UnregisterAddr
	HarbormgrFromConn
}

// HarbormgrResult 是协议回复进入 mailbox 后的消息。Env 是命令号 + 正文。
type HarbormgrResult struct {
	actor.BaseMessage
	Env harborproto.Result
}

// HarbormgrHello 是目录启动后的问候。
type HarbormgrHello struct{ actor.BaseMessage }

// HarbormgrSweep 是目录给自己的过期扫描。
type HarbormgrSweep struct{ actor.BaseMessage }

// HarbormgrUnbind 表示这条连接已经断开。目录记录保留到存活时间结束。
type HarbormgrUnbind struct {
	actor.BaseMessage
	Token uint64 `json:"-"`
}

// HarbormgrSeq 是节点编号已经写入 NodeSeq 之后的回执。
type HarbormgrSeq struct {
	actor.BaseMessage
	Node uint64 `json:"-"`
	OK   bool   `json:"-"`
}
