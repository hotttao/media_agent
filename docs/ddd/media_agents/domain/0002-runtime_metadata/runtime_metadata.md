---
name: runtime_metadata
description: 实例级不可变元数据产物的准备、发布与应用结果
dependencies: []
related_workflow: [agent_metadata_update]
---

## 一句话职责

Runtime Metadata 拥有实例对已确认 Metadata Release 的应用状态与版本产物，配置的选择和授权仍归 Control。

## 边界

### 做什么

准备完整校验后的 Prompt/Skill/运行文件，原子发布实例可用版本，保留旧 Session 固定引用，记录可重建应用结果。

### 不做什么

Control 拥有 Catalog 和用户选择；Gateway 生成 Platform Prompt；Runtime adapter 转换配置。此 Domain 不编辑 Catalog、不决定用户选择、不以能力开关绕过版本隔离，不保存明文凭据。

## 能力清单

| 能力 | 说明 | 接口 |
|:---|:---|:---|
| Prepare | 校验统一响应 schema、资源哈希及路径，生成隔离 staging | 内部 port |
| Publish | 同作用域选择序号单调发布当前实例可用引用 | 内部 service |
| ResolveVersion | 返回 Session 固定引用的产物目录 | Runtime preparation port |
| RecordApplication | 保存成功/失败与待上报事实 | 内部 service |

## 聚合与不变量

| 聚合根 | 包含对象 | 一致性边界 | 核心不变量 |
|:---|:---|:---|:---|
| Metadata Application | 实例已应用版本、当前结果、待上报结果 | 一次原子 marker 替换 | 选择序号不倒退；成功引用只能指向完整校验的不可变产物 |

Metadata Application 以 workspace/agent_config/replica/incarnation 为实例作用域根；选择序号和产物发布/结果 marker 一致推进。用户明确选择旧资源版本仍是新的选择序号。共享 artifact 按内容 hash 幂等发布，实例状态各自独立。未知 schema、checksum 不符、路径逃逸或部分文件失败时不发布半成品。

Session 配置不会因为实例 current 指针变化而改变。HOME 仍属于共享 Workspace，但 Runtime 的 Prompt/Skill 配置必须绑定不可变版本目录；禁止原地改写旧 Session 引用的全局文件。

## 领域事件

无额外总线事件；应用结果经 Gateway API 上报 Control，RabbitMQ ACK 不代替业务结果。

## 数据模型

本 Domain 使用文件持久化，不建立业务表。每个实例的当前应用和待上报结果在同一 marker 内原子替换。

### 不可变产物 manifest

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| context_version | string | 已确认 Release 固定版本 |
| bundle_checksum | string | 完整产物内容摘要 |
| metadata_schema_version | integer | 响应/产物结构版本 |
| resources | resource reference array | 固定资源版本、相对路径和 checksum |

### 实例 application marker

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| workspace_id, agent_config_id | string | 配置作用域 |
| replica_id, incarnation_id | string | 副本与当前启动代次 |
| selection_revision | integer | 当前观测的用户确认次序 |
| applied_context_version | string nullable | 最后完整成功发布的版本 |
| artifact_ref | relative path nullable | 对应不可变产物目录 |
| reports | application result array | 稳定 report_id、目标/状态及待上报标记 |

#### 约束与索引

- 已发布目录按 context/hash 唯一，禁止覆盖；staging 使用唯一 UUID。
- 先校验并原子发布完整目录，再原子替换包含版本引用和待上报结果的 marker。前一步完成而 marker 未提交时，产物尚未应用，恢复允许复用；不把孤立目录当成功。
- marker 按 replica/incarnation 隔离；同实例串行发布，CAS/锁保持 selection_revision 单调，旧 Session 始终引用固定目录。

### 存储位置

| 对象 | 位置/身份 | 规则 |
|:---|:---|:---|
| 不可变产物 | Workspace 内按 release/hash 分目录 | 已发布不覆盖；staging 唯一 UUID |
| 实例应用 marker | 按 replica/incarnation 单独保存 | 原子替换，包含 selection_revision/target/status |
| 待上报结果 | 与应用 marker 同一可靠提交 | report_id 稳定，上报失败不重新安装 |

## 能力详细说明

- Prepare 输入已授权的固定 manifest 与资源内容，完整校验 schema/hash/路径；不兼容和文件失败返回明确错误，保留旧成功引用。
- Publish 输入准备完成的不可变产物与选择序号；拒绝旧序号覆盖，原子记录应用引用及稳定报告，不修改既有 Session。
- ResolveVersion 按 Session 固定 context/hash 查询已校验目录；不存在时返回需准备的状态，不退回任意 latest。
- RecordApplication 记录 applied/failed/incompatible/superseded；上报失败仅保留待上报标记，同一选择/实例成功不能被晚到失败撤销。

## 关联关系

### 依赖的 Domain

无直接聚合依赖；固定选择与 Session 引用由调用方提供。

### 被依赖的 Domain

无直接聚合依赖，Runtime 加载通过 preparation port 取得固定目录。

### 参与的 Workflow

[agent_metadata_update](../../../workflow/0010-agent-metadata-update/agent_metadata_update.md) 负责拉取、当前选择再核对和上报；Domain 仅决定产物可发布及应用状态。恢复从 marker 和实际文件校验重建，不使用进程内记忆证明成功。

### 关联术语

Runtime Metadata Application、Metadata Release、Session Context Version。
