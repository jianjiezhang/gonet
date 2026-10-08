package friend

import "gonet"

const (
	CmdApply  = "friend.apply"
	CmdAgree  = "friend.agree"
	CmdReject = "friend.reject"
	CmdDelete = "friend.delete"
	CmdList   = "friend.list"
	CmdNotify = "friend.notify"

	KindApply  = "apply"
	KindAgree  = "agree"
	KindReject = "reject"
	KindDelete = "delete"
)

// MaxFriends 是一方好友人数上限。
const MaxFriends = 50

// friendLimit 是实际用来判断的上限。测试可以临时改小。
var friendLimit = MaxFriends

// ApplyMsg 是 from 向 to 发起申请。FromName 只用于通知，不入库。
type ApplyMsg struct {
	gonet.BaseMessage
	From     string `json:"from"`
	To       string `json:"to"`
	FromName string `json:"from_name"`
}

// DecideMsg 是 self 同意或拒绝 from 的申请。
type DecideMsg struct {
	gonet.BaseMessage
	Self     string `json:"self"`
	From     string `json:"from"`
	SelfName string `json:"self_name"`
}

// DeleteMsg 是 self 删除 target。
type DeleteMsg struct {
	gonet.BaseMessage
	Self     string `json:"self"`
	Target   string `json:"target"`
	SelfName string `json:"self_name"`
}

// ListMsg 拉取 self 的好友和申请。
type ListMsg struct {
	gonet.BaseMessage
	Self string `json:"self"`
}

// Req 是一条申请。
type Req struct {
	RoleID string `json:"roleid"`
	Time   int64  `json:"time"`
}

// View 是好友服务的回复。Err 非空表示这次操作没做成。
type View struct {
	Err      string   `json:"err,omitempty"`
	Friends  []string `json:"friends,omitempty"`
	Incoming []Req    `json:"incoming,omitempty"`
	Outgoing []Req    `json:"outgoing,omitempty"`
}

// NotifyMsg 推给对方 role，再由 role 写给客户端。
type NotifyMsg struct {
	gonet.BaseMessage
	Kind   string `json:"kind"`
	RoleID string `json:"roleid"`
	Name   string `json:"name,omitempty"`
	Time   int64  `json:"time,omitempty"`
}

// RoleAlias 是玩家 actor 的别名，和 role.Alias 保持一致。
func RoleAlias(roleID string) string {
	if roleID == "" {
		return ""
	}
	return "role/" + roleID
}
