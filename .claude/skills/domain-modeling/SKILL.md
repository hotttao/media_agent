---
name: domain-modeling
description: 搭建并完善项目领域模型。适用于用户需要明确领域术语、统一通用语言、领域划分、记录架构决策，或是其他技能需要维护领域模型的场景。
---


# 领域建模
在设计过程中主动搭建、打磨项目领域模型。这是一套**主动式**工作规范：校验术语冲突、确定领域范围和职责、构造边界场景、一旦确定术语与决策便立刻写入文档。
仅读取 `CONTEXT.md` 获取词汇不属于本技能范畴，所有技能都可简单读取该文件；本技能用于**修改、完善领域模型**，而非单纯查阅使用。

## 文件目录结构

```
/
├── CONTEXT-MAP.md                                  # 全局业务术语路由表，标注每个业务域的存放路径
├── docs/
│   └── adr/                                    # 全局系统级架构决策记录
│   └── workflow/                               # 全系统跨服务工作流
│       ├── 0001-event-sourced-orders.md 
│       └── 0002-postgres-for-write-model.md
│   └── <service>                                # 服务层文档
│       ├── CONTEXT.md                           # 服务层业务术语
│       ├── 0001-<domain>                             
│       |   ├── <domain>.md                      # 服务层领域文档
│       |   ├── change.md                        # 服务层领域变更记录
│       ├── 0001-<workflow>                      
│       |   ├── <workflow>.md                    # 服务层工作流文档
│       |   ├── change.md                        # 服务层工作流变更记录
│   └── plan/                                    
│   └── spec/ 
```

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

### 实时同步更新 CONTEXT.md
每敲定一个业务术语，立刻同步写入 `CONTEXT.md`，禁止攒到最后批量修改；文档格式严格遵循同目录 `CONTEXT-FORMAT.md`。

`CONTEXT.md` 仅用作纯术语词典，严禁写入任何代码实现细节。不可将它当作需求文档、临时草稿，也不能存放各类技术实现决策，仅此一个用途。

### 实时更新 `<domain>.md` 和 `workflow.md`
每次方案或者业务流程改动，都需要同步更新 `<domain>.md` 和 `workflow.md`。文档格式严格遵循同目录 `DOMAIN-FORMAT.md` 和 `WORKFLOW-FORMAT.md`。

这两个文档记录的是最新达成的设计目标，里面不应该包含任何记录，为什么这么变更，以及变更历史。同目录下的 `change.md` 文件用于记录每次变更的详细信息，文件格式参考 `CHANGE-FORMAT.md`。

### 谨慎创建架构决策记录(ADR)
仅当同时满足以下全部3个条件，才建议新建ADR文档：
1. **难以回退**：后续推翻该决策的改造成本很高；
2. **脱离上下文难以理解**：后续维护者看到该设计，会疑惑“当初为什么这么设计”；
3. **经过多方权衡取舍**：当初存在多种可行方案，基于明确理由选定当前方案。

任意一条不满足，则无需编写ADR；ADR文档格式遵循同目录 `ADR-FORMAT.md`。

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
3. 按照排序后的 domian 遍历，参考 Harness 架构规范，找出 domain 所有代码文件
4. 对比 domain 文档，找出所有实现与设计文档不一致的地方。


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
1. 如果找到项目的 Harness 架构规范文档，可以参考架构规范，重建 domain 和 workflow 文档。
2. 如果没有，可以针对目标语言实现一个类或者函数的行数统计工具，先统计所有类和函数的行数。并按照行数倒叙找到最复杂的类和函数。
3. 对最复杂的类和函数，参考其代码实现，重建 domain 和 workflow 文档。

