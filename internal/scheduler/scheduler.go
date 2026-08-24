package scheduler

import (
	"log/slog"

	"github.com/robfig/cron/v3"
)

// Scheduler 定时任务封装（对齐 Java 版 Spring Task 的用途：关单扫描、对账）。
type Scheduler struct {
	cron *cron.Cron
}

func New() *Scheduler {
	return &Scheduler{cron: cron.New(cron.WithSeconds())}
}

// Add 注册一个 cron 表达式任务。spec 支持秒级（如 "0 */1 * * * *" 每分钟）。
func (s *Scheduler) Add(spec string, fn func()) error {
	_, err := s.cron.AddFunc(spec, func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("scheduler task panic", "spec", spec, "panic", r)
			}
		}()
		fn()
	})
	return err
}

func (s *Scheduler) Start() { s.cron.Start() }
func (s *Scheduler) Stop()  { s.cron.Stop() }
