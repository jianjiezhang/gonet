package actormsg

import "gonet/actor"

const CmdMonitor = "monitor"

// MonitorMsg 让 monitor 读花名册后 Reply。monitor 不向其它 actor 发 Call。
type MonitorMsg struct {
	actor.BaseMessage
}
