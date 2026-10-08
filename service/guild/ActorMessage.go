package guild

import "gonet"

const (
	CmdApply   = "guild.apply"
	CmdAgree   = "guild.agree"
	CmdReject  = "guild.reject"
	CmdLeave   = "guild.leave"
	CmdKick    = "guild.kick"
	CmdList    = "guild.list"
	CmdDisband = "guild.disband"
	CmdNotify  = "guild.notify"
)

const aliasPrefix = "guild/"

// Alias 是一个公会 actor 的别名。公会 id 全集群唯一。
func Alias(id string) string {
	if id == "" {
		return ""
	}
	return aliasPrefix + id
}

// ID 从公会 actor 别名取出公会 id。
func ID(alias string) string {
	if len(alias) > len(aliasPrefix) && alias[:len(aliasPrefix)] == aliasPrefix {
		return alias[len(aliasPrefix):]
	}
	return ""
}

func roleAlias(roleID string) string {
	if roleID == "" {
		return ""
	}
	return "role/" + roleID
}

// OpMsg 是一次公会操作。RoleID 是操作者，Target 是对方。
type OpMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	Target string `json:"target,omitempty"`
	Name   string `json:"name,omitempty"`
}

// Member 是回复里的一个成员。
type Member struct {
	RoleID string `json:"roleid"`
	Rank   int    `json:"rank"`
	Time   int64  `json:"time"`
}

// Apply 是一条入会申请。
type Apply struct {
	RoleID string `json:"roleid"`
	Time   int64  `json:"time"`
}

// Reply 是公会 actor 的回复。Err 非空表示没做成。
type Reply struct {
	Err     string   `json:"err,omitempty"`
	GuildID string   `json:"guildid,omitempty"`
	Name    string   `json:"name,omitempty"`
	Notice  string   `json:"notice,omitempty"`
	Leader  string   `json:"leader,omitempty"`
	Members []Member `json:"members,omitempty"`
	Applies []Apply  `json:"applies,omitempty"`
}

// NotifyMsg 推给 role，再由 role 写给客户端。
type NotifyMsg struct {
	gonet.BaseMessage
	Kind    string `json:"kind"`
	GuildID string `json:"guildid,omitempty"`
	RoleID  string `json:"roleid,omitempty"`
	Name    string `json:"name,omitempty"`
	Time    int64  `json:"time,omitempty"`
}
