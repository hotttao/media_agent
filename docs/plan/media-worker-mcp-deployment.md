# 媒体 MCP 部署与验收

实现规格见 [spec](../spec/media-worker-mcp-server.md)。下列为部署步骤；2026-10-29 已在本机环境部署，实际验收状态见文末。

1. 为 Gateway 准备独立 RSA 私钥文件（0600），设置 `MCP_DELEGATION_SIGNING_KEY_FILE` 和稳定 `MCP_DELEGATION_ISSUER`。私钥只交给 Gateway。准备 Cursor signing key ring，配置 `AGENT_RUNTIME_CREDENTIAL_KEYS_FILE`；User 入口不要求启用托管 Runtime。
2. 为 Workflow 与 Materials 分别生成至少 32 字符服务凭据，使用私密环境配置 `MCP_WORKFLOW_SERVICE_TOKEN` / `MCP_MATERIALS_SERVICE_TOKEN`。各凭据仅分发至其目标服务与 MCP Worker，不能互换。
3. 目标服务设置 `MEDIA_MCP_ENABLED=true`、`MCP_DELEGATION_ISSUER`、`MCP_DELEGATION_JWKS_URL=http://<gateway-private>:8006/.well-known/mcp-jwks.json`，以及现有 Auth/Control 内部凭据与地址。先部署目标服务。托管 Agent 使用 Plugin 时，同步部署包含 `restricted_definition_ids` 的 Plugin 内部合同，再部署 Gateway，确保显式停用的固定来源不会被自动加入。
4. 编译 `go build -o media_worker_mcp ./cmd/media_worker_mcp`。使用 `media_worker/conf/mcp.yaml`，配置同一 issuer/JWKS URL、目标独立服务凭据、Workflow/Materials 内网地址。默认仅监听 `127.0.0.1:8015`；跨主机时按部署网络配置地址及防火墙。该进程不运行浏览器。
5. Gateway 设置 `AGENT_MCP_ENABLED=true`、`MEDIA_MCP_ENABLED=true`、现有 Auth/Control 地址与凭据、Edge issuer/JWKS，及精确的 `AGENT_MCP_ALLOWED_UPSTREAM_HOSTS`（含 MCP Worker 的 host:port）。允许浏览器来源时设置 `MEDIA_MCP_ALLOWED_ORIGINS`；普通 MCP 客户端无需 Origin。
6. 超级管理员通过既有 MCP Definition 管理 API 登记以下平台来源。初次 POST `/api/v1/mcp-definitions`，更新沿既有版本冲突合同进行；不得写数据库跳过授权或覆盖管理员已停用的来源。

```json
{"definition_id":"media-workflow","organization_id":"","service_name":"media-workflow","endpoint":"http://<worker-private>:8015/mcp","transport":"streamable_http","authentication_type":"session_context","status":"enabled","expected_version":0}
```

7. 部署带 Token 入口的 UI Platform。登录后打开个人 Settings → API Token（`/settings/tokens`），申请显式 `mcp.access`，保存一次性 Secret。Token 不赋予额外业务权限。超级管理员仍须为该 User 开放 Workflow 模块使用权限。
8. 最后部署 Edge `docker-compose/auth/oathkeeper/rules.yaml` 与 `docker-compose/traefik/dynamic.yml` 中 `/mcp` 规则。Ory introspection 缓存保持关闭。外部客户端访问 `http://mini-001.local:18080/mcp`，携带 `Authorization: Bearer <User Token>`、`X-Media-Organization-ID`，创建 Workflow/提交 Job 另需 `X-Media-Workspace-ID`。原始 User Token 只到 Edge。当前 Traefik file provider 配置为 watch=false，修改动态路由后须重启 Traefik 容器加载；仅写文件不会生效。Linux Docker 使用 host.docker.internal 时，MCP Worker 需配置 host-gateway 映射（随 docker-compose.mcp.yml 提供）。

标准 `tools/list` 应显示 `media-workflow__` 前缀的 13 个批量工具。用模板/工作流的创建、验证及明确发布工具准备已注册版本，再以各项独立幂等键提交 `media-workflow__jobs_submit`。成功项返回 Job ID；之后用 `media-workflow__jobs_get` 批量查询，NEED_CONFIRM 仍在既有页面处理。

真实验收先确认生成 Worker 与已登录账号在线，再分别提交一个 GPT/豆包任务，记录 ID、状态、产物和重试次数。不能将本地 fake API 协议测试计作真实生成成功。撤销 User 模块权限后，不应连接该来源；旧工具名必须被拒绝。撤销 Token 后，Edge 下次 introspection 必须拒绝。

回滚时先停用公网规则/平台来源，再停止 MCP Worker；已提交 Job 继续由独立生成 Worker 执行，不删除 Job 或重置生成检查点。

## 2026-10-29 本机部署与验收记录

