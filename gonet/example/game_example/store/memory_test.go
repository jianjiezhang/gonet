package store

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryCreateLoadSave(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	ok, err := s.HasRole(ctx, "r1")
	if err != nil || ok {
		t.Fatalf("empty: ok=%v err=%v", ok, err)
	}
	if _, err := s.LoadRole(ctx, "r1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("load missing: %v", err)
	}
	if err := s.CreateRole(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRole(ctx, "r1"); !errors.Is(err, ErrExists) {
		t.Fatalf("dup create: %v", err)
	}
	row, err := s.LoadRole(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if row.Name != "r1" || row.Level != 1 {
		t.Fatalf("got %+v", row)
	}
	row.Level = 3
	if err := s.SaveRole(ctx, row); err != nil {
		t.Fatal(err)
	}
	row2, err := s.LoadRole(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if row2.Level != 3 {
		t.Fatalf("save: %+v", row2)
	}

	blob, err := s.LoadMission(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if string(blob.Data) != string(EmptyMissionJSON()) {
		t.Fatalf("empty mission: %s", blob.Data)
	}
	blob.Data = []byte(`{"list":[{"id":1001,"status":1,"progress":1}]}`)
	if err := s.SaveMission(ctx, blob); err != nil {
		t.Fatal(err)
	}
	blob2, err := s.LoadMission(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if string(blob2.Data) != string(blob.Data) {
		t.Fatalf("mission save: %s", blob2.Data)
	}
}
