package actormsg

import "gonet/actor"

const (
	CmdLauncherPing       = "ping"
	CmdLauncherNewService = "newservice"
)

// LauncherPing 是进程启动时探测 launcher 是否在岗。
type LauncherPing struct {
	actor.BaseMessage
}

// LauncherNewService 是本进程内的创建请求。Impl 不能序列化，调用方走 CallMemory。
type LauncherNewService struct {
	actor.BaseMessage
	Impl        actor.ActorContextInterface `json:"-"`
	ServiceName string                      `json:"-"`
}
