# 架构

本文档定义基于 FastAPI 的 Python web 服务的架构规则。

## Coding Agent 工作流

修改本项目时，Coding Agent 必须遵循如下流程：

1. 先阅读本架构文档，识别变更类型：API / schema、路由 handler、业务规则、用例、workflow、数据库 schema、repository / DAL、config。
2. 按本文档"目录结构"与"依赖与 import 规则"两节，把代码放到对应位置。
3. FastAPI 无代码生成：路由、schema、ORM model 均为手写，不存在"生成文件不可改"的约束，但仍须遵守分层职责与 import 边界。
4. API 文档变更时，更新 `idl/<domain>.yaml`（OpenAPI）与 `schemas/<domain>.py`（Pydantic），保持二者一致。
5. 数据库 schema 变更时，先用 Alembic 生成迁移脚本并编辑为纯 SQL/手写 op，执行迁移，再按需更新 `biz/dal/models/`。
6. 保持路由 handler 轻薄：请求解析 -> 转 Command -> 调 `service` / `workflow` -> 响应映射。
7. `service` 与 `policy` 远离 FastAPI、Pydantic DTO 与具体基础设施（`biz/dal/**` 是唯一 import SQLAlchemy 的入口）。
8. 完成前运行项目检查（见"强制检查"）：
    - `ruff check .` + `ruff format --check .`（lint + 格式）
    - `mypy adapter codex goose claude_code`（类型）
    - `lint-imports`（层级依赖 + 同层隔离，对标 Go archcheck）
    - `vulture adapter codex goose claude_code --min-confidence 60`（死代码，对标 Go deadcheck）
    - `pytest`（单测）
    - 已配置 pre-commit：`pre-commit install` 后 commit 前自动跑上述全部检查
9. 如果某条规则确实无法遵守，必须在最终回复中解释原因，不允许默默绕过架构。

---

## 目录结构

### 项目布局（flat layout）

```text
media_agents/                      # 项目根 = 包根
├── pyproject.toml
├── alembic.ini
├── alembic/
│   ├── env.py
│   └── versions/                  # 🟢 手写迁移脚本
├── idl/                           # OpenAPI schema 源（手写 .yaml）
│   └── <domain>.yaml
├── schemas/                       # 🔴 API DTO (Pydantic v2)，对标 hertz_gen/model
│   └── <domain>/
│       ├── request.py
│       └── response.py
├── biz/                           # 业务层（对标 media_example/biz/）
│   ├── api/                       # 接入层
│   │   └── v1/
│   │       ├── routes/
│   │       │   └── <domain>/
│   │       │       └── router.py  # FastAPI APIRouter
│   │       └── deps.py            # 依赖注入（DB session / auth subject）
│   ├── middleware/                # 全局中间件（request id / logging / auth / CORS）
│   ├── domain/                    # 稳定业务核心
│   │   ├── shared/                # domain 安全的共享基础类型
│   │   └── <domain>/
│   │       ├── entity.py          # 实体 / 值对象
│   │       ├── repository.py      # 仓库接口（Protocol）
│   │       └── errors.py          # domain 错误
│   ├── service/                   # 用例 service
│   │   └── <domain>/
│   │       ├── command.py         # 用例输入
│   │       └── service.py
│   ├── workflow/                  # 跨 domain 业务流程编排
│   │   └── <flow>_flow.py
│   ├── policy/                    # 易变业务规则
│   │   └── <domain>/
│   │       └── policy.py
│   ├── dal/                       # 数据访问层（SQLAlchemy 唯一收口）
│   │   ├── db.py                  # engine / session_factory
│   │   ├── session.py             # TxRunner + ctx 传递
│   │   ├── models/                # ORM 行模型
│   │   └── repos/                 # domain 仓库接口实现
│   └── shared/                    # 跨 domain 应用工具（分页 / Clock / ID）
├── config/                        # 配置定义（pydantic-settings）
│   └── config.py
├── infra/                         # 跨业务基础设施（对标 hertz_infra，暂无服务治理）
│   └── logging.py                 # 日志配置 setup
├── clients/                       # 下游 client 装配（httpx）
│   ├── __init__.py
│   └── init_<downstream>.py
└── main.py                        # FastAPI app 入口
```

