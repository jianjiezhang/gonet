package redis

import (
	"errors"

	goredis "github.com/redis/go-redis/v9"
)

var (
	errEmptyAddr = errors.New("gonet/components/redis: 地址为空")
	errBadDB     = errors.New("gonet/components/redis: db 不能为负")
	errClosed    = errors.New("gonet/components/redis: 连接已关闭")

	// ErrNil 表示 key 或 field 不存在。
	ErrNil = goredis.Nil
)

func IsNil(err error) bool {
	return errors.Is(err, goredis.Nil)
}
