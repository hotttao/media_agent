# 架构

本文档定义基于 Hertz 的 Go web 服务的架构规则。

## Coding Agent 工作流

修改本项目时，Coding Agent 必须遵循如下流程：

1. 先阅读本架构文档，识别变更类型：API / IDL、handler、业务规则、用例、workflow、数据库 schema、repository / DAL、config。
2. 按本文档"DDD 目录结构"与"依赖与 import 规则"两节，把代码放到对应位置。
3. 不要移动 Hertz 生成文件（`hertz_gen/**`、`router_gen.go`、`biz/router/<service>/<service>.go`、`biz/router/register.go`、`biz/dal/query/**`）。
4. 不要手工编辑生成的 router / model / query 文件，除非本文档"生成代码修改权限矩阵"明确标注为 🟡 谨慎修改。
5. IDL 变更后用项目脚本重新生成 Hertz 代码：`make hz-model` / `make hz-client` / `make -C <app> hz-update`。
6. API 文档变更时，更新 `idl/<app>/*.proto` 的 `google.api.http` 与 `api.*` 双重注解，再运行 `make swagger-gen`，必要时 `make skills-gen` 同步 Skill 文档。
7. 数据库 schema 变更时，先添加或更新 `migrations/` 迁移，再按需重新生成 GORM Gen 代码。
8. 保持 handler 轻薄：请求绑定 → 转 Command → 调 `service` / `workflow` → 响应映射。
9. `service` 与 `policy` 远离 Hertz、生成 DTO 与具体基础设施（`hertz_infra/**` 是唯一 import 入口）。
10. 完成前运行项目检查：
    - `gofmt -l .`
    - `go test ./...`
    - `make arch-check`（或 `cd <app> && go run ../harness/tools/archcheck`）
11. 如果某条规则确实无法遵守，必须在最终回复中解释原因，不允许默默绕过架构。

---

## 目录结构

### Hertz Base

```text
├── go.work
├── go.work.sum
├── hertz_gen
│   ├── media_example
│   │   ├── hello_service.go
│   │   └── hertz_client.go
│   └── model
│       └── media_example
│           └── hello
│               └── hello.pb.go
├── idl
│   ├── config
│   ├── media_example
│   │   └── hello.proto
├── Makefile                      # 提供 hz-model hz-client 命令
└── media_example
    ├── biz
    │   ├── handler
    │   │   ├── hello
    │   │   │   └── hello_service.go
    │   │   └── ping.go
    │   └── router
    │       ├── hello
    │       │   ├── hello.go
    │       │   └── middleware.go
    │       └── register.go
    ├── build.sh
    ├── main.go
    ├── Makefile                      # 提供 hz-update 命令
    ├── router_gen.go
    ├── router.go
    └── script
        └── bootstrap.sh
```

---

#### 术语约定

| 占位符 | 含义 | 示例 |
|--------|------|------|
| `<app>` | 微服务名（go module / proto package / IDL 一级目录） | `media_example` |
| `<service>` | IDL `service` 名（`go_package` 最后一段，handler/router 子目录名） | `hello` |

> IDL 约定：`option go_package = "<app>/<service>";`

---

#### 通用映射规则

```text
idl/<app>/<file>.proto
  option go_package = "<app>/<service>";
        │
        ├── hz model  ──→ hertz_gen/model/<app>/<service>/<file>.pb.go        🔴 共享 API DTO
        ├── hz client ──→ hertz_gen/<app>/<service>_service.go                 🔴 调用服务的 client stub
        │                 hertz_gen/<app>/hertz_client.go                      🔴
        └── hz update ──→ <app>/biz/handler/<service>/<service>_service.go    🟡 handler 骨架
                          <app>/biz/router/<service>/<service>.go             🔴 路由注册
                          <app>/biz/router/<service>/middleware.go             🟡 中间件 hook
                          <app>/biz/router/register.go                         🔴 路由聚合
```

#### 生成代码修改权限矩阵

