package match

import "gonet"

const (
	CmdCreate  = "match.create"
	CmdJoin    = "match.join"
	CmdLeave   = "match.leave"
	CmdStart   = "match.start"
	CmdDone    = "match.done"
	CmdRelease = "match.release"
	CmdNotify  = "room.notify"

	// PhaseWait 是等人。PhasePlay 是场景已经拉起。
	PhaseWait = "wait"
	PhasePlay = "play"

	KindJoin   = "join"
	KindLeave  = "leave"
	KindStart  = "start"
	KindSettle = "settle"

	phaseStarting = "starting"
	maxCapacity   = 8
)

// CreateMsg 是创建一个等人的房间。RoleID 坐第一个座位。
type CreateMsg struct {
	gonet.BaseMessage
	RoleID   string `json:"roleid"`
	Mode     int    `json:"mode"`
	Capacity int    `json:"capacity"`
}

// JoinMsg 是按房间号加入。
type JoinMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	RoomID string `json:"roomid"`
}

// LeaveMsg 是离开还在等人的房间。
type LeaveMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

// StartMsg 是人满后开场。开场才拉起玩法场景。
type StartMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

// DoneMsg 是场景结束。目录忘掉这场，还在座的人可以再开下一场。
type DoneMsg struct {
	gonet.BaseMessage
	RoomID string `json:"roomid"`
}

// ReleaseMsg 是开场后有人离开。目录放开这一个 roleid，场景还在。
type ReleaseMsg struct {
	gonet.BaseMessage
	RoomID string `json:"roomid"`
	RoleID string `json:"roleid"`
}

// View 是目录的回复。Err 非空表示没做成。
type View struct {
	Err      string   `json:"err,omitempty"`
	RoomID   string   `json:"roomid,omitempty"`
	Mode     int      `json:"mode,omitempty"`
	Capacity int      `json:"capacity,omitempty"`
	Seats    []string `json:"seats,omitempty"`
	Phase    string   `json:"phase,omitempty"`
}

// NotifyMsg 推给 role，再由 role 写给客户端。
type NotifyMsg struct {
	gonet.BaseMessage
	Kind     string   `json:"kind"`
	RoomID   string   `json:"roomid"`
	RoleID   string   `json:"roleid,omitempty"`
	Mode     int      `json:"mode,omitempty"`
	Capacity int      `json:"capacity,omitempty"`
	Seats    []string `json:"seats,omitempty"`
	Phase    string   `json:"phase,omitempty"`
}
