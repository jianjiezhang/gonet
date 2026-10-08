package room

import "gonet"

const (
	CmdSettle = "room.settle"
	CmdLeave  = "room.leave"
	CmdOp     = "room.op"
	CmdDead   = "room.dead"
	CmdTick   = "room.tick"
	CmdBegin  = "room.begin"
	CmdFrame  = "room.frame"
	CmdResult = "room.result"

	aliasPrefix = "room/"
)

// Alias 是一场玩法场景的别名。房间号在目录创建时已经定下。
func Alias(id string) string {
	if id == "" {
		return ""
	}
	return aliasPrefix + id
}

// ID 从场景别名取出房间号。
func ID(alias string) string {
	if len(alias) > len(aliasPrefix) && alias[:len(aliasPrefix)] == aliasPrefix {
		return alias[len(aliasPrefix):]
	}
	return ""
}

// SettleMsg 是在座的人结束这场。空房间或对局中都可用来强行结束。
type SettleMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

// LeaveMsg 是开场之后离开座位。人走空则场景结束。
type LeaveMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

// OpMsg 是一个座位交给场景的下一帧操作。
type OpMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	Ax     int    `json:"ax"`
	Ay     int    `json:"ay"`
	Dash   bool   `json:"dash"`
}

// DeadMsg 是这个座位在某一帧被蛇咬到。Frame 不能大于已经广播的帧。
type DeadMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	Frame  int    `json:"frame"`
}

// TickMsg 是场景自己给自己的定帧。
type TickMsg struct {
	gonet.BaseMessage
}

// BeginMsg 推给 role：开局包。
type BeginMsg struct {
	gonet.BaseMessage
	RoomID  string        `json:"roomid"`
	Mode    int           `json:"mode"`
	Seed    uint32        `json:"seed"`
	Seats   []string      `json:"seats"`
	Zones   []Zone        `json:"zones"`
	Players []PlayerState `json:"players"`
	TargetX float64       `json:"targetx"`
	TargetY float64       `json:"targety"`
	Index   int           `json:"index"`
	Until   int           `json:"until"`
	Speed   float64       `json:"speed"`
	FrameHz int           `json:"framehz"`
	Targets int           `json:"targets"`
}

// FrameMsg 推给 role：一帧操作和事件。
type FrameMsg struct {
	gonet.BaseMessage
	RoomID  string        `json:"roomid"`
	Frame   int           `json:"frame"`
	Ops     []Op          `json:"ops"`
	Events  []Event       `json:"events,omitempty"`
	Players []PlayerState `json:"players"`
}

// ResultMsg 推给 role：这场结束。
type ResultMsg struct {
	gonet.BaseMessage
	RoomID string `json:"roomid"`
	Win    bool   `json:"win"`
	Reason string `json:"reason"`
	Index  int    `json:"index"`
	Frame  int    `json:"frame"`
}

// Reply 是场景的回复。Err 非空表示没做成。
type Reply struct {
	Err string `json:"err,omitempty"`
}
