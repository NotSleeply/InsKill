package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"inskill/internal/model"
	"inskill/internal/mq"
	"inskill/internal/pkg/errs"

	"github.com/redis/go-redis/v9"
)

// VoucherOrderLifecycleRepository 订单生命周期所需的数据操作（VoucherOrderRepo 满足）。
type VoucherOrderLifecycleRepository interface {
	Create(ctx context.Context, o *model.VoucherOrder) error
	GetByID(ctx context.Context, id int64) (*model.VoucherOrder, error)
	MarkPaid(ctx context.Context, orderID int64) (bool, error)
	FindTimeoutIDs(ctx context.Context, before time.Time) ([]int64, error)
	CancelIfUnpaid(ctx context.Context, orderID int64) (bool, error)
	CountByVoucher(ctx context.Context, voucherID int64) (int64, error)
}

// SeckillStockRepository 库存回补所需操作（SeckillVoucherRepo 满足）。
type SeckillStockRepository interface {
	IncrStock(ctx context.Context, voucherID int64) error
	GetByID(ctx context.Context, voucherID int64) (*model.SeckillVoucher, error)
	ListFinished(ctx context.Context) ([]*model.SeckillVoucher, error)
}

// refundMessage 退款消息体（Topic stock-refund）。
type refundMessage struct {
	OrderID   int64 `json:"orderId"`
	VoucherID int64 `json:"voucherId"`
}

type OrderLifecycleService interface {
	PayCallback(ctx context.Context, orderID int64) error
	CloseTimeoutOrders(ctx context.Context) error
	HandleRefundMessage(ctx context.Context, body []byte) error
	ReconcileSeckill(ctx context.Context, voucherID int64) error
	ReconcileAllFinished(ctx context.Context) error
}

type orderLifecycleService struct {
	repo      VoucherOrderLifecycleRepository
	stockRepo SeckillStockRepository
	pub       mq.Publisher
	rdb       *redis.Client
	timeout   time.Duration
}

func NewOrderLifecycleService(repo VoucherOrderLifecycleRepository, stockRepo SeckillStockRepository,
	pub mq.Publisher, rdb *redis.Client, timeout time.Duration) OrderLifecycleService {
	return &orderLifecycleService{repo: repo, stockRepo: stockRepo, pub: pub, rdb: rdb, timeout: timeout}
}

// PayCallback 模拟第三方支付回调：乐观锁置已支付；失败说明已被关单，触发退款流程（不允许改回已支付）。
func (s *orderLifecycleService) PayCallback(ctx context.Context, orderID int64) error {
	ok, err := s.repo.MarkPaid(ctx, orderID)
	if err != nil {
		return err
	}
	if !ok {
		// 订单已被超时关单但用户已付款：进入原路退回流程（对齐 README 结论）
		slog.Error("order closed but payment arrived, refund required", "order", orderID)
		return fmt.Errorf("order %d: %w", orderID, errs.ErrOrderClosed)
	}
	return nil
}

// CloseTimeoutOrders 超时未支付订单关单：先查出超时未支付订单，再逐单乐观锁取消，
// 只有取消成功（影响行数 1）才发送库存回补消息（对齐 README「只有更新成功才释放库存」）。
func (s *orderLifecycleService) CloseTimeoutOrders(ctx context.Context) error {
	ids, err := s.repo.FindTimeoutIDs(ctx, time.Now().Add(-s.timeout))
	if err != nil {
		return err
	}
	for _, id := range ids {
		ok, err := s.repo.CancelIfUnpaid(ctx, id)
		if err != nil || !ok {
			continue // 乐观锁失败 = 已被支付回调抢先，跳过
		}
		order, err := s.repo.GetByID(ctx, id)
		if err != nil {
			continue
		}
		body, _ := json.Marshal(refundMessage{OrderID: id, VoucherID: order.VoucherID})
		if err := s.pub.Publish(ctx, mq.TopicStockRefund, body); err != nil {
			// 发布失败：记日志。订单已取消，下一轮 FindTimeoutIDs 不会重复选中，由对账任务兜底发现库存差异。
			slog.Error("publish stock refund failed", "order", id, "err", err)
		}
	}
	return nil
}

// HandleRefundMessage 库存回补消费者：幂等占位（SETNX）成功才回补，Redis 与 DB 双写。
func (s *orderLifecycleService) HandleRefundMessage(ctx context.Context, body []byte) error {
	var msg refundMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return err
	}
	refundKey := "seckill:refund:" + strconv.FormatInt(msg.OrderID, 10)
	ok, err := s.rdb.SetNX(ctx, refundKey, "1", time.Hour).Result()
	if err != nil {
		return err
	}
	if !ok {
		return nil // 已回补过（重复消费）
	}
	if err := s.stockRepo.IncrStock(ctx, msg.VoucherID); err != nil {
		_ = s.rdb.Del(ctx, refundKey).Err() // 释放占位，等待重试
		return err
	}
	return s.rdb.IncrBy(ctx, "seckill:stock:"+strconv.FormatInt(msg.VoucherID, 10), 1).Err()
}

// ReconcileSeckill 对账：Redis 剩余库存/订单集与 DB 比对，不一致打错误日志（对齐 README 对账策略）。
func (s *orderLifecycleService) ReconcileSeckill(ctx context.Context, voucherID int64) error {
	sv, err := s.stockRepo.GetByID(ctx, voucherID)
	if err != nil {
		return err
	}
	redisStock, err := s.rdb.Get(ctx, "seckill:stock:"+strconv.FormatInt(voucherID, 10)).Int64()
	if err != nil && err != redis.Nil {
		return err
	}
	orderCount, err := s.repo.CountByVoucher(ctx, voucherID)
	if err != nil {
		return err
	}
	memberCount, err := s.rdb.SCard(ctx, "seckill:order:"+strconv.FormatInt(voucherID, 10)).Result()
	if err != nil {
		return err
	}
	if int64(sv.Stock) != redisStock || memberCount != orderCount {
		slog.Error("seckill reconcile mismatch",
			"voucher", voucherID,
			"db_stock", sv.Stock, "redis_stock", redisStock,
			"db_orders", orderCount, "redis_members", memberCount)
	}
	return nil
}

// ReconcileAllFinished 对账所有已结束的秒杀券。
func (s *orderLifecycleService) ReconcileAllFinished(ctx context.Context) error {
	finished, err := s.stockRepo.ListFinished(ctx)
	if err != nil {
		return err
	}
	for _, sv := range finished {
		if err := s.ReconcileSeckill(ctx, sv.VoucherID); err != nil {
			slog.Error("reconcile failed", "voucher", sv.VoucherID, "err", err)
		}
	}
	return nil
}
