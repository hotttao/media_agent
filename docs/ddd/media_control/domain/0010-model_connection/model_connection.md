# Model Connection

个人模型连接是 media_control 的独立聚合，以连接 ID 为根，owner_user_id 不可修改。它保存名称、认证类型 codex_auth_json、加密后的原生 auth.json、active/revoked、乐观版本和时间。仅 owner 可创建、列表、替换认证、重命名及撤销；其他用户以404响应。所有公开响应只包含配置元数据，认证材料只写不回显，不写入元数据 Release、Outbox、审计内容或日志。现有 CredentialReference 仍只代表外部 Secret Manager 引用，不承担此能力。

导入接受至多64KiB JSON对象：原生 ChatGPT tokens 至少有非空 access_token/refresh_token，或非空 OPENAI_API_KEY；其他字段保留原生格式。拒绝空对象、非对象、缺失认证字段、超限及无效 JSON，不回显输入。密文绑定连接 ID 与 owner，解密时校验作用域；使用既有 Control AES-GCM key，数据库不得存原文。

Workspace 当前和历史配置只引用连接 ID，与 API Key 互斥；仅 Codex 可选择 codex_auth_json。新绑定需本人拥有有效连接和 Workspace manage/create 授权。绑定后有权执行该 Workspace 的成员可以经 Agent 使用连接，但无法读取原文或浏览连接库。未提供更新字段保留原选择，空 ID 明确断开；改用 API Key 清空连接。

替换认证递增连接版本，不改变身份、Workspace 选择或 Session 元数据版本；下一次 claim 获取最新有效认证。撤销不撤回已在执行的一轮，但拒绝后续 claim，原文清空，仍保留连接与引用历史。没有跨服务外键。

## 写能力与 Workflow 判定

创建/替换/撤销是单聚合事务。绑定及运行加载跨 Workspace、个人连接和原生文件，见工作流0004-model_connection_binding。

公开连接 version 用于 CAS；私有认证 revision 仅在导入/替换文件时变化。仅重命名不能使 Agent 覆盖已由原生 Codex 刷新的认证。
