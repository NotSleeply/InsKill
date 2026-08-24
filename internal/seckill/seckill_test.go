package seckill

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestPreDeductHappyPath(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	rdb.Set(ctx, "seckill:stock:10", 100, 0)
	r, err := PreDeduct(ctx, rdb, 10, 1)
	if err != nil || r != 0 {
		t.Fatalf("PreDeduct = %d, %v, want 0", r, err)
	}
	stock, _ := rdb.Get(ctx, "seckill:stock:10").Int64()
	if stock != 99 {
		t.Errorf("stock = %d, want 99", stock)
	}
	isMember, _ := rdb.SIsMember(ctx, "seckill:order:10", "1").Result()
	if !isMember {
		t.Error("user 1 should be in order set")
	}
}

func TestPreDeductDuplicated(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	rdb.Set(ctx, "seckill:stock:10", 100, 0)
	_, _ = PreDeduct(ctx, rdb, 10, 1)
	r, err := PreDeduct(ctx, rdb, 10, 1)
	if err != nil || r != 2 {
		t.Fatalf("second PreDeduct = %d, %v, want 2", r, err)
	}
}

func TestPreDeductStockEmpty(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	rdb.Set(ctx, "seckill:stock:10", 0, 0)
	r, err := PreDeduct(ctx, rdb, 10, 1)
	if err != nil || r != 1 {
		t.Fatalf("PreDeduct = %d, %v, want 1", r, err)
	}
}

func TestPreDeductNoStockKey(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	r, err := PreDeduct(context.Background(), rdb, 99, 1)
	if err != nil || r != -1 {
		t.Fatalf("PreDeduct = %d, %v, want -1", r, err)
	}
}
