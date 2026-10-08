package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRedisDefaults(t *testing.T) {
	path := writeConfig(t, "")
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	cfg := Get()
	if cfg.RedisAddr() != "127.0.0.1:6379" {
		t.Fatalf("addr %s", cfg.RedisAddr())
	}
	if cfg.RedisUser != "default" || cfg.RedisPassword != "" || cfg.RedisDB != 0 {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoadRedis(t *testing.T) {
	path := writeConfig(t, `
redis_host = "10.0.0.2"
redis_port = 6380
redis_user = "game"
redis_password = "secret"
redis_db = 2
`)
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	cfg := Get()
	if cfg.RedisAddr() != "10.0.0.2:6380" {
		t.Fatalf("addr %s", cfg.RedisAddr())
	}
	if cfg.RedisUser != "game" || cfg.RedisPassword != "secret" || cfg.RedisDB != 2 {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoadRedisRejectsBadDB(t *testing.T) {
	path := writeConfig(t, "redis_db = -1\n")
	if err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
