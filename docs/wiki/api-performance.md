# API 性能优化记录（2026-10-10）

已有功能提交：`a7fc7a7`。用户的 `ui_workflow/DESIGN.md` 不包含在提交中。本轮不修改 UI、不执行生成/历史恢复，不开启观测后端，不改 Conductor 编排或权限规则。

## 测量方式

`harness/tools/api_latency.mjs` 从忽略文件读取现有 User API Token 或浏览器 Cookie，经 LAN 统一网关只读请求。每项顺序请求 3 次，记录完整响应耗时中位数、最大值、状态、字节和记录数；不输出凭据、响应正文或签名 URL。任何非 200 使测量失败，401 等错误响应不能计入优化结果。

```bash
# 仓库根目录，已有私有 E2E 凭据/浏览器会话
PERF_ROUNDS=3 node harness/tools/api_latency.mjs
PERF_AUTH_MODE=cookie PERF_ROUNDS=3 node harness/tools/api_latency.mjs
# 单独复测热点
PERF_CASES=products.page200,generated_assets.page200 node harness/tools/api_latency.mjs
```

固定样本为已有成功四节点 Job 和已有 owner Workflow。工具不会提交、审批、重试、恢复 Job，也不会启动 Worker。三次样本不是生产 P95 或吞吐测试；宿主机运行多个外部服务，后半轮 load average 超过 190，CPU PSI some 超过 70%，Cookie 小页耗时反而变差，不能承诺所有接口同比例提速。

## 第一轮 Materials 部署对比

完整响应耗时中位数（毫秒），相同账号/入口/过滤器/数据量，每组 3 次且全为 200：

| 接口 | Token 前 | Token 后 | Cookie 前 | Cookie 后 |
|---|---:|---:|---:|---:|
| 商品 20 条 | 658 | 519 | 941 | 1961 |
| 商品 page_size=200（实际 99 条） | 1410 | 410 | 1994 | 1669 |
| 生成素材 20 条 | 917 | 827 | 1427 | 1946 |
| 生成素材 200 条 | 8844 | 3751 | 10162 | 4056 |

商品 99 条响应仍为 133336 字节；生成素材 200 条仍为 234331 字节。不是通过裁剪图片、删除字段或少返回记录提速。网关回归分别验证 20/200 条页前缀、总数、图片顺序、存储 namespace、未认证拒绝及有企业过滤契约的跨企业拒绝。

GeneratedAsset 公开 DTO 没有 organization_id，其列表 IDL 也不接受该过滤器，读取范围由已验证 principal 与实时企业成员事实决定。测试遵循这个现有契约，不能把忽略额外 query 参数的 200 当作权限绕过或新增过滤器。

## 实现

1. Product Repository：商品页一次批量读取对应图片，按 product_id 归属、organization_id 二次核对和 sort_order 组装；固定 Count + Products + Images 三次查询。Get 共用加载器，空页不查图片。
2. GeneratedAsset List：最多 8 个存储解析并行，保持列表顺序；失败/取消返回错误、不返回部分成功。原生对象仍确认存在；历史对象使用 S3 `PresignVerifiedGet` 在同一次 HEAD 核对摘要和正大小后签名。没有该端口的适配器仍走原校验路径。没有存储 URL 长期缓存或跳过完整性检查。
3. JWT Verifier：配置了 issuer 时，在 key callback 提前拒绝其他 issuer，避免每次普通用户请求先访问 Agent JWKS。只是拒绝，最终仍验证 RS256 签名、issuer、audience、时间和身份。兼容 `WithIssuer("")` 的旧行为；Materials Compose 明确填写已有 Oathkeeper issuer。初次部署的空配置回归导致 401，修正并补充测试后恢复，不纳入速度结果。
4. ResourceAuthority：独立企业/部门/Workspace 查询重叠，部门授权最大并行 4，最后按原顺序取交集。请求外没有会员或授权缓存；权威故障失败关闭，Agent 企业/Workspace 上限与撤销检查不变。
5. Job：Get 首次只读 Job，不读取随后丢弃的 StepRuns；已有成功产物的 render/invoke 不重复执行本来不会写入的锁/读事务。相同 Job status/reason 不重复写 updated_at，减少 WAL 和阅读造成的列表重排。每次仍读 Conductor，包括终态；审批、完成产物、取消、generation/retry epoch 的事务保护不移除。没有执行快照长期缓存或自建重试语义。

## 回归证据与未完成项

已通过：

- Materials 与 Workflow `go test ./...`；Materials 生成素材/存储、Workflow Job/Authority client、共享 JWT 的 `-race` 测试；两个服务 `arch-check`。
- 隔离 PostgreSQL `TestProductPageImageBatchQueries`：1/20/200 页固定 3 次查询，空页 2 次，跨企业图片隔离与排序。
- 存储并行有界/取消/失败，单次 HEAD 校验与摘要冲突不签名，Service 校验失败不降级。
- JWT 外部 issuer 0 JWKS 调用、伪造匹配 issuer 的签名拒绝、错误 audience 拒绝及空 issuer 配置兼容。
- `media-cli/e2e/TestAPIPerformanceReadGateway`：真实网关只读四类列表、分页与权限回归。

