package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"gorm.io/gorm"
)

type ShopRepo struct{ db *gorm.DB }

func NewShopRepo(db *gorm.DB) *ShopRepo { return &ShopRepo{db: db} }

func (r *ShopRepo) GetByID(ctx context.Context, id int64) (*model.Shop, error) {
	var s model.Shop
	err := r.db.WithContext(ctx).First(&s, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("shop %d: %w", id, errs.ErrNotFound)
	}
	return &s, err
}

// GetByIDsInOrder 按 ids 顺序返回，等价 Java 版 order by field(id,...)。MySQL 方言。
func (r *ShopRepo) GetByIDsInOrder(ctx context.Context, ids []int64) ([]*model.Shop, error) {
	if len(ids) == 0 {
		return []*model.Shop{}, nil
	}
	placeholders := make([]string, len(ids))
	for i := range ids {
		placeholders[i] = "?"
	}
	var shops []*model.Shop
	err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Order("FIELD(id, " + strings.Join(placeholders, ",") + ")").
		Find(&shops).Error
	return shops, err
}

func (r *ShopRepo) PageByType(ctx context.Context, typeID int64, offset, limit int) ([]*model.Shop, error) {
	var shops []*model.Shop
	err := r.db.WithContext(ctx).
		Where("type_id = ?", typeID).
		Offset(offset).Limit(limit).Find(&shops).Error
	return shops, err
}

func (r *ShopRepo) PageByName(ctx context.Context, name string, offset, limit int) ([]*model.Shop, error) {
	q := r.db.WithContext(ctx).Model(&model.Shop{})
	if name != "" {
		q = q.Where("name LIKE ?", "%"+name+"%")
	}
	var shops []*model.Shop
	err := q.Offset(offset).Limit(limit).Find(&shops).Error
	return shops, err
}

func (r *ShopRepo) ListAll(ctx context.Context) ([]*model.Shop, error) {
	var shops []*model.Shop
	err := r.db.WithContext(ctx).Find(&shops).Error
	return shops, err
}

func (r *ShopRepo) Create(ctx context.Context, s *model.Shop) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *ShopRepo) Update(ctx context.Context, s *model.Shop) error {
	return r.db.WithContext(ctx).Model(&model.Shop{}).Where("id = ?", s.ID).
		Updates(map[string]any{
			"name": s.Name, "type_id": s.TypeID, "images": s.Images,
			"area": s.Area, "address": s.Address, "x": s.X, "y": s.Y,
			"avg_price": s.AvgPrice, "sold": s.Sold, "comments": s.Comments,
			"score": s.Score, "open_hours": s.OpenHours,
		}).Error
}
