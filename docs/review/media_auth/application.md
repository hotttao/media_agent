> 本文件为第一阶段验收记录。后续业务接入与真实 Kratos 验收见 [应用平台接入验收](../application-platform-access.md)，下文“未实现/profile-only”仅描述首阶段。

# 应用认证与用户映射首阶段验收

2026-11-07，platform 分支。范围：应用身份闭环与Ory边缘接入，不包括通用业务API代表访问、Agent创建或MCP Caller。

## 能力覆盖

| 能力 | 事实所有者 / 编排 | 状态 |
|---|---|---|
| 应用注册、列举、停用恢复、secret轮换 | application / Application | covered |
| Basic认证、profile短期凭证、每次查询撤销 | application / Credential | covered |
| 托管用户资料与禁止交互身份 | auth / User | covered |
| 稳定外部ID映射、不可通用解绑 | identity / Identity | covered |
| 原子创建与重复恢复 | ApplicationUsers workflow | covered |
| 管理员边缘认证与内部JWT | Ory Oathkeeper，Auth映射业务User | external-owner |
| 登录会话与恢复 | Ory Kratos | external-owner |
| 组织部门权限投影 | 既有Auth领域及Ory Keto | external-owner |

首阶段入口均覆盖；原002更广范围中的应用团队、资源发布、ensureAgent、对外Caller、SDK和通用业务接入仍未实现，不以本报告关闭这些缺口。

## 验证结果

- 全量media_auth单元测试通过（umask 022）；架构检查通过。
- TestApplicationIsolationLifecycleAndConcurrency：12并发upsert只创建一个User；跨应用同external_id隔离；Kratos同邮箱不合并托管User；LDAP/改密/解绑旁路拒绝；轮换、停用、恢复、软删除、过期均按版本拒绝旧凭证。
- TestApplicationProvisioningRollback：Identity落库失败后无孤立User，重试恢复；有托管用户时down migration拒绝且事务回滚保留身份边界。
- TestApplicationRegisteredHTTPBoundary：实际生成路由、错误凭据不回退Cookie、请求body不能覆盖应用和外部ID、admin权限、数据库故障503且无内部错误泄漏。
- TestApplicationOryBoundary：实际Oathkeeper v26.2.0、Auth handler、PostgreSQL；Kratos Session/Admin为测试替身。普通成员403、管理员201、限定协议凭证透传与验证、停用后401、未开放路径404均通过。没有运行真实Kratos登录或Keto投影测试。
- race未运行：环境无CGO编译器；数据库并发用例已执行，不能据此宣称Go竞态检测通过。

## 分层与生成

handler只解析协议/转换DTO；application service无其他service依赖；ApplicationUsers协调auth/identity/application；SQL只在dal/repo，持久化模型在dal/model。IDL生成共享DTO、服务路由与OpenAPI；User字段变更后使用隔离PostgreSQL运行GORM Gen。main仅装配，未手改生成路由/DTO/query。

## 上线边界

未部署、未运行生产迁移。0018先加表/字段；Oathkeeper示例host需匹配实际部署域名并采用HTTPS。应用访问凭证当前只有profile用途，仅/apps/v1/me接受，不能调用普通业务API。既有Oathkeeper Cookie链及Auth验证仍需部署正确配置；协议路径的noop仅将认证交回专用Auth handler，不能扩展到/api或/internal。
