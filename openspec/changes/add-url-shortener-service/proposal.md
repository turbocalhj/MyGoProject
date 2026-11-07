# Proposal: 短链服务 REST API

## Why

需要一个可独立部署、零外部依赖的短链服务：把长 URL 压缩成短码，支持跳转与访问统计。
当前仓库 `MyGoProject` 为空仓库，目标是用 Go 生态中最主流的 Web 技术栈（Gin 89.6k stars、
database/sql + GORM 40k stars 级别生态）建立一个结构规范、可测试、可交付的最小生产级服务，
作为后续迭代的基线工程。

## What Changes

- 新建 Go module `github.com/turbocalhj/MyGoProject`，采用标准 Go 项目布局（`cmd/` + `internal/`）。
- 新增 HTTP API 服务，基于 Gin：
  - `POST /api/v1/links` 创建短链（支持自定义别名、过期时间）
  - `GET /:code` 302 跳转到原始 URL，并记录一次访问
  - `GET /api/v1/links` 分页列出链接，`GET /api/v1/links/:code` 查询详情
  - `PATCH /api/v1/links/:code` 更新目标地址/过期时间/启用状态，`DELETE /api/v1/links/:code` 删除
  - `GET /api/v1/links/:code/stats` 查询访问统计
  - `GET /healthz`、`GET /readyz` 健康检查
- 持久化使用 `database/sql` + 纯 Go SQLite 驱动（`modernc.org/sqlite`），启动时自动建表，
  通过 `DATABASE_DSN` 支持内存库（测试）与文件库（生产）。
- 统一错误响应、请求日志、请求 ID、限流（基于内存令牌桶）中间件。
- 测试：纯单元测试（短码生成、校验、限流）+ `httptest` 集成测试（完整 CRUD 与跳转）。
- 工程化：`Makefile`、`Dockerfile`、`.golangci.yml`、GitHub Actions CI（lint/test/build）。
- 文档：ASCII 架构说明与 API 使用示例的 `README.md`。

## Capabilities

### New Capabilities

- `shortlink-lifecycle`: 短链的创建、唯一短码/自定义别名、过期与启停等生命周期规则。
- `shortlink-redirection`: 按短码解析并跳转的行为，包含 302/404/410 语义与访问计数。
- `link-management`: 链接列表、详情、更新、删除与访问统计查询的 API 契约。
- `service-operations`: 健康检查、请求日志/请求 ID、限流与优雅关闭等运行期行为。

### Modified Capabilities

无（仓库为新建项目，尚无可修改的能力）。

## Impact

- 新增代码：`cmd/server`、`internal/{config,httpapi,store,shortener,observability}`。
- 新增依赖：`github.com/gin-gonic/gin`、`modernc.org/sqlite`、`github.com/stretchr/testify`（测试）。
- 新增配置项：`HTTP_ADDR`、`DATABASE_DSN`、`BASE_URL`、`RATE_LIMIT_RPS`、`LOG_LEVEL`。
- 不涉及破坏性变更；不影响任何既有系统。
