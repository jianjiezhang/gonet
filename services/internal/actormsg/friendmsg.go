// friendmsg.go 是好友服务和玩家之间的消息。
package actormsg

import "gonet"

const (
	CmdFriendApply  = "friend.apply"
	CmdFriendAgree  = "friend.agree"
	CmdFriendReject = "friend.reject"
	CmdFriendDelete = "friend.delete"
	CmdFriendList   = "friend.list"
	CmdFriendNotify = "friend.notify"
	CmdFriendView   = "friend.view"

	KindFriendApply  = "apply"
	KindFriendAgree  = "agree"
	KindFriendReject = "reject"
	KindFriendDelete = "delete"
)

// FriendApplyMsg 是 from 向 to 发起申请。FromName 只用于通知，不入库。
type FriendApplyMsg struct {
	gonet.BaseMessage
	From     string `json:"from"`
	To       string `json:"to"`
	FromName string `json:"from_name"`
}

// FriendDecideMsg 是 self 同意或拒绝 from 的申请。
type FriendDecideMsg struct {
	gonet.BaseMessage
	Self     string `json:"self"`
	From     string `json:"from"`
	SelfName string `json:"self_name"`
}

// FriendDeleteMsg 是 self 删除 target。
type FriendDeleteMsg struct {
	gonet.BaseMessage
	Self     string `json:"self"`
	Target   string `json:"target"`
	SelfName string `json:"self_name"`
}

// FriendListMsg 拉取 self 的好友和申请。
type FriendListMsg struct {
	gonet.BaseMessage
	Self string `json:"self"`
}

// FriendReq 是一条申请。
type FriendReq struct {
	RoleID string `json:"roleid"`
	Time   int64  `json:"time"`
}

// FriendView 是好友服务的回复。Err 非空表示这次操作没做成。
type FriendView struct {
	gonet.BaseMessage
	Err      string      `json:"err,omitempty"`
	Friends  []string    `json:"friends,omitempty"`
	Incoming []FriendReq `json:"incoming,omitempty"`
	Outgoing []FriendReq `json:"outgoing,omitempty"`
}

// FriendNotifyMsg 推给对方 role，再由 role 写给客户端。
type FriendNotifyMsg struct {
	gonet.BaseMessage
	Kind   string `json:"kind"`
	RoleID string `json:"roleid"`
	Name   string `json:"name,omitempty"`
	Time   int64  `json:"time,omitempty"`
}
