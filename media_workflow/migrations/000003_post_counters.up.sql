-- 000003_post_counters.up.sql：counter domain —— 帖子计数表。
--
-- 维护帖子的聚合计数（点赞数等）。与 likes 表同事务更新：
-- 点赞 = 插入 likes 行 + post_counters.likes_count +1（见 workflow/like_flow）。
-- 注意：pg 无 ON UPDATE 语法，updated_at 由 counter repo（gorm UpdateColumn）显式更新。

CREATE TABLE IF NOT EXISTS post_counters (
    post_id      VARCHAR(64) NOT NULL PRIMARY KEY,
    likes_count  BIGINT      NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
