---
name: domain-modeling
description: 搭建并完善项目领域模型。适用于用户需要明确领域术语、统一通用语言、领域划分、记录架构决策，或是其他技能需要维护领域模型的场景。
---


# 领域建模
在设计过程中主动搭建、打磨项目领域模型。这是一套**主动式**工作规范：校验术语冲突、确定领域范围和职责、构造边界场景、一旦确定术语与决策便立刻写入文档。
仅读取 `CONTEXT.md` 获取词汇不属于本技能范畴，所有技能都可简单读取该文件；本技能用于**修改、完善领域模型**，而非单纯查阅使用。

## DDD 建模对象

建模时必须区分以下层次，禁止把它们混称为 domain：

| 层次 | 回答的问题 | 文档位置 |
|:---|:---|:---|
| 限界上下文（Bounded Context） | 哪套统一语言和模型在什么边界内有效 | `CONTEXT-MAP.md`、`docs/<service>/CONTEXT.md` |
| Domain | 同一变化原因下的一组业务能力和规则 | `docs/<service>/domain/<nnnn-domain>/<domain>.md` |
| 聚合（Aggregate） | 哪些实体和值对象必须在一个一致性边界内修改 | Domain 文档“聚合与不变量” |
| 实体/值对象 | 什么有身份和生命周期；什么只由值定义 | Domain 文档数据模型之前 |
| Domain Event | 聚合已经发生且其他能力需要知道的业务事实 | Domain 文档“领域事件” |
| Application Workflow | 如何跨 Domain、跨事务、跨服务或协调外部副作用 | service/global `workflow` 文档 |

数据库表是聚合的持久化设计，不是划分 Domain 的起点。一个 Domain 可以有多个聚合和表；一个 Workflow 不拥有业务事实表，只协调各 Domain 的能力。

### 聚合边界判定

满足以下任一条件的实体通常属于同一聚合：必须原子维护同一个不变量、没有聚合根就不能独立修改、生命周期完全依附聚合根。仅因为存在外键、页面一起展示或接口一次返回，不能归入同一聚合。

跨聚合只保存 ID；跨服务不得建立数据库外键。强一致只覆盖单聚合或同库明确事务边界，跨服务使用幂等命令、Outbox/Inbox、Saga 或补偿。

### Workflow 判定

出现以下任一情况，必须评估并通常创建 Workflow：

- 协调两个及以上 Domain/聚合；
- 调用外部服务、对象存储、消息系统或第三方系统并需要处理失败；
- 跨多个本地事务或持续时间明显超过一次同步请求；
- 需要幂等键、重试、超时、补偿、恢复或至少一次事件消费；
- 同一业务目标包含“先提交本地事实，再触发外部副作用”。

以下情况不创建 Workflow：单聚合内部状态机、单个仓库事务、纯查询组合、简单 DTO 转换。聚合自己的状态机与不变量写入 Domain；不要为了让每个 Domain 都有 Workflow 而创建空壳流程。

### Workflow 质量要求

Workflow 不是步骤标题清单。仅写成 `鉴权 -> 写库 -> 调服务 -> 完成` 的单行箭头不合格，因为它无法指导实现，也无法审查一致性和失败语义。每份重要 Workflow 必须让程序员仅阅读该文档就能回答：

1. 谁触发流程，各参与者按什么顺序交换命令、结果和事件；
2. 每个本地事务提交哪些业务事实，外部副作用发生在事务之前还是之后；
3. 哪个 Domain/聚合决定状态转换，Workflow 只协调什么；
4. 客户端连接、后台任务、业务状态等不同生命周期是否相互影响；
5. 每个可能重复的步骤使用什么幂等键，结果不明确时是查询、重试、补偿还是人工恢复；
6. 中途失败、超时、取消、晚到消息和部分成功分别产生什么可观察结果；
7. 用户身份、Workspace、服务身份和凭证从哪里取得，哪一方执行最终资源授权。

执行时间线必须使用 Mermaid sequenceDiagram 或等价的多参与者文本时序，标出关键事务边界和外部调用；图后必须解释容易误实现的步骤。涉及生命周期时增加状态图或状态转换表，并写出终态、并发和晚到事件规则。简单 Workflow 可以省略不适用的章节，但不得用抽象词替代关键语义，例如只写“失败后重试”而不说明重试条件和幂等边界。

