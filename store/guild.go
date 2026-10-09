package store

import (
	"context"

	"gonet/mysql"
)

func (s *mysqlStore) LoadGuilds(ctx context.Context) ([]GuildRow, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, leader, notice, created FROM guild`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GuildRow
	for rows.Next() {
		var row GuildRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Leader, &row.Notice, &row.Created); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *mysqlStore) LoadGuildMembers(ctx context.Context) ([]GuildMember, error) {
	rows, err := s.db.Query(ctx, "SELECT roleid, guildid, `rank`, time FROM guild_member")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GuildMember
	for rows.Next() {
		var m GuildMember
		if err := rows.Scan(&m.RoleID, &m.GuildID, &m.Rank, &m.Time); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *mysqlStore) LoadGuild(ctx context.Context, id string) (GuildRow, []GuildMember, []GuildApply, error) {
	if id == "" {
		return GuildRow{}, nil, nil, ErrEmptyAlias
	}
	var row GuildRow
	var members []GuildMember
	var applies []GuildApply
	err := s.db.Within(ctx, func(tx *mysql.Tx) error {
		q, err := tx.QueryRow(ctx, `SELECT id, name, leader, notice, created FROM guild WHERE id = ?`, id)
		if err != nil {
			return err
		}
		err = q.Scan(&row.ID, &row.Name, &row.Leader, &row.Notice, &row.Created)
		if mysql.IsNoRows(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		mrows, err := tx.Query(ctx, "SELECT roleid, guildid, `rank`, time FROM guild_member WHERE guildid = ?", id)
		if err != nil {
			return err
		}
		defer mrows.Close()
		for mrows.Next() {
			var m GuildMember
			if err := mrows.Scan(&m.RoleID, &m.GuildID, &m.Rank, &m.Time); err != nil {
				return err
			}
			members = append(members, m)
		}
		if err := mrows.Err(); err != nil {
			return err
		}
		arows, err := tx.Query(ctx, `SELECT guildid, roleid, time FROM guild_apply WHERE guildid = ?`, id)
		if err != nil {
			return err
		}
		defer arows.Close()
		for arows.Next() {
			var a GuildApply
			if err := arows.Scan(&a.GuildID, &a.RoleID, &a.Time); err != nil {
				return err
			}
			applies = append(applies, a)
		}
		return arows.Err()
	})
	if err != nil {
		return GuildRow{}, nil, nil, err
	}
	return row, members, applies, nil
}

func (s *mysqlStore) CreateGuild(ctx context.Context, row GuildRow, leader GuildMember) error {
	if row.ID == "" || row.Name == "" || leader.RoleID == "" {
		return ErrEmptyAlias
	}
	return s.db.Within(ctx, func(tx *mysql.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO guild (id, name, leader, notice, created) VALUES (?, ?, ?, ?, ?)`,
			row.ID, row.Name, row.Leader, row.Notice, row.Created); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			"INSERT INTO guild_member (roleid, guildid, `rank`, time) VALUES (?, ?, ?, ?)",
			leader.RoleID, leader.GuildID, leader.Rank, leader.Time)
		return err
	})
}

func (s *mysqlStore) AddGuildMember(ctx context.Context, m GuildMember) error {
	if m.RoleID == "" || m.GuildID == "" {
		return ErrEmptyAlias
	}
	return s.db.Within(ctx, func(tx *mysql.Tx) error {
		if _, err := tx.Exec(ctx,
			"INSERT INTO guild_member (roleid, guildid, `rank`, time) VALUES (?, ?, ?, ?)",
			m.RoleID, m.GuildID, m.Rank, m.Time); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM guild_apply WHERE guildid = ? AND roleid = ?`, m.GuildID, m.RoleID)
		return err
	})
}

func (s *mysqlStore) RemoveGuildMember(ctx context.Context, roleID string) error {
	if roleID == "" {
		return ErrEmptyAlias
	}
	_, err := s.db.Exec(ctx, `DELETE FROM guild_member WHERE roleid = ?`, roleID)
	return err
}

func (s *mysqlStore) AddGuildApply(ctx context.Context, a GuildApply) error {
	if a.GuildID == "" || a.RoleID == "" {
		return ErrEmptyAlias
	}
	_, err := s.db.Exec(ctx,
		`INSERT INTO guild_apply (guildid, roleid, time) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE time = VALUES(time)`,
		a.GuildID, a.RoleID, a.Time)
	return err
}

func (s *mysqlStore) RemoveGuildApply(ctx context.Context, guildID, roleID string) error {
	if guildID == "" || roleID == "" {
		return ErrEmptyAlias
	}
	_, err := s.db.Exec(ctx, `DELETE FROM guild_apply WHERE guildid = ? AND roleid = ?`, guildID, roleID)
	return err
}

func (s *mysqlStore) DeleteGuild(ctx context.Context, id string) error {
	if id == "" {
		return ErrEmptyAlias
	}
	return s.db.Within(ctx, func(tx *mysql.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM guild_apply WHERE guildid = ?`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM guild_member WHERE guildid = ?`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM guild WHERE id = ?`, id)
		return err
	})
}

