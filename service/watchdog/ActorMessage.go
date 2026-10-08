package watchdog

import (
	"net"

	"protocol/client"

	"gonet"
)

const cmdPing = "ping"
const cmdSocketOpen = "socket.open"
const cmdSocketClose = "socket.close"
const cmdKickDone = "kick.done"
const cmdRetryLogin = ".watchdog.retry"

type PingMsg struct {
	gonet.BaseMessage
}

type socketOpenMsg struct {
	gonet.BaseMessage
	Conn net.Conn `json:"-"`
}

type socketCloseMsg struct {
	gonet.BaseMessage
	Alias string `json:"alias"`
	Link  uint64 `json:"link"`
}

type kickDoneMsg struct {
	gonet.BaseMessage
	Conn   net.Conn `json:"-"`
	RoleID string   `json:"-"`
	Old    uint64   `json:"-"`
	Err    error    `json:"-"`
}

type retryLoginMsg struct {
	gonet.BaseMessage
	Conn   net.Conn `json:"-"`
	RoleID string   `json:"-"`
}

func (m *retryLoginMsg) Command() string { return cmdRetryLogin }

// loginConnMsg 的正文是 client.LoginReq。Conn 进程内附上，不进 JSON。
type loginConnMsg struct {
	gonet.BaseMessage
	Conn net.Conn `json:"-"`
	client.LoginReq
}

func (a *Actor) registerClientCmds() error {
	return gonet.RegisterCmd(a, client.Login, func() gonet.MessageInterface { return &loginConnMsg{} }, a.onLogin)
}

func (a *Actor) RegisterCmds() error {
	if err := a.registerActorCmds(); err != nil {
		return err
	}
	return a.registerClientCmds()
}

func (a *Actor) registerActorCmds() error {
	if err := gonet.RegisterCmd(a, cmdPing, func() gonet.MessageInterface { return &PingMsg{} }, a.onPing); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, cmdSocketOpen, func() gonet.MessageInterface { return &socketOpenMsg{} }, a.onSocketOpen); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, cmdSocketClose, func() gonet.MessageInterface { return &socketCloseMsg{} }, a.onSocketClose); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, cmdKickDone, func() gonet.MessageInterface { return &kickDoneMsg{} }, a.onKickDone); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, cmdRetryLogin, func() gonet.MessageInterface { return &retryLoginMsg{} }, a.onRetryLogin); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, gonet.CmdResponse, func() gonet.MessageInterface { return &gonet.CallResponse{} }, a.onCallResponse)
}
