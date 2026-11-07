# shortlink-redirection Specification

## Purpose
定义短码解析与跳转这一核心对外行为，明确可用、未知、过期、停用四种情形下的 HTTP 语义，并规定访问计数只在成功跳转时发生。

## Requirements

### Requirement: 短码跳转

系统 SHALL 提供 `GET /{code}` 路由，将有效短码解析为目标地址并返回 `302 Found`，`Location` 指向原始 URL。

#### Scenario: 有效短码跳转

- **WHEN** 客户端请求 `GET /abc1234`，且该短码存在、已启用、未过期
- **THEN** 系统返回 `302`，`Location` 为原始 URL
- **AND** 响应体为空，且 `Cache-Control: no-store` 被设置

#### Scenario: 短码不存在

- **WHEN** 客户端请求一个不存在的短码
- **THEN** 系统返回 `404`，错误码为 `link_not_found`

#### Scenario: 短码已过期

- **WHEN** 客户端请求的短码 `expires_at` 早于当前时间
- **THEN** 系统返回 `410`，错误码为 `link_expired`

#### Scenario: 短码已停用

- **WHEN** 客户端请求的短码 `enabled` 为 `false`
- **THEN** 系统返回 `403`，错误码为 `link_disabled`

### Requirement: 访问计数

每次成功跳转 SHALL 使该链接的 `hits` 加一并更新 `last_access_at`；失败的分支（`404`/`410`/`403`）MUST NOT 改变计数器。

#### Scenario: 成功跳转累加计数

- **WHEN** 同一短码被成功请求 3 次
- **THEN** `GET /api/v1/links/{code}/stats` 返回的 `hits` 恰好增加 3

#### Scenario: 失败请求不计数

- **WHEN** 客户端请求已过期的短码并收到 `410`
- **THEN** 该链接的 `hits` 保持不变

### Requirement: 短码大小写与保留路径

短码匹配 SHALL 区分大小写；`/api/`、`/healthz`、`/readyz`、`/metrics` MUST NOT 被当作短码解析。

#### Scenario: 保留路径不进入短码解析

- **WHEN** 客户端请求 `GET /healthz`
- **THEN** 系统走健康检查处理器，不查询短码表