func (m *Memory) LoadGuilds(_ context.Context) ([]GuildRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]GuildRow, 0, len(m.guilds))
	for _, row := range m.guilds {
		out = append(out, row)
	}
	return out, nil
}

func (m *Memory) LoadGuildMembers(_ context.Context) ([]GuildMember, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]GuildMember, 0, len(m.members))
	for _, member := range m.members {
		out = append(out, member)
	}
	return out, nil
}

func (m *Memory) LoadGuild(_ context.Context, id string) (GuildRow, []GuildMember, []GuildApply, error) {
	if id == "" {
		return GuildRow{}, nil, nil, ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.guilds[id]
	if !ok {
		return GuildRow{}, nil, nil, ErrNotFound
	}
	var members []GuildMember
	for _, member := range m.members {
		if member.GuildID == id {
			members = append(members, member)
		}
	}
	var applies []GuildApply
	for roleID, tm := range m.applies[id] {
		applies = append(applies, GuildApply{GuildID: id, RoleID: roleID, Time: tm})
	}
	return row, members, applies, nil
}

func (m *Memory) CreateGuild(_ context.Context, row GuildRow, leader GuildMember) error {
	if row.ID == "" || row.Name == "" || leader.RoleID == "" {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, g := range m.guilds {
		if g.Name == row.Name {
			return ErrExists
		}
	}
	if _, ok := m.members[leader.RoleID]; ok {
		return ErrExists
	}
	if _, ok := m.guilds[row.ID]; ok {
		return ErrExists
	}
	m.guilds[row.ID] = row
	m.members[leader.RoleID] = leader
	return nil
}

func (m *Memory) AddGuildMember(_ context.Context, member GuildMember) error {
	if member.RoleID == "" || member.GuildID == "" {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.guilds[member.GuildID]; !ok {
		return ErrNotFound
	}
	if _, ok := m.members[member.RoleID]; ok {
		return ErrExists
	}
	m.members[member.RoleID] = member
	delete(m.applies[member.GuildID], member.RoleID)
	if len(m.applies[member.GuildID]) == 0 {
		delete(m.applies, member.GuildID)
	}
	return nil
}

func (m *Memory) RemoveGuildMember(_ context.Context, roleID string) error {
	if roleID == "" {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.members, roleID)
	return nil
}

func (m *Memory) AddGuildApply(_ context.Context, a GuildApply) error {
	if a.GuildID == "" || a.RoleID == "" {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	set := m.applies[a.GuildID]
	if set == nil {
		set = make(map[string]int64)
		m.applies[a.GuildID] = set
	}
	set[a.RoleID] = a.Time
	return nil
}

func (m *Memory) RemoveGuildApply(_ context.Context, guildID, roleID string) error {
	if guildID == "" || roleID == "" {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.applies[guildID], roleID)
	if len(m.applies[guildID]) == 0 {
		delete(m.applies, guildID)
	}
	return nil
}

func (m *Memory) DeleteGuild(_ context.Context, id string) error {
	if id == "" {
		return ErrEmptyAlias
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.guilds, id)
	delete(m.applies, id)
	for roleID, member := range m.members {
		if member.GuildID == id {
			delete(m.members, roleID)
		}
	}
	return nil
}
