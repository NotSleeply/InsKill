package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type fakeLifecycleRepo struct {
	orders        map[int64]*model.VoucherOrder
	stockByVoucher map[int64]int32
}

func (f *fakeLifecycleRepo) Create(_ context.Context, o *model.VoucherOrder) error {
	f.orders[o.ID] = o
	return nil
}

func (f *fakeLifecycleRepo) GetByID(_ context.Context, id int64) (*model.VoucherOrder, error) {
	o, ok := f.orders[id]
	if !ok {
		return nil, errs.ErrNotFound
	}
	return o, nil
}

func (f *fakeLifecycleRepo) MarkPaid(_ context.Context, id int64) (bool, error) {
	o := f.orders[id]
	if o == nil || o.Status != model.OrderStatusUnpaid {
		return false, nil
	}
	o.Status = model.OrderStatusPaid
	return true, nil
}

func (f *fakeLifecycleRepo) FindTimeoutIDs(_ context.Context, before time.Time) ([]int64, error) {
	var ids []int64
	for id, o := range f.orders {
		if o.Status == model.OrderStatusUnpaid && o.CreateTime.Before(before) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (f *fakeLifecycleRepo) CancelIfUnpaid(_ context.Context, id int64) (bool, error) {
	o := f.orders[id]
	if o == nil || o.Status != model.OrderStatusUnpaid {
		return false, nil
	}
	o.Status = model.OrderStatusCanceled
	return true, nil
}

func (f *fakeLifecycleRepo) CountByVoucher(_ context.Context, voucherID int64) (int64, error) {
	var n int64
	for _, o := range f.orders {
		if o.VoucherID == voucherID && o.Status != model.OrderStatusCanceled {
			n++
		}
	}
	return n, nil
}

func TestPayCallbackMarksPaid(t *testing.T) {
	repo := &fakeLifecycleRepo{orders: map[int64]*model.VoucherOrder{
		1: {ID: 1, Status: model.OrderStatusUnpaid},
	}, stockByVoucher: map[int64]int32{}}
	svc := &orderLifecycleService{repo: repo, rdb: nil, pub: &fakePublisher{}, timeout: time.Hour}
	if err := svc.PayCallback(context.Background(), 1); err != nil {
		t.Fatalf("PayCallback: %v", err)
	}
	if repo.orders[1].Status != model.OrderStatusPaid {
		t.Errorf("status = %d, want paid", repo.orders[1].Status)
	}
}

func TestPayCallbackOnClosedOrder(t *testing.T) {
	repo := &fakeLifecycleRepo{orders: map[int64]*model.VoucherOrder{
		1: {ID: 1, Status: model.OrderStatusCanceled},
	}, stockByVoucher: map[int64]int32{}}
	svc := &orderLifecycleService{repo: repo, rdb: nil, pub: &fakePublisher{}, timeout: time.Hour}
	err := svc.PayCallback(context.Background(), 1)
	if !errors.Is(err, errs.ErrOrderClosed) {
		t.Fatalf("err = %v, want ErrOrderClosed", err)
	}
}

func TestCloseTimeoutOrdersRefundsStock(t *testing.T) {
	pub := &fakePublisher{}
	repo := &fakeLifecycleRepo{orders: map[int64]*model.VoucherOrder{
		1: {ID: 1, Status: model.OrderStatusUnpaid, VoucherID: 10, CreateTime: time.Now().Add(-time.Hour)},
		2: {ID: 2, Status: model.OrderStatusUnpaid, VoucherID: 10, CreateTime: time.Now()}, // 未超时
	}, stockByVoucher: map[int64]int32{}}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := &orderLifecycleService{repo: repo, rdb: rdb, pub: pub, timeout: time.Hour}
	if err := svc.CloseTimeoutOrders(context.Background()); err != nil {
		t.Fatalf("CloseTimeoutOrders: %v", err)
	}
	if repo.orders[1].Status != model.OrderStatusCanceled {
		t.Error("order 1 should be canceled")
	}
	if repo.orders[2].Status != model.OrderStatusUnpaid {
		t.Error("order 2 should remain unpaid")
	}
	if len(pub.msgs) != 1 {
		t.Fatalf("refund messages = %d, want 1", len(pub.msgs))
	}
}

type fakeStockRepoForLifecycle struct {
	stockByVoucher map[int64]int32
	refunded       []int64
}

func (f *fakeStockRepoForLifecycle) IncrStock(_ context.Context, voucherID int64) error {
	f.stockByVoucher[voucherID]++
	return nil
}

func (f *fakeStockRepoForLifecycle) GetByID(_ context.Context, voucherID int64) (*model.SeckillVoucher, error) {
	return &model.SeckillVoucher{VoucherID: voucherID, Stock: f.stockByVoucher[voucherID]}, nil
}

func (f *fakeStockRepoForLifecycle) ListFinished(_ context.Context) ([]*model.SeckillVoucher, error) {
	return nil, nil
}

func TestHandleRefundMessageIdempotent(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	rdb.Set(ctx, "seckill:stock:10", 5, 0)
	stockRepo := &fakeStockRepoForLifecycle{stockByVoucher: map[int64]int32{10: 5}}
	svc := &orderLifecycleService{repo: &fakeLifecycleRepo{orders: map[int64]*model.VoucherOrder{}},
		stockRepo: stockRepo, rdb: rdb, pub: &fakePublisher{}, timeout: time.Hour}
	body := []byte(`{"orderId":3001,"voucherId":10}`)
	if err := svc.HandleRefundMessage(ctx, body); err != nil {
		t.Fatalf("first refund: %v", err)
	}
	if err := svc.HandleRefundMessage(ctx, body); err != nil {
		t.Fatalf("duplicate refund should be nil, got %v", err)
	}
	if stockRepo.stockByVoucher[10] != 6 {
		t.Errorf("db stock = %d, want 6 (refunded once)", stockRepo.stockByVoucher[10])
	}
	redisStock, _ := rdb.Get(ctx, "seckill:stock:10").Int64()
	if redisStock != 6 {
		t.Errorf("redis stock = %d, want 6", redisStock)
	}
}
