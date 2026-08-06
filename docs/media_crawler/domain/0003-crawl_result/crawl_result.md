---
name: crawl_result
description: Crawl Job 在 Workspace 内可访问的平台结果索引
dependencies:
    - crawl_job
    - xhs_data
related_workflow:
    - media_workflow_execution
---

## 一句话职责

Crawl Result 把外部爬虫数据标识绑定到发起 Crawl Job 和 Workspace，形成可授权查询的结果事实。

## 边界

### 做什么
- 从爬虫摘要登记内容、评论或创作者的外部标识。
- 按 Workspace、Job 和平台分页查询结果。
- 授权后通过平台 Data Adapter 加载详细内容。

### 不做什么
- 不复制第三方平台完整原始数据。
- 不允许业务接口绕过 Result 映射查询共享爬虫库。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 登记结果 | 将外部数据标识绑定 Crawl Job | `RegisterCrawlResults` |
| 查询结果 | 按 Workspace/Job 查询索引 | `CrawlResultService.List/Get` |
| 加载详情 | 授权后调用平台 Adapter | `ResolveCrawlResult` |

## 数据模型

### `crawl_results`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Crawl Result ID |
| workspace_id | text | 所属 Workspace |
| crawl_job_id | text | 来源 Crawl Job ID |
| platform | text | xhs/douyin 等来源平台 |
| result_type | text | content/comment/creator |
| external_id | text | 平台或爬虫库中的业务标识 |
| discovered_at | timestamptz | 本次任务发现时间 |
| metadata | jsonb | 标题、摘要等轻量索引信息 |
| source_missing | boolean | 外部原始数据是否已不可用 |
| created_at | timestamptz | 创建时间 |

### UUID 生成规则

Result 使用 ULID；平台 external_id 原样保存。

## 能力详细说明

### ResolveCrawlResult

1. 按 result ID 加载真实 workspace_id 并完成授权。
2. 根据 platform/result_type 路由只读 Adapter。
3. Adapter 返回当前外部数据；不存在时 Result 保留但标记 source_missing。
