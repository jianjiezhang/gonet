package store

import (
	"context"
	"sync"
)

// Memory 进程内实现，给单测用。
type Memory struct {
	mu       sync.Mutex
	roles    map[string]RoleRow
	missions map[string][]byte
	friends  map[string]map[string]struct{}
	requests map[string]map[string]int64
	guilds   map[string]GuildRow
	members  map[string]GuildMember
	applies  map[string]map[string]int64
}

func NewMemory() *Memory {
	return &Memory{
		roles:    make(map[string]RoleRow),
		missions: make(map[string][]byte),
		friends:  make(map[string]map[string]struct{}),
		requests: make(map[string]map[string]int64),
		guilds:   make(map[string]GuildRow),
		members:  make(map[string]GuildMember),
		applies:  make(map[string]map[string]int64),
	}
}

func (m *Memory) HasRole(_ context.Context, roleid string) (bool, error) {
	if roleid == "" {
		return false, ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.roles[roleid]
	return ok, nil
}

func (m *Memory) CreateRole(_ context.Context, roleid string) error {
	if roleid == "" {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.roles[roleid]; ok {
		return ErrExists
	}
	m.roles[roleid] = NewDefaultRow(roleid)
	return nil
}

func (m *Memory) LoadRole(_ context.Context, roleid string) (RoleRow, error) {
	if roleid == "" {
		return RoleRow{}, ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.roles[roleid]
	if !ok {
		return RoleRow{}, ErrNotFound
	}
	return row, nil
}

func (m *Memory) SaveRole(_ context.Context, row RoleRow) error {
	if row.RoleID == "" {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.roles[row.RoleID]; !ok {
		return ErrNotFound
	}
	m.roles[row.RoleID] = row
	return nil
}

func (m *Memory) LoadMission(_ context.Context, roleid string) (MissionBlob, error) {
	if roleid == "" {
		return MissionBlob{}, ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.missions[roleid]
	if !ok {
		return MissionBlob{RoleID: roleid, Data: EmptyMissionJSON()}, nil
	}
	out := make([]byte, len(data))
	copy(out, data)
	return MissionBlob{RoleID: roleid, Data: out}, nil
}

func (m *Memory) LoadPlayer(_ context.Context, roleid string) (RoleRow, MissionBlob, error) {
	if roleid == "" {
		return RoleRow{}, MissionBlob{}, ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.roles[roleid]
	if !ok {
		return RoleRow{}, MissionBlob{}, ErrNotFound
	}
	data, ok := m.missions[roleid]
	if !ok {
		return row, MissionBlob{RoleID: roleid, Data: EmptyMissionJSON()}, nil
	}
	out := make([]byte, len(data))
	copy(out, data)
	return row, MissionBlob{RoleID: roleid, Data: out}, nil
}

func (m *Memory) SavePlayer(_ context.Context, row RoleRow, blob MissionBlob) error {
	if row.RoleID == "" || blob.RoleID == "" || row.RoleID != blob.RoleID {
		return ErrEmptyAlias
	}
	data := blob.Data
	if len(data) == 0 {
		data = EmptyMissionJSON()
	}
	out := make([]byte, len(data))
	copy(out, data)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.roles[row.RoleID]; !ok {
		return ErrNotFound
	}
	m.roles[row.RoleID] = row
	m.missions[blob.RoleID] = out
	return nil
}

func (m *Memory) SaveMission(_ context.Context, blob MissionBlob) error {
	if blob.RoleID == "" {
		return ErrEmptyAlias
	}
	data := blob.Data
	if len(data) == 0 {
		data = EmptyMissionJSON()
	}
	out := make([]byte, len(data))
	copy(out, data)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.missions[blob.RoleID] = out
	return nil
}

func (m *Memory) LoadFriends(_ context.Context) ([]FriendEdge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []FriendEdge
	for a, set := range m.friends {
		for b := range set {
			out = append(out, FriendEdge{RoleID: a, FriendID: b})
		}
	}
	return out, nil
}

func (m *Memory) LoadFriendRequests(_ context.Context) ([]FriendRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []FriendRequest
	for from, tos := range m.requests {
		for to, tm := range tos {
			out = append(out, FriendRequest{FromID: from, ToID: to, Time: tm})
		}
	}
	return out, nil
}

func (m *Memory) AddFriends(_ context.Context, a, b string) error {
	if a == "" || b == "" || a == b {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.linkFriend(a, b)
	m.linkFriend(b, a)
	return nil
}

func (m *Memory) AcceptFriend(_ context.Context, self, from string) error {
	if self == "" || from == "" || self == from {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.linkFriend(self, from)
	m.linkFriend(from, self)
	delete(m.requests[from], self)
	if len(m.requests[from]) == 0 {
		delete(m.requests, from)
	}
	return nil
}

func (m *Memory) RemoveFriends(_ context.Context, a, b string) error {
	if a == "" || b == "" {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unlinkFriend(a, b)
	m.unlinkFriend(b, a)
	return nil
}

func (m *Memory) AddFriendRequest(_ context.Context, req FriendRequest) error {
	if req.FromID == "" || req.ToID == "" || req.FromID == req.ToID {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tos := m.requests[req.FromID]
	if tos == nil {
		tos = make(map[string]int64)
		m.requests[req.FromID] = tos
	}
	tos[req.ToID] = req.Time
	return nil
}

func (m *Memory) RemoveFriendRequest(_ context.Context, fromID, toID string) error {
	if fromID == "" || toID == "" {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.requests[fromID], toID)
	if len(m.requests[fromID]) == 0 {
		delete(m.requests, fromID)
	}
	return nil
}

func (m *Memory) linkFriend(a, b string) {
	set := m.friends[a]
	if set == nil {
		set = make(map[string]struct{})
		m.friends[a] = set
	}
	set[b] = struct{}{}
}

func (m *Memory) unlinkFriend(a, b string) {
	delete(m.friends[a], b)
	if len(m.friends[a]) == 0 {
		delete(m.friends, a)
	}
}

func (m *Memory) Close() error { return nil }
