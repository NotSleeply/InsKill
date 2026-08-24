package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"github.com/redis/go-redis/v9"
)

const (
	blogLikedKey = "blog:liked:"
	feedKey      = "feed:"
	feedPageSize = 2 // 对齐 Java 版 reverseRangeByScoreWithScores count=2
)

// BlogRepository 探店笔记数据访问接口。
type BlogRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Blog, error)
	HotPage(ctx context.Context, offset, limit int) ([]*model.Blog, error)
	PageByUser(ctx context.Context, userID int64, offset, limit int) ([]*model.Blog, error)
	GetByIDsInOrder(ctx context.Context, ids []int64) ([]*model.Blog, error)
	Create(ctx context.Context, b *model.Blog) error
	IncrLiked(ctx context.Context, id int64, delta int) error
}

// FollowReader 博客推送所需的关注查询（FollowRepo 满足）。
type FollowReader interface {
	ListFollowerIDs(ctx context.Context, followUserID int64) ([]int64, error)
}

// UserReader 博客展示所需的用户查询（UserRepo 满足）。
type UserReader interface {
	GetByIDs(ctx context.Context, ids []int64) ([]*model.User, error)
}

type BlogService interface {
	HotPage(ctx context.Context, page int) ([]*model.Blog, error)
	Like(ctx context.Context, userID, blogID int64) error
	LikeUsers(ctx context.Context, blogID int64) ([]*model.UserDTO, error)
	Create(ctx context.Context, userID int64, blog *model.Blog) error
	Feed(ctx context.Context, userID, lastID int64, offset int) (*model.ScrollResult, error)
	GetByID(ctx context.Context, userID, id int64) (*model.Blog, error)
	MyPage(ctx context.Context, userID int64, page int) ([]*model.Blog, error)
	UserPage(ctx context.Context, userID int64, page int) ([]*model.Blog, error)
}

type blogService struct {
	repo       BlogRepository
	followRepo FollowReader
	userRepo   UserReader
	rdb        *redis.Client
}

func NewBlogService(repo BlogRepository, followRepo FollowReader, rdb *redis.Client, userRepo UserReader) BlogService {
	return &blogService{repo: repo, followRepo: followRepo, rdb: rdb, userRepo: userRepo}
}

// HotPage 热门笔记：liked DESC 分页。
func (s *blogService) HotPage(ctx context.Context, page int) ([]*model.Blog, error) {
	blogs, err := s.repo.HotPage(ctx, (page-1)*10, 10)
	if err != nil {
		return nil, err
	}
	for _, b := range blogs {
		s.fillUser(ctx, b)
		s.fillIsLike(ctx, b, 0) // 未登录不查点赞
	}
	return blogs, nil
}

// Like 点赞/取消：ZSet score 判重，DB liked 同步 ±1（对齐 Java updateLike）。
func (s *blogService) Like(ctx context.Context, userID, blogID int64) error {
	key := blogLikedKey + strconv.FormatInt(blogID, 10)
	_, err := s.rdb.ZScore(ctx, key, strconv.FormatInt(userID, 10)).Result()
	if err == redis.Nil {
		if err := s.repo.IncrLiked(ctx, blogID, 1); err != nil {
			return err
		}
		return s.rdb.ZAdd(ctx, key, redis.Z{Score: float64(time.Now().UnixMilli()), Member: strconv.FormatInt(userID, 10)}).Err()
	}
	if err != nil {
		return err
	}
	if err := s.repo.IncrLiked(ctx, blogID, -1); err != nil {
		return err
	}
	return s.rdb.ZRem(ctx, key, strconv.FormatInt(userID, 10)).Err()
}

// LikeUsers 点赞排行榜 top5，按 ZSet 顺序返回用户（对齐 Java queryBlogLikes）。
func (s *blogService) LikeUsers(ctx context.Context, blogID int64) ([]*model.UserDTO, error) {
	key := blogLikedKey + strconv.FormatInt(blogID, 10)
	members, err := s.rdb.ZRange(ctx, key, 0, 4).Result()
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []*model.UserDTO{}, nil
	}
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		id, _ := strconv.ParseInt(m, 10, 64)
		ids = append(ids, id)
	}
	if s.userRepo == nil {
		return []*model.UserDTO{}, nil
	}
	users, err := s.userRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*model.UserDTO, 0, len(users))
	for _, u := range users {
		out = append(out, &model.UserDTO{ID: u.ID, NickName: u.NickName, Icon: u.Icon})
	}
	return out, nil
}