---

#### 术语约定

| 占位符 | 含义 | 示例 |
|--------|------|------|
| `<app>` | 微服务名（pyproject 里的项目名） | `media_agents` |
| `<domain>` | 业务领域名（目录子包 / 路由组 / schema 子目录） | `media` |
| `<flow>` | 跨 domain 业务流程名 | `publish_flow` |

> Python 无 IDL `go_package` 约定，`<domain>` 由目录名与 APIRouter prefix 统一。

---

#### 文件职责与修改约定

FastAPI 无代码生成，所有文件均为手写。但各层文件仍须遵守职责边界：

| 文件 / 目录 | 职责 | 修改约束 |
|------------|------|----------|
| `schemas/<domain>/**` | API 请求/响应 DTO（Pydantic v2） | 🟢 手写；仅 `biz/api/**` 可 import |
| `idl/<domain>.yaml` | OpenAPI schema 源 | 🟢 手写；与 `schemas/` 保持一致 |
| `biz/api/v1/routes/<domain>/router.py` | FastAPI APIRouter，路由注册 + handler | 🟢 手写；handler 保持轻薄，调 service |
| `biz/api/v1/deps.py` | 依赖注入（DB session / auth / config） | 🟢 手写 |
| `biz/middleware/**` | 全局中间件 | 🟢 手写 |
| `biz/domain/<domain>/**` | 实体 / 值对象 / 仓库接口 / domain 错误 | 🟢 手写；不依赖框架与 ORM |
| `biz/service/<domain>/**` | 用例编排 | 🟢 手写 |
| `biz/workflow/<flow>_flow.py` | 跨 domain 编排 | 🟢 手写 |
| `biz/policy/<domain>/**` | 纯决策逻辑 | 🟢 手写 |
| `biz/dal/models/**` | SQLAlchemy ORM 行模型 | 🟢 手写；schema 变更同步更新 |
| `biz/dal/repos/**` | domain 仓库接口实现 | 🟢 手写；唯一 import `dal/models` 与 SQLAlchemy query 的位置 |
| `biz/dal/db.py` / `session.py` | engine / session / 事务 | 🟢 手写 |
| `biz/shared/**` | 应用级工具 | 🟢 手写 |
| `config/config.py` | 配置定义（pydantic-settings） | 🟢 手写；叶子模块，不依赖业务层 |
| `infra/logging.py` | 日志配置 setup（dictConfig 应用） | 🟢 手写；读 `config`，被 `main.py` 调用 |
| `clients/**` | 下游 client 装配 | 🟢 手写 |
| `main.py` | FastAPI app 入口 | 🟡 谨慎；只做 wiring（app 创建 / middleware 注册 / router include / lifespan / 日志初始化），禁止业务逻辑 |
| `alembic/versions/**` | 迁移脚本 | 🟢 手写；schema 变更必须同步 |

---

#### 修改流程速查

| 想要的变更 | 正确路径 | 错误路径 |
|-----------|----------|----------|
| 新增 / 修改 HTTP 端点 | 加 `schemas/<domain>/*.py` -> 加 `biz/api/v1/routes/<domain>/router.py` 端点 -> 实现 handler 调 service | ❌ 在 handler 写 SQL / 复杂规则 |
| 修改 API 请求 / 响应字段 | 改 `schemas/<domain>/*.py` + 同步 `idl/<domain>.yaml` | ❌ 直接返回 ORM model |
| 修改数据库查询 | 改 `biz/dal/repos/<domain>/**` | ❌ 在 service / handler 写 SQLAlchemy query |
| 修改数据库 schema | 加 Alembic 迁移 -> 执行 -> 更新 `biz/dal/models/` | ❌ 用 ORM `create_all()` 替代版本化迁移 |
| 实现 handler 业务逻辑 | 编辑 `biz/api/v1/routes/<domain>/router.py` 函数体（保持轻薄，调用 service 层） | ❌ 在 handler 写复杂业务规则 |
| 添加全局中间件 | 编辑 `biz/middleware/**` 并在 `main.py` 注册 | ❌ 在 router 内写全局逻辑 |
| 添加路由组依赖 | 在 `biz/api/v1/routes/<domain>/router.py` 的 APIRouter `dependencies=` 声明 | ❌ 在 handler 内重复注入 |
| 调用下游服务 | 经 `clients/` 取装配好的 httpx client | ❌ 在 service 内裸建 client |
| 修改配置项 | 改 `config/config.py`（加字段 + 默认值） | ❌ 在业务层读环境变量 |
| 调整日志格式 / 级别 | 改 `config/config.py` 的日志配置 -> `infra/logging.py` 应用 | ❌ 在业务层手动配 logger |

