package mysql

import (
	"context"
	"testing"
)

func TestTxRequiresDB(t *testing.T) {
	var db *DB
	if _, err := db.Begin(context.Background()); err != errClosed {
		t.Fatalf("begin: %v", err)
	}
	if err := db.Within(context.Background(), func(*Tx) error { return nil }); err != errClosed {
		t.Fatalf("within: %v", err)
	}
	var tx *Tx
	if _, err := tx.Exec(context.Background(), "SELECT 1"); err != errClosed {
		t.Fatalf("exec: %v", err)
	}
	if err := tx.Commit(); err != errClosed {
		t.Fatalf("commit: %v", err)
	}
	if err := tx.Rollback(); err != errClosed {
		t.Fatalf("rollback: %v", err)
	}
}
