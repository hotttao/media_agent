# Agent 可用产品功能规格

本规格补充现有 Agent Runtime：交付可由用户操作的资源管理、配置和正式 Agent 对话链路。隔离、扩缩容、故障接管及 SRE 专项建设延期，已有必要接口和业务权限保留。

## 已确认产品契约

- Prompt、Skill 是企业资源库资源，可跨同企业 Workspace 复用。默认 private，按部门授予 viewer/editor；public 仅企业内读取。owner 管理 ACL，editor 编辑内容，viewer 读取和选用；配置给 Agent 还要求 Workspace manage。
- Prompt 支持创建、编辑、历史读取和回滚。Agent 配置采用不设置、直接填写或引用一份库固定版本三种互斥方式，Platform Prompt 仍独立生成。内容实际变化创建历史，回滚保留历史。
- Skill 支持 ZIP/tar.gz、Git 导入及手动同步、在线文件树和增删改、草稿发布、历史查看和下载。发布版本不可变，草稿不能选给 Agent。
- 平台管理员维护平台 MCP；企业管理员可登记本企业外部 MCP。普通成员按授权选用，Workspace 配置要求 manage，Agent 统一连接 Gateway。目录事实在 Gateway，绑定和具体实现仍归 Plugin。
- 资源编辑、Git 同步或发布不自动升级 Agent。用户明确确认配置后保存不可变 Release 并通知，旧 Session/Turn 固定版本。

## Prompt 资源管理接口

沿用 `/api/v1/prompts`；新增必需企业上下文，旧 `workspace_id` 不再表示资源归属。创建接受 organization_id、name、content、visibility 和部门 grants；owner 从认证主体取得。列表只返回指定企业中可见资源，详情返回 my_permission。

更新正文/名称要求 owner/editor 和 expected_version；ACL 更新要求 owner 和 expected_version。历史列表与指定版本读取需要当前资源可读；回滚需要 owner/editor 和 expected_version，读取历史正文形成新的当前版本。归档要求 owner，保留已有 Release 需要的历史。

Auth 通过既有内部企业成员与部门权限接口提供当前事实；资源服务保存 Grant 并最终判定资源权限，不能仅依赖客户端或 JWT 中的企业、角色声明。企业已停用、非成员、跨企业及部门已停用均不能扩大资源权限；Auth 故障返回服务不可用。

迁移为当前资源增加企业归属及可见性、部门 Grant 表。已有 Workspace 资源仅从其真实 Workspace 回填企业；无法确定企业的历史资源保留数据并明确列为待归属，不猜测或自动公开。不得因删除原 Workspace 级联删除企业资源。

## 后续切片接口范围

Skill 扩展现有 CRUD 为内容管理与发布 API；MCP 扩展现有 Gateway 目录 API 并实现 Plugin 的绑定及固定配置查询。具体数据契约在对应实现切片前补充，保持以上已确认语义。管理界面使用真实 API，不以编辑内部存储路径作为上传功能，不以模型替身测试作为正式模型可用性证据。

正式使用入口包括企业资源库、Agent 配置（含显式更新）、会话创建、消息发送、流式输出、历史和工具权限确认；Docker 仅负责创建一个可用 Agent。
