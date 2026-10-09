package mysql

import (
	"context"
	"database/sql"
)

// Tx 是一条已经开始的事务。Exec、Query、QueryRow 都走这一条连接。
type Tx struct {
	tx *sql.Tx
}

// Begin 开始一个事务。用完必须 Commit 或 Rollback。
func (d *DB) Begin(ctx context.Context) (*Tx, error) {
	if d == nil || d.sql == nil {
		return nil, errClosed
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Tx{tx: tx}, nil
}

// Within 执行 fn。fn 返回 nil 就提交，否则回滚。提交失败时也会回滚。
func (d *DB) Within(ctx context.Context, fn func(*Tx) error) (err error) {
	tx, err := d.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (t *Tx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if t == nil || t.tx == nil {
		return nil, errClosed
	}
	return t.tx.ExecContext(ctx, query, args...)
}

func (t *Tx) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if t == nil || t.tx == nil {
		return nil, errClosed
	}
	return t.tx.QueryContext(ctx, query, args...)
}

func (t *Tx) QueryRow(ctx context.Context, query string, args ...any) (*sql.Row, error) {
	if t == nil || t.tx == nil {
		return nil, errClosed
	}
	return t.tx.QueryRowContext(ctx, query, args...), nil
}

func (t *Tx) Commit() error {
	if t == nil || t.tx == nil {
		return errClosed
	}
	return t.tx.Commit()
}

func (t *Tx) Rollback() error {
	if t == nil || t.tx == nil {
		return errClosed
	}
	return t.tx.Rollback()
}
