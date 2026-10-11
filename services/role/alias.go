package role

import "game/services/internal/names"

const AliasPrefix = "role/"

// Alias 是玩家在集群里的别名。roleid 全集群唯一，加上前缀后不和其他服务撞名。
func Alias(roleID string) string { return names.Role(roleID) }

// RoleID 从集群别名取出玩家 id。没有前缀时原样返回，方便当库存键。
func RoleID(alias string) string { return names.RoleID(alias) }
