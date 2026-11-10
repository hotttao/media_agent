
## 应用身份映射
provider 为 app:<application_id> 的映射只能由 ApplicationUsers Workflow 创建，不能使用通用 BindIdentity/Upsert/解绑覆盖或删除。CreateApplicationIdentity 只做不可覆盖写入，User/Identity原子性由工作流事务保证；provider+external_id与user+provider双唯一约束继续生效。映射保留至历史审计，不因User软删除释放。
