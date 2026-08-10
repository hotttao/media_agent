-- =====================================================================
-- media_auth — 0002_teams: 团队与成员、邀请
-- =====================================================================
-- 范围:team domain 三张表(teams / team_members / team_invites)。
-- 来源:控制面 migrations/001_init.sql 中 team 三表段(teamwork 不迁)。
-- 语义:team 是 user 之上组织 user 的命名分组,仅作 Resource Grant ACL 基础数据;
--      不承担多租户隔离、不承担 billing。详见 docs/ddd/media_auth/domain/0007-team/team.md。
-- =====================================================================

BEGIN;

-- ─────────────────────────────────────────────────────────────────────
-- 1. teams —— 团队主表
-- ─────────────────────────────────────────────────────────────────────
-- id 由应用层 generateId() 产生(短随机串),非自增。
-- created_by: 创建者 user id(创建时自动成为首任 admin)。
-- 删除级联清 team_members + team_invites(见下两表 FK ON DELETE CASCADE)。
CREATE TABLE IF NOT EXISTS media_auth.teams (
    id          text PRIMARY KEY,
    name        text NOT NULL,
    description text,
    created_by  text NOT NULL REFERENCES media_auth.users(id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- ─────────────────────────────────────────────────────────────────────
-- 2. team_members —— user ↔ team 多对多(含角色)
-- ─────────────────────────────────────────────────────────────────────
-- role: 'admin' | 'member'(CHECK 约束)。
-- 复合主键 (team_id, user_id):一 user 在一 team 至多一条成员记录。
-- ON DELETE CASCADE:team 删则成员清;user 删则其成员关系清。
-- 末位 admin 保护由应用层在写前校验(UpdateMemberRole / RemoveMember)。
CREATE TABLE IF NOT EXISTS media_auth.team_members (
    team_id   text NOT NULL REFERENCES media_auth.teams(id) ON DELETE CASCADE,
    user_id   text NOT NULL REFERENCES media_auth.users(id) ON DELETE CASCADE,
    role      text NOT NULL,
    joined_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT team_members_role_check CHECK (role IN ('admin', 'member')),
    PRIMARY KEY (team_id, user_id)
);

CREATE INDEX IF NOT EXISTS team_members_user_idx ON media_auth.team_members (user_id);

-- ─────────────────────────────────────────────────────────────────────
-- 3. team_invites —— 邀请链接(tinv_xxx)
-- ─────────────────────────────────────────────────────────────────────
-- token: 'tinv_' + base64url(randomBytes(24)),PK,全局唯一。
-- expires_at: 可空(空=不过期);应用层判过期。
-- ON DELETE CASCADE:team 删则邀请清。created_by 不级联删(历史邀请留痕)。
-- 接受后 token 不删(留痕 + 防合法邀请反复 retry);如需"一次性消费"再加 consumed_at。
CREATE TABLE IF NOT EXISTS media_auth.team_invites (
    token      text PRIMARY KEY,
    team_id    text NOT NULL REFERENCES media_auth.teams(id) ON DELETE CASCADE,
    created_by text NOT NULL REFERENCES media_auth.users(id),
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS team_invites_team_idx ON media_auth.team_invites (team_id);
CREATE INDEX IF NOT EXISTS team_invites_expires_idx ON media_auth.team_invites (expires_at);

COMMIT;