| 文件 / 目录 | 来源 | 修改权限 | 说明 |
|------------|------|----------|------|
| `hertz_gen/model/<app>/<service>/**` | `hz model` / `hz update` | 🔴 禁止 | API DTO，IDL 变更 → 重新生成 |
| `hertz_gen/<app>/<service>_service.go` | `hz client` | 🔴 禁止 | 下游 client stub |
| `hertz_gen/<app>/hertz_client.go` | `hz client` | 🔴 禁止 | client 聚合入口 |
| `<app>/biz/router/<service>/<service>.go` | `hz update` | 🔴 禁止 | 路由注册，每次 `hz update` 覆盖 |
| `<app>/biz/router/register.go` | `hz update` | 🔴 禁止 | 路由聚合入口 |
| `<app>/router_gen.go` | `hz new` | 🔴 禁止 | 生成器入口（除非项目 own 模板） |
| `<app>/biz/dal/query/**` | GORM Gen | 🔴 禁止 | 查询代码，schema 变更 → 重新生成 |
| `<app>/main.go` | `hz new` 骨架 | 🟡 谨慎 | 只允许初始化 / wiring，禁止业务逻辑 |
| `<app>/router.go` | `hz new` 骨架 | 🟡 谨慎 | 只允许非 IDL 自定义路由（健康检查、Swagger UI） |
| `<app>/biz/router/<service>/middleware.go` | `hz update` 扩展点 | 🟡 谨慎 | 路由组中间件，禁止动路由注册逻辑 |
| `<app>/biz/handler/<service>/<service>_service.go` | `hz update` 骨架 | 🟡 谨慎 | 仅填充函数体，保持轻薄；签名禁动 |
| `<app>/biz/middleware/**` | 手写 | 🟢 可改 | 全局中间件 |

#### 修改流程速查

| 想要的变更 | 正确路径 | 错误路径 |
|-----------|----------|----------|
| 新增 / 修改 HTTP 端点 | 改 `idl/<app>/<file>.proto` → `make hz-update` → `make swagger-gen` → `make skills-gen` | ❌ 直接编辑 `<app>/biz/router/<service>/*.go` |
| 修改下游 `<app>` 的 client 接口 | 改下游 IDL → `make hz-client` | ❌ 直接编辑 `hertz_gen/<app>/*.go` |
| 修改 API 请求 / 响应字段 | 改 IDL message → `make hz-model` | ❌ 直接编辑 `hertz_gen/model/<app>/<service>/*.pb.go` |
| 修改数据库查询 | 改 `<app>/biz/dal/model` 或迁移 → 重新跑 GORM Gen | ❌ 直接编辑 `<app>/biz/dal/query/**` |
| 实现 handler 业务逻辑 | 编辑 `<app>/biz/handler/<service>/<service>_service.go` 函数体（保持轻薄，调用 service 层） | ❌ 在 handler 写 SQL / 复杂规则 |
| 添加路由组中间件 | 编辑 `<app>/biz/router/<service>/middleware.go` | ❌ 编辑 `<app>/biz/router/<service>/<service>.go` |

### DDD
基于 DDD 易变性的分解：按"为何变化"组织代码，而非主要按业务实体。扩展后的 `<app>/biz` 目录结构如下:

```text
<app>/biz/
├── dal             # 数据访问层（DB / GORM / GORM Gen 唯一收口）
├── domain          # 稳定业务核心（实体、值对象、仓库接口）
├── handler         # Hertz HTTP handler（hz 生成骨架）
├── middleware      # 全局 Hertz middleware（手写）
├── policy          # 易变业务规则（纯决策逻辑）
├── router          # Hertz 路由注册（hz 生成）
├── service         # 用例 service（编排单 domain）
├── shared          # 稳定的应用级工具
└── workflow        # 跨 domain 业务流程编排
```

---

#### `biz/dal` — 数据访问层

数据库访问唯一收口。

```text
biz/dal/
├── db.go                # 数据库初始化
├── transaction.go       # 事务实现
├── model/               # GORM / GORM Gen 行模型
├── query/               # 🔴 GORM Gen 生成的查询代码
└── repo/                # domain 仓库接口的实现
```

| 子目录 | 职责 | 修改权限 |
|--------|------|----------|
| `db.go` / `transaction.go` | 连接、事务封装 | 🟢 手写 |
| `model/` | DB 行模型，承载 GORM tag | 🟢 手写 |
| `query/` | GORM Gen 生成的类型安全查询 | 🔴 禁改，schema 变更 → 重新生成 |
| `repo/` | 实现 `domain/<feature>` 中定义的仓库接口 repository；唯一可 import `dal/query` 的位置 | 🟢 手写 |

**职责边界**：
- `repo` 把 DB 行模型转为 domain 对象，禁止把 GORM 行模型泄漏给上层
- 业务规则禁止下沉到 repository 实现

