package service

import (
	"context"
	"testing"
	"time"

	"inskill/internal/cache"
	"inskill/internal/lock"
	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type fakeShopRepo struct {
	shops map[int64]*model.Shop
}

func (f *fakeShopRepo) GetByID(_ context.Context, id int64) (*model.Shop, error) {
	if s, ok := f.shops[id]; ok {
		return s, nil
	}
	return nil, errs.ErrNotFound
}

// 其余方法占位实现，测试未用到的返回空即可
func (f *fakeShopRepo) GetByIDsInOrder(context.Context, []int64) ([]*model.Shop, error) {
	return nil, nil
}
func (f *fakeShopRepo) PageByType(context.Context, int64, int, int) ([]*model.Shop, error) {
	return nil, nil
}
func (f *fakeShopRepo) PageByName(context.Context, string, int, int) ([]*model.Shop, error) {
	return nil, nil
}
func (f *fakeShopRepo) ListAll(context.Context) ([]*model.Shop, error) { return nil, nil }
func (f *fakeShopRepo) Create(context.Context, *model.Shop) error     { return nil }
func (f *fakeShopRepo) Update(context.Context, *model.Shop) error     { return nil }

func TestShopGetByIDRebuildsStaleCache(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c, err := cache.NewRedisClient(mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeShopRepo{shops: map[int64]*model.Shop{1: {ID: 1, Name: "海底捞"}}}
	svc := NewShopService(repo, c, lock.NewRedisLock(rdb), rdb)
	// 预置过期缓存
	_ = c.Set(context.Background(), "cache:shop:1",
		`{"data":{"id":1,"name":"旧店名","typeId":0,"images":"","area":"","address":"","x":0,"y":0,"avgPrice":0,"sold":0,"comments":0,"score":0,"openHours":""},"expireTime":"2000-01-01T00:00:00Z"}`, 0)
	got, err := svc.GetByID(context.Background(), 1)
	if err != nil || got.Name != "旧店名" {
		t.Fatalf("GetByID = %v, %v (stale should be returned)", got, err)
	}
	// 等待异步重建完成
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err = svc.GetByID(context.Background(), 1)
		if err == nil && got.Name == "海底捞" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("after rebuild GetByID = %v, %v", got, err)
}
