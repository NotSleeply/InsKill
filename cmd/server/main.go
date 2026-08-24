package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"inskill/internal/cache"
	"inskill/internal/config"
	"inskill/internal/handler"
	"inskill/internal/lock"
	"inskill/internal/middleware"
	"inskill/internal/mq"
	"inskill/internal/repository"
	"inskill/internal/scheduler"
	"inskill/internal/server"
	"inskill/internal/service"

	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger) // service 层包级 slog.Info 统一走 JSON 输出
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "err", err)
		os.Exit(1)
	}
	db, err := repository.NewMySQL(cfg.MySQLDSN)
	if err != nil {
		logger.Error("connect mysql failed", "err", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	srv := server.New(cfg, logger)

	// 全局中间件：token 刷新（不拦截）
	srv.Engine().Use(middleware.RefreshToken(rdb))

	// 依赖注入与路由装配
	userSvc := service.NewUserService(repository.NewUserRepo(db), rdb)
	userInfoSvc := service.NewUserInfoService(repository.NewUserInfoRepo(db))
	handler.NewUserHandler(userSvc, userInfoSvc).Register(srv.Engine().Group("/user"))

	cacheClient, err := cache.NewRedisClient(cfg.RedisAddr)
	if err != nil {
		logger.Error("init cache failed", "err", err)
		os.Exit(1)
	}
	shopSvc := service.NewShopService(repository.NewShopRepo(db), cacheClient, lock.NewRedisLock(rdb), rdb)
	if err := shopSvc.PreloadGeo(context.Background()); err != nil {
		logger.Error("preload geo failed", "err", err)
	}
	handler.NewShopHandler(shopSvc).Register(srv.Engine().Group("/shop"))
	handler.NewShopTypeHandler(service.NewShopTypeService(repository.NewShopTypeRepo(db), rdb)).
		Register(srv.Engine().Group("/shop-type"))

	orderPub, err := mq.NewRocketMQPublisher(cfg.RocketMQNameSrv, "inskill-producer")
	if err != nil {
		logger.Error("init rocketmq producer failed", "err", err)
		os.Exit(1)
	}
	defer orderPub.Shutdown()

	voucherOrderSvc := service.NewVoucherOrderService(
		repository.NewVoucherOrderRepo(db), repository.NewSeckillVoucherRepo(db), orderPub, rdb)
	handler.NewVoucherOrderHandler(voucherOrderSvc).Register(srv.Engine().Group("/voucher-order",
		middleware.RateLimit(rdb, "limit:seckill:", time.Minute, 60, middleware.IPDimension)))

	voucherSvc := service.NewVoucherService(
		repository.NewVoucherRepo(db), repository.NewSeckillVoucherRepo(db), rdb)
	handler.NewVoucherHandler(voucherSvc).Register(srv.Engine().Group("/voucher",
		middleware.RateLimit(rdb, "limit:voucher:", time.Minute, 10, middleware.UserDimension)))

	// 秒杀订单消费者
	orderConsumer, err := mq.NewSeckillConsumer(cfg.RocketMQNameSrv, "inskill-seckill-consumer")
	if err != nil {
		logger.Error("init rocketmq consumer failed", "err", err)
		os.Exit(1)
	}
	if err := orderConsumer.Start(context.Background(), voucherOrderSvc.HandleOrderMessage); err != nil {
		logger.Error("start rocketmq consumer failed", "err", err)
		os.Exit(1)
	}
	defer orderConsumer.Shutdown()

	lifecycleSvc := service.NewOrderLifecycleService(
		repository.NewVoucherOrderRepo(db), repository.NewSeckillVoucherRepo(db),
		orderPub, rdb, cfg.OrderTimeout)
	handler.NewPayCallbackHandler(lifecycleSvc).Register(srv.Engine().Group("/voucher-order"))

	sched := scheduler.New()
	_ = sched.Add("0 */1 * * * *", func() { _ = lifecycleSvc.CloseTimeoutOrders(context.Background()) })
	_ = sched.Add("0 */5 * * * *", func() { _ = lifecycleSvc.ReconcileAllFinished(context.Background()) })
	sched.Start()
	defer sched.Stop()

	blogSvc := service.NewBlogService(
		repository.NewBlogRepo(db), repository.NewFollowRepo(db), rdb, repository.NewUserRepo(db))
	blogGroup := srv.Engine().Group("/blog", middleware.UVCount(rdb, "blog"))
	handler.NewBlogHandler(blogSvc, rdb).Register(blogGroup)
	handler.NewUploadHandler(cfg.UploadDir).Register(srv.Engine().Group("/upload"))

	followSvc := service.NewFollowService(repository.NewFollowRepo(db), rdb, repository.NewUserRepo(db))
	handler.NewFollowHandler(followSvc).Register(srv.Engine().Group("/follow"))

	refundConsumer, err := mq.NewRefundConsumer(cfg.RocketMQNameSrv, "inskill-refund-consumer")
	if err != nil {
		logger.Error("init refund consumer failed", "err", err)
		os.Exit(1)
	}
	if err := refundConsumer.Start(context.Background(), lifecycleSvc.HandleRefundMessage); err != nil {
		logger.Error("start refund consumer failed", "err", err)
		os.Exit(1)
	}
	defer refundConsumer.Shutdown()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := srv.Run(ctx); err != nil {
		logger.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}
