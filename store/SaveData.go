package store

import (
	"context"
	"log/slog"

	"gonet/mysql"
)

func (s *mysqlStore) CreateRole(ctx context.Context, roleid string) error {
	if roleid == "" {
		return ErrEmptyAlias
	}
	row := NewDefaultRow(roleid)
	_, err := s.db.Exec(ctx,
		`INSERT INTO role (roleid, level, name, gender) VALUES (?, ?, ?, ?)`,
		row.RoleID, row.Level, row.Name, row.Gender)
	if err == nil {
		slog.Info("create_role mysql", "roleid", roleid, "level", row.Level, "name", row.Name, "gender", row.Gender)
		return nil
	}
	if mysql.IsDuplicate(err) {
		return ErrExists
	}
	return err
}

func (s *mysqlStore) SaveRole(ctx context.Context, row RoleRow) error {
	if row.RoleID == "" {
		return ErrEmptyAlias
	}
	res, err := s.db.Exec(ctx,
		`UPDATE role SET level = ?, name = ?, gender = ?, lastlogintime = ?, lastlogouttime = ? WHERE roleid = ?`,
		row.Level, row.Name, row.Gender, row.LastLoginTime, row.LastLogoutTime, row.RoleID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *mysqlStore) SaveMission(ctx context.Context, blob MissionBlob) error {
	if blob.RoleID == "" {
		return ErrEmptyAlias
	}
	data := blob.Data
	if len(data) == 0 {
		data = EmptyMissionJSON()
	}
	_, err := s.db.Exec(ctx,
		`INSERT INTO t_mission (roleid, data) VALUES (?, CAST(? AS JSON))
		 ON DUPLICATE KEY UPDATE data = CAST(? AS JSON)`,
		blob.RoleID, data, data)
	return err
}
