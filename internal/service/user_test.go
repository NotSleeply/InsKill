package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type fakeUserRepo struct {
	users map[string]*model.User // key: phone
}

func (f *fakeUserRepo) GetByPhone(_ context.Context, phone string) (*model.User, error) {
	if u, ok := f.users[phone]; ok {
		return u, nil
	}
	return nil, errs.ErrNotFound
}

func (f *fakeUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, errs.ErrNotFound
}

func (f *fakeUserRepo) Create(_ context.Context, u *model.User) error {
	u.ID = int64(len(f.users) + 1)
	f.users[u.Phone] = u
	return nil
}

func newTestUserService(t *testing.T) (*userService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &userService{repo: &fakeUserRepo{users: map[string]*model.User{}}, rdb: rdb}, mr
}

func TestSendCodeInvalidPhone(t *testing.T) {
	svc, _ := newTestUserService(t)
	if err := svc.SendCode(context.Background(), "12345"); !errors.Is(err, errs.ErrInvalidPhone) {
		t.Fatalf("err = %v, want ErrInvalidPhone", err)
	}
}

func TestSendCodeStoresInRedis(t *testing.T) {
	svc, mr := newTestUserService(t)
	if err := svc.SendCode(context.Background(), "13800138000"); err != nil {
		t.Fatalf("SendCode: %v", err)
	}
	if code, err := svc.rdb.Get(context.Background(), "login:code:13800138000").Result(); err != nil || len(code) != 6 {
		t.Fatalf("stored code = %q, %v", code, err)
	}
	if ttl := mr.TTL("login:code:13800138000"); ttl < 119*time.Second || ttl > 120*time.Second {
		t.Errorf("ttl = %v, want ~120s", ttl)
	}
}

func TestLoginWrongCode(t *testing.T) {
	svc, _ := newTestUserService(t)
	_ = svc.SendCode(context.Background(), "13800138000")
	if _, err := svc.Login(context.Background(), "13800138000", "000000"); !errors.Is(err, errs.ErrCodeMismatch) {
		t.Fatalf("err = %v, want ErrCodeMismatch", err)
	}
}

func TestLoginCreatesUserAndToken(t *testing.T) {
	svc, mr := newTestUserService(t)
	_ = svc.SendCode(context.Background(), "13800138000")
	code, _ := svc.rdb.Get(context.Background(), "login:code:13800138000").Result()
	token, err := svc.Login(context.Background(), "13800138000", code)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	got, err := svc.rdb.HGetAll(context.Background(), "login:token:"+token).Result()
	if err != nil || got["id"] != "1" || !strings.HasPrefix(got["nickName"], "user_") {
		t.Fatalf("token hash = %v, %v", got, err)
	}
	if ttl := mr.TTL("login:token:" + token); ttl < 1799*time.Second || ttl > 1800*time.Second {
		t.Errorf("ttl = %v, want ~1800s", ttl)
	}
}

// TestSign 只验证 SETBIT 写入（miniredis 不支持 BITFIELD，SignCount 的
// 连续签到统计测试在 integration/ 中用真实 Redis 覆盖）。
func TestSign(t *testing.T) {
	svc, _ := newTestUserService(t)
	if err := svc.Sign(context.Background(), 1); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	now := time.Now()
	key := "sign:1:" + now.Format("200601")
	bit, err := svc.rdb.GetBit(context.Background(), key, int64(now.Day()-1)).Result()
	if err != nil || bit != 1 {
		t.Fatalf("sign bit = %d, %v, want 1", bit, err)
	}
}
