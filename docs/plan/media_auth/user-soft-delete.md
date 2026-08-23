# media_auth 用户软删除与访问撤销落地计划

- [x] 核对 CONTEXT、Domain、Workflow 与 Harness/UI 规范。
- [x] 增加用户状态、认证版本与撤销 Outbox migration。
- [x] 实现软删除事务、最后 active admin 保护与 Outbox worker。
- [x] 使 Cookie/OAuth JWT 和 Service Token 校验服从用户状态与认证版本。
- [x] 更新 IDL、OpenAPI、CLI 与管理员用户页面。
- [x] 完成单元测试、架构检查、Go HTTP E2E、真实 media-cli 进程 E2E 与 Playwright E2E。
- [x] 将能力覆盖矩阵全部更新为 covered 并独立提交。
