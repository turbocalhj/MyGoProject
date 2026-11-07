# Spec Delta

## Purpose

定义服务运行期可观测性与稳定性要求，包括健康检查、统一错误响应、请求日志与请求 ID、限流以及优雅关闭，使服务可被容器编排与监控系统可靠托管。

## ADDED Requirements

### Requirement: 健康检查

系统 SHALL 提供 `GET /healthz` 表示进程存活，`GET /readyz` 表示依赖（数据库）可用。

#### Scenario: 进程存活

- **WHEN** 客户端请求 `GET /healthz`
- **THEN** 返回 `200` 与 `{"status":"ok"}`，且不访问数据库

#### Scenario: 数据库不可用

- **WHEN** 客户端请求 `GET /readyz` 且数据库 `Ping` 失败
- **THEN** 返回 `503` 与 `{"status":"unavailable"}`

### Requirement: 统一错误响应

所有非 `2xx` 响应 SHALL 使用统一 JSON 结构 `{"error":{"code":"...","message":"...","request_id":"..."}}`，且 `Content-Type` 为 `application/json`。

#### Scenario: 参数错误响应结构

- **WHEN** 客户端提交非法请求体
- **THEN** 响应体包含 `error.code`、`error.message`、`error.request_id` 三个字段

### Requirement: 请求 ID 与访问日志

每个响应 SHALL 携带 `X-Request-ID` 头；若请求已带该头则原样透传，否则由服务生成 UUID。服务 SHALL 记录包含 `method`、`path`、`status`、`latency_ms`、`request_id` 的访问日志。

#### Scenario: 透传上游请求 ID

- **WHEN** 请求头包含 `X-Request-ID: trace-123`
- **THEN** 响应头 `X-Request-ID` 为 `trace-123`，访问日志中同一值

#### Scenario: 生成请求 ID

- **WHEN** 请求未携带 `X-Request-ID`
- **THEN** 响应头返回一个 UUID 格式的新值

### Requirement: 限流

系统 SHALL 按客户端 IP 实施令牌桶限流，默认每秒 20 个请求、突发 40；超限返回 `429` 并携带 `Retry-After`。

#### Scenario: 超出限流阈值

- **WHEN** 同一 IP 在 1 秒内发送超过配置的突发上限请求
- **THEN** 超出部分返回 `429`，错误码为 `rate_limited`，且含 `Retry-After` 头

#### Scenario: 健康检查不受限流影响

- **WHEN** 限流已触发，客户端仍请求 `GET /healthz`
- **THEN** 返回 `200`

### Requirement: 优雅关闭

服务 SHALL 在收到 `SIGINT` 或 `SIGTERM` 后停止接受新连接，并在最多 10 秒内完成在途请求后关闭数据库连接。

#### Scenario: 收到终止信号

- **WHEN** 进程收到 `SIGTERM` 且存在在途请求
- **THEN** 在途请求被处理完成，进程随后以退出码 `0` 结束
