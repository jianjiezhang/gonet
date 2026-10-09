package store

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"game/config"
)

var (
	ErrEmptyAlias = errors.New("store: roleid 不能为空")
	ErrNotFound   = errors.New("store: 不存在")
	ErrExists     = errors.New("store: 已存在")
)

// RoleRow 是 role 表的一行，和 BaseData 对齐。store 不引用 role 包，避免循环依赖。
type RoleRow struct {
	RoleID         string
	Level          int
	Name           string
	Gender         int
	LastLoginTime  int64 // unix 秒，0 表示还没有登录过
	LastLogoutTime int64 // unix 秒，0 表示还没有下线过
}

func NewDefaultRow(roleid string) RoleRow {
	return RoleRow{
		RoleID: roleid,
		Level:  1,
		Name:   roleid,
		Gender: 0,
	}
}

// MissionBlob 是 t_mission 一行。Data 是该玩家全部任务进度的 JSON。
type MissionBlob struct {
	RoleID string
	Data   []byte
}

func EmptyMissionJSON() []byte {
	return []byte(`{"list":[]}`)
}

// FriendEdge 是一条单向好友边。双方各存一行。
type FriendEdge struct {
	RoleID   string
	FriendID string
}

// FriendRequest 是一条还没处理的申请。Time 是 unix 秒。
type FriendRequest struct {
	FromID string
	ToID   string
	Time   int64
}

type Store interface {
	HasRole(ctx context.Context, roleid string) (bool, error)
	CreateRole(ctx context.Context, roleid string) error
	LoadRole(ctx context.Context, roleid string) (RoleRow, error)
	SaveRole(ctx context.Context, row RoleRow) error
	LoadMission(ctx context.Context, roleid string) (MissionBlob, error)
	SaveMission(ctx context.Context, blob MissionBlob) error
	LoadPlayer(ctx context.Context, roleid string) (RoleRow, MissionBlob, error)
	SavePlayer(ctx context.Context, row RoleRow, blob MissionBlob) error
	LoadFriends(ctx context.Context) ([]FriendEdge, error)
	LoadFriendRequests(ctx context.Context) ([]FriendRequest, error)
	AddFriends(ctx context.Context, a, b string) error
	RemoveFriends(ctx context.Context, a, b string) error
	AcceptFriend(ctx context.Context, self, from string) error
	AddFriendRequest(ctx context.Context, req FriendRequest) error
	RemoveFriendRequest(ctx context.Context, fromID, toID string) error
	LoadGuilds(ctx context.Context) ([]GuildRow, error)
	LoadGuildMembers(ctx context.Context) ([]GuildMember, error)
	LoadGuild(ctx context.Context, id string) (GuildRow, []GuildMember, []GuildApply, error)
	CreateGuild(ctx context.Context, row GuildRow, leader GuildMember) error
	AddGuildMember(ctx context.Context, m GuildMember) error
	RemoveGuildMember(ctx context.Context, roleID string) error
	AddGuildApply(ctx context.Context, a GuildApply) error
	RemoveGuildApply(ctx context.Context, guildID, roleID string) error
	DeleteGuild(ctx context.Context, id string) error
	Close() error
}

// GuildRow 是一个公会。
type GuildRow struct {
	ID      string
	Name    string
	Leader  string
	Notice  string
	Created int64
}

// GuildMember 是公会成员。RoleID 主键，一个人只能在一个公会。
type GuildMember struct {
	RoleID  string
	GuildID string
	Rank    int
	Time    int64
}

// GuildApply 是还没处理的入会申请。
type GuildApply struct {
	GuildID string
	RoleID  string
	Time    int64
}

var (
	currentMu sync.RWMutex
	current   Store
)

// Set 装上本进程唯一的存储。Open 会自己装上。测试换内存实现时调用。
func Set(s Store) {
	currentMu.Lock()
	current = s
	currentMu.Unlock()
}

// Get 返回本进程的存储。还没装上时为 nil。
func Get() Store {
	currentMu.RLock()
	defer currentMu.RUnlock()
	return current
}

func Open(cfg config.Config) (Store, error) {
	s, err := openMySQL(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("store: 打开 MySQL: %w", err)
	}
	Set(s)
	return s, nil
}
