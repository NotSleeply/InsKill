package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"inskill/internal/cache"
	"inskill/internal/lock"
	"inskill/internal/model"
	"inskill/internal/mq"
	"inskill/internal/pkg/errs"

	"github.com/redis/go-redis/v9"
)

const (
	shopCacheKey = "cache:shop:"
	shopLockKey  = "lock:shop:"
	shopCacheTTL = 30 * time.Minute
	shopGeoKey   = "shop:geo:"

	shopPageSize    = 5  // 按类型分页大小（对齐 Java DEFAULT_PAGE_SIZE）
	shopNamePageMax = 10 // 名称搜索分页大小（对齐 Java MAX_PAGE_SIZE）
)

// ShopRepository 商户数据访问接口（消费方定义）。
type ShopRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Shop, error)
	GetByIDsInOrder(ctx context.Context, ids []int64) ([]*model.Shop, error)
	PageByType(ctx context.Context, typeID int64, offset, limit int) ([]*model.Shop, error)
	PageByName(ctx context.Context, name string, offset, limit int) ([]*model.Shop, error)
	ListAll(ctx context.Context) ([]*model.Shop, error)
	Create(ctx context.Context, s *model.Shop) error
	Update(ctx context.Context, s *model.Shop) error
}

type ShopService interface {
	GetByID(ctx context.Context, id int64) (*model.Shop, error)
	Create(ctx context.Context, s *model.Shop) error
	Update(ctx context.Context, s *model.Shop) error
	PageByType(ctx context.Context, typeID int64, page int, x, y *float64) ([]*model.Shop, error)
	PageByName(ctx context.Context, name string, page int) ([]*model.Shop, error)
	PreloadGeo(ctx context.Context) error
}

type shopService struct {
	repo ShopRepository
	c    cache.Client
	lock lock.Lock
	rdb  *redis.Client
	pub  mq.Publisher
}

func NewShopService(repo ShopRepository, c cache.Client, l lock.Lock, rdb *redis.Client, pub mq.Publisher) ShopService {
	return &shopService{repo: repo, c: c, lock: l, rdb: rdb, pub: pub}
}

// GetByID 逻辑过期策略（Java 版 queryById 当前实现）。
func (s *shopService) GetByID(ctx context.Context, id int64) (*model.Shop, error) {
	key := shopCacheKey + strconv.FormatInt(id, 10)
	shop, err := cache.QueryWithLogicalExpire(s.c, s.lock, ctx, key, shopLockKey+strconv.FormatInt(id, 10),
		func(ctx context.Context) (*model.Shop, error) { return s.repo.GetByID(ctx, id) },
		shopCacheTTL)
	if errors.Is(err, cache.ErrNil) {
		return nil, fmt.Errorf("shop %d: %w", id, errs.ErrNotFound)
	}
	return shop, err
}

// Create 新增商户（对齐 Java saveShop）。
func (s *shopService) Create(ctx context.Context, shop *model.Shop) error {
	return s.repo.Create(ctx, shop)
}

// Update 先更新数据库，再删缓存；删除失败由 MQ 补偿异步重删，TTL 兜底最终一致性。
func (s *shopService) Update(ctx context.Context, shop *model.Shop) error {
	if shop.ID == 0 {
		return fmt.Errorf("%w: shop id required", errs.ErrNotFound)
	}
	if err := s.repo.Update(ctx, shop); err != nil {
		return err
	}
	cache.DelAndCompensate(ctx, s.c, s.pub, shopCacheKey+strconv.FormatInt(shop.ID, 10))
	return nil
}

// PageByType 有坐标走 GEO 5km 搜索，无坐标走数据库分页（对齐 Java queryShopByType）。
func (s *shopService) PageByType(ctx context.Context, typeID int64, page int, x, y *float64) ([]*model.Shop, error) {
	if x == nil || y == nil {
		return s.repo.PageByType(ctx, typeID, (page-1)*shopPageSize, shopPageSize)
	}
	from := int64((page - 1) * shopPageSize)
	end := int64(page * shopPageSize)
	results, err := s.rdb.GeoSearchLocation(ctx, shopGeoKey+strconv.FormatInt(typeID, 10),
		&redis.GeoSearchLocationQuery{
			GeoSearchQuery: redis.GeoSearchQuery{
				Longitude: *x, Latitude: *y, Radius: 5000, RadiusUnit: "m",
				Sort: "ASC", Count: int(end),
			},
			WithDist: true,
		}).Result()
	if err != nil {
		return nil, err
	}
	if int64(len(results)) <= from {
		return []*model.Shop{}, nil
	}
	ids := make([]int64, 0, len(results))
	dist := make(map[int64]float64, len(results))
	for _, r := range results[from:] {
		id, _ := strconv.ParseInt(r.Name, 10, 64)
		ids = append(ids, id)
		dist[id] = r.Dist
	}
	shops, err := s.repo.GetByIDsInOrder(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, shop := range shops {
		shop.Distance = dist[shop.ID]
	}
	return shops, nil
}

func (s *shopService) PageByName(ctx context.Context, name string, page int) ([]*model.Shop, error) {
	return s.repo.PageByName(ctx, name, (page-1)*shopNamePageMax, shopNamePageMax)
}

// PreloadGeo 启动时把全部商户写入 GEO（原 Java 版依赖手工预热，Go 版自动化）。
func (s *shopService) PreloadGeo(ctx context.Context) error {
	shops, err := s.repo.ListAll(ctx)
	if err != nil {
		return err
	}
	byType := map[int64][]*redis.GeoLocation{}
	for _, shop := range shops {
		byType[shop.TypeID] = append(byType[shop.TypeID], &redis.GeoLocation{
			Name: strconv.FormatInt(shop.ID, 10), Longitude: shop.X, Latitude: shop.Y,
		})
	}
	for typeID, locs := range byType {
		if err := s.rdb.GeoAdd(ctx, shopGeoKey+strconv.FormatInt(typeID, 10), locs...).Err(); err != nil {
			return err
		}
	}
	return s.preloadShopCache(ctx, shops)
}

// preloadShopCache 商户详情缓存的逻辑过期预热：逻辑过期策略要求缓存先存在
// （首次查询无缓存时返回空，对齐 Java 版 queryWithLogicalExpire 语义），
// 启动时一次性写入全部商户，避免首查失败。
func (s *shopService) preloadShopCache(ctx context.Context, shops []*model.Shop) error {
	expireAt := time.Now().Add(shopCacheTTL)
	for _, shop := range shops {
		b, err := json.Marshal(shop)
		if err != nil {
			return err
		}
		rd := cache.RedisData{Data: b, ExpireTime: expireAt}
		out, err := json.Marshal(rd)
		if err != nil {
			return err
		}
		if err := s.c.Set(ctx, shopCacheKey+strconv.FormatInt(shop.ID, 10), string(out), 0); err != nil {
			return err
		}
	}
	return nil
}
