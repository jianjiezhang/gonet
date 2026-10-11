// roommsg.go 是玩法场景和玩家之间的消息，以及帧包里的位置和操作。
package actormsg

import "gonet"

const (
	CmdRoomSettle = "room.settle"
	CmdRoomLeave  = "room.leave"
	CmdRoomOp     = "room.op"
	CmdRoomDead   = "room.dead"
	CmdRoomTick   = "room.tick"
	CmdRoomBegin  = "room.begin"
	CmdRoomFrame  = "room.frame"
	CmdRoomResult = "room.result"
	CmdRoomReply  = "room.reply"
)

// RoomOp 是一个座位这一帧的操作。Ax/Ay 只能是 -1、0、1。
type RoomOp struct {
	Ax   int  `json:"ax"`
	Ay   int  `json:"ay"`
	Dash bool `json:"dash"`
}

// RoomZone 是场上固定区域。
type RoomZone struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	R    float64 `json:"r"`
	Kind string  `json:"kind"`
}

// RoomEvent 是这一帧服务器判定出来的事。
type RoomEvent struct {
	Kind   string  `json:"kind"`
	Index  int     `json:"index,omitempty"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Speed  float64 `json:"speed,omitempty"`
	Until  int     `json:"until,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

// RoomPlayerState 是服务器推的玩家位置。
type RoomPlayerState struct {
	RoleID string  `json:"roleid"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Alive  bool    `json:"alive"`
}

// RoomSettleMsg 是在座的人结束这场。空房间或对局中都可用来强行结束。
type RoomSettleMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

// RoomLeaveMsg 是开场之后离开座位。人走空则场景结束。
type RoomLeaveMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

// RoomOpMsg 是一个座位交给场景的下一帧操作。
type RoomOpMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	Ax     int    `json:"ax"`
	Ay     int    `json:"ay"`
	Dash   bool   `json:"dash"`
}

// RoomDeadMsg 是这个座位在某一帧被蛇咬到。Frame 不能大于已经广播的帧。
type RoomDeadMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	Frame  int    `json:"frame"`
}

// RoomTickMsg 是场景自己给自己的定帧。
type RoomTickMsg struct {
	gonet.BaseMessage
}

// RoomBeginMsg 推给 role：开局包。
type RoomBeginMsg struct {
	gonet.BaseMessage
	RoomID  string            `json:"roomid"`
	Mode    int               `json:"mode"`
	Seed    uint32            `json:"seed"`
	Seats   []string          `json:"seats"`
	Zones   []RoomZone        `json:"zones"`
	Players []RoomPlayerState `json:"players"`
	TargetX float64           `json:"targetx"`
	TargetY float64           `json:"targety"`
	Index   int               `json:"index"`
	Until   int               `json:"until"`
	Speed   float64           `json:"speed"`
	FrameHz int               `json:"framehz"`
	Targets int               `json:"targets"`
}

// RoomFrameMsg 推给 role：一帧操作和事件。
type RoomFrameMsg struct {
	gonet.BaseMessage
	RoomID  string            `json:"roomid"`
	Frame   int               `json:"frame"`
	Ops     []RoomOp          `json:"ops"`
	Events  []RoomEvent       `json:"events,omitempty"`
	Players []RoomPlayerState `json:"players"`
}

// RoomResultMsg 推给 role：这场结束。
type RoomResultMsg struct {
	gonet.BaseMessage
	RoomID string `json:"roomid"`
	Win    bool   `json:"win"`
	Reason string `json:"reason"`
	Index  int    `json:"index"`
	Frame  int    `json:"frame"`
}

// RoomReply 是场景的回复。Err 非空表示没做成。
type RoomReply struct {
	gonet.BaseMessage
	Err string `json:"err,omitempty"`
}
