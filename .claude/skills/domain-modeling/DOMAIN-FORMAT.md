# Domain 文档规范格式
所有领域（domain）统一存放于 `docs/<service>/0001-<domain>/<domain>.md`，采用连续数字编号命名：`0001-标识名.md`、`0002-标识名.md`，以此类推。

采用懒创建机制，仅在需要时创建。

## 设计原则：总分结构
domain 文档必须采用**总分结构**，让程序员打开文档后 30 秒内就能看清这个 domain 的**能力与边界**，再决定是否深入细节。

- **总**（上半部分）：职责、边界、能力清单。不展开实现，只回答"这个 domain 是什么、做什么、不做什么、对外提供哪些能力"。
- **分**（下半部分）：数据模型、能力详细实现、关联关系。给需要动手改代码的人看。

禁止把关联关系、表结构等细节放在"总"之前，避免程序员第一眼被次要信息淹没。

## 标准模板
``````md
---
name: domain 名称
description: domain 职责的简单描述
dependencies: 依赖的其他 domain
    - domain1
    - domain2
related_workflow: 参与的 workflow
    - workflow1
    - workflow2
---

## 一句话职责
一句话说明这个 domain 在系统中承担的核心职责。
eg: `GenerationJob 负责单次 AI 生成任务的持久化、状态流转与跨服务调度锚点。`

## 边界
明确划出"做什么 / 不做什么"。边界与能力同等重要——明确"不做什么"可以避免其他服务错误依赖。

### 做什么
- 列出本 domain 直接负责的能力，每条一行动词开头。
- eg: `维护 job 生命周期（PENDING → RUNNING → COMPLETED/FAILED/NEED_CONFIRM）`

### 不做什么
- 列出本 domain 显式不承担、由其他 domain 负责的职责，标注归属。
- eg: `不负责实际执行生成——执行由 browser-extension 完成，本 domain 仅持有 job 状态`

## 能力清单
**必须严格按下表格式生成，不得改成段落或散点列举。** 表格让程序员扫一眼即知 domain 对外提供哪些能力，是"总"的核心抓手。每行一个能力，三列缺一不可：

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 创建生成任务 | 接收业务方请求，写入 job 记录并触发调度 | `CreateJob` |
| 查询任务状态 | 返回当前 job 状态及关联 asset | `GetJob` |
| 选主图（多图场景） | NEED_CONFIRM 态下由用户选定最终主图 | `SelectVariant` |

规则：
- "能力"用动宾短语，一句话讲清这个能力干什么，不要堆实现细节。
- "对外接口/函数"填真实代码符号（HTTP path、RPC method、service 方法名均可），必须可被程序员直接 grep 到。
- 一个能力对应表里一行，禁止合并多个能力到一行，也禁止一行拆成多行。
- 内部 helper 方法不进清单，只列对外暴露的能力。

---

## 数据模型

### Tables
通常每个 domain 对应一到多个 table，列出 table 的详细描述。

| 字段 | 描述 | 类型 | 关联表 | 关联说明 |
|:---|:---|:---|:---|:---|
| field1 | field1 描述 | field1 类型 | table2.field1 | 来自 table 的冗余字段 |

### UUID 生成规则
每个 record 唯一 id 的生成规则描述。
eg: `job_id = ulid()，全局唯一，单调递增，作为跨服务传递的主键。`

## 能力详细说明
对"能力清单"中每一项展开伪代码实现。必须包含关键实现细节，包括但不限于：
1. 关键的数据传递：如果入参和返回值是 workflow 中的重要交互接口，需要详细说明。
2. 关键的校验规则：校验规则，以及如何做异常处理。
3. 关键的处理逻辑：处理过程的详细说明。

**domain 文档只描述本 domain 内部的处理逻辑，不描述跨 domain/跨 service 的时序、状态机、幂等等协作语义——那些属于 workflow 文档的范畴，由 workflow 记录，避免与 workflow 重复。**

下面是一个可供参考的示例：

### CreateJob
```
1. 入参：business_id, workflow_id, input, run_time
2. ResolveWorkflowBindings
   - 合并 input 与 binding 默认值
   - 用 input_schema 校验合并后的值，失败返回 InvalidInput
3. user_team_auth 校验
   - 校验调用方对 workflow + binding 资源的 team 访问权，失败返回 Forbidden
4. CreateGeneratedAsset
   - asset_id = ulid()
   - job_id = exec_id（即本 job 的 id）
   - run_time, index=1, is_primary=true
5. 写 GenerationJob 表
   - status=PENDING
   - input=合并后值, workflow_id, run_time
6. 返回 job 对象 + asset (index=1)
```

## 关联关系

### 依赖的 domain
- [001_job](001_job.md) — 依赖其提供 xxx 能力
- [003_prompt_template](003_prompt_template.md) — 依赖其提供 xxx 能力

### 被依赖的 domain
- [004_workflow_execution](004_workflow_execution.md) — 向其提供 xxx 能力

### 参与的 workflow
- [002_workflow_execution_reconcile](../workflow/002_workflow_execution.md) — 在 workflow 中作为 xxx 角色参与

### 关联术语
domain 相关的 CONTEXT.md 术语，仅列出与本 domain 强相关的。
``````

## 编号规则
遍历 `docs/<service>` 目录，读取当前已存在 domain 目录的最大编号，在此基础上加一作为新 domain 的编号。如果 Domain 已经废弃，直接在目录名后添加 -del 即可。

## 何时需要创建 change.md
domain 的如下变更需要创建 change.md 记录变更历史：
1. 接口变化
2. 数据库变化
3. 重要且复杂的业务逻辑变化，简单的不重要的业务逻辑无需记录。
