//go:build integration

package integration

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"inskill/internal/model"
	"inskill/internal/repository"
	"inskill/internal/seckill"
	"inskill/internal/service"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"gorm.io/gorm"
)

// TestSeckillConcurrentNoOversell 并发 200 请求抢 100 库存：
// 断言 Lua 后 Redis 库存为 0 且不为负、订单集大小 = 100（不超卖、一人一单）。
func TestSeckillConcurrentNoOversell(t *testing.T) {
	ctx := context.Background()
	rdb, db, cleanup := newInfra(t)
	defer cleanup()
	execSchema(t, db)

	const stock = 100
	const users = 200
	rdb.Set(ctx, "seckill:stock:1", stock, 0)

	var wg sync.WaitGroup
	success := make(chan bool, users)
	for i := 1; i <= users; i++ {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()
			r, err := seckill.PreDeduct(ctx, rdb, 1, userID)
			success <- err == nil && r == seckill.ResultOK
		}(int64(i))
	}
	wg.Wait()
	close(success)

	okCount := 0
	for s := range success {
		if s {
			okCount++
		}
	}
	if okCount != stock {
		t.Fatalf("successful orders = %d, want %d (stock)", okCount, stock)
	}
	left, _ := rdb.Get(ctx, "seckill:stock:1").Int64()
	if left != 0 {
		t.Errorf("redis stock left = %d, want 0", left)
	}
	members, _ := rdb.SCard(ctx, "seckill:order:1").Result()
	if members != stock {
		t.Errorf("order set members = %d, want %d (one per user)", members, stock)
	}
}

// TestHandleOrderMessageWritesDB 消息消费落库：订单 Create + 库存扣减（真实 MySQL 乐观锁），重复消费幂等。
func TestHandleOrderMessageWritesDB(t *testing.T) {
	ctx := context.Background()
	rdb, db, cleanup := newInfra(t)
	defer cleanup()
	execSchema(t, db)
	begin := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)
	sv := &model.SeckillVoucher{VoucherID: 1, Stock: 10, BeginTime: &begin, EndTime: &end}
	if err := db.Create(sv).Error; err != nil {
		t.Fatal(err)
	}
	svc := service.NewVoucherOrderService(
		repository.NewVoucherOrderRepo(db), repository.NewSeckillVoucherRepo(db), nil, rdb)
	body := []byte(`{"id":2001,"userId":1,"voucherId":1,"payType":1,"status":1}`)
	if err := svc.HandleOrderMessage(ctx, body); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if err := svc.HandleOrderMessage(ctx, body); err != nil {
		t.Fatalf("duplicate handle should be nil, got %v", err)
	}
	var count int64
	db.Model(&model.VoucherOrder{}).Where("id = 2001").Count(&count)
	if count != 1 {
		t.Errorf("orders with id 2001 = %d, want 1", count)
	}
	var after model.SeckillVoucher
	db.First(&after, "voucher_id = 1")
	if after.Stock != 9 {
		t.Errorf("db stock = %d, want 9 (deducted once)", after.Stock)
	}
}

// TestSignCountWithRealRedis 真实 Redis 的 BITFIELD 连续签到（miniredis 不支持 BITFIELD）。
func TestSignCountWithRealRedis(t *testing.T) {
	ctx := context.Background()
	rdb, db, cleanup := newInfra(t)
	defer cleanup()
	execSchema(t, db)
	svc := service.NewUserService(repository.NewUserRepo(db), rdb)
	if err := svc.Sign(ctx, 1); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	n, err := svc.SignCount(ctx, 1)
	if err != nil {
		t.Fatalf("SignCount: %v", err)
	}
	if n != 1 {
		t.Errorf("SignCount = %d, want 1", n)
	}
}

// newInfra 启动 MySQL + Redis 容器，返回连接与清理函数。
func newInfra(t *testing.T) (*redis.Client, *gorm.DB, func()) {
	t.Helper()
	ctx := context.Background()
	redisContainer, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatal(err)
	}
	redisAddr, _ := redisContainer.Endpoint(ctx, "")
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})

	mysqlContainer, err := mysql.Run(ctx, "mysql:8.0",
		mysql.WithDatabase("inskill"), mysql.WithUsername("root"), mysql.WithPassword("root"))
	if err != nil {
		t.Fatal(err)
	}
	dsn, _ := mysqlContainer.ConnectionString(ctx)
	// multiStatements 允许一次执行整份 schema.sql
	db, err := repository.NewMySQL(dsn + "?multiStatements=true")
	if err != nil {
		t.Fatal(err)
	}
	return rdb, db, func() {
		_ = redisContainer.Terminate(ctx)
		_ = mysqlContainer.Terminate(ctx)
	}
}

// execSchema 执行生产权威的建表脚本（scripts/schema.sql），同时验证脚本本身可用。
func execSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	b, err := os.ReadFile("../scripts/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(b)).Error; err != nil {
		t.Fatalf("exec schema: %v", err)
	}
}
