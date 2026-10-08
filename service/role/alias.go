package role

const AliasPrefix = "role/"

// Alias 是玩家在集群里的别名。roleid 全集群唯一，加上前缀后不和其他服务撞名。
func Alias(roleID string) string {
	if roleID == "" {
		return ""
	}
	return AliasPrefix + roleID
}

// RoleID 从集群别名取出玩家 id。没有前缀时原样返回，方便当库存键。
func RoleID(alias string) string {
	if len(alias) >= len(AliasPrefix) && alias[:len(AliasPrefix)] == AliasPrefix {
		return alias[len(AliasPrefix):]
	}
	return alias
}
