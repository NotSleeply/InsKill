package service

import (
	"context"
	"strconv"
	"testing"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type fakeBlogRepo struct {
	blogs  map[int64]*model.Blog
	nextID int64
}

func (f *fakeBlogRepo) GetByID(_ context.Context, id int64) (*model.Blog, error) {
	if b, ok := f.blogs[id]; ok {
		return b, nil
	}
	return nil, errs.ErrNotFound
}

func (f *fakeBlogRepo) HotPage(_ context.Context, _, _ int) ([]*model.Blog, error) { return nil, nil }
func (f *fakeBlogRepo) PageByUser(_ context.Context, _ int64, _, _ int) ([]*model.Blog, error) {
	return nil, nil
}
func (f *fakeBlogRepo) GetByIDsInOrder(_ context.Context, ids []int64) ([]*model.Blog, error) {
	out := make([]*model.Blog, 0, len(ids))
	for _, id := range ids {
		if b, ok := f.blogs[id]; ok {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeBlogRepo) Create(_ context.Context, b *model.Blog) error {
	f.nextID++
	b.ID = f.nextID
	f.blogs[b.ID] = b
	return nil
}

func (f *fakeBlogRepo) IncrLiked(_ context.Context, id int64, delta int) error {
	if b, ok := f.blogs[id]; ok {
		b.Liked += int32(delta)
	}
	return nil
}

type fakeFollowRepoForBlog struct {
	followers map[int64][]int64 // followUserID -> follower userIDs
}

func (f *fakeFollowRepoForBlog) ListFollowerIDs(_ context.Context, followUserID int64) ([]int64, error) {
	return f.followers[followUserID], nil
}

func TestBlogLikeToggle(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	repo := &fakeBlogRepo{blogs: map[int64]*model.Blog{1: {ID: 1, Liked: 0}}}
	svc := NewBlogService(repo, nil, rdb, nil)
	ctx := context.Background()
	if err := svc.Like(ctx, 1, 1); err != nil {
		t.Fatalf("first like: %v", err)
	}
	if repo.blogs[1].Liked != 1 {
		t.Errorf("liked = %d, want 1", repo.blogs[1].Liked)
	}
	// 重复点赞应取消
	if err := svc.Like(ctx, 1, 1); err != nil {
		t.Fatalf("second like: %v", err)
	}
	if repo.blogs[1].Liked != 0 {
		t.Errorf("liked = %d, want 0", repo.blogs[1].Liked)
	}
}

func TestBlogCreatePushesFeed(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	repo := &fakeBlogRepo{blogs: map[int64]*model.Blog{}}
	followRepo := &fakeFollowRepoForBlog{followers: map[int64][]int64{2: {11, 22}}}
	svc := NewBlogService(repo, followRepo, rdb, nil)
	ctx := context.Background()
	if err := svc.Create(ctx, 2, &model.Blog{Title: "探店"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, followerID := range []int64{11, 22} {
		score, err := rdb.ZScore(ctx, "feed:"+strconv.FormatInt(followerID, 10), "1").Result()
		if err != nil {
			t.Fatalf("follower %d feed: %v", followerID, err)
		}
		if score <= 0 {
			t.Errorf("follower %d feed score = %v", followerID, score)
		}
	}
}

func TestBlogFeedScrollPagination(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	// 预置 feed：3 条，其中 2 条同分数（模拟同一毫秒推送）
	rdb.ZAdd(ctx, "feed:1", redis.Z{Score: 100, Member: "1"}, redis.Z{Score: 100, Member: "2"}, redis.Z{Score: 99, Member: "3"})
	repo := &fakeBlogRepo{blogs: map[int64]*model.Blog{1: {ID: 1}, 2: {ID: 2}, 3: {ID: 3}}}
	svc := NewBlogService(repo, nil, rdb, nil)
	res, err := svc.Feed(ctx, 1, 0, 0) // lastID=0 → max=now
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if len(res.List) != 2 {
		t.Fatalf("first page size = %d, want 2", len(res.List))
	}
	if res.Offset != 2 {
		t.Errorf("offset = %d, want 2 (two entries share minTime)", res.Offset)
	}
	res2, err := svc.Feed(ctx, 1, res.MinTime, res.Offset)
	if err != nil {
		t.Fatalf("second Feed: %v", err)
	}
	if len(res2.List) != 1 || res2.List[0].ID != 3 {
		t.Fatalf("second page = %v, want blog 3", res2.List)
	}
}
