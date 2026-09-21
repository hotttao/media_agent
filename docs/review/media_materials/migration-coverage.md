# media_ai → media_materials 领域迁移覆盖审查

本审查以旧仓库 `/home/tao/code/github/media_ai` 的 Prisma 模型、`domains/materials`、`domains/virtual-ip`、`domains/product`、`domains/movement-material` 与对应 API 路由为事实来源。范围只包含 Materials、Virtual IP、Products；`covered` 表示目标 DDD 已有明确归属，不表示代码已实现。

| 原能力 | 目标限界上下文 | Domain / 聚合或读模型 | Workflow | 状态 |
|:---|:---|:---|:---|:---|
| Material CRUD、搜索、类型/标签过滤 | `media_materials` | `asset` / Material | -（单聚合） | covered |
| Scene、Pose、Makeup、Accessory、Clip Template、Music、Cover 等素材类型 | `media_materials` | `asset_kind` + `asset` / Material | - | covered |
| Material 图片 AI 识别并回填草稿 | `media_gen` | 图像理解能力；结果只作为 Material 创建草稿 | 生成任务执行 | external-owner |
| IP 专属妆容、配饰、定制服装、多视图及派生链 | 本轮不迁移 | 旧 `IpMaterial` 保留为未来输入 | - | deprecated-with-reason：用户明确 IP 专属素材暂不考虑，不并入 Material 或 Asset Binding |
| Virtual IP / Product 场景集合 GET/PUT 整组替换 | 后续 Materials 阶段 | Asset Binding 候选模型 | 未纳入本轮 | deprecated-with-reason：第一阶段明确不实现 Binding |
| Movement Material 的文本/文件载荷、通用/专用、服装适用范围 | `media_materials` | `asset` / Material（kind=movement） | -（单聚合） | covered |
| Pose 与 Movement 兼容关系 | 后续 Materials 阶段 | Material Compatibility 候选模型 | 未纳入本轮 | deprecated-with-reason：第一阶段只实现三个核心聚合 CRUD |
| Virtual IP CRUD、列表和身份图片 | `media_materials` | `virtual_ip` / Virtual IP | -（单聚合） | covered |
| Virtual IP 年龄、体型、教育、职业、性格、爱好、口头禅等完整人设 | `media_materials` | `virtual_ip` / Persona Profile 值对象 | - | covered |
| Virtual IP 资源级 owner/visibility/team grant | `media_materials` + `media_auth` | `virtual_ip` + Team Membership 查询 | CRUD 内授权；Workflow 未纳入本轮 | covered |
| Virtual IP 专属 Workflow Template、Pose/Scene/Movement 编排 | `media_workflow` | Workflow/Template | 工作流定义与执行 | external-owner |
| Product CRUD、受众、详情、展示动作、标签 | `media_materials` | `product` / Product | -（单聚合） | covered |
| Product 主图/详情图有序集合及单图增删 | `media_materials` | `product` / Product Image | -（聚合内事务） | covered |
| Product 外部电商标识、标题、分类、价格、平台作品 ID | `media_materials` | `product` / External Commerce Reference | - | covered |
| Product 图片 AI 识别并回填草稿 | `media_gen` | 图像理解能力；结果只作为 Product 创建草稿 | 生成任务执行 | external-owner |
| Product 模特图、风格图、首帧、视频生成与候选确认 | `media_workflow` + `media_gen` | Candidate/Job；最终产物回写 Generated Asset | 工作流执行与人工审批 | external-owner |
| Product 生成素材和视频历史查询 | `media_workflow` + `media_materials` | 后续 Generated Asset 读模型 | 未纳入本轮 | deprecated-with-reason：第一阶段不实现生成与 Workflow |
| 按 Virtual IP 搜索带发布统计的 Product | 后续发布上下文 | 发布计划/发布结果读模型 | 未纳入本轮 | deprecated-with-reason：本轮明确不迁移发布能力，Product 本身仍可按名称/标签查询 |
| `/api/ips` 与 `/api/virtual-ips`、`/api/movements` 与 `/api/movement-materials` 重复入口 | `media_materials` | 统一 API 契约 | - | deprecated-with-reason：同义重复路由不保留 |
| 删除 Product 时级联删除生成图并解绑视频 | `media_materials` | 第一阶段只软删 Product | 未纳入本轮 | deprecated-with-reason：不迁移旧级联删除；本期没有 Binding/生成历史写入，Storage 对象也不随 CRUD 删除 |

## 发现与结论

- 覆盖矩阵没有 `missing` 项；Binding、兼容关系、生成和发布能力按第一阶段边界明确交给外部所有者或有理由暂缓。
- 旧系统的 `IpMaterial` 有独立多视图与派生链语义，不能误折叠成 `material_bindings`；但用户已明确本轮暂不考虑，因此目标 DDD 不引入该聚合。
- 旧系统 Movement Material 的字段 `scope` 与当前业务 Scope 同名异义；目标统一改为 `applicable_clothing`，迁移不得原名直拷。
- Product 延续已确认的新规则：没有 owner/visibility/grants；但所有读写先校验 Organization Membership，资源只在同 Organization、同 Scope 内共享。
- 旧系统保存完整 URL；目标只保存 Storage `object_path`。迁移阶段必须先完成 URL 到逻辑对象路径的转换，不能把域名、bucket 或签名 URL 写入业务库。
- 当前代码和初始迁移尚未完全实现本次 DDD（尤其 `organization_id`、Product Image、Persona Profile 和兼容关系）；本文件只证明需求归属完整，不代表实现完成。

## Harness 实现落点

| DDD 构件 | Harness 落点 | 约束 |
|:---|:---|:---|
| Material、Virtual IP、Product 聚合与 Policy | `media_materials/biz/domain/<domain>` | 第一阶段只实现三个聚合 CRUD；Domain 之间不得直接依赖 |
| Reachability 及其他 Workflow | 后续阶段 | 第一阶段不得加入 Workflow 入口、后台任务或编排逻辑 |
| HTTP/RPC DTO 与认证上下文提取 | `media_materials/biz/service` + `idl/media_materials` | Service 只适配协议；user/organization 不得从 body 信任 |
| PostgreSQL Repository | `media_materials/infra` | 只实现 Domain Port；业务唯一键全部包含 Organization 边界 |
| Storage Object 上传/签名 URL | `media_storage` 适配器或独立服务 | Materials 只保存 `object_path`，不把 Provider/域名带入 Domain |

本轮未进入实现阶段；IDL、Migration、Domain、Service 与 e2e 计划必须在开始迁移代码前依据上述落点另行固化，第一阶段计划不得包含 Workflow 或 Binding。
