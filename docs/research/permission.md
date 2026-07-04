# 资源级 ACL 的 SQL 实现（以 skill 为例）

> 本文逐段展示 `control-plane/src/services/skill-repository.ts` 里和 ACL 相关的几段真实 SQL，逐行解释用到的 PostgreSQL 语法。读完应该能看懂代码在做什么，以及为什么这么写。

---
## 统一权限模型

```
                    资源 (Prompt / Skill / Template / Provider)
                    │
        ┌───────────┼───────────┐
        ▼           ▼           ▼
    visibility   grants     my_permission
        │           │           │
        │           │           ▼
        │           │     推导规则:
        │           │     1. isOwner → 'owner'
        │           │     2. team grant + editor → 'editor'
        │           │     3. team grant + viewer → 'viewer'
        │           │     4. visibility=public 可见 → 'public'
        │           │
        ▼           ▼
  private/team/  team_id +
  public         permission
                 (viewer/editor)
```

**visibility 三档**：

| 值 | 谁能看到 |
|----|----------|
| `private` | 仅 owner |
| `team` | owner + 通过 team grant 共享的团队成员 |
| `public` | 所有人 |

**my_permission 四档**（前端根据这个决定按钮显示/隐藏）：

| 值 | 能做什么 |
|----|----------|
| `owner` | 全部操作：编辑、删除、分享、改 visibility |
| `editor` | 编辑内容，不能删除/分享/改 visibility |
| `viewer` | 只读 |
| `public` | 只读（通过 public visibility 看到） |

**特例**：Provider 的 team grant 只允许 `viewer`（`control-plane/src/routes/providers.ts:62-73`），因为编辑 provider 会暴露上游凭证。

## 1. 三张表

```sql
-- 资源本身
CREATE TABLE skills (
  id          uuid PRIMARY KEY,
  user_id     text NOT NULL,                         -- 谁拥有
  visibility  text NOT NULL
    CHECK (visibility IN ('private','team','public')),
  name        text NOT NULL,
  description text DEFAULT '' NOT NULL,
  category    text,
  ...
);

-- 把资源共享给某个 team
CREATE TABLE skill_grants (
  skill_id    uuid REFERENCES skills(id) ON DELETE CASCADE,
  team_id     text REFERENCES teams(id) ON DELETE CASCADE,
  permission  text DEFAULT 'viewer'
    CHECK (permission IN ('viewer','editor')),
  granted_by  text NOT NULL,
  granted_at  timestamptz DEFAULT now(),
  PRIMARY KEY (skill_id, team_id)                    -- 同一 skill × 同一 team 只有一条
);

-- 用户加入了哪些 team
CREATE TABLE team_members (
  team_id  text,
  user_id  text,
  role     text,
  PRIMARY KEY (team_id, user_id)
);
```

**为什么是这三张**：
- `skills` 有 `visibility` 字段决定"默认谁能看"（私有/团队/公开）
- `skill_grants` 是**显式授权**：owner 主动签发"我愿意让这个 team 看"
- `team_members` 是**受众**：用户和 team 的多对多关系

注意 `skill_grants` 的主键是 `(skill_id, team_id)`——同一个 skill 同一个 team 只能有一条 grant。要更新权限就用 `INSERT ... ON CONFLICT DO UPDATE`。

---

## 2. `listVisibleToUser`：列出我能看到的 skill

源码：`skill-repository.ts:181-238`。作用是：给定一个用户 id，返回他能看到的所有 skill，每条还附带"我对它是什么权限"。

完整 SQL：

```sql
WITH my_grants AS (
  SELECT sg.skill_id, sg.team_id, sg.permission, t.name AS team_name
    FROM skill_grants sg
    JOIN team_members tm ON tm.team_id = sg.team_id AND tm.user_id = $1
    JOIN teams t ON t.id = sg.team_id
),
visible AS (
  SELECT s.id FROM skills s WHERE s.user_id = $1
  UNION
  SELECT s.id FROM skills s WHERE s.visibility = 'public'
  UNION
  SELECT skill_id FROM my_grants
)
SELECT ${META_COLS},
       (s.user_id = $1) AS is_owner,
       COALESCE(
         (SELECT json_agg(json_build_object(
            'id', mg.team_id, 'name', mg.team_name, 'permission', mg.permission))
            FROM my_grants mg WHERE mg.skill_id = s.id),
         '[]'::json
       ) AS shared_via_teams,
       COALESCE(
         (SELECT MAX(CASE permission
                       WHEN 'editor' THEN 2
                       WHEN 'viewer' THEN 1 END)
            FROM my_grants mg WHERE mg.skill_id = s.id),
         0
       ) AS grant_rank
  FROM skills s
  JOIN users u ON u.id = s.user_id
  JOIN skill_sources ss ON ss.id = s.source_id
 WHERE s.id IN (SELECT id FROM visible)
   AND ($2::text IS NULL OR s.name ILIKE '%' || $2 || '%' OR s.description ILIKE '%' || $2 || '%')
   AND ($3::text IS NULL OR s.user_id = $3)
   AND (
     cardinality($4::text[]) = 0
     AND $5::boolean = false
     OR s.category = ANY($4::text[])
     OR ($5::boolean AND s.category IS NULL)
   )
   AND ($6::text IS NULL OR s.visibility = $6)
 ORDER BY s.name;
```