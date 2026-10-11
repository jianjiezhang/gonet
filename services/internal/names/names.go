// Package names 是业务 actor 的别名表。发消息时从这里取名字，不用 import 对方的 actor 包。
package names

import "gonet"

const (
	// Friend 是全服好友服务。全服只有一个，不加 nodeid。
	Friend = ".friend"
	// Guildmgr 是全服公会目录。
	Guildmgr = ".guildmgr"
	// Match 是全服房间目录。
	Match = ".match"
	// Idgen 是全服发号服务。
	Idgen = ".idgen"
	// Onlinemgr 是全服在线表。全服只有一个，不加 nodeid。
	Onlinemgr = ".onlinemgr"
)

const (
	rolePrefix  = "role/"
	guildPrefix = "guild/"
	roomPrefix  = "room/"
)

// Watchdog 是本节点接入服务的别名。nodeID 为 0 时为空。
func Watchdog(nodeID uint64) string {
	return gonet.ServiceAlias("watchdog", nodeID)
}

// Role 是一个玩家 actor 的别名。roleid 全集群唯一。
func Role(roleID string) string {
	if roleID == "" {
		return ""
	}
	return rolePrefix + roleID
}

// RoleID 从玩家别名取出 roleid。没有前缀时原样返回，方便当库存键。
func RoleID(alias string) string {
	if len(alias) >= len(rolePrefix) && alias[:len(rolePrefix)] == rolePrefix {
		return alias[len(rolePrefix):]
	}
	return alias
}

// Guild 是一个公会 actor 的别名。公会 id 全集群唯一。
func Guild(id string) string {
	if id == "" {
		return ""
	}
	return guildPrefix + id
}

// GuildID 从公会别名取出公会 id。没有前缀时为空。
func GuildID(alias string) string {
	if len(alias) > len(guildPrefix) && alias[:len(guildPrefix)] == guildPrefix {
		return alias[len(guildPrefix):]
	}
	return ""
}

// Room 是一场玩法场景的别名。房间号在目录创建时已经定下。
func Room(id string) string {
	if id == "" {
		return ""
	}
	return roomPrefix + id
}

// RoomID 从场景别名取出房间号。没有前缀时为空。
func RoomID(alias string) string {
	if len(alias) > len(roomPrefix) && alias[:len(roomPrefix)] == roomPrefix {
		return alias[len(roomPrefix):]
	}
	return ""
}
