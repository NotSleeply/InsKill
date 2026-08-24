# InsKill 设计文档：黑马点评 Go 化

日期：2026-08-24
状态：已确认（待用户复审）

## 背景与目标

将 D:\Code\CityHub 的黑马点评 Java 项目（Spring Boot 2.3 + MyBatis-Plus + Redis + RocketMQ）转换为 Go 项目 InsKill。要求：Go 风格的架构设计，不做机械翻译——分层骨架因业务形态而保留，命名、组织、依赖注入、错误处理、接口设计全部按 Go 惯例重做。

## 范围

**核心功能（黑马点评全部）+ CityHub 增强项（全部保留）**：

- 用户登录：手机号验证码 + Redis token 双中间件鉴权
- 商户查询：按 id / 按类型分页 / 附近商户（GEO），缓存穿透（空值）、击穿（互斥锁 + 逻辑过期）、雪崩（TTL 随机）
- 二级缓存：ristretto 本地缓存（秒杀券详情等热 key，5 秒短 TTL）
- 优惠券与秒杀：Redis 预扣 Lua（库存 + 一人一单原子校验）→ RocketMQ 异步落库 → 订单幂等
- 订单：支付回调、超时关单（定时扫描 + 被动关单）、乐观锁防支付/关单并发冲突、库存回补走 MQ
- 对账：秒杀结束后 Redis 扣减数与 MySQL 订单数比对
- 探店笔记：发布、滚动分页 Feed、点赞排行榜（ZSet）
- 关注：关注/取关、共同关注（SINTER）、Feed 推流
- 用户签到（BitMap）、UV 统计（HyperLogLog）
- 滑动窗口限流（Redis ZSet + Lua，按 IP / 用户维度）

**实现状态说明**：以上增强项中，缓存穿透/击穿/雪崩、RabbitMQ 异步下单在 Java 代码里已实现；二级缓存（Caffeine）、滑动窗口限流、超时关单、支付回调、对账任务在 CityHub README 中有完整设计但 Java 代码未实现。Go 版按 README 设计将这些补齐为真实实现。

## 技术选型

- **语言**：Go 1.22+（slog、go:embed）
- **HTTP**：Gin，生态事实标准，中间件齐全
- **数据访问**：GORM（用户指定），简单 CRUD 用 GORM，复杂 SQL 走 Raw
- **Redis**：go-redis，事实标准客户端
- **MQ**：RocketMQ（apache/rocketmq-client-go/v2，用户最终确认），抽象 Publisher/Subscriber 接口隔离客户端。注：Java 版实际运行代码是 RabbitMQ（交换机 X/路由 XA/队列 QA、TTL 10 秒转死信 QD），README 宣称 RocketMQ 但代码未实现；Go 版以 RocketMQ 落地：Topic `seckill-order` + 消费组 + 重试/死信队列，幂等靠订单 ID 唯一键（对应 Java 版 QA/QD 双消费的结构语义）
- **本地缓存**：ristretto，Go 生态最接近 Caffeine（W-TinyLFU）
- **日志**：标准库 slog，结构化 JSON，零依赖
- **配置**：环境变量优先 + 可选 config.yaml（12-factor）
- **定时任务**：robfig/cron，关单扫描、对账任务

## 架构方案

方案 A：标准 Go 分层。备选方案 B（DDD 四层）对 CRUD+缓存项目过度设计，方案 C（垂直切片）因横切关注点过多而切不干净，均不采用。

### 目录结构