## 文件目录结构

```
/
├── docs/
|   ├──CONTEXT-MAP.md                               # 全局业务术语路由表，标注每个业务域的存放路径
│   ├── adr/                                        # 全局系统级架构决策记录
│   ├── workflow/                                   # 全系统跨服务工作流
│   │   ├── 0001-event-sourced-orders.md 
│   │   └── 0002-postgres-for-write-model.md
│   ├── <service>/                                  # 服务层文档
│   │   ├── CONTEXT.md                              # 服务层业务术语
│   │   ├── domain/                                 # 服务层领域文档归档，0001- 仅在 domain 内编号
│   │   │   └── 0001-<domain>/
│   │   │       ├── <domain>.md                     # 服务层领域文档
│   │   │       └── change.md                       # 服务层领域变更记录
│   │   └── workflow/                               # 服务层工作流文档归档，0001- 仅在 workflow 内编号
│   │       └── 0001-<workflow>/
│   │           ├── <workflow>.md                   # 服务层工作流文档
│   │           └── change.md                      # 服务层工作流变更记录
│   ├── plan/                                       # 落地计划：跨服务放顶层，服务级按 <service>/ 分目录
│   │   ├── <topic>.md                              # 跨服务计划（顶层，单文件）
│   │   └── <service>/                              # 服务级计划（一服务一目录）
│   │       └── <milestone>.md                      # 按里程碑/主题拆分
│   └── spec/                                       # 实现规格：跨服务放顶层，服务级按 <service>/ 分目录
│       ├── <topic>.md                              # 跨服务规格（顶层，单文件）
│       └── <service>/                              # 服务级规格（一服务一目录）
│           └── <milestone>.md                      # 按里程碑/主题拆分
```

> `<service>/` 下通过 `domain/` 与 `workflow/` 两个子目录区分领域与工作流：`0001-` 数字编号仅在各自子目录内计数，互不干扰。这样 `0001-auth`（domain）与 `0001-wecom-login-bind`（workflow）分属不同目录，无需打开文件即可辨别类型。

文件采用懒创建机制：仅在有内容需要记录时新建文件。
- 若无 `CONTEXT.md`，敲定第一个业务术语后再创建；
- 若无 `docs/adr/` 目录，需要撰写第一条架构决策记录时再创建。

## 工作执行细则
### 校验术语词典冲突
若用户使用的词汇与 `CONTEXT.md` 中已有定义冲突，需立刻指出矛盾。
示例话术：“术语文档中将‘取消(cancellation)’定义为X含义，但你当前想表达的是Y，请问以哪个为准？”

### 规整模糊多义词汇
当用户使用模糊、多义、指代不清的词语时，主动给出唯一标准规范术语。
示例话术：“你提到的‘账户(account)’，指客户(Customer)还是系统用户(User)？二者是完全不同的概念。”

### 结合真实场景推演校验
讨论领域实体关联关系时，使用具体业务场景验证逻辑完整性。主动构造各类边界场景，倒逼用户明确不同业务概念的划分边界。

### 交叉核对代码实现
当用户口头描述业务逻辑时，核对代码真实实现是否匹配；若发现矛盾点，直接抛出确认：
“当前代码逻辑仅支持整单取消，但你刚刚说支持部分取消，哪种逻辑是标准规范？”

### 建立能力覆盖矩阵

重构或迁移场景必须先枚举事实来源中的全部能力（旧路由、IDL、命令、定时任务、事件消费者和管理入口），建立：

```text
原能力 -> 目标限界上下文 -> Domain -> 聚合/读模型 -> Workflow -> 状态
```

状态只能是 `covered`、`external-owner`、`deprecated-with-reason`、`missing`。存在 `missing` 时领域建模不得宣称完成；`external-owner` 必须指出外部服务，`deprecated-with-reason` 必须记录删除原因。

### 实时同步更新 CONTEXT.md
每敲定一个业务术语，立刻同步写入 `CONTEXT.md`，禁止攒到最后批量修改；文档格式严格遵循同目录 `CONTEXT-FORMAT.md`。

