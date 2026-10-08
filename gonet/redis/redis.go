package redis

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Options 是连接参数。Addr 必填，形如 127.0.0.1:6379。
// Username 为空时由服务端把连接当成 default 用户。Password 为空表示不校验密码。
// DB 是逻辑库编号，从 0 开始。PoolSize<=0 时用 16。
type Options struct {
	Addr     string
	Username string
	Password string
	DB       int
	PoolSize int
}

// Client 是进程内一份 Redis 连接池。不含 key 设计、不含玩法。
type Client struct {
	rdb *goredis.Client
}

func Open(opt Options) (*Client, error) {
	if opt.Addr == "" {
		return nil, errEmptyAddr
	}
	if opt.DB < 0 {
		return nil, errBadDB
	}
	pool := opt.PoolSize
	if pool <= 0 {
		pool = 16
	}
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     opt.Addr,
		Username: opt.Username,
		Password: opt.Password,
		DB:       opt.DB,
		PoolSize: pool,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	return &Client{rdb: rdb}, nil
}

func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	err := c.rdb.Close()
	c.rdb = nil
	return err
}

func (c *Client) Ping(ctx context.Context) error {
	if err := c.ready(); err != nil {
		return err
	}
	return c.rdb.Ping(ctx).Err()
}

// Raw 逃逸到 go-redis。业务 key 仍自己设计。
func (c *Client) Raw() *goredis.Client {
	if c == nil {
		return nil
	}
	return c.rdb
}

// Set 写入字符串。ttl<=0 表示不过期。
func (c *Client) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := c.ready(); err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	if err := c.ready(); err != nil {
		return "", err
	}
	s, err := c.rdb.Get(ctx, key).Result()
	if IsNil(err) {
		return "", ErrNil
	}
	return s, err
}

func (c *Client) Del(ctx context.Context, keys ...string) (int64, error) {
	if err := c.ready(); err != nil {
		return 0, err
	}
	return c.rdb.Del(ctx, keys...).Result()
}

func (c *Client) Exists(ctx context.Context, keys ...string) (int64, error) {
	if err := c.ready(); err != nil {
		return 0, err
	}
	return c.rdb.Exists(ctx, keys...).Result()
}

// Expire 设置过期时间。key 不存在时返回 false。
func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if err := c.ready(); err != nil {
		return false, err
	}
	return c.rdb.Expire(ctx, key, ttl).Result()
}

func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	if err := c.ready(); err != nil {
		return 0, err
	}
	return c.rdb.Incr(ctx, key).Result()
}

func (c *Client) HSet(ctx context.Context, key string, values ...any) (int64, error) {
	if err := c.ready(); err != nil {
		return 0, err
	}
	return c.rdb.HSet(ctx, key, values...).Result()
}

func (c *Client) HGet(ctx context.Context, key, field string) (string, error) {
	if err := c.ready(); err != nil {
		return "", err
	}
	s, err := c.rdb.HGet(ctx, key, field).Result()
	if IsNil(err) {
		return "", ErrNil
	}
	return s, err
}

func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if err := c.ready(); err != nil {
		return nil, err
	}
	return c.rdb.HGetAll(ctx, key).Result()
}

func (c *Client) HDel(ctx context.Context, key string, fields ...string) (int64, error) {
	if err := c.ready(); err != nil {
		return 0, err
	}
	return c.rdb.HDel(ctx, key, fields...).Result()
}

func (c *Client) ready() error {
	if c == nil || c.rdb == nil {
		return errClosed
	}
	return nil
}