```
inskill/
├── cmd/server/main.go          # 组装入口：配置→DB/Redis/MQ→注入→启动
├── internal/
│   ├── config/                 # 配置加载（env + 可选 yaml）
│   ├── server/                 # 路由注册、中间件装配、优雅关闭
│   ├── handler/                # HTTP 层：参数解析、调 service、映射错误码
│   ├── service/                # 业务逻辑：接口 + 实现同包
│   ├── repository/             # GORM 实现；接口定义在 service 包（消费者一侧）
│   ├── model/                  # 贫血实体，所有层共享
│   ├── middleware/             # 鉴权双中间件、滑动窗口限流、日志
│   ├── cache/                  # 二级缓存统一接口（ristretto + Redis）
│   ├── lock/                   # 分布式锁（SETNX + Lua 解锁）
│   ├── seckill/                # 秒杀域：预扣 Lua 脚本（go:embed 内嵌）、库存、一人一单
│   ├── mq/                     # Publisher/Subscriber 接口 + RocketMQ 实现
│   ├── id/                     # 订单 ID：时间戳<<32 | Redis 日自增（直译 RedisIdWorker，非经典雪花）
│   ├── scheduler/              # 定时任务注册
│   └── pkg/errs/               # 哨兵错误 + 统一响应 Result
├── scripts/schema.sql          # 改造后的建表脚本
├── deploy/docker-compose.yml   # MySQL + Redis + RocketMQ + 应用
└── config.yaml.example
```

依赖方向单向：`handler → service → repository`，横切包（cache/lock/seckill/mq/id）只被 service 依赖。Java 版 `utils` 大杂烩拆为多个单一职责包，全局常量收进各自包。

### Go 风格关键决策

- **依赖注入**：不用 wire 类框架，main 手写组装，构造函数注入接口
- **错误处理**：service 返回 error 值，哨兵错误 + `errors.Is/As`；handler 统一映射 HTTP 状态码。响应格式 `Result{success, data, errorMsg}` 保留（前端兼容）
- **接口设计**：repository 接口定义在 service 包（消费方），实现可替换 fake 做测试；service 接口由 handler 消费。与 Java 版 mapper 接口/实现分离的最大差异点
- **GORM 用法**：CRUD 走链式，滚动分页、乐观锁 UPDATE、连表查询走 Raw SQL 直译原 MyBatis SQL
- **限流**：注解+切面 → 中间件工厂 `middleware.RateLimit(prefix, window, limit)`，路由注册时显式声明，无反射
- **鉴权**：双拦截器 → 双中间件 `RefreshToken`（无感续期，挂全部路由）+ `RequireAuth`（校验，挂受限路由组）；ThreadLocal UserHolder → context 传递
- **二级缓存**：只覆盖极热 key，短 TTL 自刷，不做广播失效（CityHub README 结论原样保留）

## 数据模型

### Schema 改造（相对 hmdp.sql）

1. 表名去 `tb_` 前缀：blog, blog_comments, follow, seckill_voucher, shop, shop_type, user, user_info, voucher, voucher_order
2. 删除 `tb_sign`（签到用 BitMap，无表依赖）
3. 补索引：
   - `voucher_order`：`idx_user_id`、`idx_voucher_id`
   - `blog`：`idx_shop_id`、`idx_user_id`
   - `blog_comments`：`idx_blog_id`、`idx_parent_id`
   - `follow`：`UNIQUE(user_id, follow_user_id)`
   - `shop`：`idx_type_id`
   - `seckill_voucher`：`idx(begin_time, end_time)`
4. 字段、数据不变；INSERT 数据改表名后可直接导入，方便与 Java 版对照验证
5. 补遗漏列：`voucher` 表补 `stock`、`begin_time`、`end_time` 三列（Java 实体和 addSeckillVoucher 依赖，原 SQL 文件遗漏）

### 其他

- GORM model 显式 `TableName()`（默认复数映射会错）
- `voucher_order.id` 为雪花 int64，非自增
- 外键约束沿用原项目惯例：应用层保证，不加 DB 外键

## 核心链路

### 登录鉴权

验证码 6 位存 Redis（`login:code:{phone}`，TTL 2 分钟）→ 登录校验发 token（UUID）存 Redis（`login:token:{token}`，TTL 30 分钟）。`RefreshToken` 中间件挂 `/api` 全部路由：token 存在即续期、UserDTO 注入 context；`RequireAuth` 挂受限路由组：context 无用户返回 401。

