package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"strconv"
	"time"

	"inskill/internal/model"
	"inskill/internal/pkg/errs"

	"github.com/redis/go-redis/v9"
)

const (
	loginCodeKey  = "login:code:"
	loginTokenKey = "login:token:"
	signKeyPrefix = "sign:"

	loginCodeTTL  = 2 * time.Minute
	loginTokenTTL = 30 * time.Minute
)

var phonePattern = regexp.MustCompile(`^1[3-9]\d{9}$`)

// UserRepository 用户数据访问接口（消费方定义，UserRepo 实现）。
type UserRepository interface {
	GetByPhone(ctx context.Context, phone string) (*model.User, error)
	GetByID(ctx context.Context, id int64) (*model.User, error)
	Create(ctx context.Context, u *model.User) error
}

type UserService interface {
	SendCode(ctx context.Context, phone string) error
	Login(ctx context.Context, phone, code string) (string, error)
	Sign(ctx context.Context, userID int64) error
	SignCount(ctx context.Context, userID int64) (int, error)
	GetDTO(ctx context.Context, id int64) (*model.UserDTO, error)
}

type userService struct {
	repo UserRepository
	rdb  *redis.Client
}

func NewUserService(repo UserRepository, rdb *redis.Client) UserService {
	return &userService{repo: repo, rdb: rdb}
}

// SendCode 校验手机号并生成 6 位验证码存入 Redis（2 分钟）。
func (s *userService) SendCode(ctx context.Context, phone string) error {
	if !phonePattern.MatchString(phone) {
		return fmt.Errorf("phone %q: %w", phone, errs.ErrInvalidPhone)
	}
	code, err := randomDigits(6)
	if err != nil {
		return err
	}
	slog.Info("sms code sent", "phone", phone, "code", code)
	return s.rdb.Set(ctx, loginCodeKey+phone, code, loginCodeTTL).Err()
}

// Login 校验验证码，用户不存在则创建，签发 token 存 Redis Hash。
func (s *userService) Login(ctx context.Context, phone, code string) (string, error) {
	if !phonePattern.MatchString(phone) {
		return "", fmt.Errorf("phone %q: %w", phone, errs.ErrInvalidPhone)
	}
	cached, err := s.rdb.Get(ctx, loginCodeKey+phone).Result()
	if err == redis.Nil || cached != code {
		return "", errs.ErrCodeMismatch
	}
	if err != nil {
		return "", err
	}
	user, err := s.repo.GetByPhone(ctx, phone)
	if err != nil {
		if !errors.Is(err, errs.ErrNotFound) {
			return "", err
		}
		suffix, err := randomDigits(10)
		if err != nil {
			return "", err
		}
		user = &model.User{Phone: phone, NickName: "user_" + suffix}
		if err := s.repo.Create(ctx, user); err != nil {
			return "", err
		}
	}
	token := uuid()
	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, loginTokenKey+token,
		"id", user.ID, "nickName", user.NickName, "icon", user.Icon)
	pipe.Expire(ctx, loginTokenKey+token, loginTokenTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", err
	}
	return token, nil
}

// Sign 当日签到：SETBIT sign:{userID}:{yyyyMM} {day-1} 1
func (s *userService) Sign(ctx context.Context, userID int64) error {
	now := time.Now()
	key := signKeyPrefix + strconv.FormatInt(userID, 10) + now.Format(":200601")
	return s.rdb.SetBit(ctx, key, int64(now.Day()-1), 1).Err()
}

// SignCount 连续签到天数：BITFIELD 取本月前 day 位，从低位起数连续 1。
func (s *userService) SignCount(ctx context.Context, userID int64) (int, error) {
	now := time.Now()
	key := signKeyPrefix + strconv.FormatInt(userID, 10) + now.Format(":200601")
	vals, err := s.rdb.BitField(ctx, key, "GET", "u"+strconv.Itoa(now.Day()), 0).Result()
	if err != nil {
		return 0, err
	}
	if len(vals) == 0 {
		return 0, nil
	}
	num := vals[0]
	count := 0
	for num > 0 && num&1 == 1 {
		count++
		num >>= 1
	}
	return count, nil
}

// GetDTO 用户精简信息（/user/{id} 端点使用）。
func (s *userService) GetDTO(ctx context.Context, id int64) (*model.UserDTO, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &model.UserDTO{ID: user.ID, NickName: user.NickName, Icon: user.Icon}, nil
}

// uuid 无连字符 UUID，对齐 Java UUID.randomUUID().toString(true)。
func uuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x%x%x%x%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// randomDigits 用 crypto/rand 生成 n 位数字串（验证码不可预测）。
func randomDigits(n int) (string, error) {
	const digits = "0123456789"
	out := make([]byte, n)
	for i := range out {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		out[i] = digits[idx.Int64()]
	}
	return string(out), nil
}
