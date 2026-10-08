package store

import (
	"context"
	"log/slog"

	"gonet/mysql"
)

func (s *mysqlStore) HasRole(ctx context.Context, roleid string) (bool, error) {
	if roleid == "" {
		return false, ErrEmptyAlias
	}
	row, err := s.db.QueryRow(ctx, `SELECT 1 FROM role WHERE roleid = ? LIMIT 1`, roleid)
	if err != nil {
		return false, err
	}
	var n int
	err = row.Scan(&n)
	if mysql.IsNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *mysqlStore) LoadRole(ctx context.Context, roleid string) (RoleRow, error) {
	if roleid == "" {
		return RoleRow{}, ErrEmptyAlias
	}
	q, err := s.db.QueryRow(ctx,
		`SELECT roleid, level, name, gender, lastlogintime, lastlogouttime FROM role WHERE roleid = ?`, roleid)
	if err != nil {
		return RoleRow{}, err
	}
	var row RoleRow
	err = q.Scan(&row.RoleID, &row.Level, &row.Name, &row.Gender, &row.LastLoginTime, &row.LastLogoutTime)
	if mysql.IsNoRows(err) {
		return RoleRow{}, ErrNotFound
	}
	if err != nil {
		return RoleRow{}, err
	}
	slog.Info("load_data mysql", "roleid", row.RoleID, "level", row.Level, "name", row.Name, "gender", row.Gender)
	return row, nil
}

func (s *mysqlStore) LoadMission(ctx context.Context, roleid string) (MissionBlob, error) {
	if roleid == "" {
		return MissionBlob{}, ErrEmptyAlias
	}
	q, err := s.db.QueryRow(ctx, `SELECT data FROM t_mission WHERE roleid = ?`, roleid)
	if err != nil {
		return MissionBlob{}, err
	}
	var data []byte
	err = q.Scan(&data)
	if mysql.IsNoRows(err) {
		return MissionBlob{RoleID: roleid, Data: EmptyMissionJSON()}, nil
	}
	if err != nil {
		return MissionBlob{}, err
	}
	if len(data) == 0 {
		data = EmptyMissionJSON()
	}
	return MissionBlob{RoleID: roleid, Data: data}, nil
}
