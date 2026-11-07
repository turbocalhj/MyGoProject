# Design

## Context

仓库 `MyGoProject` 为空，需要从零建立 Go 服务基线。约束：

- 开发环境为 Windows，未安装 C 编译器，因此不能依赖 CGO。
- 交付物需要能直接在容器与 CI 中构建，且本地 `go test` 开箱即用。
- 参考 GitHub 热门 Go 项目的工程形态（gin 89.6k stars、cobra 44.8k stars、
  golang-standards/project-layout 56.7k stars），采用社区熟悉的分层与目录约定。

动机见 `proposal.md`，行为契约见 `specs/*/spec.md`。

## Goals / Non-Goals

**Goals:**

- 面向 `cmd/` + `internal/` 的标准布局，业务逻辑可在无 HTTP 场景下单独测试。
- 零外部服务依赖：一个二进制 + 一个 SQLite 文件即可运行。
- 存储实现可替换：仓储以接口暴露，便于日后接 PostgreSQL/Redis。
- 端到端可验证：`go test ./...` 覆盖跳转、CRUD、限流、错误结构。

**Non-Goals:**

- 不做用户体系、鉴权与多租户（当前 API 视为内网受信调用）。
- 不做分布式缓存、分片与高并发写入优化。
- 不做前端管理界面、自定义域名与二维码生成。
- 不做数据迁移工具（当前为初始版本，无历史数据）。

## Decisions

### D1. Web 框架：Gin（而非 Fiber / 标准库 net/http）

选择 Gin：路由分组、参数绑定、中间件链与 `httptest` 兼容性成熟，社区示例最多。
Fiber 性能更高但基于 fasthttp，脱离 `net/http` 语义（如 `http.Handler` 互操作、
标准中间件生态）；标准库方案需要自行实现路由参数与绑定，样板代码多。
**结论**：`gin-gonic/gin`。

### D2. 存储：`database/sql` + `modernc.org/sqlite`（而非 GORM / mattn/go-sqlite3）

`mattn/go-sqlite3` 需要 CGO，Windows 无 MinGW 时无法编译；GORM 对本项目仅一张表，
收益不足且增加调试成本。`modernc.org/sqlite` 为纯 Go 实现，`CGO_ENABLED=0` 可交叉编译。
**代价**：二进制体积更大、单条查询略慢，可接受。

### D3. 数据模型：单表 `links`

| 列 | 类型 | 说明 |
| --- | --- | --- |
| `code` | TEXT PRIMARY KEY | 短码，区分大小写 |
| `target_url` | TEXT NOT NULL | 目标地址 |
| `enabled` | INTEGER NOT NULL DEFAULT 1 | 0/1 |
| `expires_at` | TIMESTAMP NULL | 空表示永久 |
| `hits` | INTEGER NOT NULL DEFAULT 0 | 访问计数 |
| `created_at` / `updated_at` | TIMESTAMP NOT NULL | UTC |

`created_at DESC` 建索引以支撑列表排序；短码唯一性由主键保证，无需额外查重。

### D4. 短码生成：`crypto/rand` 7 位 base62

62^7 ≈ 3.5×10¹² 空间，按 10 万条链接计算碰撞概率可忽略。生成后直接 `INSERT`，
命中唯一约束冲突则重试，最多 5 次，避免"先查后插"的竞态。

### D5. 访问计数：单条原子 UPDATE

跳转成功后执行
`UPDATE links SET hits = hits + 1, last_access_at = ? WHERE code = ? AND enabled = 1`，
不采用"读-改-写"，避免并发下的计数丢失。计数失败不阻断跳转（只记录告警日志）。

### D6. 分层与依赖方向

```
cmd/server        → 组装 config / store / shortener / httpapi，负责启动与优雅关闭
internal/httpapi  → 路由、中间件、请求/响应 DTO
internal/shortener→ 领域服务：校验、短码生成、状态判定
internal/store    → Repository 接口 + SQLite 实现
internal/config   → 环境变量解析与默认值
```

`httpapi` 依赖 `shortener` 的接口，`shortener` 依赖 `store` 接口，禁止反向依赖；
HTTP 层不出现 SQL，存储层不出现 `gin.Context`。

### D7. 错误模型

领域层返回带类型哨兵错误（`shortener.ErrNotFound/ErrExpired/ErrDisabled/ErrConflict/ErrInvalid`），
HTTP 层集中映射为 `404/410/403/409/400`；存储层错误统一包装为 `ErrStorage` → `503`。
响应统一为 `{"error":{"code","message","request_id"}}`（见 `specs/service-operations`）。

### D8. 限流：内存令牌桶（每 IP 一个桶）

键为 `c.ClientIP()`，桶容量 40、恢复速率 20/s，按需创建并用周期性清理（每 5 分钟
清理 10 分钟未使用的桶）防止内存增长。健康检查路由跳过限流。
**替代方案**：Redis 令牌桶（需外部依赖，违背 Non-Goals）；`golang.org/x/time/rate` 的
`Limiter` 仍是每键一个实例，仍需自己做键管理与清理，因此直接手写简化实现。

### D9. 可观测性：`log/slog`

标准库结构化日志输出 JSON，字段 `request_id`、`method`、`path`、`status`、`latency_ms`；
请求 ID 由中间件从 `X-Request-ID` 透传或生成。

### D10. 配置：仅环境变量

`HTTP_ADDR`（默认 `:8080`）、`DATABASE_DSN`（默认 `file:shortlink.db`）、`BASE_URL`
（默认由请求 Host 推导）、`RATE_LIMIT_RPS`、`RATE_LIMIT_BURST`、`LOG_LEVEL`。
启动即校验，非法值直接退出，避免运行期隐性错误。

### D11. 测试策略

- 单元测试：短码字符集/长度、URL 校验、别名正则、令牌桶时间推进（可注入时钟）。
- 集成测试：`httptest.NewServer` + 内存 DSN（`file:testdb?mode=memory&cache=shared`），
  覆盖创建→跳转→统计→更新→删除全链路，以及 `404/410/403/409/429` 分支。
- CI：`go vet ./...`、`go test -race ./...`、`go build ./cmd/server`。

## Risks / Trade-offs

- [SQLite 单写者] 高并发写入会串行化 → 本服务主要读多写少，且跳转计数为单条 UPDATE，影响有限；
  后续可换 PostgreSQL 实现同一 `Repository` 接口。
- [内存限流不跨实例] 多副本部署时每副本独立计数 → 当前为单实例部署假设，已写入 Non-Goals。
- [自动建表不可演进] `CREATE TABLE IF NOT EXISTS` 无法处理后续列变更 → 初始版本接受，
  迭代时引入版本化迁移。
- [纯 Go SQLite 体积/性能] 二进制约增大 10MB → 换取免 CGO 的构建一致性。
- [短码可枚举] 7 位 base62 非顺序可猜测但无鉴权导出接口 → 管理接口按内网假设不做鉴权。

## Migration Plan

首次发布，无历史数据。部署步骤：

1. `docker build -t mygoproject:0.1.0 .`
2. 挂载持久卷并设置 `DATABASE_DSN=file:/data/shortlink.db`
3. 启动后探活 `GET /readyz`

回滚：停止容器并回退镜像标签即可，数据文件向后兼容（仅新增表）。
