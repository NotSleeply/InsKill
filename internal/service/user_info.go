package service

import (
	"context"
	"errors"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"
)

// UserInfoRepository 用户详情数据访问接口。
type UserInfoRepository interface {
	GetByID(ctx context.Context, userID int64) (*model.UserInfo, error)
}

type UserInfoService interface {
	Get(ctx context.Context, userID int64) (*model.UserInfo, error)
}

type userInfoService struct {
	repo UserInfoRepository
}

func NewUserInfoService(repo UserInfoRepository) UserInfoService { return &userInfoService{repo: repo} }

// Get 返回用户详情；不存在返回 nil（对齐 Java info 端点首次查看返回空）。
func (s *userInfoService) Get(ctx context.Context, userID int64) (*model.UserInfo, error) {
	info, err := s.repo.GetByID(ctx, userID)
	if errors.Is(err, errs.ErrNotFound) {
		return nil, nil
	}
	return info, err
}
