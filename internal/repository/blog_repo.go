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

type BlogRepo struct{ db *gorm.DB }

func NewBlogRepo(db *gorm.DB) *BlogRepo { return &BlogRepo{db: db} }

func (r *BlogRepo) GetByID(ctx context.Context, id int64) (*model.Blog, error) {
	var b model.Blog
	err := r.db.WithContext(ctx).First(&b, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("blog %d: %w", id, errs.ErrNotFound)
	}
	return &b, err
}

func (r *BlogRepo) HotPage(ctx context.Context, offset, limit int) ([]*model.Blog, error) {
	var blogs []*model.Blog
	err := r.db.WithContext(ctx).Order("liked DESC").
		Offset(offset).Limit(limit).Find(&blogs).Error
	return blogs, err
}

func (r *BlogRepo) PageByUser(ctx context.Context, userID int64, offset, limit int) ([]*model.Blog, error) {
	var blogs []*model.Blog
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Offset(offset).Limit(limit).Find(&blogs).Error
	return blogs, err
}

// GetByIDsInOrder 按 ids 顺序返回（MySQL FIELD 方言）。
func (r *BlogRepo) GetByIDsInOrder(ctx context.Context, ids []int64) ([]*model.Blog, error) {
	if len(ids) == 0 {
		return []*model.Blog{}, nil
	}
	placeholders := make([]string, len(ids))
	for i := range ids {
		placeholders[i] = "?"
	}
	var blogs []*model.Blog
	err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Order("FIELD(id, " + strings.Join(placeholders, ",") + ")").
		Find(&blogs).Error
	return blogs, err
}

func (r *BlogRepo) Create(ctx context.Context, b *model.Blog) error {
	return r.db.WithContext(ctx).Create(b).Error
}

func (r *BlogRepo) IncrLiked(ctx context.Context, id int64, delta int) error {
	return r.db.WithContext(ctx).Model(&model.Blog{}).Where("id = ?", id).
		UpdateColumn("liked", gorm.Expr("liked + ?", delta)).Error
}
