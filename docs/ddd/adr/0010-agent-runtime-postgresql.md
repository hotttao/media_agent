# Agent Runtime 统一使用 PostgreSQL 持久化

状态：accepted

Agent Runtime Session、Turn、Message、Universal Event、Session sequence、Chat Status 与
Agent Request 使用独立的 PostgreSQL database/schema 持久化，不再采用 ADR-0009 最初写明的
MySQL。该修订只改变数据库产品，不改变 Agent 对交互事实的唯一所有权、事务边界、业务
sequence、先提交后发布及 Gateway 通过 Agent API 回放的契约。

选择 PostgreSQL 是因为 workspace 已有统一的 PostgreSQL 基础设施、迁移与 E2E 运维路径，
Python Harness 也已规定 SQLAlchemy、asyncpg 与 Alembic。继续引入 MySQL 会增加第二套数据库
镜像、驱动、备份、监控和本地开发路径，却没有领域不变量要求 MySQL 特有能力。Turn 的
“每个 Session 最多一个 running”约束可直接使用 PostgreSQL partial unique index表达，亦与
目标数据模型一致。

没有把 Agent 表放回 Gateway database 或允许 Gateway 直连，因为数据库统一不等于限界上下文
合并。`media_agents` 使用独立 database/schema 与凭证；Gateway 仍只能通过注册、命令、查询和
Replay API 访问 Agent 事实。PostgreSQL 与 Session Event Bus 不组成分布式事务，发布失败仍由
sequence 缺口、周期/终态补查和重连回放恢复。