`CONTEXT.md` 仅用作纯术语词典，严禁写入任何代码实现细节。不可将它当作需求文档、临时草稿，也不能存放各类技术实现决策，仅此一个用途。

### 实时更新 `<domain>.md` 和 `workflow.md`
每次方案或者业务流程改动，都需要同步更新 `<domain>.md` 和 `workflow.md`。文档格式严格遵循同目录 `DOMAIN-FORMAT.md` 和 `WORKFLOW-FORMAT.md`。

这两个文档记录的是最新达成的设计目标，里面不应该包含任何记录，为什么这么变更，以及变更历史。同目录下的 `change.md` 文件用于记录每次变更的详细信息，文件格式参考 `CHANGE-FORMAT.md`。

每次新增或修改 Domain 后，必须执行一次 Workflow 审查：检查该 Domain 的每个写能力、后台任务和领域事件是否触发跨聚合/跨服务协作。需要 Workflow 时同步创建或更新；不需要时无需创建占位文档。

### 谨慎创建架构决策记录(ADR)
仅当同时满足以下全部3个条件，才建议新建ADR文档：
1. **难以回退**：后续推翻该决策的改造成本很高；
2. **脱离上下文难以理解**：后续维护者看到该设计，会疑惑“当初为什么这么设计”；
3. **经过多方权衡取舍**：当初存在多种可行方案，基于明确理由选定当前方案。

任意一条不满足，则无需编写ADR；ADR文档格式遵循同目录 `ADR-FORMAT.md`。

### plan 与 spec 文档规范

`plan` 与 `spec` 是从设计文档落地到工程实现的两类文档，遵循**与领域文档一致的服务分层**：服务级与跨服务分开放置。

**目录分层规则**：
- **服务级**（只涉及单个服务的代码/数据）：放在 `docs/plan/<service>/` 与 `docs/spec/<service>/` 下，**一个服务一个目录**。目录内可放单份主文档，也可按里程碑/主题拆成多份（如 `0001-rollout.md`、`0002-migration.md`）。
- **跨服务**（涉及多个服务或全系统基础设施，如可观测性、目录结构重构）：直接放在顶层 `docs/plan/<topic>.md` 与 `docs/spec/<topic>.md`，单文件。

判定标准：若文档的全部落地项都落在同一个 service 的代码/迁移内，属服务级；若横跨 ≥2 个 service 或属于 workspace 级基础设施，属跨服务。

**spec（实现规格）**：自包含的技术实现规格——研发只看本文即可完成实现，不必再翻阅领域文档拼凑细节。基于 `<workflow>.md`、`<domain>.md`、`change.md` 与 `Harness 架构规范` 生成，内容通常包括：概述与边界、目录结构、数据模型（表/字段）、接口契约（IDL/路由）、关键能力实现、配置项等。spec 记录"实现成什么样"，不记录"为什么这么设计"（那是 ADR），也不记录"怎么排期落地"（那是 plan）。

**plan（落地计划）**：基于 spec 的工程化拆解——把 spec 的实现内容切成可执行的阶段（P1/P2/...）与带勾选的 checklist，并标注阶段间依赖、风险与对策、验收标准。plan 只描述"怎么把 spec 落地"，不重复 spec 的设计内容。

**与文档更新流程的关系**：spec 先于 plan（流程 step 6→7）。spec 定稿后才有 plan；plan 变更若触及设计，须先回改 spec 与领域文档，再同步 plan。

文件采用懒创建机制：仅在 spec/plan 真正需要撰写时创建对应目录与文件。

### Harness 架构规范
通常每一个 service 都会有自己的 Harness 架构规范文档，架构规范文档里会详细记录 domain 和 workflow 在代码中的位置。在对齐设计文档与实现时，需要详细参考 Harness 文档的规范，建立设计与代码的映射视图。

### 非常重要

对 ** adr、domain、workflow ** 更新你应该站在一个程序员的角度去考虑，这些文档是为了帮助程序员快速理解，每个服务的能力和边界、并能快速 review 设计是否合理。不要像记录流水账一样去记录这些文档。需要有层次有结构有逻辑。如果你一句话就可以说清楚，那你不应该记录他，除非他是重要的设计决策或者对理解某些逻辑非常重要。对于复杂业务逻辑，要尽可能清楚和细节。

