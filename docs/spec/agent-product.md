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

| 接口 | 行为 |
|:---|:---|
| POST /api/v1/prompts | organization_id、name、content、visibility、grants 创建企业资源 |
| GET /api/v1/prompts?organization_id=… | 当前用户在该企业可见的有效资源及 my_permission |
| GET/PUT /api/v1/prompts/{id} | 授权读取 / expected_version 更新名称与正文 |
| PUT /api/v1/prompts/{id}/access | owner 使用 expected_version 修改 visibility 与完整 grants |
| GET /api/v1/prompts/{id}/versions[/{version}] | 当前资源可读用户查询历史 |
| POST /api/v1/prompts/{id}/rollback | target_version、expected_version；新版本采用历史正文，名称与 ACL 保持当前值 |
| POST /api/v1/prompts/{id}/archive | owner 使用 expected_version 归档，保留历史 |
| GET /api/v1/prompts/unassigned | 仅原 owner 可见的未归属历史资源 |
| POST /api/v1/prompts/{id}/assign-organization | 原 owner 指定 organization_id、expected_version，一次性归入本人所属有效企业 |

private 不接受部门 Grant；team 至少需要一个 Grant；public 允许额外指定部门 editor，公共读取仍限制在企业内。Grant 数量最多100，部门必须 active 且属于同一 active 企业。资源当前版本同时作为内容/ACL 写入 CAS；历史读取以当前资源 ACL 判断，不恢复历史 ACL。旧历史缺少可信记录时间时返回 created_at=0，不伪造日期。

Agent 配置确认沿用 POST `/api/v1/workspaces/{id}/agent-metadata:confirm`，增加 `prompt={mode: none|custom|library, content, resource_id, resource_version}`。none 不接受正文或引用，custom 只接受正文，library 只接受固定 ID/版本；冲突输入拒绝。旧客户端可继续在 resources 中传单个 Prompt 引用，等价 library；不能与显式 none/custom 混用。

GET `/api/v1/workspaces/{id}/agent-metadata` 返回已确认来源、固定正文/引用与当前配置 CAS version；无已确认配置返回404。浏览器读取库来源正文仍要求资源可读，不能靠 Workspace view 泄露私有 Prompt。资源归档后已有发布保留，不能将归档资源选入新的配置。完整运行响应和消息格式保持兼容，Prompt 方式只进入 Control 的非秘密 Release，不要求修改 Agent 协议。

## 后续切片接口范围

Skill 扩展现有 CRUD 为内容管理与发布 API；MCP 扩展现有 Gateway 目录 API 并实现 Plugin 的绑定及固定配置查询。具体数据契约在对应实现切片前补充，保持以上已确认语义。管理界面使用真实 API，不以编辑内部存储路径作为上传功能，不以模型替身测试作为正式模型可用性证据。

正式使用入口包括企业资源库、Agent 配置（含显式更新）、会话创建、消息发送、流式输出、历史和工具权限确认；Docker 仅负责创建一个可用 Agent。
