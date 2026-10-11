// guildmsg.go 是单个公会 actor 和玩家之间的消息。
package actormsg

import "gonet"

const (
	CmdGuildApply   = "guild.apply"
	CmdGuildAgree   = "guild.agree"
	CmdGuildReject  = "guild.reject"
	CmdGuildLeave   = "guild.leave"
	CmdGuildKick    = "guild.kick"
	CmdGuildList    = "guild.list"
	CmdGuildDisband = "guild.disband"
	CmdGuildNotify  = "guild.notify"
	CmdGuildReply   = "guild.reply"
)

// GuildOpMsg 是一次公会操作。RoleID 是操作者，Target 是对方。
type GuildOpMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	Target string `json:"target,omitempty"`
	Name   string `json:"name,omitempty"`
}

// GuildMember 是回复里的一个成员。
type GuildMember struct {
	RoleID string `json:"roleid"`
	Rank   int    `json:"rank"`
	Time   int64  `json:"time"`
}

// GuildApply 是一条入会申请。
type GuildApply struct {
	RoleID string `json:"roleid"`
	Time   int64  `json:"time"`
}

// GuildReply 是公会 actor 的回复。Err 非空表示没做成。
type GuildReply struct {
	gonet.BaseMessage
	Err     string        `json:"err,omitempty"`
	GuildID string        `json:"guildid,omitempty"`
	Name    string        `json:"name,omitempty"`
	Notice  string        `json:"notice,omitempty"`
	Leader  string        `json:"leader,omitempty"`
	Members []GuildMember `json:"members,omitempty"`
	Applies []GuildApply  `json:"applies,omitempty"`
}

// GuildNotifyMsg 推给 role，再由 role 写给客户端。
type GuildNotifyMsg struct {
	gonet.BaseMessage
	Kind    string `json:"kind"`
	GuildID string `json:"guildid,omitempty"`
	RoleID  string `json:"roleid,omitempty"`
	Name    string `json:"name,omitempty"`
	Time    int64  `json:"time,omitempty"`
}
