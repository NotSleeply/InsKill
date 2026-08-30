# InsKill

黑马点评的 Go 版实现：类大众点评的本地生活服务平台，覆盖登录、商户缓存、优惠券秒杀、探店笔记、关注 Feed、签到、UV 统计等完整功能。
Go 1.22 + Gin + GORM + Redis + RocketMQ + Mysql。

## 架构

标准 Go 分层，依赖方向 `handler → service → repository`，接口由消费方定义，main 手写依赖注入：

```bash
cmd/server           组装入口：配置→DB/Redis/MQ→注入→启动
internal/
├── config/          配置加载（环境变量优先，可选 yaml）
├── server/          路由注册、中间件装配、优雅关闭
├── handler/         HTTP 层：参数解析、调 service、映射错误码
├── service/         业务逻辑：接口 + 实现同包，repository 接口在此定义
├── repository/      GORM 数据访问实现
├── model/           贫血实体
├── middleware/      鉴权双中间件、滑动窗口限流、UV 统计
├── cache/           二级缓存（ristretto 本地 + Redis 远程）
├── lock/            分布式锁（SETNX + Lua 解锁）
├── seckill/         秒杀域：预扣 Lua 脚本、库存、一人一单
├── mq/              RocketMQ Publisher/Consumer 封装
├── id/              订单 ID：时间戳<<32 | Redis 日自增
├── scheduler/       定时任务：超时关单、库存对账
└── pkg/errs/        哨兵错误 + 统一响应 Result
```

## 快速启动

```bash
docker compose -f deploy/docker-compose.yml up -d --build
# 服务监听 :8081，健康检查：
curl http://127.0.0.1:8081/healthz
```

本地开发（MySQL/Redis/RocketMQ 已就绪）：

```bash
go run ./cmd/server
```

## 核心设计（对照 CityHub README 的复盘）

- **缓存穿透**：查无缓存空值（TTL 2 分钟）
- **缓存击穿**：热点商户用逻辑过期策略——过期后抢互斥锁异步重建，旧数据兜底返回（AP 优先）
- **缓存雪崩**：TTL 加随机偏移
- **二级缓存**：ristretto只用于秒杀券详情等极热 key，5 秒短 TTL 自刷，不做广播失效
- **秒杀**：Lua 原子完成库存校验 + 一人一单 + 扣减（`internal/seckill/pre_deduct.lua`）→ RocketMQ 异步落库 → 订单 ID 主键幂等。
- **订单生命周期**：支付回调与超时关单都用乐观锁 `WHERE status=未支付`，关单成功的库存回补走 `stock-refund` 消息（SETNX 幂等占位）；定时任务每分钟关单扫描、每 5 分钟对账（Redis 扣减数 vs MySQL 订单数）
- **滑动窗口限流**：Redis ZSet + Lua（ZREMRANGEBYSCORE + ZCARD + ZADD 原子），秒杀接口按 IP、领券按用户
- **Feed 推流**：发布笔记推给全部粉丝（ZSet，score 毫秒时间戳），滚动分页按 score 倒序 + 同分 offset
- **点赞排行榜** ZSet、**共同关注** SINTER、**签到** BitMap、**UV** HyperLogLog、**附近商户** GEO

## API

路径与响应格式（`Result{success, errorMsg, data}`）：

- 登录：`POST /user/code`、`POST /user/login`、`GET /user/me`、`POST /user/sign`、`GET /user/sign/count`、`GET /user/info/{id}`、`GET /user/{id}`
- 商户：`GET /shop/{id}`、`GET /shop/of/type`、`GET /shop/of/name`、`POST /shop`、`PUT /shop`、`GET /shop-type/list`
- 优惠券：`POST /voucher`、`POST /voucher/seckill`、`GET /voucher/list/{shopId}`、`POST /voucher-order/seckill/{id}`、`POST /voucher-order/pay-callback`
- 探店：`POST /blog`、`PUT /blog/like/{id}`、`GET /blog/hot`、`GET /blog/{id}`、`GET /blog/likes/{id}`、`GET /blog/of/me`、`GET /blog/of/user`、`GET /blog/of/follow`、`GET /blog/uv`
- 关注：`PUT /follow/{id}/{isFollow}`、`GET /follow/or/not/{id}`、`GET /follow/common/{id}`
- 上传：`POST /upload/blog`、`GET /upload/blog/delete`

## 测试

```bash
make test          # 单测（fake repository + miniredis，无需外部依赖）
make integration   # 集成测试（testcontainers 起 MySQL/Redis：并发秒杀不超卖、消息幂等、签到）
make vet
```

压测复现见 `scripts/load/README.md`。


## 项目简介
提供商家信息查询、优惠券秒杀、推广优惠信息等功能，针对秒杀场景下的性能瓶颈，基于**缓存架构**与**消息队列**进行了性能深度优化，显著提升高并发场景下的系统稳定性。
- 设计**Redis+Lua**实现高并发库存扣减与一人一单校验，解决超卖问题，保障数据一致性。
- 引入**RocketMQ**将秒杀链路中的下单流程异步解耦，实现核心接口流量削峰并提升秒杀场景并发性能。
- 使用逻辑过期方案防止Redis热点Key的**缓存击穿**问题，使用缓存空值方案解决RedisKey的**缓存穿透**问题。
- 更新数据库后删除缓存，删除失败采取**RocketMQ补偿重试**，结合TTL兜底共同确保数据最终一致性。
- 基于Redis+Gin中间件实现**滑动窗口限流**，支持全局/IP/用户多维度，防止系统过载、抢券、爬虫。
- 基于**定时扫描**实现分钟级关单任务，自动关闭超时未支付订单，有效释放被占用的库存资源。
- 使用**乐观锁**解决支付回调与超时关单状态下的并发问题，确保订单状态流转的准确性与数据一致性。
