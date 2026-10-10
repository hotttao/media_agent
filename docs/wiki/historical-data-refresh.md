# 历史数据重新导入与测试业务数据清理

## 范围与安全边界

2026-10-10，按用户要求先提交全部工作区代码，再备份数据库、清理测试业务记录并从 `media_ai` 重新导入。

目标企业为 `8afeb26b9df643952f4f816576eb2504`，目标用户为 `f9b8a1a985735c62596e5f1fbf640ea3`，保留目标 Workspace `2e6c5e63-b343-47c4-9f4e-64eb3c7855ce`。保留可用 Provider Workflow `wf_2c1231e5-7379-4207-a0de-a9bb5d93b278@1` 及其四个模板，历史执行重新关联该定义；导入不启动 Conductor、不运行模型、不伪造原生历史。

删除的是明确核对的测试业务夹具、旧的历史执行副本及被当前 Provider 定义替代的旧演示定义。登录用户、企业、必要部门、User API Token、安全审计和系统资源类型保留。其他项目的 Workspace、Penpot、MySQL 源记录均不清空。共享媒体对象文件保留，不执行存储目录清空或未经引用分析的对象 GC；恢复数据库仍可访问原对象。

历史图片链按来源事实恢复：没有下游视频的链重新成为 `awaiting_resume`，此前 fixture 续跑生成的视频不带入新的历史事实。VideoPush 是独立历史发布记录，不编入四节点生成 Workflow。来源断链记录只列为排除，不迁入目标，也不修改旧库。

## 备份

私有备份目录：`/home/tao/backups/media_agent/snapshot-20261010T052425Z-Zw6Pco`，目录 0700，文件 0600，不在 Git 内。

- 业务 PostgreSQL：10 个非模板数据库，每库 `pg_dump -Fc`，另存角色备份。
- Ory PostgreSQL：hydra、keto、kratos、postgres，每库同样备份，另存角色备份。
- Conductor、Talos：SQLite online backup API，包含已提交的 WAL 数据；不能用单独复制 `.db` 替代。
- 全部 PostgreSQL 归档已通过 `pg_restore --list`；Workflow、Materials、Kratos 另恢复到三个新建隔离数据库并成功校验，验证后只删除自己的临时数据库。
- 两份 SQLite 备份 `PRAGMA integrity_check=ok`，SHA-256 清单保留。

PostgreSQL 是分别在线获取的数据库一致性快照，不是跨 PostgreSQL/SQLite 的全局事务快照。服务停写后，清理范围再次复核；本次没有修改认证数据。Talos 作为补充备份单独保存摘要清单，原先的 Conductor 备份没有覆盖。

备份内含身份和凭据元数据，必须按敏感数据保管。回滚会恢复旧 ID 和清理前的测试数据；不得不经新数据核对直接覆盖运行中的数据库。

## 清理实绩

| 类别 | 删除数量 |
| --- | ---: |
| 旧 Job（历史副本和测试执行） | 294 |
| 测试/旧版 Workflow | 40 |
| 相关测试/旧版 Template | 63 |
| 旧历史发布记录 | 52 |
| 旧 Generated Asset 登记 | 579 |
| 测试 Material / Product / Virtual IP | 47 / 5 / 37 |
| 本企业的权限/Resume 测试 Workspace | 5 |
| 关联业务测试的 Conductor 实例 | 34 |
| 已删除 Workflow 的 Conductor Definition | 25 |
| 独立 E2E 遗留 Conductor 实例 | 109 |
| 无引用的旧 fixture Capability / TaskDefinition | 7 / 7 |

Workflow、Materials 与内部 Worker 短暂停写。删除使用已落盘的精确 ID 集合和引用依赖顺序，不使用 TRUNCATE/CASCADE 清空整个库。冻结审计/版本/登记回执的删除保护只在维护事务内临时关闭，完成前重新开启；失败时事务回滚会恢复保护。它不是正常业务 API 的绕过机制。

Conductor 实例使用原生 `remove` / `terminate-remove`，归档参数为 false；定义使用原生 Metadata 删除 API。独立 E2E 实例通过仓库已有测试命名契约识别，清单先写入私有备份。保留真实 Provider Workflow、内部 render/finalize/restore 任务和被保留定义引用的 TaskDefinition。

`cleanup-applied-plan.json`、`removed-engine-instances.json`、`removed-engine-definitions.json`、`removed-isolated-engine-tests.json` 与相关私有日志记录具体 ID，不在公开文档展示业务输入或签名 URL。

## 重导与验收过程

1. 只读源审计：8 个普通素材、8 个 movement、1 个 Virtual IP、99 个 Product；历史候选含 121 个未完成图片组、125 个完整 Video、53 个 VideoPush，另有 50 条断链排除项。
2. 普通素材迁移工具通过统一网关更新全部 116 个来源实体；图片/视频走现有上传 API，稳定上传幂等键允许复用存储对象。产品、形象等来源映射 ID 保持可用。
3. 在进程环境中生成本轮临时迁移密钥，短暂启用 Materials/Workflow 私有导入入口，历史导入通过专用 API 完成，不直接 SQL 创建新 Job/Asset。
4. 逐组同步落盘 `history-reimport.jsonl`，记录来源 ID、目标 ID、Object Path 与摘要。失败也落盘，不把错误伪装成成功。
5. 导入结束或失败后均清空迁移环境变量、重建两个 API 服务并检查 `/health`。本次初次健康检查误用未注册的 `/ping` 返回 404，自动关闭入口后改用 `/health` 重试，相关日志保留。
6. 使用 `--verify-report` 做来源集合、目标 API 和 538 个媒体对象的只读对账；再运行网关/独立 CLI 门禁。不得重新提交或 Resume 任务来证明导入。

