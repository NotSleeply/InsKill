package service

import (
	"context"
	"testing"

	"inskill/internal/model"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type fakeFollowRepo struct {
	rows map[[2]int64]bool
}

func (f *fakeFollowRepo) Create(_ context.Context, follow *model.Follow) error {
	f.rows[[2]int64{follow.UserID, follow.FollowUserID}] = true
	return nil
}

func (f *fakeFollowRepo) Delete(_ context.Context, userID, followUserID int64) error {
	delete(f.rows, [2]int64{userID, followUserID})
	return nil
}

func (f *fakeFollowRepo) Exists(_ context.Context, userID, followUserID int64) (bool, error) {
	return f.rows[[2]int64{userID, followUserID}], nil
}

// fakeUserReader 共同关注测试用的用户查询 fake。
type fakeUserReader struct {
	users map[int64]*model.User
}

func (f *fakeUserReader) GetByIDs(_ context.Context, ids []int64) ([]*model.User, error) {
	out := make([]*model.User, 0, len(ids))
	for _, id := range ids {
		if u, ok := f.users[id]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}

func TestFollowAndUnfollow(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	svc := NewFollowService(&fakeFollowRepo{rows: map[[2]int64]bool{}}, rdb, &fakeUserReader{users: map[int64]*model.User{}})
	if err := svc.Follow(ctx, 1, 2, true); err != nil {
		t.Fatalf("follow: %v", err)
	}
	isFollow, _ := svc.IsFollow(ctx, 1, 2)
	if !isFollow {
		t.Error("IsFollow = false, want true")
	}
	if err := svc.Follow(ctx, 1, 2, false); err != nil {
		t.Fatalf("unfollow: %v", err)
	}
	isFollow, _ = svc.IsFollow(ctx, 1, 2)
	if isFollow {
		t.Error("IsFollow = true, want false after unfollow")
	}
}

func TestCommons(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	svc := NewFollowService(&fakeFollowRepo{rows: map[[2]int64]bool{}}, rdb,
		&fakeUserReader{users: map[int64]*model.User{2: {ID: 2, NickName: "小明"}}})
	// 预置关注关系（模拟历史数据）
	rdb.SAdd(ctx, "follows:1", "2", "3")
	rdb.SAdd(ctx, "follows:4", "2", "5")
	users, err := svc.Commons(ctx, 1, 4)
	if err != nil {
		t.Fatalf("Commons: %v", err)
	}
	if len(users) != 1 || users[0].ID != 2 {
		t.Fatalf("Commons = %v, want [user 2]", users)
	}
}
