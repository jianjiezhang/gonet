package role

import (
	"net"

	"game/lib/net"
	"game/service/friend"
	"game/service/guild"
	"game/service/guildmgr"
	"game/service/onlinemgr"
	"game/store"
	"protocol/client"

	"gonet"
)

func init() {
	client.Bind(gamenet.RegisterCmd)
}

const cmdPersist = "persist"
const cmdHbCheck = "hb.check"
const cmdAttachConn = "conn.attach"
const cmdFriendReady = "friend.ready"
const cmdFriendProfiles = "friend.profiles"
const cmdGuildReady = "guild.ready"
const cmdGuildCards = "guild.cards"

type PingMsg struct {
	gonet.BaseMessage
}

type KickMsg struct {
	gonet.BaseMessage
}

type PersistMsg struct {
	gonet.BaseMessage
}

type HbCheckMsg struct {
	gonet.BaseMessage
}

// attachConnMsg 把客户端连接和这次接入编号交给 role。只能 SendMemory。
type attachConnMsg struct {
	gonet.BaseMessage
	Conn net.Conn `json:"-"`
	Link uint64   `json:"-"`
}

// friendReadyMsg 是申请前查过目标玩家是否存在之后送回 mailbox 的结果。只能 SendMemory。
type friendReadyMsg struct {
	gonet.BaseMessage
	Link   uint64 `json:"-"`
	Target string `json:"-"`
	Name   string `json:"-"`
	OK     bool   `json:"-"`
	Err    error  `json:"-"`
}

// friendProfilesMsg 是列表补名字时读完库送回 mailbox 的结果。只能 SendMemory。
type friendProfilesMsg struct {
	gonet.BaseMessage
	ID   uint64                   `json:"-"`
	Rows map[string]store.RoleRow `json:"-"`
}

// guildCardsMsg 是公会目录补完会长名字和在线后送回 mailbox 的结果。只能 SendMemory。
type guildCardsMsg struct {
	gonet.BaseMessage
	Link   uint64
	Op     string
	Cards  []guildmgr.Card
	Rows   map[string]store.RoleRow
	Online map[string]bool
}

// guildReadyMsg 是公会列表补完名字和在线后送回 mailbox 的结果。只能 SendMemory。
type guildReadyMsg struct {
	gonet.BaseMessage
	Link   uint64
	View   *guild.Reply
	Rows   map[string]store.RoleRow
	Online map[string]bool
}

func (a *Actor) RegisterCmds() error {
	if err := a.registerActorCmds(); err != nil {
		return err
	}
	return a.registerClientCmds()
}

func (a *Actor) registerActorCmds() error {
	if err := gonet.RegisterCmd(a, client.Ping, func() gonet.MessageInterface { return &PingMsg{} }, a.onPing); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.Kick, func() gonet.MessageInterface { return &KickMsg{} }, a.onKick); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, cmdHbCheck, func() gonet.MessageInterface { return &HbCheckMsg{} }, a.onHbCheck); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, cmdPersist, func() gonet.MessageInterface { return &PersistMsg{} }, a.onPersist); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, cmdAttachConn, func() gonet.MessageInterface { return &attachConnMsg{} }, a.onAttachConn); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, onlinemgr.CmdPush, func() gonet.MessageInterface { return &onlinemgr.PushMsg{} }, a.onOnlinePush); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, gonet.CmdResponse, func() gonet.MessageInterface { return &gonet.CallResponse{} }, a.onCallResponse); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, cmdFriendReady, func() gonet.MessageInterface { return &friendReadyMsg{} }, a.onFriendReady); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, cmdFriendProfiles, func() gonet.MessageInterface { return &friendProfilesMsg{} }, a.onFriendProfiles); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, friend.CmdNotify, func() gonet.MessageInterface { return &friend.NotifyMsg{} }, a.onFriendNotify); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, guild.CmdNotify, func() gonet.MessageInterface { return &guild.NotifyMsg{} }, a.onGuildNotify); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, cmdGuildReady, func() gonet.MessageInterface { return &guildReadyMsg{} }, a.onGuildReady); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, cmdGuildCards, func() gonet.MessageInterface { return &guildCardsMsg{} }, a.onGuildCards)
}