---

#### `biz/domain` — 稳定业务核心

按 domain 子包组织的实体、值对象、domain 错误、domain 事件、仓库接口、稳定不变量。

```text
biz/domain/
├── shared/              # domain 安全的共享基础类型（ID、错误码、值对象）
├── <domain-a>/          # domain-a 实体、值对象、仓库接口
└── <domain-b>/          # domain-b 实体、值对象、仓库接口
```

#### `biz/middleware` — 全局 Hertz middleware

手写的全局中间件：auth token 解析、request id、日志、tracing、CORS、recovery、限流。

**职责**：提取技术上下文（request id、trace id、auth subject、tenant id），通过 `context.Context` 向后传递。

**不允许**：评估复杂业务权限（属于 `policy`）。

> 路由组级中间件应放在 `biz/router/<service>/middleware.go`，而非此处。

---

#### `biz/policy` — 易变业务规则

按 domain 组织的纯决策逻辑：权限决策、定价/折扣、订单取消、发布、注册等规则。

```text
biz/policy/
└── <domain>/
    └── policy.go
```

**职责**：接收 service 加载好的事实，返回决策结果。

**不允许**：自己加载数据（不查 DB / Redis / 外部 API）。

**协作模式**：`service 加载数据 → policy.CanXxx(...) → service 根据决策落地副作用`。

---


#### `biz/service` — 用例 service

按 domain 组织的用例编排，是 handler 与 domain / dal 之间的桥梁。

```text
biz/service/
├── command.go                    # 共享 command 类型（可选）
└── <domain>/
    ├── command.go                # 用例输入
    └── service.go                # 用例实现
```

**职责**：
- 接收 command（不收生成 DTO）
- 调用 domain 仓库接口完成持久化
- 为单个用例控制事务
- 通过可观测性接口记录业务事件

**协作边界**：跨 domain 编排归 `workflow`，service 只负责单 domain 的用例。

---

#### `biz/shared` — 稳定的应用级工具

应用层稳定通用类型：分页、Clock 接口、ID 生成器、可复用辅助类型。

**命名规则**：用具体名字（`clock.go` / `id.go` / `pagination.go`），禁止 `utils.go` / `common.go` / `helper.go`。

---

#### `biz/workflow` — 跨 domain 业务流程编排

```text
biz/workflow/
├── <flow_a>_flow.go
└── <flow_b>_flow.go
```

**适用场景**：一个特性需协调多个 service / policy / 外部 client / 副作用 / 补偿步骤。

**职责**：
- 协调多个 domain service 与 policy
- 协调副作用
- 为更大的本地数据库流程控制事务
- 跨系统流程使用 saga / outbox / 事件模式

**事务推荐模式**：
1. 仅在本地状态变更周围开事务
2. 尽量在外部副作用前先 commit
3. 跨系统补偿用 outbox / event / saga

**禁止**：在调用外部系统时仍持有数据库事务。


#### Service vs Workflow 职责边界

文档里最容易混淆的颗粒度，单独拉通：

| 维度 | `service` | `workflow` |
|------|-----------|-----------|
| 粒度 | **单用例**（一个 Command） | **业务流程**（多个用例 / 多个步骤） |
| Domain 范围 | 单 domain（单聚合根的状态变更） | 跨多个 domain |
| 事务 | 单个本地数据库事务 | 多事务 / saga / outbox / 事件驱动 |
| 副作用 | 仅本地数据库写 | 可发外部调用、发事件、触发补偿 |
| 时间维度 | 同步、短 | 可异步、可长流程 |
| 失败处理 | 事务回滚 | 显式补偿（saga 反向操作） |
| 调用对象 | domain 仓库接口 + 同 domain policy | 多个 service + 多个 policy + 外部 client |
| 同层隔离 | `service/<a>` 不可 import `service/<b>` | workflow 是跨 domain 协调的唯一合法入口 |

判别原则：

- 这件事**单事务**就能做完 → `service`
- 这件事需要**跨 domain 协调**或**跨系统补偿** → `workflow`
- service 内**绝不**调用其它 domain 的 service；要跨 domain 必须由 workflow 编排


## 依赖与 import 规则

### Layer Overview