---

## DDD

基于 DDD 易变性的分解：按"为何变化"组织代码，而非主要按业务实体。`biz/` 目录结构如下：

```text
biz/
├── api             # 接入层（FastAPI router / handler / deps）
├── middleware      # 全局 FastAPI middleware
├── domain          # 稳定业务核心（实体、值对象、仓库接口）
├── policy          # 易变业务规则（纯决策逻辑）
├── service         # 用例 service（编排单 domain）
├── workflow        # 跨 domain 业务流程编排
├── dal             # 数据访问层（SQLAlchemy / repo 唯一收口）
└── shared          # 稳定的应用级工具
```

---

### `biz/dal` - 数据访问层

数据库访问唯一收口。

```text
biz/dal/
├── db.py                # engine 创建 + session_factory
├── session.py           # 事务抽象（TxRunner + ctx 传递）
├── models/              # SQLAlchemy ORM 行模型
└── repos/               # domain 仓库接口的实现
```

| 子目录 / 文件 | 职责 | 修改约束 |
|--------|------|----------|
| `db.py` | engine / session_factory 初始化 | 🟢 手写 |
| `session.py` | 事务封装（TxRunner） | 🟢 手写 |
| `models/` | ORM 行模型，承载 `Mapped[]` 与表定义 | 🟢 手写 |
| `repos/` | 实现 `domain/<domain>` 中定义的仓库接口；唯一可 import `dal/models` 与 SQLAlchemy query 的位置 | 🟢 手写 |

**职责边界**：
- `repo` 把 ORM 行模型转为 domain 对象，禁止把 ORM 模型泄漏给上层
- 业务规则禁止下沉到 repository 实现

#### 数据库 Migration

- **工具**：Alembic，版本表 `alembic_version`。
- **位置**：`alembic/versions/`（🟢 手写）。
- **生成与编辑**：`alembic revision --autogenerate -m "<msg>"` 生成骨架后**必须人工审查并补全**--autogenerate 只能识别增删表/列，无法理解约束、索引、数据迁移语义。复杂变更手写 op 或直接 `op.execute(SQL)`。
- **执行**：`alembic upgrade head`（幂等，已最新则 no-op）。生产可由运维单独跑 CLI，或由 `main.py` lifespan 门控（`config.database.migrate_on_start`，local/dev 默认 true，prod 可关）。
- **与 ORM model 的关系**：Alembic 迁移定义 schema，`biz/dal/models/` 描述 ORM 映射，二者各司其职。schema 变更流程 = 先加迁移 -> 执行 -> 更新 ORM model。
- **禁止**：用 `Base.metadata.create_all()` 替代版本化迁移（仅适合原型，无版本/回滚/历史）。

#### 事务（TxRunner + ctx 传递）

事务边界由 `dal.TxRunner` 统一开启，通过 `contextvars.ContextVar` 隐式传递 session，`AsyncSession` 不泄漏给上层。

```python
# biz/dal/session.py
from contextvars import ContextVar
from typing import Any, Awaitable, Callable
from sqlalchemy.ext.asyncio import AsyncSession

_session_ctx: ContextVar[AsyncSession | None] = ContextVar("dal_session", default=None)

class TxRunner:
    def __init__(self, session_factory: Callable[[], AsyncSession]):
        self._factory = session_factory

    async def run_in_tx(self, fn: Awaitable[Any]) -> Any:
        """在一个 DB 事务内执行 fn；fn 抛异常回滚，正常返回提交。
        session 经 contextvars 向下传递，repo 用 current_session() 取连接。"""
        async with self._factory() as session:
            token = _session_ctx.set(session)
            try:
                result = await fn
                await session.commit()
                return result
            except Exception:
                await session.rollback()
                raise
            finally:
                _session_ctx.reset(token)

def current_session(default: AsyncSession | None = None) -> AsyncSession | None:
    return _session_ctx.get() or default
```

