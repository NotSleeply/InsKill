package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
)

// startWithRetry topic 由 mqadmin 异步预创建，consumer 启动可能早于 topic 就绪，
// 轮询重试 60 秒（对齐 docker-compose 的 rocketmq-init 服务）。
func startWithRetry(name string, start func() error) error {
	deadline := time.Now().Add(60 * time.Second)
	for {
		if err := start(); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("start %s consumer: timeout", name)
		}
		slog.Warn("consumer start failed, retrying", "consumer", name)
		time.Sleep(3 * time.Second)
	}
}

// 结构映射：Java 版 RabbitMQ（交换机 X/路由 XA/队列 QA/死信 QD）→ RocketMQ。
// 死信与重试由 RocketMQ 消费重试机制承担（16 次后进 DLQ），业务幂等靠订单 ID 主键。
const (
	TopicSeckillOrder = "seckill-order"
	TopicStockRefund  = "stock-refund"
)

// Publisher 消息发布接口（隔离 RocketMQ 客户端）。
type Publisher interface {
	Publish(ctx context.Context, topic string, body []byte) error
	Shutdown() error
}

type rocketMQPublisher struct {
	p rocketmq.Producer
}

func NewRocketMQPublisher(nameSrv, group string) (Publisher, error) {
	p, err := producer.NewDefaultProducer(
		producer.WithNameServer([]string{nameSrv}),
		producer.WithGroupName(group),
		producer.WithRetry(2),
	)
	if err != nil {
		return nil, fmt.Errorf("create rocketmq producer: %w", err)
	}
	if err := p.Start(); err != nil {
		return nil, fmt.Errorf("start rocketmq producer: %w", err)
	}
	return &rocketMQPublisher{p: p}, nil
}

func (r *rocketMQPublisher) Publish(ctx context.Context, topic string, body []byte) error {
	_, err := r.p.SendSync(ctx, primitive.NewMessage(topic, body))
	if err != nil {
		return fmt.Errorf("publish %s: %w", topic, err)
	}
	return nil
}

func (r *rocketMQPublisher) Shutdown() error { return r.p.Shutdown() }

// SeckillConsumer 秒杀订单消费者：消费失败返回 error 触发 RocketMQ 重试（幂等由订单主键保证）。
type SeckillConsumer struct {
	c      rocketmq.PushConsumer
	mu     sync.Mutex
	closed bool
}

func NewSeckillConsumer(nameSrv, group string) (*SeckillConsumer, error) {
	c, err := rocketmq.NewPushConsumer(
		consumer.WithNameServer([]string{nameSrv}),
		consumer.WithGroupName(group),
		consumer.WithConsumerModel(consumer.Clustering),
	)
	if err != nil {
		return nil, fmt.Errorf("create rocketmq consumer: %w", err)
	}
	return &SeckillConsumer{c: c}, nil
}

// Start 订阅 TopicSeckillOrder 并启动消费，handler 返回 error 时消费失败进入重试。
func (s *SeckillConsumer) Start(_ context.Context, handler func(ctx context.Context, msg []byte) error) error {
	if err := s.c.Subscribe(TopicSeckillOrder, consumer.MessageSelector{}, func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, m := range msgs {
			if err := handler(ctx, m.Body); err != nil {
				return consumer.ConsumeRetryLater, nil
			}
		}
		return consumer.ConsumeSuccess, nil
	}); err != nil {
		return err
	}
	return startWithRetry(TopicSeckillOrder, func() error { return s.c.Start() })
}

func (s *SeckillConsumer) Shutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.c.Shutdown()
}

// RefundConsumer 库存回补消费者，结构与 SeckillConsumer 相同（消费组不同）。
type RefundConsumer struct {
	c      rocketmq.PushConsumer
	mu     sync.Mutex
	closed bool
}

func NewRefundConsumer(nameSrv, group string) (*RefundConsumer, error) {
	c, err := rocketmq.NewPushConsumer(
		consumer.WithNameServer([]string{nameSrv}),
		consumer.WithGroupName(group),
		consumer.WithConsumerModel(consumer.Clustering),
	)
	if err != nil {
		return nil, fmt.Errorf("create refund consumer: %w", err)
	}
	return &RefundConsumer{c: c}, nil
}

func (r *RefundConsumer) Start(_ context.Context, handler func(ctx context.Context, msg []byte) error) error {
	if err := r.c.Subscribe(TopicStockRefund, consumer.MessageSelector{}, func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, m := range msgs {
			if err := handler(ctx, m.Body); err != nil {
				return consumer.ConsumeRetryLater, nil
			}
		}
		return consumer.ConsumeSuccess, nil
	}); err != nil {
		return err
	}
	return startWithRetry(TopicStockRefund, func() error { return r.c.Start() })
}

func (r *RefundConsumer) Shutdown() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.c.Shutdown()
}

// EncodeOrder 订单消息体 JSON 序列化。
func EncodeOrder(o any) ([]byte, error) { return json.Marshal(o) }