```
handler     接入层：API DTO ↔ Command 翻译，调用 service / workflow
workflow    流程编排：跨 domain / 跨系统的业务流程，处理副作用与补偿
service     应用服务：单用例、单事务，处理一个聚合的状态变更
policy      易变业务规则（纯决策、无副作用）
domain      稳定业务核心（实体 / 值对象 / 仓库接口 / domain 事件 / 不变量）
dal         数据访问：repo 实现 domain 仓库接口
shared      跨 domain 应用工具（分页 / Clock / ID 生成器，与业务无关）
model       生成 API DTO（hertz_gen/model）
```

**Design goal**：每条 import 都有据可循——稳定核心绝缘、易变规则隔离、生成代码不入业务。

### Dependency Rules

#### 允许方向

```text
handler   → service / workflow / model / shared
workflow  → 多个 domain 的 service / policy / domain / shared
service   → domain / policy / shared
policy    → domain / shared
dal       → domain / shared
shared    → 标准库（不依赖任何业务层）
```

#### 禁止方向

```text
domain   ─X→ model / handler / dal / shared 
policy   ─X→ model / handler / dal / 其它 policy domain 包
service  ─X→ model / handler / 具体 dal / dal query / dal model / 其它 service domain 包
handler  ─X→ 其它 handler service 包 / dal / dal query / dal model
workflow ─X→ handler / model / 原始 SQL / dal query / dal model
dal      ─X→ handler / model / service / workflow
shared   ─X→ domain / policy / service / workflow / dal / handler / model（不允许反向依赖任何业务层）
```

#### Rules of Thumb

- 任何文件 import `biz/dal/query` 或 `biz/dal/model` → 必须位于 `biz/dal/repo` 或 `biz/dal` 初始化代码
- 任何文件 import `hertz_gen/model/**`（API DTO）→ 仅 `biz/handler/**` 允许
- 任何文件 import `hertz_gen/<app>/**`（下游 client stub）→ 仅 `hertz_infra/clienthertz/` 允许
- `domain` 不出现 JSON / query / form / ORM tag；不依赖日志、配置、框架
- `policy` 不查 DB / Redis / 外部 API；只接收 service 加载好的事实做决策

### 同层隔离

**规则**：同一层级内的子包**不可相互 import**，适用范围:

| 层 | 路径模板 | 强制级别 |
|----|---------|---------|
| domain | `biz/domain/` | 🔴 强制 |
| service | `biz/service/` | 🔴 强制 |
| policy | `biz/policy/` | 🔴 强制 |
| handler | `biz/handler/` | 🔴 强制  |

#### 禁止的横向依赖

```text
biz/domain/<a>   ─X→ biz/domain/<b>
biz/service/<a>  ─X→ biz/service/<b>
biz/policy/<a>   ─X→ biz/policy/<b>
biz/handler/<a>  ─X→ biz/handler/<b>
```

#### 跨 domain 协调的合法路径

跨 domain 不可在同层直接 import，必须**上提一层**由更高层编排。合法的编排入口有**两条**：

| 编排入口 | 适用场景 | 可协调对象 |
|---------|---------|-----------|
| `biz/workflow/<flow>` | 跨 domain 业务流程（多事务、跨系统、需补偿） | 多个 `service/<domain>` + 多个 `policy/<domain>` + 外部 client |
| `biz/service/<service>` | 单 domain 用例内需读取另一 domain 的数据（单事务内） | 通过稳定接口调用另一 domain 的查询能力，**不持有对方聚合** |

### 模型边界

四类模型按变化原因物理隔离：

| 模型类型 | 位置 | 变化原因 |
|---------|------|---------|
| API DTO / 生成模型 | `hertz_gen/model/<app>/<service>/` | IDL、HTTP 请求/响应字段、tag |
| Command | `biz/service/<domain>/command.go` | 用例输入语义变化 |
| Domain Model | `biz/domain/<domain>/` | 核心业务概念变化 |
| DB 行 / Query 模型 | `biz/dal/model/` / `biz/dal/query/` | 数据库 schema、SQL、ORM 变化 |

转换链路：

```text
API DTO   ──(handler 翻译)──→  Command   ──(service 调用)──→  Domain Model
Domain Model  ──(dal/repo 翻译)──→  DB 行模型
```

### 强制检查
可以通过如下工具执行强制的依赖检查，CI 工具会捕获违规 import。

```bash
# 方式一:
make arch-check                       # 垂直层边界 + 同层 domain 隔离 + 框架 import 限制
# 方式二:
cd media_example && go run ../harness/tools/archcheck
```
