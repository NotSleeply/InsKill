package cache

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"inskill/internal/lock"

	"github.com/redis/go-redis/v9"
)

// ErrNil 缓存未命中。
var ErrNil = errors.New("cache: nil")

// Client 统一缓存接口：远程 Redis + 本地二级缓存。
type Client interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	LocalGet(key string) (any, bool)
	LocalSet(key string, v any)
	LocalDel(key string)
}

// RedisData 逻辑过期包装，字段对齐 Java RedisData。
type RedisData struct {
	Data       json.RawMessage `json:"data"`
	ExpireTime time.Time       `json:"expireTime"`
}

const nullCacheTTL = 2 * time.Minute // 空值缓存 TTL（穿透）

type redisClient struct {
	rdb   *redis.Client
	local *ristrettoCache
}

func NewRedisClient(addr string) (Client, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	local, err := newLocal()
	if err != nil {
		return nil, err
	}
	return &redisClient{rdb: rdb, local: local}, nil
}

func (c *redisClient) Get(ctx context.Context, key string) (string, error) {
	v, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrNil
	}
	return v, err
}

func (c *redisClient) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *redisClient) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

func (c *redisClient) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return c.rdb.SetNX(ctx, key, value, ttl).Result()
}

func (c *redisClient) LocalGet(key string) (any, bool) { return c.local.Get(key) }
func (c *redisClient) LocalSet(key string, v any)      { c.local.Set(key, v) }
func (c *redisClient) LocalDel(key string)             { c.local.Del(key) }

// QueryWithPassThrough 缓存穿透方案：命中返回；空值缓存返回 nil；未命中走 loader 并缓存。
func QueryWithPassThrough[T any](c Client, ctx context.Context, key string, loader func(ctx context.Context) (*T, error), ttl time.Duration) (*T, error) {
	jsonStr, err := c.Get(ctx, key)
	if err == nil {
		if jsonStr == "" {
			return nil, nil // 空值缓存
		}
		var v T
		if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
			return nil, err
		}
		return &v, nil
	}
	if !errors.Is(err, ErrNil) {
		return nil, err
	}
	value, err := loader(ctx)
	if err != nil {
		return nil, err
	}
	if value == nil {
		_ = c.Set(ctx, key, "", nullCacheTTL)
		return nil, nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	// TTL 加随机偏移抗雪崩
	if err := c.Set(ctx, key, string(b), ttl+time.Duration(time.Now().UnixNano()%60)*time.Second); err != nil {
		return nil, err
	}
	return value, nil
}

// QueryWithLogicalExpire 逻辑过期方案：数据未过期直接返回；过期则抢锁重建（异步），旧数据兜底返回。
// lockKey 为重建互斥锁 key（调用方传入，如 lock:shop:{id}）。
func QueryWithLogicalExpire[T any](c Client, l lock.Lock, ctx context.Context, key, lockKey string, loader func(ctx context.Context) (*T, error), ttl time.Duration) (*T, error) {
	jsonStr, err := c.Get(ctx, key)
	if err != nil {
		return nil, err // 无缓存，调用方自行兜底（Java 版同样返回 null）
	}
	var rd RedisData
	if err := json.Unmarshal([]byte(jsonStr), &rd); err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(rd.Data, &v); err != nil {
		return nil, err
	}
	if time.Now().Before(rd.ExpireTime) {
		return &v, nil
	}
	// 已过期：抢锁重建
	ok, err := l.TryLock(ctx, lockKey, 10*time.Second)
	if err != nil {
		return nil, err
	}
	if ok {
		go func() {
			rebuildCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			defer func() { _ = l.Unlock(rebuildCtx, lockKey) }()
			value, err := loader(rebuildCtx)
			if err != nil {
				slog.Error("logical expire rebuild failed", "key", key, "err", err)
				return
			}
			b, _ := json.Marshal(value)
			rd := RedisData{Data: b, ExpireTime: time.Now().Add(ttl)}
			out, _ := json.Marshal(rd)
			_ = c.Set(rebuildCtx, key, string(out), 0) // 不设物理 TTL，靠逻辑过期
		}()
	}
	return &v, nil
}
