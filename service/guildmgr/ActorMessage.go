package guildmgr

import "gonet"

const (
	CmdCreate  = "guild.mgr.create"
	CmdQuery   = "guild.mgr.query"
	CmdCheck   = "guild.mgr.check"
	CmdClaim   = "guild.mgr.claim"
	CmdRelease = "guild.mgr.release"
	CmdDrop    = "guild.mgr.disband"
	CmdJob     = "guild.mgr.job"
	CmdGuilds  = "guild.mgr.guilds"
	CmdGuildID = "guild.mgr.id"
	CmdByName  = "guild.mgr.name"
)

// CreateMsg 是创建一个公会。RoleID 成为会长。
type CreateMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	Name   string `json:"name"`
	Notice string `json:"notice,omitempty"`
}

// QueryMsg 问这个玩家在哪个公会。
type QueryMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

// IndexMsg 是成员变更：检查、占位、释放、解散。公会 actor 用同样的字段发过来。
type IndexMsg struct {
	gonet.BaseMessage
	GuildID string `json:"guildid"`
	RoleID  string `json:"roleid,omitempty"`
	Rank    int    `json:"rank,omitempty"`
	Time    int64  `json:"time,omitempty"`
}

// CatalogMsg 是查公会目录：全部、按 id，或按名字。
type CatalogMsg struct {
	gonet.BaseMessage
	GuildID string `json:"guildid,omitempty"`
	Name    string `json:"name,omitempty"`
}

// Card 是目录里的一条公会。Members 是人数。
type Card struct {
	GuildID string `json:"guildid"`
	Name    string `json:"name"`
	Notice  string `json:"notice"`
	Leader  string `json:"leader"`
	Members int    `json:"members"`
}

// CatalogReply 是目录查询的回复。
type CatalogReply struct {
	Err   string `json:"err,omitempty"`
	Cards []Card `json:"cards,omitempty"`
}

// Reply 是目录的回复。Err 非空表示没做成。
type Reply struct {
	Err     string `json:"err,omitempty"`
	GuildID string `json:"guildid,omitempty"`
	Name    string `json:"name,omitempty"`
	Notice  string `json:"notice,omitempty"`
	Leader  string `json:"leader,omitempty"`
}

type jobMsg struct {
	gonet.BaseMessage
	ID  uint64 `json:"-"`
	Err string `json:"-"`
}
