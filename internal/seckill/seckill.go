package seckill

import (
	"context"
	_ "embed"

	"github.com/redis/go-redis/v9"
)

// 秒杀结果码，对齐 Java 版 seckill.lua。
const (
	ResultOK         int64 = 0  // 成功
	ResultStockEmpty int64 = 1  // 库存不足
	ResultDuplicated int64 = 2  // 重复下单
	ResultNoStockKey int64 = -1 // 无库存 key（秒杀未预热/不存在）
)

// preDeductLua 原子完成：校验库存 + 一人一单 + 扣库存 + 记订单。
// Java 版 Lua 中 xadd 发 Redis Stream 是遗留代码（实际在应用层发 MQ），Go 版移除。
//
//go:embed pre_deduct.lua
var preDeductLua string

var script = redis.NewScript(preDeductLua)

// PreDeduct 执行秒杀预扣 Lua 脚本，返回结果码。
func PreDeduct(ctx context.Context, rdb *redis.Client, voucherID, userID int64) (int64, error) {
	return script.Run(ctx, rdb, nil, voucherID, userID).Int64()
}
