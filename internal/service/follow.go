package service

import (
	"context"
	"strconv"

	"inskill/internal/model"

	"github.com/redis/go-redis/v9"
)

const followKey = "follows:"

// FollowRepository 关注数据访问接口。
type FollowRepository interface {
	Create(ctx context.Context, f *model.Follow) error
	Delete(ctx context.Context, userID, followUserID int64) error
	Exists(ctx context.Context, userID, followUserID int64) (bool, error)
}

type FollowService interface {
	Follow(ctx context.Context, userID, followUserID int64, isFollow bool) error
	IsFollow(ctx context.Context, userID, followUserID int64) (bool, error)
	Commons(ctx context.Context, userID, otherID int64) ([]*model.UserDTO, error)
}

type followService struct {
	repo     FollowRepository
	rdb      *redis.Client
	userRepo UserReader
}

func NewFollowService(repo FollowRepository, rdb *redis.Client, userRepo UserReader) FollowService {
	return &followService{repo: repo, rdb: rdb, userRepo: userRepo}
}

// Follow 关注/取关：DB 与 Redis Set 双写（对齐 Java follow）。
func (s *followService) Follow(ctx context.Context, userID, followUserID int64, isFollow bool) error {
	key := followKey + strconv.FormatInt(userID, 10)
	if isFollow {
		if err := s.repo.Create(ctx, &model.Follow{UserID: userID, FollowUserID: followUserID}); err != nil {
			return err
		}
		return s.rdb.SAdd(ctx, key, strconv.FormatInt(followUserID, 10)).Err()
	}
	if err := s.repo.Delete(ctx, userID, followUserID); err != nil {
		return err
	}
	return s.rdb.SRem(ctx, key, strconv.FormatInt(followUserID, 10)).Err()
}

func (s *followService) IsFollow(ctx context.Context, userID, followUserID int64) (bool, error) {
	return s.repo.Exists(ctx, userID, followUserID)
}

// Commons 共同关注：SINTER 两个用户的关注集合，再查用户信息（对齐 Java followCommons）。
func (s *followService) Commons(ctx context.Context, userID, otherID int64) ([]*model.UserDTO, error) {
	ids, err := s.rdb.SInter(ctx, followKey+strconv.FormatInt(userID, 10), followKey+strconv.FormatInt(otherID, 10)).Result()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 || s.userRepo == nil {
		return []*model.UserDTO{}, nil
	}
	id64s := make([]int64, 0, len(ids))
	for _, id := range ids {
		v, _ := strconv.ParseInt(id, 10, 64)
		id64s = append(id64s, v)
	}
	users, err := s.userRepo.GetByIDs(ctx, id64s)
	if err != nil {
		return nil, err
	}
	out := make([]*model.UserDTO, 0, len(users))
	for _, u := range users {
		out = append(out, &model.UserDTO{ID: u.ID, NickName: u.NickName, Icon: u.Icon})
	}
	return out, nil
}
