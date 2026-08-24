package repository

import (
	"context"

	"inskill/internal/model"

	"gorm.io/gorm"
)

type ShopTypeRepo struct{ db *gorm.DB }

func NewShopTypeRepo(db *gorm.DB) *ShopTypeRepo { return &ShopTypeRepo{db: db} }

func (r *ShopTypeRepo) ListBySort(ctx context.Context) ([]*model.ShopType, error) {
	var types []*model.ShopType
	err := r.db.WithContext(ctx).Order("sort ASC").Find(&types).Error
	return types, err
}
