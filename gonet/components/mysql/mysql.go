package mysql

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Options 连接池参数。字段为 0 时用默认：Open=16、Idle=4。
type Options struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DB 是进程内一份 MySQL 连接池。不含表结构、不含玩法。
type DB struct {
	sql *sql.DB
}

func Open(dsn string, opt Options) (*DB, error) {
	if dsn == "" {
		return nil, errEmptyDSN
	}
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	open, idle := opt.MaxOpenConns, opt.MaxIdleConns
	if open <= 0 {
		open = 16
	}
	if idle <= 0 {
		idle = 4
	}
	sqlDB.SetMaxOpenConns(open)
	sqlDB.SetMaxIdleConns(idle)
	if opt.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(opt.ConnMaxLifetime)
	}
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return &DB{sql: sqlDB}, nil
}

func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

func (d *DB) Ping(ctx context.Context) error {
	if d == nil || d.sql == nil {
		return errClosed
	}
	return d.sql.PingContext(ctx)
}

func (d *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if d == nil || d.sql == nil {
		return nil, errClosed
	}
	return d.sql.ExecContext(ctx, query, args...)
}

func (d *DB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if d == nil || d.sql == nil {
		return nil, errClosed
	}
	return d.sql.QueryContext(ctx, query, args...)
}

func (d *DB) QueryRow(ctx context.Context, query string, args ...any) (*sql.Row, error) {
	if d == nil || d.sql == nil {
		return nil, errClosed
	}
	return d.sql.QueryRowContext(ctx, query, args...), nil
}

// SQL 逃逸到标准库。业务表结构仍自己写 SQL。
func (d *DB) SQL() *sql.DB {
	if d == nil {
		return nil
	}
	return d.sql
}