唯一已知来源缺失：VideoPush `d8ef3ac8-32c9-414b-bc5d-1c1eeadca214` 的成片 HTTP 404。其成员 Video 可以正常导入；该发布记录隔离，不能伪造媒体或声称全部来源都完整迁入。

### 目标数据库与公开读取门禁

| 目标事实 | 数量 |
| --- | ---: |
| Product / Virtual IP | 99 / 1 |
| Material（pose / scene / movement） | 3 / 5 / 8 |
| Generated Asset（全部 legacy_import） | 484 |
| Job succeeded / awaiting_resume | 125 / 121 |
| HistoricalVideoPush | 52 |
| 保留 Workflow / Template / Provider Capability | 1 / 4 / 4 |
| native Job / 原生生成回执 / Conductor 执行实例 | 0 / 0 / 0 |

111 个素材库媒体路径由公开 API 解析后，直接向存储请求 Range；全部返回 206 且包含 SHA-256 元数据，未把 API Token 带到存储请求。Virtual IP 原来源的空图字段保持为空，不伪造图片。

`TestWorkflowCompletedOutputsGateway`（3.34s）、`TestWorkflowUIQuerySupportGatewayAndCLI`（14.25s）、`TestAPIPerformanceReadGateway`（6.59s）通过；独立 CLI 另读取真实完成视频 Job `job_9d6927c8-7c51-4854-8e11-3454456cebf3` 的四个成功节点。全部门禁只读，不提交、恢复或批准任务。两组迁移工具单元测试亦通过。

当前私有 E2E 环境的示例 Job ID 更新为上述真实历史记录；性能工具允许 `PERF_COMPLETED_JOB_ID` 指定样本。清理后的历史 Job 无 Conductor 实例，不能将其耗时与清理前的 native Job 引擎读取耗时直接比较。

Workflow/Materials 健康检查与前端 5173 均为 200；迁移密钥为空，导入入口已关闭。登录凭据不变。

### 全量对账最终结果

真实 CLI `--verify-report` 完成全部来源集合、目标关联和媒体读取检查：

```text
reconciliation verified: imported=298 excluded=50 quarantined=1 unique_objects=538
```

退出码为 1 是工具刻意保留的已知来源隔离信号，不是忽略未知校验错误后的成功。已确认最终 verified 行及唯一失败来源 ID：121+125+52 条成功记录，50 个精确排除项，538 个历史对象均核对通过。另行检查的 111 个素材库媒体也全部可读。没有其他导入或对账失败。

本次授权范围内的提交、数据库备份、测试业务记录清理和可导入历史数据重建完成。唯一不能完整迁入的发布记录需先恢复旧成片文件，再用新报告路径幂等补导。未执行 Source DB 删除、认证审计抹除、共享媒体文件 GC 或 UI 修改。

## 可重用工具

从仓库根目录执行，秘密只由既有私有环境文件载入，不作为命令参数，也不提交。

```bash
# 新备份；禁止覆盖已有快照。
bash harness/tools/backup_databases.sh /home/tao/backups/media_agent
bash harness/tools/verify_database_backups.sh /absolute/private/snapshot

# 先输出私有精确清理清单；实际删除需另加 --apply。
node harness/tools/reset_migration_dataset.mjs \
  --backup-dir /absolute/private/snapshot \
  --organization 8afeb26b9df643952f4f816576eb2504 \
  --keep-workflow wf_2c1231e5-7379-4207-a0de-a9bb5d93b278
```

清理工具是此次已核对数据集的维护工具，不是通用“所有 native 都是测试”的规则。保留 Workflow 的七条 native 测试执行使用显式 ID allowlist；发现未分类的业务执行或停写后新增执行立即拒绝，必须重新 review 范围。不要针对未来真实执行直接重跑。

新建导入工具二进制后，`harness/tools/reimport_history.mjs` 管理一次重导、服务密钥关闭和只读对账。导入报告禁止覆盖；失败后的幂等重跑应使用原 Go 工具和**新的** `--import-all-report` 路径，保留首次报告，不再次清库。

```bash
go -C media_materials build -o /tmp/media-agent-materials-reimport ./tools/media_ai_migrate
go -C media_workflow build -o /tmp/media-agent-history-reimport ./tools/media_ai_history_migrate

node harness/tools/reimport_history.mjs \
  --backup-dir /absolute/private/snapshot \
  --workflow-id wf_2c1231e5-7379-4207-a0de-a9bb5d93b278 \
  --workspace-id 2e6c5e63-b343-47c4-9f4e-64eb3c7855ce \
  --organization-id 8afeb26b9df643952f4f816576eb2504 \
  --owner-user-id f9b8a1a985735c62596e5f1fbf640ea3
```

其他两个 Conductor 清理工具默认也是预览：`cleanup_legacy_metadata.mjs` 只删除维护清单中 Workflow 的定义；`cleanup_conductor_test_fixtures.mjs` 只处理已确认命名契约的独立 E2E 实例和无引用旧 TaskDefinition。`cleanup_legacy_fixture_capabilities.sql` 只有在已备份且清除旧模板引用后才可执行，保留四个 Provider Capability。