// connLink 是这一次接入的编号。读循环写进消息，role 内存里的 link 相同才处理。
type connLink struct {
	Link uint64 `json:"link,omitempty"`
}

func (c *connLink) SetLink(id uint64) {
	if c != nil {
		c.Link = id
	}
}

func (c connLink) LinkID() uint64 { return c.Link }

type HeartbeatMsg struct {
	gonet.BaseMessage
	connLink
}

type MissionListMsg struct {
	gonet.BaseMessage
	connLink
}

type MissionFinishMsg struct {
	gonet.BaseMessage
	connLink
	client.MissionFinishReq
}

type RoleInfoMsg struct {
	gonet.BaseMessage
	connLink
}

type SetLevelMsg struct {
	gonet.BaseMessage
	connLink
	client.SetLevelReq
}

type FriendListMsg struct {
	gonet.BaseMessage
	connLink
}

type FriendTargetMsg struct {
	gonet.BaseMessage
	connLink
	client.FriendOpReq
}

type guildCreateMsg struct {
	gonet.BaseMessage
	connLink
	client.GuildCreateReq
}

type guildPlainMsg struct {
	gonet.BaseMessage
	connLink
}

type guildApplyMsg struct {
	gonet.BaseMessage
	connLink
	client.GuildApplyReq
}

type guildTargetMsg struct {
	gonet.BaseMessage
	connLink
	client.GuildTargetReq
}

type guildIDMsg struct {
	gonet.BaseMessage
	connLink
	client.GuildIDReq
}

type guildNameMsg struct {
	gonet.BaseMessage
	connLink
	client.GuildNameReq
}

func (a *Actor) registerClientCmds() error {
	if err := gonet.RegisterCmd(a, client.Heartbeat, func() gonet.MessageInterface { return &HeartbeatMsg{} }, a.onHeartbeat); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.MissionList, func() gonet.MessageInterface { return &MissionListMsg{} }, a.onMissionList); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.MissionFinish, func() gonet.MessageInterface { return &MissionFinishMsg{} }, a.onMissionFinish); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.RoleInfo, func() gonet.MessageInterface { return &RoleInfoMsg{} }, a.onRoleInfo); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.SetLevel, func() gonet.MessageInterface { return &SetLevelMsg{} }, a.onSetLevel); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.FriendList, func() gonet.MessageInterface { return &FriendListMsg{} }, a.onFriendList); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.FriendApply, func() gonet.MessageInterface { return &FriendTargetMsg{} }, a.onFriendApply); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.FriendAgree, func() gonet.MessageInterface { return &FriendTargetMsg{} }, a.onFriendAgree); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.FriendReject, func() gonet.MessageInterface { return &FriendTargetMsg{} }, a.onFriendReject); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.FriendDelete, func() gonet.MessageInterface { return &FriendTargetMsg{} }, a.onFriendDelete); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.GuildCreate, func() gonet.MessageInterface { return &guildCreateMsg{} }, a.onGuildCreate); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.GuildList, func() gonet.MessageInterface { return &guildPlainMsg{} }, a.onGuildList); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.GuildApply, func() gonet.MessageInterface { return &guildApplyMsg{} }, a.onGuildApply); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.GuildAgree, func() gonet.MessageInterface { return &guildTargetMsg{} }, a.onGuildAgree); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.GuildReject, func() gonet.MessageInterface { return &guildTargetMsg{} }, a.onGuildReject); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.GuildLeave, func() gonet.MessageInterface { return &guildPlainMsg{} }, a.onGuildLeave); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.GuildKick, func() gonet.MessageInterface { return &guildTargetMsg{} }, a.onGuildKick); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.GuildDisband, func() gonet.MessageInterface { return &guildPlainMsg{} }, a.onGuildDisband); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.Guilds, func() gonet.MessageInterface { return &guildPlainMsg{} }, a.onGuilds); err != nil {
		return err
	}
	if err := gonet.RegisterCmd(a, client.GuildID, func() gonet.MessageInterface { return &guildIDMsg{} }, a.onGuildID); err != nil {
		return err
	}
	return gonet.RegisterCmd(a, client.GuildName, func() gonet.MessageInterface { return &guildNameMsg{} }, a.onGuildName)
}
