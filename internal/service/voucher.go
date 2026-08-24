package service

import (
	"context"
	"strconv"

	"inskill/internal/model"

	"github.com/redis/go-redis/v9"
)

const seckillStockKey = "seckill:stock:"

// VoucherRepository 优惠券数据访问接口。
type VoucherRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Voucher, error)
	ListByShop(ctx context.Context, shopID int64) ([]*model.Voucher, error)
	Create(ctx context.Context, v *model.Voucher) error
}

// SeckillVoucherRepository 秒杀券数据访问接口。
type SeckillVoucherRepository interface {
	Create(ctx context.Context, sv *model.SeckillVoucher) error
}

type VoucherService interface {
	Create(ctx context.Context, v *model.Voucher) error
	ListByShop(ctx context.Context, shopID int64) ([]*model.Voucher, error)
}

type voucherService struct {
	repo        VoucherRepository
	seckillRepo SeckillVoucherRepository
	rdb         *redis.Client
}

func NewVoucherService(repo VoucherRepository, seckillRepo SeckillVoucherRepository, rdb *redis.Client) VoucherService {
	return &voucherService{repo: repo, seckillRepo: seckillRepo, rdb: rdb}
}

// Create 保存优惠券；秒杀券额外写 seckill_voucher 并预热 Redis 库存（对齐 Java addSeckillVoucher）。
func (s *voucherService) Create(ctx context.Context, v *model.Voucher) error {
	if err := s.repo.Create(ctx, v); err != nil {
		return err
	}
	if v.Type != 1 {
		return nil
	}
	sv := &model.SeckillVoucher{
		VoucherID: v.ID,
		Stock:     v.Stock,
		BeginTime: v.BeginTime,
		EndTime:   v.EndTime,
	}
	if err := s.seckillRepo.Create(ctx, sv); err != nil {
		return err
	}
	return s.rdb.Set(ctx, seckillStockKey+strconv.FormatInt(v.ID, 10), v.Stock, 0).Err()
}

func (s *voucherService) ListByShop(ctx context.Context, shopID int64) ([]*model.Voucher, error) {
	return s.repo.ListByShop(ctx, shopID)
}