**事务开启点（统一规则）**：

| 场景 | 开启位置 | 说明 |
|------|---------|------|
| 单 domain 用例 | `service/<domain>` 顶部 `run_in_tx` | service 编排单聚合的状态变更 |
| 跨 domain 用例 | `workflow/<flow>` 顶部 `run_in_tx` | workflow 是跨 domain 协调唯一合法入口 |
| service 内部方法 / repo | **不开事务**，只透传 ctx | 永远 `current_session(db)` 取连接 |

**跨 domain 事务模式**（以「点赞」为例：like domain 插入点赞记录 + counter domain 帖子点赞数 +1，同事务保证一致性）：

```python
# biz/workflow/like_flow.py -- 持有 TxRunner，编排两个 domain 的 service
# 点赞 = 记录点赞关系（like）+ 帖子点赞数 +1（counter），二者须同事务，
# 避免「记了点赞但计数没涨」的不一致。
class LikeFlow:
    def __init__(self, tx: TxRunner, like_svc: LikeService, counter_svc: CounterService):
        self._tx = tx
        self._like_svc = like_svc
        self._counter_svc = counter_svc

    async def like(self, ctx, user_id: str, post_id: str) -> None:
        async def _():
            await self._like_svc.like(ctx, user_id, post_id)            # 自动回滚
            await self._counter_svc.incr_likes(ctx, post_id)            # 任一失败整体回滚
        await self._tx.run_in_tx(_())

# biz/service/like/service.py -- 不开事务，用传入 ctx
class LikeService:
    async def like(self, ctx, user_id: str, post_id: str) -> None:
        await self._repo.save(ctx, Like(user_id=user_id, post_id=post_id))

# biz/dal/repos/like/like.py -- 从 ctx 取 session（自动加入上层事务）
class LikeRepo:
    def __init__(self, db: AsyncSession):
        self._db = db

    async def save(self, ctx, like: Like) -> None:
        session = current_session(self._db)
        session.add(like_model_from(like))
        await session.flush()

# biz/service/counter/service.py -- 同样不开事务，透传 ctx
class CounterService:
    async def incr_likes(self, ctx, post_id: str) -> None:
        await self._repo.incr(ctx, post_id, "likes")
```

**规则**：
- `service/like` 不可 import `service/counter`（同层隔离）；跨 domain 一律经 workflow 持有 `TxRunner` 编排。
- `domain` / `policy` 不持有 `TxRunner`，不感知事务。
- `repo` 永远从 ctx 取 session，自身不开事务--这样无论上层是否在事务内，repo 行为一致。
- 跨系统流程（外部副作用）不在持有 DB 事务时调用外部系统，用 saga / outbox（见 workflow 节）。

---

### `biz/domain` - 稳定业务核心

按 domain 子包组织的实体、值对象、domain 错误、domain 事件、仓库接口、稳定不变量。

```text
biz/domain/
├── shared/              # domain 安全的共享基础类型（ID、错误码、值对象）
└── <domain-a>/          # domain-a 实体、值对象、仓库接口
    ├── entity.py
    ├── repository.py    # Protocol（仓库接口，dal/repos 实现它）
    └── errors.py
```

仓库接口用 `typing.Protocol` 定义，domain 不依赖 SQLAlchemy：

```python
# biz/domain/like/repository.py
from typing import Protocol

class LikeRepository(Protocol):
    async def save(self, ctx, like: Like) -> None: ...
    async def find_by_user(self, ctx, user_id: str) -> list[Like]: ...
```

---

### `biz/middleware` - 全局 FastAPI middleware

手写的全局中间件：auth token 解析、request id、日志、tracing、CORS、限流。

**职责**：提取技术上下文（request id、trace id、auth subject、tenant id），通过 `contextvars` 向后传递。

**不允许**：评估复杂业务权限（属于 `policy`）。

> 路由组级依赖应放在 `biz/api/v1/routes/<domain>/router.py` 的 APIRouter `dependencies=`，而非此处。

---

### `biz/policy` - 易变业务规则

按 domain 组织的纯决策逻辑：权限决策、定价/折扣、订单取消、发布、注册等规则。

