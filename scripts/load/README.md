# 压测复现

对齐 CityHub README 的压测结论：并发秒杀不超卖、一人一单、数据库只写入库存数量的订单。

## 准备

1. 启动依赖：

```bash
docker compose -f deploy/docker-compose.yml up -d mysql redis rocketmq-namesrv rocketmq-broker
```

2. 启动应用：

```bash
go run ./cmd/server
```

3. 登录拿 token：

```bash
curl -X POST "http://127.0.0.1:8081/user/code?phone=13800138000"
# 从应用日志取验证码（sms code sent）
curl -X POST http://127.0.0.1:8081/user/login \
  -H "Content-Type: application/json" \
  -d '{"phone":"13800138000","code":"<验证码>"}'
# 响应 data 字段即 token
```

4. 创建秒杀券（type=1，库存 100）：

```bash
curl -X POST http://127.0.0.1:8081/voucher/seckill \
  -H "Content-Type: application/json" \
  -d '{"shopId":1,"title":"测试秒杀券","subTitle":"压测","rules":"无","payValue":1,"actualValue":100,"type":1,"status":1,"stock":100,"beginTime":"2024-01-01T00:00:00Z","endTime":"2030-01-01T00:00:00Z"}'
# 响应 data 即 voucherId
```

## 压测

```bash
go install github.com/rakyll/hey@latest
hey -n 5000 -c 100 -H "authorization: <token>" -m POST http://127.0.0.1:8081/voucher-order/seckill/<voucherId>
```

## 预期

- 成功响应数 = 库存数（其余为「库存不足」/「不能重复下单」）
- `voucher_order` 表行数 = 库存数（异步落库后）
- `seckill_voucher.stock` = 0，Redis `seckill:stock:{id}` = 0
- 对账任务（每 5 分钟）日志无 mismatch
