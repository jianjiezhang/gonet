package actormsg

import (
	"gonet/actor"

	"gonet/services/internal/harborproto"
)

const (
	CmdHarborHello          = "harbor.hello"
	CmdHarborRegisterName   = "harbor.registername"
	CmdHarborUnregisterName = "harbor.unregistername"
	CmdHarborQueryAddr      = "harbor.queryaddr"
	CmdHarborSetAddr        = "harbor.setaddr"
	CmdHarborResult         = "harbor.result"

	ErrHarborBusy    = "busy"
	ErrHarborTimeout = "timeout"

	CmdHarborBeat        = ".harbor.beat"
	CmdHarborExpire      = ".harbor.expire"
	CmdHarborLink        = ".harbor.link"
	CmdHarborMgrResult   = ".harbor.mgr"
	CmdHarborPeer        = ".harbor.peer"
	CmdHarborRemoteSend  = ".harbor.send"
	CmdHarborRemoteReply = ".harbor.reply"
)

// HarborRegisterName 把别名挂到本节点地址上。结果经 harbor.result 异步返回。
// Done 只给本进程同步等待用，不进 JSON。
type HarborRegisterName struct {
	actor.BaseMessage
	ID   uint64            `json:"id,omitempty"`
	Name string            `json:"name,omitempty"`
	Done chan HarborResult `json:"-"`
}

// HarborUnregisterName 从本节点摘掉别名。
// Done 只给本进程同步等待用，不进 JSON。
type HarborUnregisterName struct {
	actor.BaseMessage
	ID   uint64            `json:"id,omitempty"`
	Name string            `json:"name,omitempty"`
	Done chan HarborResult `json:"-"`
}

// HarborQueryAddr 向 harbormgr 查询别名对应的地址，不读本地缓存。
type HarborQueryAddr struct {
	actor.BaseMessage
	ID   uint64 `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// HarborSetAddr 更换本节点对外地址。节点 ID 不变，结果在登记成功后经 harbor.result 返回。
type HarborSetAddr struct {
	actor.BaseMessage
	ID   uint64 `json:"id,omitempty"`
	Addr string `json:"addr,omitempty"`
}

// HarborResult 是登记、摘除和查询的异步结果，Send 回请求方。
type HarborResult struct {
	actor.BaseMessage
	ID   uint64 `json:"id,omitempty"`
	Req  string `json:"req,omitempty"`
	OK   bool   `json:"ok"`
	Err  string `json:"err,omitempty"`
	Addr string `json:"addr,omitempty"`
	Name string `json:"name,omitempty"`
}

// HarborHello 是本节点 harbor 启动后的问候。
type HarborHello struct{ actor.BaseMessage }

// HarborBeat 是 harbor 给自己的心跳。
type HarborBeat struct{ actor.BaseMessage }

// HarborExpire 是 harbor 给自己的超时扫描。
type HarborExpire struct{ actor.BaseMessage }

// HarborMgrResult 是 harbormgr 回复进 harbor 邮箱的包装。线上正文仍是 harborproto.Result。
type HarborMgrResult struct {
	actor.BaseMessage
	Env harborproto.Result
}
