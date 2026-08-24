package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"inskill/internal/config"

	"github.com/gin-gonic/gin"
)

func TestHealthz(t *testing.T) {
	cfg := &config.Config{HTTPAddr: ":0"}
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	s.Engine().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != `{"success":true,"data":"ok"}` {
		t.Errorf("body = %s", body)
	}
}

func TestRecovery(t *testing.T) {
	cfg := &config.Config{HTTPAddr: ":0"}
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Engine().GET("/panic", func(c *gin.Context) { panic("boom") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	s.Engine().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if body := rec.Body.String(); body != `{"success":false,"errorMsg":"服务器异常"}` {
		t.Errorf("body = %s", body)
	}
}
