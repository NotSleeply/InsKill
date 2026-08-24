package service

import (
	"context"
	"errors"
	"testing"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type fakePublisher struct {
	msgs [][]byte
}

func (f *fakePublisher) Publish(_ context.Context, _ string, body []byte) error {
	f.msgs = append(f.msgs, body)
	return nil
}
func (f *fakePublisher) Shutdown() error { return nil }

type fakeOrderRepo struct {
	orders map[int64]*model.VoucherOrder
}

func (f *fakeOrderRepo) Create(_ context.Context, o *model.VoucherOrder) error {
	if _, exists := f.orders[o.ID]; exists {
		return errors.New("duplicate primary key")
	}
	f.orders[o.ID] = o
	return nil
}

type fakeStockRepo struct {
	stock int32
}

func (f *fakeStockRepo) DecrStock(_ context.Context, _ int64) (bool, error) {
	if f.stock <= 0 {
		return false, nil
	}
	f.stock--
	return true, nil
}

func TestSeckillPublishesOrderMessage(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	rdb.Set(ctx, "seckill:stock:10", 100, 0)
	pub := &fakePublisher{}
	svc := NewVoucherOrderService(&fakeOrderRepo{orders: map[int64]*model.VoucherOrder{}},
		&fakeStockRepo{stock: 100}, pub, rdb)
	orderID, err := svc.Seckill(ctx, 1, 10)
	if err != nil {
		t.Fatalf("Seckill: %v", err)
	}
	if orderID == 0 {
		t.Fatal("empty order id")
	}
	if len(pub.msgs) != 1 {
		t.Fatalf("published messages = %d, want 1", len(pub.msgs))
	}
}

func TestSeckillDuplicatedReturnsError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	rdb.Set(ctx, "seckill:stock:10", 100, 0)
	svc := NewVoucherOrderService(&fakeOrderRepo{orders: map[int64]*model.VoucherOrder{}},
		&fakeStockRepo{stock: 100}, &fakePublisher{}, rdb)
	_, _ = svc.Seckill(ctx, 1, 10)
	_, err := svc.Seckill(ctx, 1, 10)
	if !errors.Is(err, errs.ErrDuplicatedOrder) {
		t.Fatalf("err = %v, want ErrDuplicatedOrder", err)
	}
}

func TestSeckillStockEmptyReturnsError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	rdb.Set(ctx, "seckill:stock:10", 0, 0)
	svc := NewVoucherOrderService(&fakeOrderRepo{orders: map[int64]*model.VoucherOrder{}},
		&fakeStockRepo{stock: 0}, &fakePublisher{}, rdb)
	_, err := svc.Seckill(ctx, 1, 10)
	if !errors.Is(err, errs.ErrStockEmpty) {
		t.Fatalf("err = %v, want ErrStockEmpty", err)
	}
}

func TestHandleOrderMessageIdempotent(t *testing.T) {
	repo := &fakeOrderRepo{orders: map[int64]*model.VoucherOrder{}}
	svc := NewVoucherOrderService(repo, &fakeStockRepo{stock: 100}, &fakePublisher{}, nil)
	body := []byte(`{"id":1001,"userId":1,"voucherId":10,"payType":1,"status":1}`)
	if err := svc.HandleOrderMessage(context.Background(), body); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if err := svc.HandleOrderMessage(context.Background(), body); err != nil {
		t.Fatalf("duplicate handle should be nil, got %v", err)
	}
	if len(repo.orders) != 1 {
		t.Fatalf("orders = %d, want 1", len(repo.orders))
	}
}
