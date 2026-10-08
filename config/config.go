package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

type Config struct {
	Host       string `toml:"host"`
	Port       int    `toml:"port"`
	DebugHost  string `toml:"debug_host"`
	DebugPort  int    `toml:"debug_port"`
	LoginToken string `toml:"login_token"`
	DBHost     string `toml:"db_host"`
	DBPort     int    `toml:"db_port"`
	DBUser     string `toml:"db_user"`
	DBPassword string `toml:"db_password"`
	DBName     string `toml:"db_name"`

	RedisHost     string `toml:"redis_host"`
	RedisPort     int    `toml:"redis_port"`
	RedisUser     string `toml:"redis_user"`
	RedisPassword string `toml:"redis_password"`
	RedisDB       int    `toml:"redis_db"`
	Center        bool   `toml:"center"`
	HarborMgrPort int    `toml:"harbormgr_port"`
	HarborPort    int    `toml:"harbor_port"`
	WebHost       string `toml:"web_host"`
	WebPort       int    `toml:"web_port"`
}

var (
	mu     sync.RWMutex
	loaded Config
)

func Get() Config {
	mu.RLock()
	defer mu.RUnlock()
	return loaded
}

func (c Config) Addr() string {
	host := c.Host
	if host == "" {
		host = "0.0.0.0"
	}
	port := c.Port
	if port == 0 {
		port = 8888
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// WebAddr 是登录网页。WebPort==0 表示不提供网页。
func (c Config) WebAddr() string {
	if c.WebPort == 0 {
		return ""
	}
	host := c.WebHost
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(c.WebPort))
}

// GameDialAddr 是本进程玩家口的回环地址，网页登录从这里打进去。
func (c Config) GameDialAddr() string {
	port := c.Port
	if port == 0 {
		port = 8888
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

// DebugAddr 是本机 debug 口。DebugPort==0 表示不开。
func (c Config) DebugAddr() string {
	if c.DebugPort == 0 {
		return ""
	}
	host := c.DebugHost
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(c.DebugPort))
}

// DSN 是 MySQL 连接串，给 database/sql 用。
func (c Config) DSN() string {
	host := c.DBHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.DBPort
	if port == 0 {
		port = 3306
	}
	user := c.DBUser
	if user == "" {
		user = "game"
	}
	name := c.DBName
	if name == "" {
		name = "game"
	}
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&charset=utf8mb4&clientFoundRows=true",
		user, c.DBPassword, net.JoinHostPort(host, strconv.Itoa(port)), name)
}

// RedisAddr 是 Redis 地址，形如 127.0.0.1:6379。
func (c Config) RedisAddr() string {
	host := c.RedisHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.RedisPort
	if port == 0 {
		port = 6379
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func Load(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	cfg, err := parseTOML(string(b))
	if err != nil {
		return err
	}
	if cfg.Host == "" {
		cfg.Host = "0.0.0.0"
	}
	if cfg.Port == 0 {
		cfg.Port = 8888
	}
	if cfg.DebugHost == "" {
		cfg.DebugHost = "127.0.0.1"
	}
	if cfg.DBHost == "" {
		cfg.DBHost = "127.0.0.1"
	}
	if cfg.DBPort == 0 {
		cfg.DBPort = 3306
	}
	if cfg.DBUser == "" {
		cfg.DBUser = "game"
	}
	if cfg.DBName == "" {
		cfg.DBName = "game"
	}
	if cfg.RedisHost == "" {
		cfg.RedisHost = "127.0.0.1"
	}
	if cfg.RedisPort == 0 {
		cfg.RedisPort = 6379
	}
	if cfg.RedisUser == "" {
		cfg.RedisUser = "default"
	}
	if cfg.HarborMgrPort == 0 {
		cfg.HarborMgrPort = 8008
	}
	if cfg.HarborPort == 0 {
		cfg.HarborPort = 8018
	}
	mu.Lock()
	loaded = cfg
	mu.Unlock()
	return nil
}

func parseTOML(s string) (Config, error) {
	var cfg Config
	for i, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if hash := strings.Index(line, "#"); hash >= 0 {
			line = strings.TrimSpace(line[:hash])
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("config: 第 %d 行不是 key = value", i+1)
		}
		key := strings.TrimSpace(k)
		val := unquote(strings.TrimSpace(v))
		switch key {
		case "host":
			cfg.Host = val
		case "port":
			n, err := strconv.Atoi(val)
			if err != nil || n <= 0 || n > 65535 {
				return Config{}, fmt.Errorf("config: port 无效")
			}
			cfg.Port = n
		case "debug_host":
			cfg.DebugHost = val
		case "debug_port":
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 || n > 65535 {
				return Config{}, fmt.Errorf("config: debug_port 无效")
			}
			cfg.DebugPort = n
		case "login_token":
			cfg.LoginToken = val
		case "db_host":
			cfg.DBHost = val
		case "db_port":
			n, err := strconv.Atoi(val)
			if err != nil || n <= 0 || n > 65535 {
				return Config{}, fmt.Errorf("config: db_port 无效")
			}
			cfg.DBPort = n
		case "db_user":
			cfg.DBUser = val
		case "db_password":
			cfg.DBPassword = val
		case "db_name":
			cfg.DBName = val
		case "redis_host":
			cfg.RedisHost = val
		case "redis_port":
			n, err := strconv.Atoi(val)
			if err != nil || n <= 0 || n > 65535 {
				return Config{}, fmt.Errorf("config: redis_port 无效")
			}
			cfg.RedisPort = n
		case "redis_user":
			cfg.RedisUser = val
		case "redis_password":
			cfg.RedisPassword = val
		case "redis_db":
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 {
				return Config{}, fmt.Errorf("config: redis_db 无效")
			}
			cfg.RedisDB = n
		case "center":
			on, err := strconv.ParseBool(val)
			if err != nil {
				return Config{}, fmt.Errorf("config: center 无效")
			}
			cfg.Center = on
		case "harbormgr_port":
			n, err := strconv.Atoi(val)
			if err != nil || n <= 0 || n > 65535 {
				return Config{}, fmt.Errorf("config: harbormgr_port 无效")
			}
			cfg.HarborMgrPort = n
		case "harbor_port":
			n, err := strconv.Atoi(val)
			if err != nil || n <= 0 || n > 65535 {
				return Config{}, fmt.Errorf("config: harbor_port 无效")
			}
			cfg.HarborPort = n
		case "web_host":
			cfg.WebHost = val
		case "web_port":
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 || n > 65535 {
				return Config{}, fmt.Errorf("config: web_port 无效")
			}
			cfg.WebPort = n
		}
	}
	return cfg, nil
}

func unquote(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}
