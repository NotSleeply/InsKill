package repository

import (
	"context"

	"inskill/internal/model"

	"gorm.io/gorm"
)

type FollowRepo struct{ db *gorm.DB }

func NewFollowRepo(db *gorm.DB) *FollowRepo { return &FollowRepo{db: db} }

func (r *FollowRepo) Create(ctx context.Context, f *model.Follow) error {
	return r.db.WithContext(ctx).Create(f).Error
}

func (r *FollowRepo) Delete(ctx context.Context, userID, followUserID int64) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND follow_user_id = ?", userID, followUserID).
		Delete(&model.Follow{}).Error
}

func (r *FollowRepo) Exists(ctx context.Context, userID, followUserID int64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Follow{}).
		Where("user_id = ? AND follow_user_id = ?", userID, followUserID).
		Count(&n).Error
	return n > 0, err
}

// ListFollowerIDs 某用户的全部粉丝 id（follow_user_id = ?）。
func (r *FollowRepo) ListFollowerIDs(ctx context.Context, followUserID int64) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Model(&model.Follow{}).
		Where("follow_user_id = ?", followUserID).
		Pluck("user_id", &ids).Error
	return ids, err
}