- 公网 MCP：`http://mini-001.local:18080/mcp`；个人 Token 页面：`http://mini-001.local:18080/agent/settings/tokens`。Auth、Workflow、Materials、Gateway、独立 MCP Worker 和 UI 已部署；来源 `media-workflow` 已登记。浏览器生成进程与 MCP Server 独立运行。
- 使用业务用户 `1556824234@qq.com`，企业 `8afeb26b9df643952f4f816576eb2504`，Workspace `2e6c5e63-b343-47c4-9f4e-64eb3c7855ce`；保持其全局 user 角色。经用户明确批准，将企业功能从 workflow/workspace 增加 materials，通过组织应用服务写入并保留授权投影事件。
- 通过正常 Kratos 登录和 Token 业务 API 签发显式 mcp.access 的 User Token；Secret 仅存本机私密目录，不写文档。客户端配置为 `/home/tao/.local/state/media-mcp-acceptance/mcp-connection.json`（含凭据，不得提交）。主 Token 到期 `2026-11-28T00:39:43Z`。主 Token 通过正常业务 API 签发；随后另用隔离浏览器完成页面创建、关闭明文、刷新与撤销验收。页面签发的临时 Token 在撤销前 MCP 初始化 200，撤销后 401；主 Token 不受影响。
- 真实公网标准初始化、发现 13 工具、能力查询、模板创建/发布、工作流验证/创建/发布、批量提交与批量查询已通过。首次两项提交耗时 0.272 秒：GPT 返回 Job ID，豆包因 materials 未授权返回 not_dispatched，已成功项保留。开启 materials 后仅重试豆包项。
- GPT Job `job_d8d98014-7d29-439f-b05d-3687f80480dc` 已真实生成成功，资产 `gen_18c48b9c-0eaa-49f3-97be-6c38c75fb4ed`；PNG 1254×1254、1,769,630 bytes，SHA256 `59b981ef5494f9ab6f5dd992cc6b6d120ca14e4e47a25eac5dda0f482f244e38`。已下载并视觉核验。授权后 MCP jobs_get 实时返回 artifact_access=available，无须重启或重新生成。
- 豆包 Job `job_b22b7a36-80fa-465a-a47b-69ce5fae87fe` 已提交；首次 Studio 执行在上传首帧阶段停止，日志确认未进入平台发送。通过原 Job 的正式重试流程恢复后已生成成功；素材 `gen_eb5d0b26-941e-4d42-8499-7a56b28df1cc`。MCP 返回可下载 MP4，已下载并核对 SHA256 `3226f16db4e4c6528d4fd52dc5ed0b3c97698f11346e0af6d1f14b1be0fd5ee2`、620,837 bytes；ffprobe 为 H.264/AAC、720×1280、5.056009 秒。视频首帧已视觉核验。原生 StepRun 重试计数为 1（GPT 为 0）；Studio 两个执行记录分别为发送前失败、成功生成 1 个产物，平台提交记录仅 1 次。未修改 Studio 代码或清理原检查点。临时本机诊断端口已关闭，恢复常规受控实例。
- 公网负向检查：无 Token 401、Worker Token 403、非信任 Origin 403、伪造企业 401、无 Workspace 可读企业模板但拒绝提交 Job。临时测试 Token 撤销前初始化 200，撤销后下一次请求 401；主 Token 保留。
- 修复并复测上游协议拒绝分类：仅保留已知参数/权限错误码，丢弃远端文本与 data；缺少 Workspace 明确拒绝，网络异常仍保留 outcome_unknown，禁止自动重试。
- 本次恢复的独立生成 Worker 使用 24 小时凭据（至 `2026-10-30T12:49:41Z`），长期运行需续签；不影响主 MCP Token 的 30 天期限。原有检查点与无关任务保留。

Token 页面补充：真实 HTTP 浏览器验收发现 randomUUID 在非安全上下文不可用导致空白，现改用 getRandomValues 生成签发尝试标识；剪贴板不可用时显示手动复制提示。修复后真实浏览器创建返回 201，默认仅 mcp.access，关闭/刷新后明文消失，本地存储无 Secret，页面撤销返回 200，MCP 从 200 变为 401。已部署修复，组件回归 4 tests、TypeScript、构建、依赖检查通过。

## 2026-10-30 多图与 Materials 查询扩展

真实 Worker 已支持 doubao.i2v.multi_image_prompt@1，MCP 可用能力由三项增为四项；双图真实验收及修复过程见 [多图计划](media-worker-doubao.md)。工具数不因新增生成能力改变。

新增平台来源 media-materials，复用私有 http://media-worker-mcp:8015/mcp，Gateway 以独立 materials 模块授权过滤并只签发 Materials 受众。来源已通过原 Catalog 应用服务注册；目标 Materials、MCP Worker、Gateway 已部署更新。公网标准 tools/list 共 19 项：media-workflow 13 项与 media-materials 6 项。没有素材权限时不连接素材来源；没有 Workflow 权限时仍可用获准的素材来源。固定 Binding 限制和调用时重新授权仍生效。

新增只读批量工具 virtual_ips_list/get、materials_list/get、generated_assets_list/get。正常真实 User Token 经公网测试：虚拟形象列表 1 项、原始素材列表 16 项、生成素材列表 489 项（本次数据快照），三类列表与各取一个已授权对象的详情均成功。没有创建测试资源，也没有扩大用户角色或企业功能。各列表 total 仍由原业务 API 的授权过滤和分页产生，不自行拼接跨企业结果。

本轮通过：Worker 全量 Go tests、Gateway workflow、Materials admission/policy、真实 SDK 双来源工具隔离与 Materials-only 委托、撤权拒绝旧工具名、目标只读路由白名单、定向 race/vet、Worker/Materials 架构检查。既有 Gateway 跨层架构问题仍保留在原记录，不宣称所有项目门禁通过。CLI 已打开的会话需重新连接 MCP 才能刷新工具目录。

补充：公网无 Workspace 的 User 调用 media-materials__virtual_ips_list 已验证成功（200），查询只需组织与素材权限。生成 Worker 当前正常运行；临时 CDP 诊断端口已关闭，Studio ready/rendererReady 均为 true。
