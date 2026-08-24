package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"inskill/internal/cache"
	"inskill/internal/config"
	"inskill/internal/handler"
	"inskill/internal/lock"
	"inskill/internal/middleware"
	"inskill/internal/repository"
	"inskill/internal/server"
	"inskill/internal/service"

	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := srv.Run(ctx); err != nil {
		logger.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}
