package onlinemgr

import "gonet"

const (
	CmdLogin         = "online.login"
	CmdLogout        = "online.logout"
	CmdQuery         = "online.query"
	CmdBroadcastAll  = "online.broadcast.all"
	CmdBroadcastSome = "online.broadcast.some"
	CmdPush          = "online.push"
)

// LoginMsg 是 role 接上连接后发给 onlinemgr 的上线同步。
type LoginMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	Alias  string `json:"alias"`
	PID    uint64 `json:"pid"`
}

// LogoutMsg 是 role 退出时发给 onlinemgr 的下线同步。PID 对不上当前在线记录时忽略。
type LogoutMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	PID    uint64 `json:"pid"`
}

// QueryMsg 查询这个 roleid 是否在线。回复 bool。
type QueryMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

const CmdQueryBatch = "online.query.batch"

// QueryBatchMsg 一次查询一批 roleid。回复 QueryBatchReply。
type QueryBatchMsg struct {
	gonet.BaseMessage
	RoleIDs []string `json:"roleids"`
}

// QueryBatchReply 的 Online 对每个问过的 roleid 给出是否在线。
type QueryBatchReply struct {
	Online map[string]bool `json:"online"`
}

// BroadcastAllMsg 把一条客户端消息推给当前全部在线玩家。Data 是 payload 的 JSON 文本。回复实际入队的人数。
type BroadcastAllMsg struct {
	gonet.BaseMessage
	Cmd  string `json:"cmd"`
	Data string `json:"data"`
}

// BroadcastSomeMsg 只推给 roleids 里仍在线的玩家。回复实际入队的人数。
type BroadcastSomeMsg struct {
	gonet.BaseMessage
	RoleIDs []string `json:"roleids"`
	Cmd     string   `json:"cmd"`
	Data    string   `json:"data"`
}

// PushMsg 是 onlinemgr 转给 role 的待下发客户端消息。
type PushMsg struct {
	gonet.BaseMessage
	Cmd  string `json:"cmd"`
	Data string `json:"data"`
}
