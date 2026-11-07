# MyGoProject — 短链服务

基于 Go + Gin + SQLite 的短链服务 REST API：创建短链、302 跳转、访问统计、链接管理。

技术选型参考了 GitHub 上热门的 Go 项目：Web API 生态（gin 89.6k stars）、CLI 生态
（cobra 44.8k stars）与标准项目布局（golang-standards/project-layout 56.7k stars）。

## 架构

```
        HTTP 请求
           │
           ▼
┌──────────────────────────┐
│ internal/httpapi         │  路由 / 处理器 / 中间件（请求 ID、日志、限流）
└─────────────┬────────────┘
              │ 领域调用
              ▼
┌──────────────────────────┐
│ internal/shortener       │  校验、短码生成、过期/启停判定
└─────────────┬────────────┘
              │ Repository 接口
              ▼
┌──────────────────────────┐
│ internal/store           │  database/sql + modernc.org/sqlite（纯 Go，无 CGO）
└──────────────────────────┘

cmd/server 负责配置加载、依赖组装、启动与优雅关闭。
```

## 目录结构

```
cmd/server/            程序入口（组装 + 优雅关闭）
internal/config/       环境变量配置与校验
internal/httpapi/      HTTP 路由、处理器、DTO、中间件
internal/shortener/    领域服务：短码生成、校验、生命周期
internal/store/        仓储接口与 SQLite 实现
internal/observability/结构化日志
openspec/              OpenSpec 规格与变更提案
```

## 快速开始

要求 Go 1.21+（当前开发使用 Go 1.27.3）。

```bash
go mod download
go run ./cmd/server
```

默认监听 `:8080`，数据库文件为 `shortlink.db`（自动建表）。

```bash
curl -s -X POST http://localhost:8080/api/v1/links \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com/very/long/path"}'
```

```bash
curl -i http://localhost:8080/<返回的短码>
```

## 配置

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | 监听地址 |
| `DATABASE_DSN` | `file:shortlink.db` | SQLite DSN，可设为 `file::memory:?cache=shared` |
| `BASE_URL` | 空（返回相对路径） | 生成 `short_url` 用的公开域名 |
| `RATE_LIMIT_RPS` | `20` | 每 IP 每秒补充令牌数 |
| `RATE_LIMIT_BURST` | `40` | 每 IP 桶容量 |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

## API

| 方法 | 路径 | 说明 | 成功状态 |
| --- | --- | --- | --- |
| POST | `/api/v1/links` | 创建短链（`url` 必填，`alias`、`expires_at` 可选） | 201 |
| GET | `/:code` | 跳转到目标地址 | 302 |
| GET | `/api/v1/links` | 列表，支持 `page`、`page_size`、`q` | 200 |
| GET | `/api/v1/links/:code` | 详情 | 200 |
| PATCH | `/api/v1/links/:code` | 部分更新 `target_url` / `expires_at` / `enabled` | 200 |
| DELETE | `/api/v1/links/:code` | 删除 | 204 |
| GET | `/api/v1/links/:code/stats` | 访问统计 | 200 |
| GET | `/healthz` | 存活检查 | 200 |
| GET | `/readyz` | 依赖检查（数据库 Ping） | 200 / 503 |

错误响应统一为：

```json
{"error": {"code": "link_not_found", "message": "link not found", "request_id": "..."}}
```

错误码：`invalid_url`、`invalid_alias`、`invalid_expires_at`、`invalid_pagination`、
`alias_taken`、`link_not_found`、`link_expired`、`link_disabled`、`rate_limited`、
`storage_unavailable`、`internal_error`。

## 测试

```bash
go vet ./...
go test ./...
go test -race ./...   # 需要 CGO：Linux/CI 可直接运行；Windows 无 GCC 时会提示 -race requires cgo
```

覆盖：短码生成与校验、仓储 CRUD/分页/原子计数、服务层生命周期、HTTP 集成链路
（`201/302/403/404/409/410/429`）、请求 ID 透传与限流。

## 开发流程

- 规格驱动：`openspec/changes/add-url-shortener-service/` 内含 proposal、design、
  specs 与 tasks，先评审规格再实现。
- Git 协作：`main` 保持可发布；功能在 `feat/*` 分支按层提交（store → shortener →
  httpapi → server），通过 PR 合入。

## 容器

```bash
docker build -t mygoproject:dev .
docker run -p 8080:8080 -e DATABASE_DSN=file:/data/shortlink.db -v $(pwd)/data:/data mygoproject:dev
```
