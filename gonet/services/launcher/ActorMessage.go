package launcher

import "gonet"

const cmdPing = "ping"
const cmdNewService = "newservice"

type PingMsg struct {
	gonet.BaseMessage
}

// newServiceMsg 本进程内请求：Impl 不能序列化，调用方走 CallMemory。
type newServiceMsg struct {
	gonet.BaseMessage
	Impl        gonet.ActorContextInterface `json:"-"`
	ServiceName string                      `json:"-"`
}

func (a *Actor) RegisterCmds() error {
	if err := gonet.RegisterCmd(a, cmdPing, func() gonet.MessageInterface { return &PingMsg{} }, a.onPing); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, gonet.CmdResponse, func() gonet.MessageInterface { return &gonet.CallResponse{} }, a.onCallResponse); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, cmdNewService, func() gonet.MessageInterface { return &newServiceMsg{} }, a.onNewService)
}
