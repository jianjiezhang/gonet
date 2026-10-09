package store

import (
	"context"

	"gonet/mysql"
)

func (s *mysqlStore) LoadFriends(ctx context.Context) ([]FriendEdge, error) {
	rows, err := s.db.Query(ctx, `SELECT roleid, friendid FROM friend`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FriendEdge
	for rows.Next() {
		var e FriendEdge
		if err := rows.Scan(&e.RoleID, &e.FriendID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *mysqlStore) LoadFriendRequests(ctx context.Context) ([]FriendRequest, error) {
	rows, err := s.db.Query(ctx, `SELECT fromid, toid, time FROM friend_request`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FriendRequest
	for rows.Next() {
		var req FriendRequest
		if err := rows.Scan(&req.FromID, &req.ToID, &req.Time); err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

func (s *mysqlStore) AddFriends(ctx context.Context, a, b string) error {
	if a == "" || b == "" || a == b {
		return ErrEmptyAlias
	}
	_, err := s.db.Exec(ctx,
		`INSERT IGNORE INTO friend (roleid, friendid) VALUES (?, ?), (?, ?)`,
		a, b, b, a)
	return err
}

// AcceptFriend 把 from 向 self 的申请变成双向好友，并删掉这条申请。
func (s *mysqlStore) AcceptFriend(ctx context.Context, self, from string) error {
	if self == "" || from == "" || self == from {
		return ErrEmptyAlias
	}
	return s.db.Within(ctx, func(tx *mysql.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT IGNORE INTO friend (roleid, friendid) VALUES (?, ?), (?, ?)`,
			self, from, from, self); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM friend_request WHERE fromid = ? AND toid = ?`, from, self)
		return err
	})
}

func (s *mysqlStore) RemoveFriends(ctx context.Context, a, b string) error {
	if a == "" || b == "" {
		return ErrEmptyAlias
	}
	_, err := s.db.Exec(ctx,
		`DELETE FROM friend WHERE (roleid = ? AND friendid = ?) OR (roleid = ? AND friendid = ?)`,
		a, b, b, a)
	return err
}

func (s *mysqlStore) AddFriendRequest(ctx context.Context, req FriendRequest) error {
	if req.FromID == "" || req.ToID == "" || req.FromID == req.ToID {
		return ErrEmptyAlias
	}
	_, err := s.db.Exec(ctx,
		`INSERT INTO friend_request (fromid, toid, time) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE time = VALUES(time)`,
		req.FromID, req.ToID, req.Time)
	return err
}

func (s *mysqlStore) RemoveFriendRequest(ctx context.Context, fromID, toID string) error {
	if fromID == "" || toID == "" {
		return ErrEmptyAlias
	}
	_, err := s.db.Exec(ctx, `DELETE FROM friend_request WHERE fromid = ? AND toid = ?`, fromID, toID)
	return err
}