首轮受阻记录（历史记录，后续状态见下一节）：

- 最后 Workflow 构建卡在 managed-worker 构建步骤，最新镜像未部署；已向本轮构建命令发送停止信号，其 Docker 清理也可能等待内核阻塞解除。现有 Workflow 常驻服务保留原版本，不替换到无法启动的容器。
- `TestExecutionProjectionAtomicPersistence` 在 Docker Ryuk 的 ContainerStart 阶段 3 分钟超时，尚未创建测试数据库/执行断言。先前测试通过不代表本轮事务门禁通过。
- 最新补充的成功节点投影事务预算测试进程在内核 `netlink_dump_start` D 状态等待，不能将其算为通过。
- Workflow/Template/Job 部署后性能、完成结果 HTTP/CLI 读取复验待宿主机恢复。未修改宿主机内核、重启 Docker/k3s 或停止外部服务。

现有部署前 Token 基线：auth.me 386、workspaces 367、普通素材 20/200 条 175/276、数字人 284、模板 791、Workflow 1231、Capability 477、Job 20/200 条 9583/11894、成功 Job 详情 3704、Workflow 详情 935 毫秒。没有将这些未部署的接口宣称已提速。

首轮记录的续跑命令：

```bash
INFRA=unit MEDIA_WORKFLOW_MIGRATION_TOKEN= docker compose -f media_workflow/docker-compose.yml build media_workflow
INFRA=unit MEDIA_WORKFLOW_MIGRATION_TOKEN= docker compose -f media_workflow/docker-compose.yml up -d --no-deps media_workflow
go -C media_workflow test ./biz/service/job ./biz/shared/client -race -count=1
go -C media_workflow test -tags=e2e ./e2e -run '^TestExecutionProjectionAtomicPersistence$' -race -count=1 -v -timeout=3m
```

再运行上述 Token/Cookie 全部测量。通过现有私有环境加载 `MEDIA_API_TOKEN`、LAN `MEDIA_GATEWAY_ADDR`、企业及独立 CLI binary，执行 `TestWorkflowUIQuerySupportGatewayAndCLI` 与 `TestWorkflowCompletedOutputsGateway`（使用已有完整成功样本）。不运行 Submit/Retry/Resume 门禁，不动原 119 条 awaiting_resume 记录。

## 后续部署与全部读接口复测

宿主机网络等待短暂解除后，Workflow 已构建并部署。API 容器镜像为 `sha256:14c61749594edabfd3d7f19f341306c3cf160411f67ce39530125e6aeb8b60b5`，迁移入口保持关闭。没有更新或启动模型 Worker，没有重启外部 Docker/k3s/Provider 服务。

同一只读工具、入口、账号、过滤器及每项 3 次测量，全部响应为 200；以下是完整本轮结果，不按最优样本挑选。

| 接口 | Token 优化前 ms | Token 本轮 ms | Cookie 优化前 ms | Cookie 本轮 ms |
|---|---:|---:|---:|---:|
| auth.me | 386 | 426 | 725 | 760 |
| Workspace 列表 | 367 | 362 | 628 | 470 |
| 普通素材 20 条 | 175 | 456 | 862 | 936 |
| 普通素材 page_size=200（实际 16 条） | 276 | 335 | 595 | 590 |
| 商品 20 条 | 658 | 501 | 941 | 514 |
| 商品 page_size=200（实际 99 条） | 1410 | 507 | 1994 | 606 |
| 数字人（实际 1 条） | 284 | 239 | 696 | 596 |
| 生成素材 20 条 | 917 | 461 | 1427 | 884 |
| 生成素材 200 条 | 8844 | 2382 | 10162 | 2846 |
| 模板 20 条 | 791 | 1223 | 1237 | 1121 |
| Workflow 20 条 | 1231 | 600 | 1173 | 733 |
| Capability（实际 4 条） | 477 | 309 | 477 | 350 |
| Job 20 条 | 9583 | 4365 | 9641 | 3470 |
| Job 200 条 | 11894 | 4625 | 15159 | 5611 |
| 完整成功 Job 详情 | 3704 | 1708 | 2828 | 1490 |
| Workflow 详情 | 935 | 455 | 707 | 747 |

商品 99 条、生成素材 200 条及完整 Job 详情分别保持 133336、234331、21661 字节。请求数/记录数不减少，不移除图片/候选/结果字段。模板 Token、普通素材小页等没有测得改善；宿主机负载不稳定，本次不能得出“所有接口都变快”或生产 P95 的结论。

部署后通过：Materials 真实网关只读分页/读取边界、Workflow query support 的 HTTP 与独立 CLI、原完整四节点 Job 四个产物无 Token 的 Range 206（3 张 PNG + 1 个 MP4）、独立 CLI Job get（4 个成功节点/产物）、Workflow Job/Authority client 竞态测试（含成功节点事务预算）以及共享 JWT/隔离数据库工具安全测试、两个服务架构检查。

