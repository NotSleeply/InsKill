package lock

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Lock 分布式锁接口。
type Lock interface {
	TryLock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Unlock(ctx context.Context, key string) error
}

type redisLock struct{ rdb *redis.Client }

func NewRedisLock(rdb *redis.Client) Lock { return &redisLock{rdb: rdb} }

func (l *redisLock) TryLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return l.rdb.SetNX(ctx, key, "1", ttl).Result()
}

// unlockScript 比对 value 后删除，防止误删他人锁。
var unlockScript = redis.NewScript(`
if redis.call('get', KEYS[1]) == ARGV[1] then
  return redis.call('del', KEYS[1])
else
  return 0
end`)

func (l *redisLock) Unlock(ctx context.Context, key string) error {
	return unlockScript.Run(ctx, l.rdb, []string{key}, "1").Err()
}
