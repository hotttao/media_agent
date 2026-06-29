-- 000001_init.up.sql：初始化 schema_migrations 之外的首个业务表。
--
-- 这里建立 hello 例子的问候记录表（与现有 hello domain 对齐的占位）。
-- 后续 migration 增量加表（见 000002_likes / 000003_post_counters）。
--
-- 约定（harness/contributing/architecture.md「biz/dal — 数据库 Migration」）：
--   - 每个 migration 一个 .up.sql + .down.sql，纯 SQL，由 golang-migrate 顺序执行；
--   - schema 变更 → 加 migration → 执行 → 重跑 GORM Gen。

CREATE TABLE IF NOT EXISTS greetings (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name       VARCHAR(64)     NOT NULL DEFAULT '',
    body       VARCHAR(255)    NOT NULL DEFAULT '',
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_greetings_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
