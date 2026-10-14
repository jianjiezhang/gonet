// onlinemsg.go 是在线表和玩家之间的消息。
package actormsg

import "gonet"

const (
	CmdOnlineLogin           = "online.login"
	CmdOnlineLogout          = "online.logout"
	CmdOnlineQuery           = "online.query"
	CmdOnlineQueryBatch      = "online.query.batch"
	CmdOnlineBroadcastAll    = "online.broadcast.all"
	CmdOnlineBroadcastSome   = "online.broadcast.some"
	CmdOnlinePush            = "online.push"
	CmdOnlineQueryBatchReply = "online.query.batch.reply"
)

// OnlineLoginMsg 是 role 接上连接后发给 onlinemgr 的上线同步。
type OnlineLoginMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	Alias  string `json:"alias"`
	PID    uint64 `json:"pid"`
}

// OnlineLogoutMsg 是 role 退出时发给 onlinemgr 的下线同步。PID 对不上当前在线记录时忽略。
type OnlineLogoutMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	PID    uint64 `json:"pid"`
}

// OnlineQueryMsg 查询这个 roleid 是否在线。回复 bool。
type OnlineQueryMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

// OnlineQueryBatchMsg 一次查询一批 roleid。回复 OnlineQueryBatchReply。
type OnlineQueryBatchMsg struct {
	gonet.BaseMessage
	RoleIDs []string `json:"roleids"`
}

// OnlineQueryBatchReply 的 Online 对每个问过的 roleid 给出是否在线。
type OnlineQueryBatchReply struct {
	gonet.BaseMessage
	Online map[string]bool `json:"online"`
}

// OnlineBroadcastAllMsg 把一条客户端消息推给当前全部在线玩家。Data 是 payload 的 JSON 文本。回复实际入队的人数。
type OnlineBroadcastAllMsg struct {
	gonet.BaseMessage
	Cmd  string `json:"cmd"`
	Data string `json:"data"`
}

// OnlineBroadcastSomeMsg 只推给 roleids 里仍在线的玩家。回复实际入队的人数。
type OnlineBroadcastSomeMsg struct {
	gonet.BaseMessage
	RoleIDs []string `json:"roleids"`
	Cmd     string   `json:"cmd"`
	Data    string   `json:"data"`
}

// OnlinePushMsg 是 onlinemgr 转给 role 的待下发客户端消息。
type OnlinePushMsg struct {
	gonet.BaseMessage
	Cmd  string `json:"cmd"`
	Data string `json:"data"`
}
