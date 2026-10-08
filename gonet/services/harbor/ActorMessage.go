package harbor

import (
	"gonet/actor"
	"gonet/services/harbormgr"
)

const (
	CmdHello          = "harbor.hello"
	CmdRegisterName   = "harbor.registername"
	CmdUnregisterName = "harbor.unregistername"
	CmdQueryAddr      = "harbor.queryaddr"
	CmdSetAddr        = "harbor.setaddr"
	CmdResult         = "harbor.result"

	ErrBusy    = "busy"
	ErrTimeout = "timeout"

	cmdBeat        = ".harbor.beat"
	cmdExpire      = ".harbor.expire"
	cmdLink        = ".harbor.link"
	cmdPeer        = ".harbor.peer"
	cmdRemoteSend  = ".harbor.send"
	cmdRemoteReply = ".harbor.reply"
)

// RegisterName 把别名挂到本节点地址上。结果经 harbor.result 异步返回。
// done 只给本进程同步等待用，不进 JSON。
type RegisterName struct {
	actor.BaseMessage
	ID   uint64 `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	done chan Result
}

// UnregisterName 从本节点摘掉别名。
// done 只给本进程同步等待用，不进 JSON。
type UnregisterName struct {
	actor.BaseMessage
	ID   uint64 `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	done chan Result
}

// QueryAddr 向 harbormgr 查询别名对应的地址，不读本地缓存。
type QueryAddr struct {
	actor.BaseMessage
	ID   uint64 `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// SetAddr 更换本节点对外地址。节点 ID 不变，结果在登记成功后经 harbor.result 返回。
type SetAddr struct {
	actor.BaseMessage
	ID   uint64 `json:"id,omitempty"`
	Addr string `json:"addr,omitempty"`
}

// Result 是登记、摘除和查询的异步结果，Send 回请求方。
type Result struct {
	actor.BaseMessage
	ID   uint64 `json:"id,omitempty"`
	Req  string `json:"req,omitempty"`
	OK   bool   `json:"ok"`
	Err  string `json:"err,omitempty"`
	Addr string `json:"addr,omitempty"`
	Name string `json:"name,omitempty"`
}

type helloMsg struct {
	actor.BaseMessage
}

type beatMsg struct {
	actor.BaseMessage
}

type expireMsg struct {
	actor.BaseMessage
}

func (a *Actor) RegisterCmds() error {
	if err := a.RegisterCmd(CmdHello, func() actor.MessageInterface { return &helloMsg{} }, a.onHello); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdRegisterName, func() actor.MessageInterface { return &RegisterName{} }, a.onRegisterName); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdUnregisterName, func() actor.MessageInterface { return &UnregisterName{} }, a.onUnregisterName); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdQueryAddr, func() actor.MessageInterface { return &QueryAddr{} }, a.onQueryAddr); err != nil {
		return err
	}
	if err := a.RegisterCmd(CmdSetAddr, func() actor.MessageInterface { return &SetAddr{} }, a.onSetAddr); err != nil {
		return err
	}
	if err := a.RegisterCmd(harbormgr.CmdKey(harbormgr.CmdResult), func() actor.MessageInterface { return &harbormgr.Result{} }, a.onMgrResult); err != nil {
		return err
	}
	if err := a.RegisterCmd(cmdBeat, func() actor.MessageInterface { return &beatMsg{} }, a.onBeat); err != nil {
		return err
	}
	if err := a.RegisterCmd(cmdExpire, func() actor.MessageInterface { return &expireMsg{} }, a.onExpire); err != nil {
		return err
	}
	if err := a.RegisterCmd(cmdLink, func() actor.MessageInterface { return &linkMsg{} }, a.onLink); err != nil {
		return err
	}
	if err := a.RegisterCmd(cmdPeer, func() actor.MessageInterface { return &peerEvent{} }, a.onPeer); err != nil {
		return err
	}
	if err := a.RegisterCmd(cmdRemoteSend, func() actor.MessageInterface { return &remoteSend{} }, a.onRemoteSend); err != nil {
		return err
	}
	return a.RegisterCmd(cmdRemoteReply, func() actor.MessageInterface { return &remoteReply{} }, a.onRemoteReply)
}