只读队列审计仍为 271 条：145 succeeded、7 cancelled、119 awaiting_resume、0 active。原样本未重跑/取消/重新审批，历史记录未恢复。

隔离事务复验发现旧测试仍要求 AcceptReview 立即把 StepRun 改为 claimed，与仓储已有“只存决定，由 Conductor 推进”契约不符；只修订测试，不修改生产执行语义。增加 pending receipt、原状态/候选/提示词/选择不变、旧产物不能重挂的断言。新 Docker 容器再次启动超时，所以提供显式的 PostgreSQL 专用临时数据库测试路径（默认仍是 Testcontainers）；该路径创建随机新库、只迁移新库、关闭后删除确切新库，不修改任何业务库。最终 SQL 门禁结果另行补记。

### 维护前未完成的验证（历史记录）

修订断言后的 Workflow Testcontainers 复跑及商品 `-race` 数据库复跑都在 ContainerStart 阶段超时，未执行业务断言。备用 `MEDIA_E2E_POSTGRES_ADMIN_DSN` 路径的测试进程也在主测试开始前处于 `netlink_dump_start` D 等待；调用进程 240 秒超时，输出没有 `=== RUN`，不能计为数据库回归通过。备用工具的 URL/前缀/数据库覆盖防护单测与架构检查通过，但其实际建库/迁移/清理流程尚未验收。

原非 `-race` 商品 PostgreSQL 查询预算测试已通过；最终 Workflow 事务门禁与备用路径仍需宿主机恢复后补验。停止重复启动测试，不自行重启 Docker/k3s、停止外部服务或修改内核设置。部署后功能与耗时结果保留，不宣称整体 SQL 验收完成。

通过独立 psql 只读审计确认 `e2e_workflow_<32hex>` / `e2e_materials_<32hex>` 临时数据库数量为 0。回收时发现本轮孤儿测试进程已退出，内核等待再次解除；其失去调用进程后的结果不能作为验收记录。

解除后仅重试一次专用路径：出现 `=== RUN TestExecutionProjectionAtomicPersistence`，但调用仍在 150 秒超时且没有 PASS。后续目录审计发现已创建 1 个临时数据库，因此不能把最后一次超时描述为“完全未开始建库”；仍无法确认迁移或业务断言完成，也不能只用环境问题排除测试失败。停止继续重跑，最终 SQL 门禁仍未完成，需要稳定宿主机后保留完整阶段诊断补验。

该临时库按随机名称、owner 与 0 活跃连接核对后，删除确切本轮测试库并再次审计数量为 0。只删除可由夹具重建的测试数据，没有修改业务库或原历史记录。

## 宿主机维护与最终验收

用户明确授权后取得 sudo 权限，只读核对 Docker/k3s 状态、持久数据挂载与内核日志。日志明确记录多个 Go/Node/Exporter 进程在 `__netlink_dump_start` 等待互斥锁超过 120 秒，IPv6 `addrconf_verify_work` 同时等待；这是宿主机网络等待，不是单个测试进程的业务错误。

k3s 管理的旧 `ddd-learn` 系列工作负载存在数百到数千重启计数，并有近期 BackOff/Unhealthy 事件。当前仓库服务、认证、Conductor、对象存储及 Penpot 属于独立 Docker 容器；数据库使用已确认的 bind/named volumes。

最小维护动作是临时 `systemctl stop k3s` 后恢复 `systemctl start k3s`，未删除 Kubernetes 对象、改变副本数、执行 killall/uninstall、重启 Docker、停止其容器或修改用户/内核限制。k3s 停止期间，接口枚举与容器启动恢复，数据库门禁取得实际 PASS：

| 门禁 | 专用 PostgreSQL 临时库 | 默认 Testcontainers |
|---|---:|---:|
| Workflow 事务/取消/轮次/审批旧快照保护（`-race`） | PASS，23.45s | PASS，51.74s |
| 商品图片页固定查询预算/企业隔离/排序（`-race`） | PASS，8.64s | PASS，33.31s |

专用库关闭/删除审计为 0；默认测试容器由测试清理。上述恢复只说明停止 k3s 期间阻塞缓解，不能证明确切锁持有者或长期根因已修复，不把旧应用退出原因归咎于 Conductor。

维护后 k3s active/running，节点 Ready=true，Docker active（未重启）。5173 Vite 与 9001 Penpot 返回 200；恢复后的网关三项门禁（素材只读、Workflow query HTTP/独立 CLI、完整四节点产物读取）再次全部通过，四个输出仍为 PNG/PNG/PNG/MP4 的 Range 206。队列仍为 271 条（145 succeeded、7 cancelled、119 awaiting_resume、0 active），迁移入口仍保持关闭。用户 `ui_workflow/DESIGN.md` 未修改。

本轮性能实现、部署与回归已完成。旧 k3s 工作负载的重启风暴和宿主机网络长期稳定性仍需独立治理；没有为了结束接口任务擅改这些应用配置。性能结果使用前文 k3s 原运行状态下的同口径测量，未用停止集群后的数字替换基线。
