package role

import (
	"errors"
	"strconv"
	"testing"

	"game/service/friend"
	"game/store"

	"gonet"
)

func TestNextAliasUsesNode(t *testing.T) {
	prev := gonet.ClusterNode()
	gonet.SetClusterNode(17)
	t.Cleanup(func() { gonet.SetClusterNode(prev) })

	a := NextAlias()
	b := NextAlias()
	na, err := strconv.ParseUint(a, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	nb, err := strconv.ParseUint(b, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if na/1_000_000 != 17 || nb/1_000_000 != 17 || nb != na+1 {
		t.Fatalf("a=%s b=%s", a, b)
	}
}

func TestFriendAlias(t *testing.T) {
	if friend.RoleAlias("abc") != Alias("abc") {
		t.Fatalf("friend %s role %s", friend.RoleAlias("abc"), Alias("abc"))
	}
}

func TestStoreCreateThenLoad(t *testing.T) {
	store.Set(store.NewMemory())
	t.Cleanup(func() { store.Set(nil) })

	ok, err := HasRole("r1")
	if err != nil || ok {
		t.Fatalf("empty: ok=%v err=%v", ok, err)
	}
	if _, err := LoadData("r1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("load missing: %v", err)
	}
	if err := CreateRole("r1"); err != nil {
		t.Fatal(err)
	}
	if err := CreateRole("r1"); !errors.Is(err, ErrExists) {
		t.Fatalf("dup create: %v", err)
	}
	d, err := LoadData("r1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Base.Name() != "r1" || d.Base.Level() != 1 {
		t.Fatalf("got name=%s level=%d", d.Base.Name(), d.Base.Level())
	}
	if d.Base.LastLoginTime() != 0 || d.Base.LastLogoutTime() != 0 {
		t.Fatalf("times login=%d logout=%d", d.Base.LastLoginTime(), d.Base.LastLogoutTime())
	}
	if err := SaveRole(store.RoleRow{
		RoleID: "r1", Level: 1, Name: "r1", LastLoginTime: 10, LastLogoutTime: 20,
	}); err != nil {
		t.Fatal(err)
	}
	d, err = LoadData("r1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Base.LastLoginTime() != 10 || d.Base.LastLogoutTime() != 20 {
		t.Fatalf("saved times login=%d logout=%d", d.Base.LastLoginTime(), d.Base.LastLogoutTime())
	}
	if d.Missions == nil {
		t.Fatal("missions nil")
	}
	d.Missions.Bootstrap(d.Base.Level())
	list := d.Missions.List()
	if len(list) != 1 || list[0].ID != 1001 {
		t.Fatalf("missions: %+v", list)
	}
}
