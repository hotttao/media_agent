# Workflow UI 查询支持规格

## 范围

补齐 Provider Capability 目录、Template/Workflow 当前有效权限与授权列表、Workflow 冻结业务版本。之后接入已有 standalone UI 并联调提交/审批/再生成/失败继续/输入覆盖重跑/历史补缺。排除 media_platform 接入、Agent/Worker 身份扩展、性能优化、历史全量迁移和自建引擎调度。

## 接口

| 接口 | 契约与权限 |
| --- | --- |
| GET /api/v1/capabilities | 已认证用户；仅已发布 Provider 契约；provider/generation_method/generation_type/execution_target 可选；page≥1、page_size 1..200，默认50；items/total/page/page_size |
| GET /api/v1/capabilities/:key/versions/:version | 已认证用户；明确 Key 和正整数版本；未发布/旧业务分类/不存在404 |
| GET /api/v1/templates/:id | 增加 my_permission 和 allowed_actions，只读投影 |
| GET /api/v1/workflows/:id | 同上；保留真实发布回执 |
| GET /api/v1/templates/:id/grants | owner；items 含 team_id/permission/granted_by/granted_at，稳定部门ID排序 |
| GET /api/v1/workflows/:id/grants | 同上 |
| GET /api/v1/workflows/:id/versions/:version | 先按当前头记录验证企业/Workspace/资源ACL，再读取指定不可变定义；未找到404；历史不可写 |

Capability DTO：key/version/provider/generation_method/generation_type/execution_target/inputs/output_media_type/retry_count/retry_logic/retry_delay_seconds/timeout_seconds/status/created_at/published_at。inputs：name/type/required/media_type/min_items/max_items。不得暴露 conductor_task_def、managed_pool_key、Secret、内部JWT。非法页码/筛选400；无身份401；仓储错误503且不泄露数据库细节。

权限投影使用动作名 read/update/archive/publish/change_visibility/grant/revoke，Workflow 另提供 run。owner/editor 的 update 受非归档与实时 authoring 功能控制，publish 仅 owner 且非归档、authoring 通过；archive editor+；change_visibility/grant/revoke 仅 owner。旧无企业记录不允许共享，但允许 owner 收回旧 Grants。run 必须资源可读、定义未归档且实际已注册、Workspace 准入通过；该投影不是提交回执或最终授权。权限权威不可用时查询失败关闭，功能明确拒绝时仅去掉 authoring 动作而保留清理。

授权目标继续走 Auth 现有部门查询及 TeamOrganization 命令校验，不以查询到的 Grant 推导部门仍启用。UI 显示可查询目录范围，不扩展企业 IAM 管理能力。

## 不可变 Workflow 版本

新增 workflow_versions(workflow_id,version,definition_snapshot,created_at)，复合主键、同库 Workflow 引用、写入不可覆盖、非空 down 拒绝数据丢失。Repo Save/Update 在已有调用事务中保存业务快照，更新前保留旧定义，更新后保存新定义；不把动态ACL/发布回执写进冻结定义。现有当前定义从实际表精确回填，旧编译仅恢复能由冻结IR及模板契约证明的字段，无法证实的历史返回明确缺失，不替换为新图。版本读取使用当前资源的动态可见性/归属，不恢复历史授权；发布状态按该版本原回执判定。

版本 DTO 增加 `definition_source`：`snapshot` 是完整保存的业务定义；`compiled` 是已注册 schema v3 IR 与其引用 TemplateVersion 证明的拓扑、绑定、审批/复用策略与入口契约。IR 没有历史展示名称/描述，这些字段保持空值，UI 可另显示当前资源标题但不能当作冻结字段；缺失 TemplateVersion、无法解析或契约矛盾均返回版本缺失，不读当前模板替代。不改变或重新注册原编译。

Handler 仅绑定→Service→DTO；Policy 做纯权限动作推导；DAL 唯一数据库入口。IDL 同时提供 api.* 和 google.api.http，用生成器生成 model/router/OpenAPI，禁止手改生成文件。Capability 新路由加入仓库网关配置，保持统一认证。

## 验证

服务/策略测试：无身份、未发布、旧业务能力隔离、筛选分页、owner/editor/viewer/管理员不提升、功能撤销、权限权威失败、授权列表 owner-only、历史当前ACL/撤销/不可变。真实隔离PG：迁移回填、旧/新快照、事务回滚、冲突/降级保护、旧编译读取证据。公开网关 HTTP 与独立 media-cli 查询命令均验证新接口；Token 从忽略文件注入，不输出。

接口与部署门禁通过后再接 UI：能力选择器用真实 Key/Version，Editor 动作按 allowed_actions，完整授权列表及撤销，执行/恢复/重跑读冻结版本。使用独立测试数据完成真实 User UI + 外部 media_worker mock 客户端一条完整流程，先审计 User 队列，固定 Job 不盲重提；测试不内嵌 Worker，不调用真实厂商，不改借用历史。失败继续和恢复另建明确隔离场景，不能把只读图验收当业务写入完成。

Resume 表单还需读取同一Job的StepRun保留事实，与冻结步骤/模板版本一致、origin为legacy_import、succeeded且有产物的前缀不再要求入口输入。只收集未完成步骤绑定的缺失Workflow Input，不要求已恢复图片阶段的pose/scene；证据缺失禁止提交，后端仍决定补缺合法性。候选缓存按企业、版本和显示输入契约隔离，不能将Resume子集缓存当作普通Submit/Rerun完整契约。

旧兼容Worker返回的候选存储URL可在Job/Step查询响应中重签名：仅识别已配置存储origin，提取并校验该Job的Organization/Scope对象路径，经现有内部解析器生成新URL。读操作不改候选、产物、输入或任务状态，不持久化签名；同origin越界失败关闭，不把外部ProviderURL当作内部存储对象。覆盖过期签名、跨企业/Scope、响应副本不修改原记录、HTTP206及真实浏览器视频加载。
