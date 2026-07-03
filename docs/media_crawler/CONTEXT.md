# media_crawler 抓取编排上下文

media 平台统一的社交数据抓取编排与查询微服务：以 Go/hertz 上层通过子进程调用独立 Python 爬虫（MediaCrawlerPro-Python）执行抓取，自身管理抓取任务生命周期，并跨库只读查询爬虫落库的内容数据；对外经 MCP（HTTP transport）供 agent 调用、经 REST 供前端展示。自治 Postgres 存 job；不修改 Python 爬虫代码。

## 业务术语词典

### 抓取编排

**Crawler（爬虫执行器）**:
独立的 Python 工具 MediaCrawlerPro-Python，负责实际抓取与落库。media_crawler 通过 subprocess 调用，不修改其代码。
_禁用替代词_：Spider、Scraper。Crawler 专指外部 Python 工具，**不指 media_crawler 服务本身**（后者是"编排服务"）。

**Crawl Job（抓取任务）**:
media_crawler 发起的一次抓取生命周期，对应一次 `python main.py` 子进程调用。状态 queued→running→completed/failed/cancelled；**全局串行**（同时仅一个 running，防账号/代理风控）。
_禁用替代词_：Task、Run。

**Platform（目标平台）**:
抓取目标社交平台，枚举 xhs（小红书）/dy（抖音）/ks（快手）/bili（B站）/wb（微博）/tieba（贴吧）/zhihu（知乎）。
_禁用替代词_：Site、Channel（channel 语义留给登录通道）。

**Crawler Type（抓取类型）**:
平台数据的抓取方式，枚举 search/detail/creator/homefeed/hot_search。
_禁用替代词_：Mode。

### 续爬与结果

**Checkpoint（断点）**:
Python 爬虫生成的续爬状态文件 `data/checkpoints/{platform}_{mode}_{ts}.json`，记录已爬内容列表 `crawled_note_list`、当前关键词 `current_search_keyword`、当前页码 `current_search_page`、`is_completed`。media_crawler 只读解析，用于续爬与进度推算。
_禁用替代词_：Snapshot、Cursor。

**Crawl Summary（抓取摘要）**:
Python 爬虫结束前 stdout 输出的 `---CRAWL_SUMMARY: {...}---`，仅含 platform/crawler_type/status/error（**无爬取统计**）。media_crawler 捕获解析作为 job 终态。
_禁用替代词_：Result。

### 数据查询

**Crawl Data（抓取数据）**:
Python 爬虫落库的内容数据（笔记/评论/创作者），存于爬虫的 MySQL。media_crawler 跨库只读查询。
_禁用替代词_：Record。

**Platform Data Mapping（平台数据映射）**:
各平台落库表名/字段差异的参考表，如 xhs→xhs_note/note_id/time、dy→douyin_aweme/aweme_id/create_time。各平台独立 domain（如 `xhs_data`）DTO 直接对齐真实表，**不做跨平台运行时映射**；此表仅供 list_platforms 元数据与文档参考。
_禁用替代词_：—。

**Content / Comment / Creator（内容/评论/创作者）**:
三类抓取数据。Content=笔记或帖子主体，Comment=评论，Creator=创作者主页信息。
_禁用替代词_：Note（留给小红书具体表名 xhs_note）、Post。
