package repository

import (
	"context"
	"errors"
	"testing"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Shop{}, &model.ShopType{}, &model.Voucher{},
		&model.SeckillVoucher{}, &model.VoucherOrder{}, &model.Blog{}, &model.BlogComments{},
		&model.Follow{}, &model.UserInfo{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestUserRepoGetByPhoneNotFound(t *testing.T) {
	repo := NewUserRepo(newTestDB(t))
	_, err := repo.GetByPhone(context.Background(), "13800000000")
	if !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUserRepoCreateAndGet(t *testing.T) {
	repo := NewUserRepo(newTestDB(t))
	u := &model.User{Phone: "13800000000", NickName: "user_test"}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected auto-increment id")
	}
	got, err := repo.GetByPhone(context.Background(), "13800000000")
	if err != nil {
		t.Fatalf("get by phone: %v", err)
	}
	if got.NickName != "user_test" {
		t.Errorf("nick_name = %q", got.NickName)
	}
}

func TestVoucherOrderMarkPaidOptimistic(t *testing.T) {
	repo := NewVoucherOrderRepo(newTestDB(t))
	o := &model.VoucherOrder{ID: 1001, UserID: 1, VoucherID: 1, Status: model.OrderStatusUnpaid}
	if err := repo.Create(context.Background(), o); err != nil {
		t.Fatalf("create: %v", err)
	}
	ok, err := repo.MarkPaid(context.Background(), 1001)
	if err != nil || !ok {
		t.Fatalf("first MarkPaid = %v, %v", ok, err)
	}
	ok, err = repo.MarkPaid(context.Background(), 1001)
	if err != nil {
		t.Fatalf("second MarkPaid err: %v", err)
	}
	if ok {
		t.Fatal("second MarkPaid should fail (status already paid)")
	}
}

func TestVoucherOrderCancelIfUnpaid(t *testing.T) {
	repo := NewVoucherOrderRepo(newTestDB(t))
	o := &model.VoucherOrder{ID: 1002, UserID: 1, VoucherID: 1, Status: model.OrderStatusUnpaid}
	if err := repo.Create(context.Background(), o); err != nil {
		t.Fatalf("create: %v", err)
	}
	ok, err := repo.CancelIfUnpaid(context.Background(), 1002)
	if err != nil || !ok {
		t.Fatalf("first CancelIfUnpaid = %v, %v", ok, err)
	}
	ok, _ = repo.CancelIfUnpaid(context.Background(), 1002)
	if ok {
		t.Fatal("second CancelIfUnpaid should fail")
	}
}

func TestSeckillVoucherDecrStock(t *testing.T) {
	repo := NewSeckillVoucherRepo(newTestDB(t))
	sv := &model.SeckillVoucher{VoucherID: 1, Stock: 2}
	if err := repo.Create(context.Background(), sv); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 2; i++ {
		ok, err := repo.DecrStock(context.Background(), 1)
		if err != nil || !ok {
			t.Fatalf("DecrStock #%d = %v, %v", i+1, ok, err)
		}
	}
	ok, err := repo.DecrStock(context.Background(), 1)
	if err != nil {
		t.Fatalf("DecrStock err: %v", err)
	}
	if ok {
		t.Fatal("DecrStock should fail when stock is 0")
	}
}
