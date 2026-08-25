package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"inskill/internal/mq"

	"github.com/alicebob/miniredis/v2"
)

type fakeLock struct {
	held atomic.Bool
}

func (f *fakeLock) TryLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return f.held.CompareAndSwap(false, true), nil
}

func (f *fakeLock) Unlock(_ context.Context, _ string) error {
	f.held.Store(false)
	return nil
}

func newTestCache(t *testing.T) (Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	c, err := NewRedisClient(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedisClient: %v", err)
	}
	return c, mr
}

type testShop struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func TestPassThroughCachesValue(t *testing.T) {
	c, _ := newTestCache(t)
	var calls atomic.Int32
	loader := func(_ context.Context) (*testShop, error) {
		calls.Add(1)
		return &testShop{ID: 1, Name: "海底捞"}, nil
	}
	got, err := QueryWithPassThrough(c, context.Background(), "cache:shop:1", loader, 30*time.Minute)
	if err != nil || got.Name != "海底捞" {
		t.Fatalf("first query = %v, %v", got, err)
	}
	got, err = QueryWithPassThrough(c, context.Background(), "cache:shop:1", loader, 30*time.Minute)
	if err != nil || got.Name != "海底捞" {
		t.Fatalf("second query = %v, %v", got, err)
	}
	if calls.Load() != 1 {
		t.Errorf("loader calls = %d, want 1 (cache hit)", calls.Load())
	}
}

func TestPassThroughCachesEmptyValue(t *testing.T) {
	c, mr := newTestCache(t)
	var calls atomic.Int32
	loader := func(_ context.Context) (*testShop, error) {
		calls.Add(1)
		return nil, nil // 模拟 DB 无此记录
	}
	got, err := QueryWithPassThrough(c, context.Background(), "cache:shop:999", loader, 30*time.Minute)
	if err != nil || got != nil {
		t.Fatalf("got = %v, %v", got, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("loader calls = %d", calls.Load())
	}
	// 空值已缓存
	got, err = QueryWithPassThrough(c, context.Background(), "cache:shop:999", loader, 30*time.Minute)
	if err != nil || got != nil {
		t.Fatalf("second got = %v, %v", got, err)
	}
	if calls.Load() != 1 {
		t.Errorf("loader calls = %d, want 1", calls.Load())
	}
	if ttl := mr.TTL("cache:shop:999"); ttl < 119*time.Second || ttl > 120*time.Second {
		t.Errorf("empty cache ttl = %v, want ~120s", ttl)
	}
}

func TestLogicalExpireReturnsStaleAndRebuilds(t *testing.T) {
	c, _ := newTestCache(t)
	l := &fakeLock{}
	ctx := context.Background()
	// 预置一条已过期的逻辑缓存
	if err := c.Set(ctx, "cache:shop:1", `{"data":{"id":1,"name":"旧店名"},"expireTime":"2000-01-01T00:00:00Z"}`, 0); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	loader := func(_ context.Context) (*testShop, error) {
		calls.Add(1)
		return &testShop{ID: 1, Name: "新店名"}, nil
	}
	got, err := QueryWithLogicalExpire(c, l, ctx, "cache:shop:1", "lock:shop:1", loader, 30*time.Minute)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Name != "旧店名" {
		t.Errorf("stale data = %q, want 旧店名", got.Name)
	}
	// 重建是异步 goroutine，轮询等待其完成
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Errorf("rebuild loader calls = %d, want 1", calls.Load())
	}
}

func TestLogicalExpireRebuildLocked(t *testing.T) {
	c, _ := newTestCache(t)
	l := &fakeLock{}
	l.held.Store(true) // 模拟别的请求已抢到重建锁
	ctx := context.Background()
	_ = c.Set(ctx, "cache:shop:1", `{"data":{"id":1,"name":"旧店名"},"expireTime":"2000-01-01T00:00:00Z"}`, 0)
	var calls atomic.Int32
	loader := func(_ context.Context) (*testShop, error) {
		calls.Add(1)
		return &testShop{ID: 1, Name: "新店名"}, nil
	}
	got, err := QueryWithLogicalExpire(c, l, ctx, "cache:shop:1", "lock:shop:1", loader, 30*time.Minute)
	if err != nil || got.Name != "旧店名" {
		t.Fatalf("got = %v, %v", got, err)
	}
	if calls.Load() != 0 {
		t.Errorf("loader calls = %d, want 0 (lock held)", calls.Load())
	}
}

// recordingPublisher 记录发往各 topic 的消息，供补偿链路测试。
type recordingPublisher struct {
	mu    sync.Mutex
	delMsgs []cacheDelMessage
}

func (r *recordingPublisher) Publish(_ context.Context, topic string, body []byte) error {
	if topic != mq.TopicCacheDel {
		return nil
	}
	var msg cacheDelMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delMsgs = append(r.delMsgs, msg)
	return nil
}

func (r *recordingPublisher) Shutdown() error { return nil }

func TestDelAndCompensateNoMessageWhenDelSucceeds(t *testing.T) {
	c, _ := newTestCache(t)
	pub := &recordingPublisher{}
	if err := c.Set(context.Background(), "k", "v", time.Minute); err != nil {
		t.Fatal(err)
	}
	DelAndCompensate(context.Background(), c, pub, "k")
	if len(pub.delMsgs) != 0 {
		t.Errorf("del messages = %d, want 0 (del succeeded)", len(pub.delMsgs))
	}
}

// failingClient 模拟删除失败（如 Redis 连接异常）。
type failingClient struct{ Client }

func (failingClient) Del(context.Context, ...string) error { return errors.New("redis down") }

func TestDelAndCompensateSendsMessageOnDelFailure(t *testing.T) {
	c, _ := newTestCache(t)
	pub := &recordingPublisher{}
	DelAndCompensate(context.Background(), failingClient{c}, pub, "cache:shop:1")
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.delMsgs) != 1 || pub.delMsgs[0].Key != "cache:shop:1" {
		t.Fatalf("del messages = %+v, want [cache:shop:1]", pub.delMsgs)
	}
}

func TestHandleCacheDelMessageDeletesKey(t *testing.T) {
	c, _ := newTestCache(t)
	if err := c.Set(context.Background(), "k", "v", time.Minute); err != nil {
		t.Fatal(err)
	}
	handle := HandleCacheDelMessage(c)
	body, _ := json.Marshal(cacheDelMessage{Key: "k"})
	if err := handle(context.Background(), body); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "k"); !errors.Is(err, ErrNil) {
		t.Errorf("key still present after compensate del: %v", err)
	}
}
