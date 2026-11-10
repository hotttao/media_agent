---
name: application
description: 平台接入应用信任、凭据和受限短期代表访问
dependencies: []
related_workflow: [application_user_provisioning]
---

## 一句话职责
持有 Platform Application 的可信注册、状态、secret 版本及 Application Access Credential。

## 边界
管理应用，不管理 OAuth consent、交互登录、企业成员或 Workspace；应用托管 User 归 auth，映射归 identity。由 Workflow 组织跨领域写入。

## 能力清单
| 能力 | 接口 | 规则 |
|---|---|---|
| 注册/列举 | POST/GET /api/applications | 平台管理员；secret 仅首次返回 |
| 停用/恢复 | PATCH /api/applications/:id/status | 管理员；变化时 credential_version++，恢复不复活旧凭证 |
| 轮换 | POST /api/applications/:id/secret | 管理员；替换 hash 并递增版本；旧 secret 和访问凭证失效 |
| 托管用户 | PUT/GET /apps/v1/users/:external_id | Basic；通过 provisioning Workflow |
| 签发代表凭证 | POST /apps/v1/users/:external_id/token | Basic；一小时；缺省 profile，或显式 Team + scopes；无 refresh token |
| 代表用户资料 | GET /apps/v1/me | 仅 Application Bearer，不能用 Basic/Cookie |

## 聚合与不变量
Application 聚合持有 id/name/status/secret_hash/credential_version。id 不可更改，状态 active ↔ disabled；停用及恢复都递增版本。Credential 是短期授权记录，只保存随机 secret 的 SHA256，不保存明文。随机32字节 secret 与访问凭证分别使用不同前缀；profile 凭证只适用于 /apps/v1/me；业务凭证只进入已接入的 Control/Gateway 应用协议，验证当前 Grant 与成员状态。普通 OAuth Client、API Token、Cookie 和内部服务身份不能认证应用。

## 领域事件
无。当前状态和版本在每次访问时查询数据库，无异步撤销缓存。

## 数据模型
### applications
| 字段 | 类型 | 含义 |
|---|---|---|
| id | text PK | 管理员给定受限slug，永久稳定 |
| name | text | 展示名 |
| secret_hash | text | secret SHA256 |
| status | text | active/disabled |
| credential_version | bigint | 单调递增撤销版本 |
| created_by | text | 管理员User ID |
| created_at | timestamptz | 创建时间 |
### application_access_credentials
| 字段 | 类型 | 含义 |
|---|---|---|
| token_hash | text PK | 随机访问凭证SHA256 |
| application_id | text | Application引用 |
| user_id | text | User引用 |
| external_id | text | 签发时稳定外部ID |
| application_version | bigint | 签发时应用版本 |
| auth_version | bigint | 签发时用户版本 |
| expires_at | timestamptz | 一小时过期 |
约束：应用ID主键，凭证hash主键、application_id外键与过期索引。同库User/应用引用，不跨服务外键。每次签发清理本应用过期凭证。

## 能力详细说明
注册只接受admin和合法slug/name，应用已存在409；不自动生成system用户和团队。Basic校验使用常量时间hash比较，事务用例锁定应用行后重新校验。状态更新幂等，不重复增加版本；轮换每次产生新secret，响应丢失需再次轮换。列表绝不返回hash或secret。
凭证验证联合当前User/应用/Identity事实，应用或用户禁用、版本变化、映射缺失、过期均拒绝。签发不使用户获得任何Organization、Team或资源权限，不能代用户调用普通管理/业务API。

## 关联关系
[Application用户工作流](../../workflow/0015-application-user-provisioning/application_user_provisioning.md) 协调 auth 与 identity。新增服务不需要；协议解析留在handler/middleware，业务规则在service/domain，SQL在repository。

## 业务访问授权
Application Team Grant以(application_id,team_id)唯一，记录当前organization_id和允许scopes。平台admin创建/替换或撤销；每次实际变更递增应用credential_version，使旧凭证失效。允许scope固定为workspace.read、workspace.ensure、session.chat，不支持通配符。应用不能创建自己的Grant或自动获得成员关系。

业务凭证绑定一个Team及由事实源确定的Organization，scope必须是Grant子集；当前User必须同时为企业和部门成员且企业/部门active。签发和每次校验都查询当前事实与权限：read要求view、ensure要求create_workspace、chat要求manage（与既有RuntimeContext契约相同）。profile凭证仍不能访问业务入口。认证与业务资源最终授权均须通过。

applications增加部门授权子实体application_team_grants(application_id text,team_id text,organization_id text,scopes text[])，同库外键只引用本上下文Team。application_access_credentials增加team_id、organization_id、scopes；空team_id表示仅profile。内部/apps校验入口严格要求已配置服务凭据，不公开到Oathkeeper。

### Application MCP Target
平台管理员登记Definition ID→应用及精确HTTPS Origin，Definition ID唯一，应用Basic无权修改。Caller映射仅对活跃应用的托管User及app identity成立，必须具备当前组织、团队成员和session.chat Grant。未注册目标返回未启用，已注册目标不匹配返回拒绝。该记录不授予MCP工具权限。
