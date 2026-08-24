package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	for _, k := range []string{"INSKILL_HTTP_ADDR", "INSKILL_MYSQL_DSN", "INSKILL_REDIS_ADDR", "INSKILL_ROCKETMQ_NAMESRV", "INSKILL_CONFIG_FILE"} {
		os.Unsetenv(k)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":8081" {
		t.Errorf("HTTPAddr = %q, want :8081", cfg.HTTPAddr)
	}
	if cfg.OrderTimeout != 30*time.Minute {
		t.Errorf("OrderTimeout = %v, want 30m", cfg.OrderTimeout)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("INSKILL_HTTP_ADDR", ":9999")
	t.Setenv("INSKILL_REDIS_ADDR", "127.0.0.1:6380")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":9999" {
		t.Errorf("HTTPAddr = %q, want :9999", cfg.HTTPAddr)
	}
	if cfg.RedisAddr != "127.0.0.1:6380" {
		t.Errorf("RedisAddr = %q, want 127.0.0.1:6380", cfg.RedisAddr)
	}
}
