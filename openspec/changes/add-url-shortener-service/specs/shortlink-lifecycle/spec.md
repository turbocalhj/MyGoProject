# Spec Delta

## Purpose

定义短链从创建到失效的完整生命周期规则，包括输入校验、短码生成与唯一性、自定义别名、过期时间和启用状态，确保外部调用方可以稳定地创建和持有短链资源。

## ADDED Requirements

### Requirement: 创建短链

系统 SHALL 提供 `POST /api/v1/links` 接口创建短链，请求体包含必填的 `url`，可选 `alias` 与 `expires_at`。创建成功返回 `201` 及新建链接的完整表示。

#### Scenario: 使用自动生成的短码创建

- **WHEN** 客户端提交 `{"url":"https://example.com/a/very/long/path"}`
- **THEN** 系统返回 `201`，响应体含 7 位 base62 短码、`short_url`、`target_url`、`created_at`
- **AND** 该短码在数据库中唯一且未被占用

#### Scenario: 使用自定义别名创建

- **WHEN** 客户端提交 `{"url":"https://example.com","alias":"my-link"}`
- **THEN** 系统返回 `201`，`code` 为 `my-link`

#### Scenario: 目标地址不合法被拒绝

- **WHEN** 客户端提交的 `url` 为空、超过 2048 字符、或协议不是 `http`/`https`
- **THEN** 系统返回 `400`，错误码为 `invalid_url`，且不创建任何记录

### Requirement: 短码格式与唯一性

自动生成的短码 SHALL 为 7 位 base62 字符；自定义别名 SHALL 匹配 `^[a-zA-Z0-9_-]{3,32}$`。短码在同一时刻 MUST 全局唯一。

#### Scenario: 自定义别名冲突

- **WHEN** 客户端提交的 `alias` 已被其它链接占用
- **THEN** 系统返回 `409`，错误码为 `alias_taken`

#### Scenario: 自动短码发生碰撞

- **WHEN** 自动生成的短码已存在
- **THEN** 系统 MUST 重新生成，最多重试 5 次，全部失败时返回 `500` 并记录错误日志

#### Scenario: 别名格式非法

- **WHEN** 客户端提交 `{"alias":"a b!"}` 这类不符合字符集或长度的别名
- **THEN** 系统返回 `400`，错误码为 `invalid_alias`

### Requirement: 过期时间与启用状态

链接 SHALL 支持可选的 `expires_at`（RFC3339 时间戳，必须晚于当前时间）以及 `enabled` 状态，默认启用。

#### Scenario: 过期时间早于当前时间

- **WHEN** 客户端提交的 `expires_at` 早于当前时间
- **THEN** 系统返回 `400`，错误码为 `invalid_expires_at`

#### Scenario: 未设置过期时间

- **WHEN** 客户端提交时省略 `expires_at`
- **THEN** 创建的链接 `expires_at` 为 `null`，表示永久有效

### Requirement: 存储持久化

链接数据 MUST 持久化到关系型存储，服务重启后仍可读取；存储层失败时接口 SHALL 返回 `503` 而不是 `500`。

#### Scenario: 存储不可用

- **WHEN** 数据库连接失败或查询报错
- **THEN** 系统返回 `503`，错误码为 `storage_unavailable`，并记录包含 `request_id` 的错误日志
