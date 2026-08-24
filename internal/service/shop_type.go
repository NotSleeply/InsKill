package service

import (
	"context"
	"encoding/json"
	"errors"

	"inskill/internal/model"

	"github.com/redis/go-redis/v9"
)

const shopTypeListKey = "shop_type:"

// ShopTypeRepository 类型数据访问接口。
type ShopTypeRepository interface {
	ListBySort(ctx context.Context) ([]*model.ShopType, error)
}

type ShopTypeService interface {
	List(ctx context.Context) ([]*model.ShopType, error)
}

type shopTypeService struct {
	repo ShopTypeRepository
	rdb  *redis.Client
}

func NewShopTypeService(repo ShopTypeRepository, rdb *redis.Client) ShopTypeService {
	return &shopTypeService{repo: repo, rdb: rdb}
}

// List 优先读 Redis List 缓存，未命中查库并回填（对齐 Java querySort）。
func (s *shopTypeService) List(ctx context.Context) ([]*model.ShopType, error) {
	strs, err := s.rdb.LRange(ctx, shopTypeListKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	if len(strs) > 0 {
		out := make([]*model.ShopType, 0, len(strs))
		for _, str := range strs {
			var t model.ShopType
			if err := json.Unmarshal([]byte(str), &t); err != nil {
				return nil, err
			}
			out = append(out, &t)
		}
		return out, nil
	}
	types, err := s.repo.ListBySort(ctx)
	if err != nil {
		return nil, err
	}
	if len(types) == 0 {
		return nil, errors.New("没有分类数据")
	}
	pipe := s.rdb.Pipeline()
	for _, t := range types {
		b, _ := json.Marshal(t)
		pipe.RPush(ctx, shopTypeListKey, string(b))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	return types, nil
}
