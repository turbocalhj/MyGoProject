# link-management Specification

## Purpose
定义短链资源的管理接口契约，包括分页列表、详情查询、部分更新、删除和访问统计，供运营后台与自动化脚本使用。

## Requirements

### Requirement: 链接列表

系统 SHALL 提供 `GET /api/v1/links`，支持 `page`（默认 1）、`page_size`（默认 20，最大 100）、`q`（按短码或目标地址模糊匹配）参数，按创建时间倒序返回。

#### Scenario: 默认分页

- **WHEN** 客户端请求 `GET /api/v1/links` 且存在 25 条记录
- **THEN** 响应含 `items` 20 条、`page` 为 1、`page_size` 为 20、`total` 为 25

#### Scenario: 非法分页参数

- **WHEN** `page` 小于 1 或 `page_size` 大于 100
- **THEN** 系统返回 `400`，错误码为 `invalid_pagination`

### Requirement: 链接详情

系统 SHALL 提供 `GET /api/v1/links/{code}` 返回单条链接的完整表示。

#### Scenario: 查询存在的链接

- **WHEN** 客户端请求存在的短码
- **THEN** 返回 `200` 及 `code`、`target_url`、`short_url`、`hits`、`enabled`、`expires_at`、`created_at`、`last_access_at`

#### Scenario: 查询不存在的链接

- **WHEN** 客户端请求不存在的短码
- **THEN** 返回 `404`，错误码为 `link_not_found`

### Requirement: 部分更新

系统 SHALL 提供 `PATCH /api/v1/links/{code}`，仅更新请求体中出现的字段，允许修改 `target_url`、`expires_at`、`enabled`；`code` 不允许修改。

#### Scenario: 只更新目标地址

- **WHEN** 客户端提交 `{"target_url":"https://example.org/new"}`
- **THEN** 返回 `200`，仅 `target_url` 发生变化，`hits` 与 `created_at` 不变

#### Scenario: 停用链接

- **WHEN** 客户端提交 `{"enabled":false}`
- **THEN** 返回 `200`，随后对 `GET /{code}` 的请求返回 `403`

#### Scenario: 清空过期时间

- **WHEN** 客户端提交 `{"expires_at":null}`
- **THEN** 返回 `200`，该链接变为永久有效

### Requirement: 删除链接

系统 SHALL 提供 `DELETE /api/v1/links/{code}`，删除成功返回 `204`，短码立即失效。

#### Scenario: 删除后跳转失效

- **WHEN** 客户端删除短码后再请求 `GET /{code}`
- **THEN** 跳转返回 `404`

#### Scenario: 重复删除

- **WHEN** 客户端删除一个不存在的短码
- **THEN** 返回 `404`，错误码为 `link_not_found`

### Requirement: 访问统计

系统 SHALL 提供 `GET /api/v1/links/{code}/stats` 返回该链接的 `code`、`hits`、`last_access_at`、`created_at` 与 `expires_at`。

#### Scenario: 从未被访问的链接

- **WHEN** 客户端查询一条创建后从未被跳转的链接
- **THEN** `hits` 为 `0`，`last_access_at` 为 `null`
