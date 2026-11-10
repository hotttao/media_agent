# 平台应用接入落地计划

依据 [规格](../../spec/media_auth/application.md)。

- [x] P1：词典、Domain、Workflow、规格一致；定义应用与OAuth、Agent身份边界。
- [x] P2：IDL、生成代码、0018 migration和repository；验证并发upsert/回滚/现有User隔离。
- [x] P3：应用管理、独立认证、托管用户、短期凭证与实时撤销；阻止登录/改密/解绑旁路。
- [x] P4：真实数据库+HTTP协议测试、单元测试、go build、arch-check、文档生成。

依赖：P1→P2→P3→P4。主要风险是认证旁路和旧身份自动合并，须用跨应用同ID、同邮箱Kratos、禁用恢复、secret轮换、Bearer混用和Cookie降级测试覆盖。仅使用隔离数据库验证，不部署或改现有数据。


## 验证记录（2026-11-07）

- `umask 022; go -C media_auth test ./...` 通过。默认umask 0002使Go测试临时目录组可写，既有platformaudit安全测试因此失败；未修改安全逻辑，改用正常权限后通过。
- `go -C media_auth run ../harness/tools/archcheck` 通过。
- `go -C media_auth test -tags=e2e ./e2e -run '^TestApplication' -count=1 -v` 覆盖隔离PostgreSQL上的并发唯一映射、跨应用隔离、事务回滚、身份旁路、凭证停用/轮换/恢复/过期、注册HTTP路由。
- Ory边缘验收使用真实Oathkeeper v26.2.0、真实Auth handler及数据库，Kratos Session/Admin API为可控替身；验证admin/member、协议路由隔离与凭证撤销，不宣称覆盖真实SSO登录或Keto投影。
- IDL DTO/路由、User GORM query及OpenAPI均由工具生成；服务没有启用media-auth技能产物，未新增API skill。

后续实施已记录在 [应用平台接入计划](../application-platform-access.md)：专用业务凭证、Agent绑定、会话入口、Caller与SDK；成员开通复用既有组织流程，不自动提权。未部署或执行现有环境的数据迁移。
