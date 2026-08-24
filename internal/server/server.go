package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"inskill/internal/config"
	"inskill/internal/middleware"
	"inskill/internal/pkg/errs"

	"github.com/gin-gonic/gin"
)

// Server 持有 HTTP 引擎与生命周期管理。业务路由由各 handler 注册到 Engine()。
type Server struct {
	cfg    *config.Config
	logger *slog.Logger
	engine *gin.Engine
	http   *http.Server
}

func New(cfg *config.Config, logger *slog.Logger) *Server {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.Use(middleware.Recovery(logger), middleware.Logging(logger))
	s := &Server{cfg: cfg, logger: logger, engine: e}
	s.registerRoutes()
	return s
}

// Engine 暴露给 handler 注册业务路由。
func (s *Server) Engine() *gin.Engine { return s.engine }

func (s *Server) registerRoutes() {
	s.engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, errs.OK("ok"))
	})
}

// Run 启动 HTTP 服务，ctx 取消时优雅关闭（10 秒宽限）。
func (s *Server) Run(ctx context.Context) error {
	s.http = &http.Server{Addr: s.cfg.HTTPAddr, Handler: s.engine}
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("http server listening", "addr", s.cfg.HTTPAddr)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	}
}
