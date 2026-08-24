package id

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// beginTimestamp 对齐 Java RedisIdWorker.BEGIN_TIMESTAMP（2022-01-01 00:00:00 UTC）。
const beginTimestamp int64 = 1640995200

// NextID 分布式订单 ID：高 32 位为相对时间戳（秒），低 32 位为 Redis 按日自增序列。
func NextID(ctx context.Context, rdb *redis.Client, prefix string) (int64, error) {
	now := time.Now()
	timestamp := now.Unix() - beginTimestamp
	date := now.Format("2006:01:02")
	count, err := rdb.Incr(ctx, "icr:"+prefix+":"+date).Result()
	if err != nil {
		return 0, fmt.Errorf("next id: %w", err)
	}
	return timestamp<<32 | count, nil
}
