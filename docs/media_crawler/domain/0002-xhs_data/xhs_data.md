---
name: xhs_data
description: 跨库只读查询小红书（xhs）爬虫落库的内容数据
dependencies: []
related_workflow: []
---

## 一句话职责
xhs_data 负责按已授权 Crawl Result 映射只读查询 Python 爬虫落库的 MySQL 小红书内容数据。它是内部平台数据适配器，不直接定义 Workspace 可见性。

## 边界

### 做什么
- 只读查 xhs 的 content（note）与 comment
- DTO 直接对齐 xhs_note / xhs_note_comment 真实字段
- 处理 xhs_note 字段特性：varchar 数字排序（CAST）、保留字反引号（`desc`/`type`）

### 不做什么
- 不查其他平台（dy/ks/bili/wb/tieba/zhihu）——各平台独立 domain（如 `dy_data`），互不依赖
- 不写数据——写归 Python 爬虫
- 不触发抓取——归 crawl_job domain
- 不存数据——数据在爬虫的 MySQL，本 domain 仅只读查询

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 查小红书笔记列表 | 筛选+分页+排序查 xhs_note | `QueryXhsNotes` / `ListXhsNotes`（REST）/ `query_xhs_notes`（MCP） |
| 查小红书笔记详情 | 单条 xhs_note | `GetXhsNote`（REST） |
| 查小红书评论 | 按 note_id 查 xhs_note_comment | `QueryXhsComments` / `ListXhsComments`（REST）/ `query_xhs_comments`（MCP） |

## 数据模型

### 只读 MySQL（爬虫库，不自治）
表结构由爬虫 `schema/tables.sql` 定义，本 domain 仅只读查询。

### xhs 表

| 数据类型 | 表 | 业务主键 | 时间字段 |
|:---|:---|:---|:---|
| content | xhs_note | note_id | time |
| comment | xhs_note_comment | comment_id | create_time |
| creator | xhs_creator | user_id | add_ts |

### xhs_note 关键事实（核对自 schema/tables.sql）
- 主键自增 `id`，`note_id` 是业务唯一键（有索引，查询用 note_id）
- `liked_count` / `comment_count` / `collected_count` / `share_count` 是 **varchar(16)**（存 "1.2k"），按其排序必须 `CAST(... AS UNSIGNED)`
- `desc` / `type` 是 **MySQL 保留字**，SQL 中须反引号 `` `desc` `` / `` `type` ``
- `source_keyword` 存在（varchar(255)）

## 能力详细说明

### QueryXhsNotes（ListXhsNotes / query_xhs_notes）
```
1. 入参: filters{ keywords[], ids[], creator_ids[], time_range{start,end}, source_keyword },
         sort{ field: time|liked_count|comment_count, order: desc|asc },
         pagination{ limit 默认50 上限200, offset }
2. 构建 SQL:
   SELECT * FROM xhs_note
   WHERE 1=1
     [note_id IN (ids)]
     [source_keyword = ?  OR  title LIKE %keyword%]
     [time BETWEEN start AND end]
   ORDER BY
     time DESC                           -- sort=time
     CAST(liked_count AS UNSIGNED) DESC  -- sort=liked_count（varchar!）
     CAST(comment_count AS UNSIGNED) DESC
   LIMIT ? OFFSET ?
   注: 涉及 `desc`/`type` 字段须反引号
3. 执行（只读 MySQL 连接池）
4. 返回 XhsNote[]
```

### GetXhsNote
```
1. 入参: note_id
2. SELECT * FROM xhs_note WHERE note_id = ?
3. 返回 XhsNote
```

### QueryXhsComments（ListXhsComments / query_xhs_comments）
```
1. 入参: note_id, pagination
2. SELECT * FROM xhs_note_comment WHERE note_id = ? ORDER BY create_time DESC LIMIT ? OFFSET ?
3. 返回 XhsComment[]
```

## 双库说明
media_crawler 自有数据（jobs）落自治 Postgres，爬虫内容数据在爬虫的 MySQL，本 domain 跨库只读查 MySQL。分库原因：爬虫是独立模块、自带 MySQL schema，media_crawler 不耦合其表结构演进。代价：Go 服务维护两条 DB 连接（PG 读写 + MySQL 只读）。详见 [ADR-0003](../../../adr/0003-crawler-subprocess-isolation.md)。

## 关联关系

### 依赖的 domain
- 无

### 被依赖的 domain
- 无（由 REST handler / MCP service 直接调用）

### 参与的 workflow
- 无（纯只读查询，无跨 domain 协作）

### 关联术语
- Crawl Data / Content·Comment·Creator / Platform（见 [CONTEXT.md](../../CONTEXT.md)）
