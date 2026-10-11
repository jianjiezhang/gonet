package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestOpenRejectsBadOptions(t *testing.T) {
	if _, err := Open(Options{}); err != errEmptyAddr {
		t.Fatalf("empty addr: %v", err)
	}
	if _, err := Open(Options{Addr: "127.0.0.1:1", DB: -1}); err != errBadDB {
		t.Fatalf("bad db: %v", err)
	}
}

func TestStringAndHash(t *testing.T) {
	s := runRedis(t)
	c := openTest(t, Options{Addr: s.Addr()})
	ctx := context.Background()

	if err := c.Set(ctx, "sess", "abc", time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get(ctx, "sess")
	if err != nil || got != "abc" {
		t.Fatalf("get %q %v", got, err)
	}
	n, err := c.Exists(ctx, "sess", "missing")
	if err != nil || n != 1 {
		t.Fatalf("exists %d %v", n, err)
	}
	if _, err := c.Get(ctx, "missing"); !IsNil(err) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := c.Incr(ctx, "n"); err != nil {
		t.Fatal(err)
	}
	if v, err := c.Incr(ctx, "n"); err != nil || v != 2 {
		t.Fatalf("incr %d %v", v, err)
	}
	if added, err := c.HSet(ctx, "role", "name", "a", "lv", "3"); err != nil || added != 2 {
		t.Fatalf("hset %d %v", added, err)
	}
	name, err := c.HGet(ctx, "role", "name")
	if err != nil || name != "a" {
		t.Fatalf("hget %q %v", name, err)
	}
	all, err := c.HGetAll(ctx, "role")
	if err != nil || all["lv"] != "3" {
		t.Fatalf("hgetall %+v %v", all, err)
	}
	if _, err := c.HGet(ctx, "role", "nope"); !IsNil(err) {
		t.Fatalf("missing field: %v", err)
	}
	if deleted, err := c.HDel(ctx, "role", "lv"); err != nil || deleted != 1 {
		t.Fatalf("hdel %d %v", deleted, err)
	}
	if deleted, err := c.Del(ctx, "sess"); err != nil || deleted != 1 {
		t.Fatalf("del %d %v", deleted, err)
	}
}

func TestExpire(t *testing.T) {
	s := runRedis(t)
	c := openTest(t, Options{Addr: s.Addr()})
	ctx := context.Background()
	if err := c.Set(ctx, "k", "v", 0); err != nil {
		t.Fatal(err)
	}
	ok, err := c.Expire(ctx, "k", time.Second)
	if err != nil || !ok {
		t.Fatalf("expire %v %v", ok, err)
	}
	s.FastForward(2 * time.Second)
	if _, err := c.Get(ctx, "k"); !IsNil(err) {
		t.Fatalf("after ttl: %v", err)
	}
	ok, err = c.Expire(ctx, "missing", time.Second)
	if err != nil || ok {
		t.Fatalf("expire missing %v %v", ok, err)
	}
}

func TestDBIsolation(t *testing.T) {
	s := runRedis(t)
	ctx := context.Background()
	a := openTest(t, Options{Addr: s.Addr(), DB: 0})
	b := openTest(t, Options{Addr: s.Addr(), DB: 1})
	if err := a.Set(ctx, "k", "db0", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Get(ctx, "k"); !IsNil(err) {
		t.Fatalf("db1 saw db0 key: %v", err)
	}
}

func TestAuth(t *testing.T) {
	s := runRedis(t)
	s.RequireUserAuth("game", "secret")
	if _, err := Open(Options{Addr: s.Addr(), Username: "game", Password: "nope"}); err == nil {
		t.Fatal("expected auth failure")
	}
	c := openTest(t, Options{Addr: s.Addr(), Username: "game", Password: "secret"})
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestClosed(t *testing.T) {
	s := runRedis(t)
	c := openTest(t, Options{Addr: s.Addr()})
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "k"); err != errClosed {
		t.Fatalf("closed get: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func runRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func openTest(t *testing.T, opt Options) *Client {
	t.Helper()
	c, err := Open(opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