### 商户查询缓存

`cache.Client` 统一接口：Get/Set/Delete + 互斥锁重建 + 逻辑过期重建。穿透缓存空值（TTL 2 分钟）；击穿：互斥锁策略用于普通商户详情，逻辑过期策略（异步 goroutine 重建、先返回旧数据）用于热点商户；雪崩 TTL 随机偏移。二级缓存只用于秒杀券详情等极热 key。

### 秒杀链路（核心）

Redis 预扣 Lua（校验库存 + 一人一单 Set，原子）→ 返回「排队中」→ RocketMQ 异步消费落库。幂等：订单雪花 ID 唯一键，重复消息插库失败即跳过。对账任务：秒杀结束比对 Redis 扣减数与 MySQL 订单数，不一致告警。

### 订单支付与关单

支付回调、超时关单均乐观锁：`UPDATE ... WHERE id=? AND status='未支付'`，影响行数 0 即对方先成功。关单库存回补走 MQ 重试补偿。scheduler 每分钟扫描超时未支付订单，订单列表查询时顺带被动关单。若关单成功而支付回调后到（用户已付款），触发退款流程原路退回，不允许强制改回已支付（防止库存二次释放导致超卖）。

### 社交与 Redis 特性

- 点赞：ZSet `blog:liked:{id}`，score 时间戳
- 关注：Redis Set + DB 双写，共同关注 SINTER
- Feed：关注推流，滚动分页 `score DESC, id DESC`（GORM Raw）
- 签到 BitMap、UV HyperLogLog、附近商户 GEOSEARCH：逻辑直译

### 滑动窗口限流

中间件工厂 + Lua（ZREMRANGEBYSCORE + ZCARD + ZADD 原子）。秒杀接口按 IP 限流（防爬虫刷单），领券按用户限流。

## API 兼容

路由路径和响应格式对齐原项目（`/shop/{id}`、`/voucher-order/seckill/{id}`、`Result{success,data,errorMsg}`），原 Vue 前端改 baseURL 即可使用。部署拓扑为单体直连前端，内建网关友好特性：无状态鉴权、`/healthz`、优雅关闭、结构化日志、配置外置。

## 测试策略

- **service 单测**：手写 fake repository（接口在 service 包）；Redis 用 miniredis，Lua 直接加载执行
- **handler 测试**：httptest + fake service
- **集成测试**（秒杀链路）：testcontainers 起 MySQL + Redis，N goroutine 并发下单，断言库存不为负、一人一单不破
- **压测**：hey/vegeta 脚本放 `scripts/`，README 写复现步骤

## 部署与运行

- `docker-compose.yml`：MySQL 8 + Redis 7 + RocketMQ（namesrv + broker）+ 应用（多阶段 Dockerfile）- 配置：环境变量优先，config.yaml 可选覆盖
- Makefile：`build / run / test / up`；schema.sql 挂 MySQL 初始化目录
- 应用四件套：优雅关闭（signal + http.Server.Shutdown + 消费者关停）、`/healthz`、slog 结构化日志、配置外置

## 验收标准

- API 兼容原前端，行为与 Java 版可对照
- 并发秒杀不超卖、一人一单（集成测试 + 压测证明）
- `go vet` + 全部测试通过
- 四件套就位

## 实施里程碑（供实施计划展开）

1. 项目骨架：go mod、config、server、errs、健康检查
2. 数据层：schema.sql、GORM model、repository
3. 登录鉴权
4. 商户查询 + cache 包（互斥锁/逻辑过期/二级缓存）
5. 秒杀 + MQ + 订单 + 关单 + 对账
6. 社交模块（博客/关注/签到/UV/附近商户）
7. 限流中间件
8. 测试补齐 + docker-compose + 压测验证
