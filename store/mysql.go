package store

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"

	"gonet/mysql"
)

//go:embed game.sql
var gameSQL string

type mysqlStore struct {
	db *mysql.DB
}

func openMySQL(dsn string) (*mysqlStore, error) {
	db, err := mysql.Open(dsn, mysql.Options{MaxOpenConns: 16, MaxIdleConns: 4})
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	if err := execSQL(ctx, db, gameSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("执行 game.sql: %w", err)
	}
	if err := ensureRoleTimeColumns(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	slog.Info("mysql ready", "table", "role")
	return &mysqlStore{db: db}, nil
}

func execSQL(ctx context.Context, db *mysql.DB, script string) error {
	var lines []string
	for _, line := range strings.Split(script, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		lines = append(lines, t)
	}
	for _, stmt := range strings.Split(strings.Join(lines, "\n"), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// ensureRoleTimeColumns 给已经存在的 role 表补上登录/下线时间。
// 新建库的 game.sql 里已经有这两列，重复执行时忽略 1060。
func ensureRoleTimeColumns(ctx context.Context, db *mysql.DB) error {
	for _, col := range []string{"lastlogintime", "lastlogouttime"} {
		_, err := db.Exec(ctx, `ALTER TABLE role ADD COLUMN `+col+` BIGINT NOT NULL DEFAULT 0`)
		if err != nil && !mysql.IsDupColumn(err) {
			return fmt.Errorf("store: 添加 %s: %w", col, err)
		}
	}
	return nil
}

func (s *mysqlStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
