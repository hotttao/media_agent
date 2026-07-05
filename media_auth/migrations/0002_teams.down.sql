-- 0002_teams down: 回滚团队三表(逆序删,先依赖方)。
BEGIN;

DROP TABLE IF EXISTS media_auth.team_invites;
DROP TABLE IF EXISTS media_auth.team_members;
DROP TABLE IF EXISTS media_auth.teams;

COMMIT;
