// guildmgrmsg.go 是公会目录和公会、玩家之间的消息。
package actormsg

import "gonet"

const (
	CmdGuildmgrCreate       = "guild.mgr.create"
	CmdGuildmgrQuery        = "guild.mgr.query"
	CmdGuildmgrCheck        = "guild.mgr.check"
	CmdGuildmgrClaim        = "guild.mgr.claim"
	CmdGuildmgrRelease      = "guild.mgr.release"
	CmdGuildmgrDrop         = "guild.mgr.disband"
	CmdGuildmgrJob          = "guild.mgr.job"
	CmdGuildmgrGuilds       = "guild.mgr.guilds"
	CmdGuildmgrGuildID      = "guild.mgr.id"
	CmdGuildmgrByName       = "guild.mgr.name"
	CmdGuildmgrReply        = "guild.mgr.reply"
	CmdGuildmgrCatalogReply = "guild.mgr.catalog"
)

// GuildmgrCreateMsg 是创建一个公会。RoleID 成为会长。
type GuildmgrCreateMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
	Name   string `json:"name"`
	Notice string `json:"notice,omitempty"`
}

// GuildmgrQueryMsg 问这个玩家在哪个公会。
type GuildmgrQueryMsg struct {
	gonet.BaseMessage
	RoleID string `json:"roleid"`
}

// GuildmgrIndexMsg 是成员变更：检查、占位、释放、解散。公会 actor 用同样的字段发过来。
type GuildmgrIndexMsg struct {
	gonet.BaseMessage
	GuildID string `json:"guildid"`
	RoleID  string `json:"roleid,omitempty"`
	Rank    int    `json:"rank,omitempty"`
	Time    int64  `json:"time,omitempty"`
}

// GuildmgrCatalogMsg 是查公会目录：全部、按 id，或按名字。
type GuildmgrCatalogMsg struct {
	gonet.BaseMessage
	GuildID string `json:"guildid,omitempty"`
	Name    string `json:"name,omitempty"`
}

// GuildmgrCard 是目录里的一条公会。Members 是人数。
type GuildmgrCard struct {
	GuildID string `json:"guildid"`
	Name    string `json:"name"`
	Notice  string `json:"notice"`
	Leader  string `json:"leader"`
	Members int    `json:"members"`
}

// GuildmgrCatalogReply 是目录查询的回复。
type GuildmgrCatalogReply struct {
	gonet.BaseMessage
	Err   string         `json:"err,omitempty"`
	Cards []GuildmgrCard `json:"cards,omitempty"`
}

// GuildmgrReply 是目录的回复。Err 非空表示没做成。
type GuildmgrReply struct {
	gonet.BaseMessage
	Err     string `json:"err,omitempty"`
	GuildID string `json:"guildid,omitempty"`
	Name    string `json:"name,omitempty"`
	Notice  string `json:"notice,omitempty"`
	Leader  string `json:"leader,omitempty"`
}