// Create 发布笔记并推送给全部粉丝的 feed（对齐 Java saveBlog）。
func (s *blogService) Create(ctx context.Context, userID int64, blog *model.Blog) error {
	blog.UserID = userID
	if err := s.repo.Create(ctx, blog); err != nil {
		return fmt.Errorf("%w: 新增笔记失败", err)
	}
	if s.followRepo == nil {
		return nil
	}
	followerIDs, err := s.followRepo.ListFollowerIDs(ctx, userID)
	if err != nil {
		return err
	}
	pipe := s.rdb.Pipeline()
	for _, fid := range followerIDs {
		pipe.ZAdd(ctx, feedKey+strconv.FormatInt(fid, 10), redis.Z{
			Score: float64(time.Now().UnixMilli()), Member: strconv.FormatInt(blog.ID, 10),
		})
	}
	_, err = pipe.Exec(ctx)
	return err
}

// Feed 关注流滚动分页：ZREVRANGEBYSCORE + 同分 offset（对齐 Java quertBlogOfFollow 的 minTime/offset 语义）。
func (s *blogService) Feed(ctx context.Context, userID, lastID int64, offset int) (*model.ScrollResult, error) {
	key := feedKey + strconv.FormatInt(userID, 10)
	max := int64(0)
	if lastID == 0 {
		max = time.Now().UnixMilli()
	} else {
		max = lastID
	}
	tuples, err := s.rdb.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min: "0", Max: strconv.FormatInt(max, 10), Offset: int64(offset), Count: feedPageSize,
	}).Result()
	if err != nil {
		return nil, err
	}
	if len(tuples) == 0 {
		return &model.ScrollResult{List: []*model.Blog{}, MinTime: 0, Offset: 0}, nil
	}
	ids := make([]int64, 0, len(tuples))
	minTime := int64(0)
	nextOffset := 1
	for _, t := range tuples {
		id, _ := strconv.ParseInt(t.Member.(string), 10, 64)
		ids = append(ids, id)
		score := int64(t.Score)
		if score == minTime {
			nextOffset++
		} else {
			minTime = score
			nextOffset = 1
		}
	}
	blogs, err := s.repo.GetByIDsInOrder(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, b := range blogs {
		s.fillUser(ctx, b)
		s.fillIsLike(ctx, b, userID)
	}
	return &model.ScrollResult{List: blogs, MinTime: minTime, Offset: nextOffset}, nil
}

// GetByID 详情：填用户信息与是否点赞。
func (s *blogService) GetByID(ctx context.Context, userID, id int64) (*model.Blog, error) {
	blog, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("blog %d: %w", id, errs.ErrNotFound)
	}
	s.fillUser(ctx, blog)
	s.fillIsLike(ctx, blog, userID)
	return blog, nil
}

func (s *blogService) MyPage(ctx context.Context, userID int64, page int) ([]*model.Blog, error) {
	return s.repo.PageByUser(ctx, userID, (page-1)*10, 10)
}

func (s *blogService) UserPage(ctx context.Context, userID int64, page int) ([]*model.Blog, error) {
	return s.repo.PageByUser(ctx, userID, (page-1)*10, 10)
}

// fillUser 填充非表字段 Name/Icon（对齐 Java queryBlogUser）。
func (s *blogService) fillUser(ctx context.Context, blog *model.Blog) {
	if s.userRepo == nil {
		return
	}
	user, err := s.userRepo.GetByIDs(ctx, []int64{blog.UserID})
	if err != nil || len(user) == 0 {
		return
	}
	blog.Name = user[0].NickName
	blog.Icon = user[0].Icon
}

// fillIsLike 填充非表字段 IsLike（userID=0 表示未登录，跳过）。
func (s *blogService) fillIsLike(ctx context.Context, blog *model.Blog, userID int64) {
	if userID == 0 || s.rdb == nil {
		return
	}
	key := blogLikedKey + strconv.FormatInt(blog.ID, 10)
	_, err := s.rdb.ZScore(ctx, key, strconv.FormatInt(userID, 10)).Result()
	blog.IsLike = err == nil
}
