基于当前框架需要调研一个 数据库自动 migrate 的方案。server suite 启动的时候，要先检验数据库迁移是否完成。没完成要先执行。

当前的架构规范，我使用的 gorm 作为数据库的封装，但是 @harness/contributing/architecture.md 架构文档里，没有对如下两个事情作约束:
1. 数据库 migration 方案，需要在服务启动时候自动检查数据库是否迁移完成
2. 跨 domain 如何使用事务

以 media_example 服务为例子，加入我有两个 domain A，B，他们各自都维护一个计数(对应两个表)。在某一个业务里，我希望在一个事务里对他们同时 +1 保证一致性。应该如何实现。

---

## 调研结论与决策（2026-06-29）

### 决策
1. **Migration 工具**：`golang-migrate/migrate`（SQL-first，版本表 `schema_migrations`，可回滚）。弃用 GORM AutoMigrate（无版本/回滚）。
2. **事务开启点**：方案 X——事务统一由 `dal.TxRunner.RunInTx` 开启；单 domain 用例在 `service` 顶部、跨 domain 在 `workflow` 顶部开启；service 内部方法 / repo 不开事务，只经 `context.Context` 透传 tx。
3. **先改架构文档**：约束已补进 `harness/contributing/architecture.md`「biz/dal」节（数据库 Migration + 事务 TxRunner + ctx 传递）。

### Migration 方案
- 位置：`<app>/migrations/*.sql`（项目根目录，手写）。
- 加载：运行时从磁盘读（不 embed）；`dal.Migrate(dir, cfg)` 用 `os.DirFS(dir)` + iofs。部署需带 migrations 目录，或 prod 用 migrate CLI 单独跑。
- 启动期：`main.go` 在 `dal.NewDB` 后、`NewServerSuite` 前调 `dal.Migrate("migrations", cfg)`，幂等（已最新 no-op）。`database.migrate_on_start` 门控（local/dev true，prod 可关）。
- 独立连接：Migrate 按 dsn 新建独立 *sql.DB，不碰业务 gorm 连接（m.Close() 会关驱动）。
- schema 变更流程：加 migration SQL → 执行迁移 → 重跑 GORM Gen。
- 不放 serverhertz.ServerSuite（hertz 治理层不知业务 DB）。

### 跨 domain 事务方案（以「点赞」为例）
- `dal/transaction.go`：`TxRunner` 接口 + `FromContext(ctx, db)`；tx 经 ctx 隐式传递，`*gorm.DB` 不泄漏上层。
- repo 永远 `dal.FromContext(ctx, db)` 取连接（ctx 带 tx 用 tx，否则默认 db）。
- 点赞 = like domain 插入点赞记录 + counter domain 帖子点赞数 +1；`service/like` 不 import `service/counter`；`workflow/like_flow` 持 `TxRunner` 编排两者，任一失败整体回滚。
- `domain`/`policy` 不持 `TxRunner`，不感知事务。

### 待实现（下一步）
- `media_example` 落地：`biz/dal/migrations/` + `dal/{db,migrate,transaction}.go` + like/counter domain 示例 + `workflow/like_flow`。
- `DatabaseConfig` 增 `migrate_on_start` 字段（改 config.proto + `make pb-config`）。
- `media_example/Makefile` 增 `migrate-new` / `migrate-up` / `migrate-down`。
