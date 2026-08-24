package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 应用配置。环境变量优先，可选 yaml 文件覆盖默认值。
type Config struct {
	HTTPAddr        string        `yaml:"http_addr"`
	MySQLDSN        string        `yaml:"mysql_dsn"`
	RedisAddr       string        `yaml:"redis_addr"`
	RocketMQNameSrv string        `yaml:"rocketmq_namesrv"`
	UploadDir       string        `yaml:"upload_dir"`
	OrderTimeout    time.Duration `yaml:"order_timeout"`
}

// Load 读取配置：环境变量优先，否则 yaml 文件（INSKILL_CONFIG_FILE），最后默认值。
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:        ":8081",
		MySQLDSN:        "root:root@tcp(127.0.0.1:3306)/inskill?charset=utf8mb4&parseTime=True&loc=Local",
		RedisAddr:       "127.0.0.1:6379",
		RocketMQNameSrv: "127.0.0.1:9876",
		UploadDir:       "uploads",
		OrderTimeout:    30 * time.Minute,
	}
	if path := os.Getenv("INSKILL_CONFIG_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, err
		}
	}
	cfg.applyEnv()
	return cfg, nil
}

func (c *Config) applyEnv() {
	if v := os.Getenv("INSKILL_HTTP_ADDR"); v != "" {
		c.HTTPAddr = v
	}
	if v := os.Getenv("INSKILL_MYSQL_DSN"); v != "" {
		c.MySQLDSN = v
	}
	if v := os.Getenv("INSKILL_REDIS_ADDR"); v != "" {
		c.RedisAddr = v
	}
	if v := os.Getenv("INSKILL_ROCKETMQ_NAMESRV"); v != "" {
		c.RocketMQNameSrv = v
	}
	if v := os.Getenv("INSKILL_UPLOAD_DIR"); v != "" {
		c.UploadDir = v
	}
}