```text
biz/policy/
└── <domain>/
    └── policy.py
```

**职责**：接收 service 加载好的事实，返回决策结果。

**不允许**：自己加载数据（不查 DB / Redis / 外部 API）。

**协作模式**：`service 加载数据 -> policy.can_xxx(...) -> service 根据决策落地副作用`。

---

### `biz/service` - 用例 service

按 domain 组织的用例编排，是 handler 与 domain / dal 之间的桥梁。

```text
biz/service/
├── command.py                    # 共享 command 类型（可选）
└── <domain>/
    ├── command.py                 # 用例输入
    └── service.py
```

**职责**：
- 接收 command（不收 Pydantic DTO）
- 调用 domain 仓库接口完成持久化
- 为单个用例控制事务
- 通过可观测性接口记录业务事件

**协作边界**：跨 domain 编排归 `workflow`，service 只负责单 domain 的用例。

---

### `biz/shared` - 稳定的应用级工具

应用层稳定通用类型：分页、Clock 接口、ID 生成器、可复用辅助类型。

下游 client 实例装配层放在项目根 `clients/`（`clients/__init__.py` 声明全局变量、`clients/init_<downstream>.py` 初始化），消费 httpx 治理能力。

**命名规则**：用具体名字（`clock.py` / `id.py` / `pagination.py`），禁止 `utils.py` / `common.py` / `helper.py`。

---

### `biz/workflow` - 跨 domain 业务流程编排

```text
biz/workflow/
├── <flow_a>_flow.py
└── <flow_b>_flow.py
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

---

### `biz/api` - 接入层

FastAPI 路由与依赖注入。

```text
biz/api/
└── v1/
    ├── routes/
    │   └── <domain>/
    │       └── router.py      # APIRouter + handler 函数
    └── deps.py                # get_db_session / get_auth_subject / get_config
```

**职责**：
- 解析请求（Pydantic DTO）-> 转 Command
- 调 `service` / `workflow`
- 响应映射（domain 对象 -> Pydantic DTO）

**不允许**：
- 在 handler 写 SQL / SQLAlchemy query
- 在 handler 写复杂业务规则（归 `policy`）
- handler 之间相互 import（同层隔离，见下文）

---

### `schemas/` - API DTO

Pydantic v2 模型，对标 Go 的 `hertz_gen/model`，位于 `biz/` 外（API 契约层）。

```text
schemas/
└── <domain>/
    ├── request.py       # 请求体 / query 参数
    └── response.py      # 响应体
