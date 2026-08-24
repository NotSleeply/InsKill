package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"inskill/internal/id"
	"inskill/internal/model"
	"inskill/internal/mq"
	"inskill/internal/pkg/errs"
	"inskill/internal/seckill"

	"github.com/redis/go-redis/v9"
)

// VoucherOrderRepository 订单数据访问接口。
type VoucherOrderRepository interface {
	Create(ctx context.Context, o *model.VoucherOrder) error
}

// SeckillVoucherStockRepository 秒杀库存操作接口。
type SeckillVoucherStockRepository interface {
	DecrStock(ctx context.Context, voucherID int64) (bool, error)
}

type VoucherOrderService interface {
	Seckill(ctx context.Context, userID, voucherID int64) (int64, error)
	HandleOrderMessage(ctx context.Context, body []byte) error
}

type voucherOrderService struct {
	repo      VoucherOrderRepository
	stockRepo SeckillVoucherStockRepository
	pub       mq.Publisher
	rdb       *redis.Client
}

func NewVoucherOrderService(repo VoucherOrderRepository, stockRepo SeckillVoucherStockRepository, pub mq.Publisher, rdb *redis.Client) VoucherOrderService {
	return &voucherOrderService{repo: repo, stockRepo: stockRepo, pub: pub, rdb: rdb}
}

// Seckill 秒杀下单：预生成订单 ID → Lua 原子预扣 → 发 RocketMQ → 立即返回订单号（排队中）。
func (s *voucherOrderService) Seckill(ctx context.Context, userID, voucherID int64) (int64, error) {
	orderID, err := id.NextID(ctx, s.rdb, "order")
	if err != nil {
		return 0, err
	}
	result, err := seckill.PreDeduct(ctx, s.rdb, voucherID, userID)
	if err != nil {
		return 0, err
	}
	switch result {
	case seckill.ResultStockEmpty:
		return 0, fmt.Errorf("%w", errs.ErrStockEmpty)
	case seckill.ResultDuplicated:
		return 0, fmt.Errorf("%w", errs.ErrDuplicatedOrder)
	case seckill.ResultNoStockKey:
		return 0, fmt.Errorf("voucher %d: %w", voucherID, errs.ErrNotFound)
	}
	order := &model.VoucherOrder{
		ID: orderID, UserID: userID, VoucherID: voucherID,
		PayType: 1, Status: model.OrderStatusUnpaid,
	}
	body, err := mq.EncodeOrder(order)
	if err != nil {
		return 0, err
	}
	if err := s.pub.Publish(ctx, mq.TopicSeckillOrder, body); err != nil {
		return 0, fmt.Errorf("publish order %d: %w", orderID, err)
	}
	return orderID, nil
}

// HandleOrderMessage 消费端：幂等落库（订单 ID 主键，重复消息 Create 失败即跳过），成功后扣 DB 库存。
func (s *voucherOrderService) HandleOrderMessage(ctx context.Context, body []byte) error {
	var order model.VoucherOrder
	if err := json.Unmarshal(body, &order); err != nil {
		return fmt.Errorf("unmarshal order: %w", err)
	}
	if err := s.repo.Create(ctx, &order); err != nil {
		// 主键冲突 = 重复消费，直接视为成功（对齐 Java 版幂等设计）
		return nil
	}
	ok, err := s.stockRepo.DecrStock(ctx, order.VoucherID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("db stock empty")
	}
	return nil
}
