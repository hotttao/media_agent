-- 000002_likes.up.sql：like domain —— 点赞关系表。
--
-- 记录「谁给哪个帖子点了赞」。与 post_counters（counter domain）配合：
-- 点赞 = 插入 likes 行 + post_counters.likes +1，二者同事务（见 workflow/like_flow）。

CREATE TABLE IF NOT EXISTS likes (
    id         BIGSERIAL   PRIMARY KEY,
    user_id    VARCHAR(64) NOT NULL,
    post_id    VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uk_likes_user_post UNIQUE (user_id, post_id)  -- 同一用户对同一帖子只能赞一次
);

CREATE INDEX IF NOT EXISTS idx_likes_post ON likes (post_id);
