package cache

import (
	"time"

	"github.com/dgraph-io/ristretto/v2"
)

// ristrettoCache 进程内二级缓存：W-TinyLFU，只用于极热 key（如秒杀券详情），
// 短 TTL 靠过期自刷，不做跨节点失效广播（对齐 CityHub README 结论）。
type ristrettoCache struct {
	cache *ristretto.Cache[string, any]
}

func newLocal() (*ristrettoCache, error) {
	c, err := ristretto.NewCache(&ristretto.Config[string, any]{
		NumCounters: 1e4,     // 10 倍 MaxCost 经验值
		MaxCost:     1 << 20, // 1MB
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}
	return &ristrettoCache{cache: c}, nil
}

const localTTL = 5 * time.Second // 秒杀券详情等极热 key：5 秒短 TTL

// Get 带 TTL 语义的本地读取。
func (r *ristrettoCache) Get(key string) (any, bool) {
	v, ok := r.cache.Get(key)
	return v, ok
}

func (r *ristrettoCache) Set(key string, v any) {
	r.cache.SetWithTTL(key, v, 1, localTTL)
	r.cache.Wait()
}

func (r *ristrettoCache) Del(key string) {
	r.cache.Del(key)
}
