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

type VoucherOrderRepo struct{ db *gorm.DB }

func NewVoucherOrderRepo(db *gorm.DB) *VoucherOrderRepo { return &VoucherOrderRepo{db: db} }

func (r *VoucherOrderRepo) Create(ctx context.Context, o *model.VoucherOrder) error {
	return r.db.WithContext(ctx).Create(o).Error
}

func (r *VoucherOrderRepo) GetByID(ctx context.Context, id int64) (*model.VoucherOrder, error) {
	var o model.VoucherOrder
	err := r.db.WithContext(ctx).First(&o, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("order %d: %w", id, errs.ErrNotFound)
	}
	return &o, err
}

// MarkPaid 支付成功乐观锁：WHERE status = 1，RowsAffected==1 表示抢到。
func (r *VoucherOrderRepo) MarkPaid(ctx context.Context, orderID int64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.VoucherOrder{}).
		Where("id = ? AND status = ?", orderID, model.OrderStatusUnpaid).
		Updates(map[string]any{"status": model.OrderStatusPaid, "pay_time": time.Now()})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// FindTimeoutIDs 超时未支付订单 id 列表（只读）。
func (r *VoucherOrderRepo) FindTimeoutIDs(ctx context.Context, before time.Time) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Model(&model.VoucherOrder{}).
		Where("status = ? AND create_time < ?", model.OrderStatusUnpaid, before).
		Pluck("id", &ids).Error
	return ids, err
}

// CancelIfUnpaid 关单乐观锁：WHERE status = 1，RowsAffected==1 表示取消成功。
func (r *VoucherOrderRepo) CancelIfUnpaid(ctx context.Context, orderID int64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.VoucherOrder{}).
		Where("id = ? AND status = ?", orderID, model.OrderStatusUnpaid).
		Update("status", model.OrderStatusCanceled)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// CountByVoucher 某券的有效订单数（对账：已取消的不算）。
func (r *VoucherOrderRepo) CountByVoucher(ctx context.Context, voucherID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.VoucherOrder{}).
		Where("voucher_id = ? AND status <> ?", voucherID, model.OrderStatusCanceled).
		Count(&n).Error
	return n, err
}
