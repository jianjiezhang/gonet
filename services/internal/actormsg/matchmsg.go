// matchmsg.go 是房间目录和场景、玩家之间的消息。
package actormsg

import "gonet"

const (
	CmdMatchCreate  = "match.create"
	CmdMatchJoin    = "match.join"
	CmdMatchLeave   = "match.leave"
	CmdMatchStart   = "match.start"
	CmdMatchDone    = "match.done"
	CmdMatchRelease = "match.release"
	CmdMatchNotify  = "room.notify"
	CmdMatchView    = "match.view"

	// PhaseMatchWait 是等人。PhaseMatchPlay 是场景已经拉起。
	PhaseMatchWait = "wait"
	PhaseMatchPlay = "play"

	KindMatchJoin   = "join"
	KindMatchLeave  = "leave"
	KindMatchStart  = "start"
	KindMatchSettle = "settle"
)

// MatchCreateMsg 是创建一个等人的房间。RoleID 坐第一个座位。
type MatchCreateMsg struct {
	gonet.BaseMessage
	RoleID   string `json:"roleid"`
	Mode     int    `json:"mode"`
	Capacity int    `json:"capacity"`
}

// MatchJoinMsg 是按房间号加入。
type MatchJoinMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	RoomID string `json:"roomid"`
}

// MatchLeaveMsg 是离开还在等人的房间。
type MatchLeaveMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

// MatchStartMsg 是人满后开场。开场才拉起玩法场景。
type MatchStartMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

// MatchDoneMsg 是场景结束。目录忘掉这场，还在座的人可以再开下一场。
type MatchDoneMsg struct {
	gonet.BaseMessage
	RoomID string `json:"roomid"`
}

// MatchReleaseMsg 是开场后有人离开。目录放开这一个 roleid，场景还在。
type MatchReleaseMsg struct {
	gonet.BaseMessage
	RoomID string `json:"roomid"`
	RoleID string `json:"roleid"`
}

// MatchView 是目录的回复。Err 非空表示没做成。
type MatchView struct {
	gonet.BaseMessage
	Err      string   `json:"err,omitempty"`
	RoomID   string   `json:"roomid,omitempty"`
	Mode     int      `json:"mode,omitempty"`
	Capacity int      `json:"capacity,omitempty"`
	Seats    []string `json:"seats,omitempty"`
	Phase    string   `json:"phase,omitempty"`
}

// MatchNotifyMsg 推给 role，再由 role 写给客户端。
type MatchNotifyMsg struct {
	gonet.BaseMessage
	Kind     string   `json:"kind"`
	RoomID   string   `json:"roomid"`
	RoleID   string   `json:"roleid,omitempty"`
	Mode     int      `json:"mode,omitempty"`
	Capacity int      `json:"capacity,omitempty"`
	Seats    []string `json:"seats,omitempty"`
	Phase    string   `json:"phase,omitempty"`
}
