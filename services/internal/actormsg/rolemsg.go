package actormsg

import "gonet"

// RoleKickMsg 是接入服务让这个玩家下线。
type RoleKickMsg struct {
	gonet.BaseMessage
}
