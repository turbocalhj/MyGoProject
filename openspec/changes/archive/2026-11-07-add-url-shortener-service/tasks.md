# Tasks

> 状态说明：实现与验证已完成并归档。少数条目因本机环境限制未执行（详见对应备注）。

## 1. 工程脚手架

- [x] 1.1 在仓库根创建 `go.mod`（module `github.com/turbocalhj/MyGoProject`，go 1.27），添加 gin、modernc.org/sqlite、testify 依赖，验证 `go mod tidy` 无报错
- [x] 1.2 创建标准目录 `cmd/server`、`internal/{config,httpapi,shortener,store,observability}` 与 `.gitignore`，验证 `go list ./...` 能列出全部包
- [x] 1.3 实现环境变量配置加载与校验（`internal/config`），附单元测试覆盖默认值、非法值报错，验证 `go test ./internal/config` 通过

## 2. 存储层

- [x] 2.1 定义 `Link` 模型与 `Repository` 接口（`Create/GetByCode/List/Update/Delete/IncrementHits/Ping`），验证接口在 `go vet` 下无未实现引用
- [x] 2.2 实现 SQLite 仓储与自动建表（`links` 表 + `created_at` 索引），验证内存 DSN 下 `go test ./internal/store` 通过
- [x] 2.3 编写仓储测试：创建/查重冲突、分页与模糊查询、部分更新、删除、原子计数、`Ping` 失败返回 `ErrStorage`，验证 `go test ./internal/store` 通过

## 3. 领域服务

- [x] 3.1 实现短码生成器（`crypto/rand` 7 位 base62，碰撞重试 5 次），验证生成测试覆盖字符集、长度与碰撞重试路径
- [x] 3.2 实现 URL/别名/过期时间校验与哨兵错误（`ErrInvalid/ErrConflict/ErrNotFound/ErrExpired/ErrDisabled/ErrStorage`），验证表驱动单元测试通过
- [x] 3.3 实现 `shortener` 服务：创建、解析（含过期/停用判定）、列表、更新、删除、统计计数，验证服务层单元测试（含假仓储）全部通过

## 4. HTTP 层

- [x] 4.1 实现请求/响应 DTO 与统一错误响应映射，验证非法请求体返回 `error.code/message/request_id` 结构
- [x] 4.2 实现 Gin 路由与处理器（`POST/GET/PATCH/DELETE /api/v1/links*`、`GET /:code`），验证 `httptest` 集成测试覆盖 `201/200/204/400/403/404/409/410`
- [x] 4.3 编写端到端集成测试：创建→跳转→统计→更新→停用→删除全链路，验证 `go test ./internal/httpapi` 通过
- [x] 4.4 在 `README.md` 记录 API 契约与 `curl` 示例，验证文档中的命令可按原样执行

## 5. 运行期能力

- [x] 5.1 实现 `/healthz`、`/readyz`（`Ping` 探活），验证不可用场景返回 `503`
- [x] 5.2 实现请求 ID 中间件与 slog 访问日志（`method/path/status/latency_ms/request_id`），验证透传与生成两种场景测试通过
- [x] 5.3 实现按 IP 令牌桶限流（`429` + `Retry-After`，健康检查豁免），验证时间推进测试与超限集成测试通过
- [ ] 5.4 实现 `cmd/server` 组装、启动与优雅关闭（10 秒超时、关闭数据库）
  - 备注：已实现并完成启动冒烟测试；Windows 本地未发送 `SIGTERM` 做信号验证。

## 6. 交付与集成验证

- [x] 6.1 编写 `Makefile`（`run/test/lint/build`）与 `.golangci.yml`
  - 备注：本机未安装 `make`，目标内容按 Go 官方约定编写，等价命令已在本地执行。
- [ ] 6.2 编写多阶段 `Dockerfile`（`CGO_ENABLED=0` 静态构建）
  - 备注：Dockerfile 已编写，本机未执行 `docker build`。
- [x] 6.3 配置 GitHub Actions CI（`go vet`、`go test -race`、`go build ./cmd/server`），workflow 已在 PR #2 上实跑通过
- [x] 6.4 执行完整验证：`go vet ./...`、`go test ./...`、`go build ./cmd/server` 全部通过，并启动服务实跑一次创建+跳转（201 → 302 → hits=1）
- [x] 6.5 补全 `README.md`（架构图、快速开始、配置项、测试说明）

## Workflow follow-up

- [x] 变更实现完成并通过验证后，执行 `openspec archive add-url-shortener-service`
- [x] 归档后同步更新 `openspec/specs/` 下的主规格（4 个能力共 17 条 Requirement）
