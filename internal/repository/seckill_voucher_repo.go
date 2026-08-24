package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"gorm.io/gorm"
)

type SeckillVoucherRepo struct{ db *gorm.DB }

func NewSeckillVoucherRepo(db *gorm.DB) *SeckillVoucherRepo { return &SeckillVoucherRepo{db: db} }

func (r *SeckillVoucherRepo) GetByID(ctx context.Context, voucherID int64) (*model.SeckillVoucher, error) {
	var sv model.SeckillVoucher
	err := r.db.WithContext(ctx).First(&sv, voucherID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("seckill voucher %d: %w", voucherID, errs.ErrNotFound)
	}
	return &sv, err
}

func (r *SeckillVoucherRepo) Create(ctx context.Context, sv *model.SeckillVoucher) error {
	return r.db.WithContext(ctx).Create(sv).Error
}

// DecrStock 乐观扣减：WHERE stock > 0，RowsAffected==1 表示成功。
func (r *SeckillVoucherRepo) DecrStock(ctx context.Context, voucherID int64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.SeckillVoucher{}).
		Where("voucher_id = ? AND stock > 0", voucherID).
		UpdateColumn("stock", gorm.Expr("stock - 1"))
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (r *SeckillVoucherRepo) IncrStock(ctx context.Context, voucherID int64) error {
	return r.db.WithContext(ctx).Model(&model.SeckillVoucher{}).
		Where("voucher_id = ?", voucherID).
		UpdateColumn("stock", gorm.Expr("stock + 1")).Error
}

// ListFinished 已结束的秒杀券（对账任务使用）。
func (r *SeckillVoucherRepo) ListFinished(ctx context.Context) ([]*model.SeckillVoucher, error) {
	var vouchers []*model.SeckillVoucher
	err := r.db.WithContext(ctx).Where("end_time < ?", time.Now()).Find(&vouchers).Error
	return vouchers, err
}
