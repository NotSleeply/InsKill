package repository

import (
	"context"
	"errors"
	"fmt"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"gorm.io/gorm"
)

type VoucherRepo struct{ db *gorm.DB }

func NewVoucherRepo(db *gorm.DB) *VoucherRepo { return &VoucherRepo{db: db} }

func (r *VoucherRepo) GetByID(ctx context.Context, id int64) (*model.Voucher, error) {
	var v model.Voucher
	err := r.db.WithContext(ctx).First(&v, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("voucher %d: %w", id, errs.ErrNotFound)
	}
	return &v, err
}

// ListByShop 店铺优惠券列表，LEFT JOIN 秒杀表补 stock/begin_time/end_time（对齐 VoucherMapper.xml）。
func (r *VoucherRepo) ListByShop(ctx context.Context, shopID int64) ([]*model.Voucher, error) {
	var vouchers []*model.Voucher
	err := r.db.WithContext(ctx).Raw(`
		SELECT v.id, v.shop_id, v.title, v.sub_title, v.rules, v.pay_value,
		       v.actual_value, v.type, sv.stock, sv.begin_time, sv.end_time
		FROM voucher v
		LEFT JOIN seckill_voucher sv ON v.id = sv.voucher_id
		WHERE v.shop_id = ? AND v.status = 1`, shopID).Scan(&vouchers).Error
	return vouchers, err
}

func (r *VoucherRepo) Create(ctx context.Context, v *model.Voucher) error {
	return r.db.WithContext(ctx).Create(v).Error
}
