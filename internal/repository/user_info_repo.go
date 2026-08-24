package repository

import (
	"context"
	"errors"
	"fmt"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"gorm.io/gorm"
)

type UserInfoRepo struct{ db *gorm.DB }

func NewUserInfoRepo(db *gorm.DB) *UserInfoRepo { return &UserInfoRepo{db: db} }

func (r *UserInfoRepo) GetByID(ctx context.Context, userID int64) (*model.UserInfo, error) {
	var info model.UserInfo
	err := r.db.WithContext(ctx).First(&info, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("user info %d: %w", userID, errs.ErrNotFound)
	}
	return &info, err
}
