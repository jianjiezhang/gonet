package client

// 玩家 TCP 的命令名和命令号。正文结构、组包和解包在 msg.go。
// 服务内部用这些名字，线上用对应的数字。
const (
	Login         = "login"
	Kick          = "kick"
	Ping          = "ping"
	Pong          = "pong"
	Heartbeat     = "heartbeat"
	MissionList   = "missionlist"
	MissionFinish = "missionfinish"
	RoleInfo      = "roleinfo"
	SetLevel      = "setlevel"
	FriendList    = "friendlist"
	FriendApply   = "friendapply"
	FriendAgree   = "friendagree"
	FriendReject  = "friendreject"
	FriendDelete  = "frienddelete"
	FriendNotify  = "friendnotify"
	GuildCreate   = "guildcreate"
	GuildList     = "guildlist"
	GuildApply    = "guildapply"
	GuildAgree    = "guildagree"
	GuildReject   = "guildreject"
	GuildLeave    = "guildleave"
	GuildKick     = "guildkick"
	GuildDisband  = "guilddisband"
	GuildNotify   = "guildnotify"
	Guilds        = "guilds"
	GuildID       = "guildid"
	GuildName     = "guildname"
)

const (
	LoginID         uint16 = 1
	KickID          uint16 = 2
	PingID          uint16 = 3
	PongID          uint16 = 4
	HeartbeatID     uint16 = 5
	MissionListID   uint16 = 6
	MissionFinishID uint16 = 7
	RoleInfoID      uint16 = 8
	SetLevelID      uint16 = 9
	FriendListID    uint16 = 10
	FriendApplyID   uint16 = 11
	FriendAgreeID   uint16 = 12
	FriendRejectID  uint16 = 13
	FriendDeleteID  uint16 = 14
	FriendNotifyID  uint16 = 15
	GuildCreateID   uint16 = 16
	GuildListID     uint16 = 17
	GuildApplyID    uint16 = 18
	GuildAgreeID    uint16 = 19
	GuildRejectID   uint16 = 20
	GuildLeaveID    uint16 = 21
	GuildKickID     uint16 = 22
	GuildDisbandID  uint16 = 23
	GuildNotifyID   uint16 = 24
	GuildsID        uint16 = 25
	GuildIDID       uint16 = 26
	GuildNameID     uint16 = 27
)

// Bind 把命令名和命令号交给收发层。同一对可以重复登记。
func Bind(register func(name string, id uint16) error) {
	pairs := []struct {
		name string
		id   uint16
	}{
		{Login, LoginID},
		{Kick, KickID},
		{Ping, PingID},
		{Pong, PongID},
		{Heartbeat, HeartbeatID},
		{MissionList, MissionListID},
		{MissionFinish, MissionFinishID},
		{RoleInfo, RoleInfoID},
		{SetLevel, SetLevelID},
		{FriendList, FriendListID},
		{FriendApply, FriendApplyID},
		{FriendAgree, FriendAgreeID},
		{FriendReject, FriendRejectID},
		{FriendDelete, FriendDeleteID},
		{FriendNotify, FriendNotifyID},
		{GuildCreate, GuildCreateID},
		{GuildList, GuildListID},
		{GuildApply, GuildApplyID},
		{GuildAgree, GuildAgreeID},
		{GuildReject, GuildRejectID},
		{GuildLeave, GuildLeaveID},
		{GuildKick, GuildKickID},
		{GuildDisband, GuildDisbandID},
		{GuildNotify, GuildNotifyID},
		{Guilds, GuildsID},
		{GuildID, GuildIDID},
		{GuildName, GuildNameID},
	}
	for _, p := range pairs {
		if err := register(p.name, p.id); err != nil {
			panic(err)
		}
	}
}
