-- 000003_post_counters.up.sql：counter domain —— 帖子计数表。
--
-- 维护帖子的聚合计数（点赞数等）。与 likes 表同事务更新：
-- 点赞 = 插入 likes 行 + post_counters.likes_count +1（见 workflow/like_flow）。

CREATE TABLE IF NOT EXISTS post_counters (
    post_id      VARCHAR(64)     NOT NULL,
    likes_count  BIGINT UNSIGNED NOT NULL DEFAULT 0,
    updated_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (post_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