## 文档更新流程

** 文档更新流程至关重要，必须严格遵守。**
1. 首先更新 CONTEXT.md，必须保证其它文档在语义上与 CONTEXT.md 保持一致。
2. 更新 `adr`，`adr` 是重大的架构决策记录，是其他文档的基础。
3. 更新 `<workflow>.md`，`<workflow>.md` 工作流很大程度上影响了 domain 需要实现的功能
4. 更新 `<domain>.md`，`<domain>.md` 是领域文档，记录了该领域的业务逻辑和实现。
5. 更新 `change.md`，`change.md` 是变更记录，记录了每次变更的详细信息。
6. 基于 `<workflow>.md`、`<domain>.md`、`change.md` 和 `Harness 架构规范` 完成 spec 技术文档开发
7. 基于 spec 文档，完成 plan

完成前增加五项门禁：术语无冲突、能力覆盖矩阵无 `missing`、每个重要写流程均完成 Workflow 判定、Workflow 满足上述可实现性质量要求、Domain/Workflow 与 Harness 分层可映射。


## Command: review

当用户想检测代码实现是否与设计文档一致时，使用 `review` 命令。

### 目标

在 `docs/review/` 目录下，按照 `<service>/` 的目录层级创建 domian 和 workflow 的review 文档，文件名同 domain 和 workflow 文档名。里面记录所有实现与设计文档不一致的地方。

### 输入

- domain name.
- workflow name.

Examples:

```text
review --domain domain_name 
review --workflow workflow_name
```

### 执行过程

1. 如果没有指定 domain 或者 workflow，找出当前所有 service 的所有 domain 和 workflow。
2. 对所有 domain 和 workflow，按照依赖关系排序，不依赖其他的靠前，依赖越多的靠后。
3. 按照排序后的 domain 遍历，参考 Harness 架构规范，找出 domain 所有代码文件。
4. 对比术语、聚合不变量、状态机、数据模型、接口、事件和错误语义。
5. 遍历所有写入口、后台任务和事件消费者，检查是否有遗漏 Workflow。
6. 若存在迁移清单，生成能力覆盖矩阵并列出 `missing` 项。


## Command: rebuild

当用户希望从**当前代码实现**反向重建领域模型文档时，使用 `rebuild` 命令。它以代码为事实来源，生成或刷新 domain 与 workflow 文档，适用于：
- 老项目已有实现但缺少 domain/workflow 文档
- 代码与文档长期脱节，文档已不可信，需要从代码重新对齐
- 大规模重构后，文档需要重新生成

### 目标
基于当前项目代码实现，生成两类文档：
1. 符合 `DOMAIN-FORMAT.md` 总分结构的 `<domain>.md`。
2. 符合 `WORKFLOW-FORMAT.md` 的 service 级 `<workflow>.md`——覆盖该 service 内跨 domain 协作的关键业务流程。

rebuild **只生成文档，不写 `change.md`**——change 用于人工设计变更追溯，机器重建不应污染变更历史。

### 输入
- `--service <service_name>`：指定服务，必填（rebuild 需要明确作用域）。
- `--domain <domain_name>`：可选，仅重建指定 domain；省略则重建该 service 下所有 domain。
- `--workflow <workflow_name>`：可选，仅重建指定 workflow；省略则基于代码自动识别并重建该 service 所有关键 workflow。
- `--dry-run`：可选，仅输出分析结果不写文件。

Examples:
```text
rebuild --service media-materials
rebuild --service media-materials --domain generation_job
rebuild --service media-materials --workflow job_submission
rebuild --service media-materials --dry-run
```

### 执行过程
1. 读取 CONTEXT、IDL、数据库 migration、路由/handler、application service、领域对象、后台任务和事件消费者。
2. 从写入路径和持久化事实识别聚合根、生命周期、不变量和事务边界；不得按文件大小或函数行数划分 Domain。
3. 从跨聚合调用、外部副作用、重试/补偿和事件链识别 Workflow。
4. 用公开接口、定时任务和事件入口反查能力覆盖，确保每项都有 Domain 或外部所有者。
5. 最后再用复杂函数作为补充线索，检查是否存在未显式建模的业务规则。