```

**规则**：
- 仅 `biz/api/**` 可 import `schemas/**`
- `service` / `domain` / `dal` 禁止 import `schemas/**`（收 Command，不收 DTO）
- 响应映射在 handler 完成：`domain 对象 -> response DTO`

---

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

- 这件事**单事务**就能做完 -> `service`
- 这件事需要**跨 domain 协调**或**跨系统补偿** -> `workflow`
- service 内**绝不**调用其它 domain 的 service；要跨 domain 必须由 workflow 编排


## 依赖与 import 规则

### Layer Overview

```
api         接入层：Pydantic DTO ↔ Command 翻译，调用 service / workflow
workflow    流程编排：跨 domain / 跨系统的业务流程，处理副作用与补偿
service     应用服务：单用例、单事务，处理一个聚合的状态变更
policy      易变业务规则（纯决策、无副作用）
domain      稳定业务核心（实体 / 值对象 / 仓库接口 / domain 事件 / 不变量）
dal         数据访问：repo 实现 domain 仓库接口
shared      跨 domain 应用工具（分页 / Clock / ID 生成器，与业务无关）
schemas     API DTO（Pydantic v2）
config      配置定义（pydantic-settings，叶子模块）
infra       跨业务基础设施（日志配置等，启动期初始化）
```

**Design goal**：每条 import 都有据可循--稳定核心绝缘、易变规则隔离、ORM 不入业务、配置不散落。

### Dependency Rules

#### 允许方向

```text
api       -> service / workflow / schemas / shared / domain（只读）/ config
workflow  -> 多个 domain 的 service / policy / domain / shared / clients / config
service   -> domain / policy / shared / config
policy    -> domain / shared
dal       -> domain / shared
shared    -> 标准库（不依赖任何业务层）
config    -> 标准库 + pydantic-settings（叶子，不依赖业务层）
infra     -> 标准库 + config（被 main 调用，不被业务层 import）
main      -> biz.api / biz.dal / biz.middleware / config / infra / clients
```

#### 禁止方向

```text
domain   ─X-> schemas / api / dal / shared / config / infra
policy   ─X-> schemas / api / dal / 其它 policy domain 包 / config / infra
service  ─X-> schemas / api / 具体 dal / dal models / 其它 service domain 包 / infra
api      ─X-> 其它 api domain 包 / dal / dal models
workflow ─X-> api / schemas / 原始 SQL / dal models / infra
dal      ─X-> api / schemas / service / workflow / infra
shared   ─X-> domain / policy / service / workflow / dal / api / schemas / config / infra（不允许反向依赖任何业务层）
config   ─X-> 任何业务层 / infra
infra    ─X-> 任何业务层（biz / schemas / clients）
```

#### Rules of Thumb

- 任何文件 import `sqlalchemy` 或 `biz.dal.models` -> 必须位于 `biz/dal/repos` 或 `biz/dal` 初始化代码
- 任何文件 import `schemas/**`（API DTO）-> 仅 `biz/api/**` 允许
- 任何文件 import `clients/**`（下游 client 装配）-> 仅 `biz/api/**`（依赖注入）或 `biz/workflow/**`（编排外部调用）允许；`biz/service/**` 如需外部 client 应经 workflow
- 任何文件 import `infra/**` -> 仅 `main.py` 允许（启动期初始化）
- `domain` 不出现 Pydantic / SQLAlchemy / FastAPI 依赖；不依赖日志、配置、框架
- `policy` 不查 DB / Redis / 外部 API；只接收 service 加载好的事实做决策
- `config` 只定义数据结构，不读环境变量之外的状态；`infra` 只做一次性 setup，不被业务层调用

### 同层隔离

**规则**：同一层级内的子包**不可相互 import**，适用范围：

| 层 | 路径模板 | 强制级别 |
|----|---------|---------|
| domain | `biz/domain/` | 🔴 强制 |
| service | `biz/service/` | 🔴 强制 |
| policy | `biz/policy/` | 🔴 强制 |
| api | `biz/api/` | 🔴 强制 |

#### 禁止的横向依赖

```text
biz/domain/<a>   ─X-> biz/domain/<b>
biz/service/<a>  ─X-> biz/service/<b>
biz/policy/<a>   ─X-> biz/policy/<b>
biz/api/<a>      ─X-> biz/api/<b>
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
| API DTO | `schemas/<domain>/` | HTTP 请求/响应字段、Pydantic 校验规则 |
| Command | `biz/service/<domain>/command.py` | 用例输入语义变化 |
| Domain Model | `biz/domain/<domain>/` | 核心业务概念变化 |
| DB 行 / ORM 模型 | `biz/dal/models/` | 数据库 schema、SQL、ORM 变化 |

转换链路：

```text
API DTO   ──(api 翻译)──->  Command   ──(service 调用)──->  Domain Model
Domain Model  ──(dal/repo 翻译)──->  ORM 行模型
```

### 强制检查

使用现成工具，配置全部收口在 `pyproject.toml` 与 `.pre-commit-config.yaml`：

| 检查项 | 工具 | 命令 | 对标 Go |
|--------|------|------|---------|
| Lint + 格式 | ruff | `ruff check adapter codex goose claude_code` | — |
| 类型 | mypy | `mypy adapter codex goose claude_code` | — |
| 层级依赖 + 同层隔离 | import-linter | `lint-imports --config adapter/pyproject.toml` | archcheck |
| 跨包边界 | check_layers | `python scripts/check_layers.py` | archcheck (monorepo) |
| 死代码 | vulture | `vulture adapter codex goose claude_code --min-confidence 60` | deadcheck |
| 单测 | pytest | `pytest` | — |

```bash
# 从 workspace 根 media_agents/ 执行
ruff check adapter codex goose claude_code && ruff format --check adapter codex goose claude_code \
  && mypy adapter codex goose claude_code \
  && lint-imports --config adapter/pyproject.toml \
  && vulture adapter codex goose claude_code --min-confidence 60 \
  && python scripts/check_layers.py \
  && pytest
```

**import-linter 规则**：`[tool.importlinter]` 在 `pyproject.toml` 用 `forbidden` contract 逐层声明禁止方向（对标上文"禁止方向"表），用 `independence` contract 声明同层 feature 隔离。新增 feature 时在对应 `independence` contract 的 `modules` 列表登记。

**vulture 说明**：基于 AST 的死代码检测，报告未用的函数/类/变量/import/属性，带 confidence 分。`--min-confidence 60` 过滤低置信噪声；不可避免的误报用 `# noqa: Vulture` 或 whitelist 文件豁免。Go 版的"re-export 别名检查"是 Go export 语义特有，Python 无对应物，不纳入。

**pre-commit**：`.pre-commit-config.yaml`（在 workspace 根 `media_agents/`）用 `language: system` 的 local hooks 跑上述全部工具 + `check_layers.py`，用 `uv sync --extra dev` 装的本地工具，不联网拉 hook 仓库。

`media_agents` 是父 git 仓库 `media_agent/` 的子目录，pre-commit 在 git 根执行 hook，故配置里路径带 `media_agents/` 前缀、`lint-imports` 用 `--config` 指向 `adapter/pyproject.toml`（import-linter contracts 在 adapter）。一次性安装：

```bash
# 1) 装全部 workspace 成员 + dev 工具（让 adapter/codex/goose/claude_code 全局可 import）
cd media_agents && uv sync --extra dev

# 2) 装钩子（在父仓库根 media_agent/ 下执行，-c 指向子项目配置）
pre-commit install -c media_agents/.pre-commit-config.yaml
```

之后每次 `git commit` 前自动闭环。`files: ^media_agents/(adapter|codex|goose|claude_code|scripts)/` 过滤保证只改父仓库其他目录（如 `harness/`）的 commit 不触发 Python 检查。手动全量跑：`pre-commit run --config media_agents/.pre-commit-config.yaml --all-files`。

---

## 配置与基础设施

### `config/` - 配置定义

配置使用 `pydantic-settings`，收口在 `config/config.py`，叶子模块（不依赖业务层）。

```python
# config/config.py
from pydantic_settings import BaseSettings, SettingsConfigDict

class DatabaseConfig(BaseSettings):
    dsn: str = "postgresql+asyncpg://localhost/media_agents"
    migrate_on_start: bool = True

class LoggingConfig(BaseSettings):
    level: str = "INFO"
    format: str = "json"           # "json" | "text"

class Config(BaseSettings):
    model_config = SettingsConfigDict(
        env_prefix="MEDIA_AGENTS_",
        env_file=".env",
        env_nested_delimiter="__",
    )
    env: str = "local"
    database: DatabaseConfig = DatabaseConfig()
    logging: LoggingConfig = LoggingConfig()

def load_config() -> Config:
    return Config()
```

**加载链**：
```text
.env (python-dotenv) -> 环境变量 -> pydantic-settings 校验/合并 -> Config 实例
```

**规则**：
- `config/config.py` 是唯一配置入口；其它模块收 `Config` 实例（经 `biz/api/deps.py` 注入），不自己读环境变量
- `main.py` 在启动期 `load_config()`，注入 lifespan 与依赖
- 配置热更新需求暂不纳入（服务治理未启用）；如后续需热更新，由订阅方自行实现，`Config` 实例本身不热更

### `infra/` - 跨业务基础设施

对标 Go 项目的 `hertz_infra/`，但**暂不引入服务治理**（限流 / 熔断 / Consul 配置中心），当前只收日志 setup。

```text
infra/
└── logging.py        # 应用 dictConfig，读 config.logging
```

**职责**：
- 启动期一次性初始化（日志框架、未来可加 tracing / metrics setup）
- 读 `config/` 的配置，不持有业务状态

**规则**：
- 仅 `main.py` 在启动期 import `infra/**`；业务层禁止 import `infra`
- `infra` 不反向依赖 `biz/` / `schemas/` / `clients/`
- 后续如引入服务治理（限流 / 熔断 / 配置中心），新增 `infra/governance.py`，不影响现有结构
