# MediaCrawlerPro MCP Server 研究文档

本目录记录把 [MediaCrawlerPro-Python](https://github.com/MediaCrawlerPro/MediaCrawlerPro-Python) 改造为 MCP Server 的研究与设计成果。

## 背景一句话

原项目只能通过 `python main.py ...` 启动，且所有可变参数（平台、关键词、爬虫类型、URL 列表等）都写在 `config/*.py` 的全局变量里。目标是加一个**上层 MCP Server**，让 agent 通过 tool 调用来执行爬取与查询，**不深入修改原作者代码**。

## 关键决策

- **存储后端 = MySQL**（`db_config.py` 默认 `DB_TYPE=mysql`）。aiomysql 连接池并发安全，对只读查询友好。
- **不修改原代码**：上层通过 `subprocess` 调用 `python main.py`（或薄垫片 `mcp_runner.py`），配置经 CLI 参数 + 环境变量注入；只读查询由上层自有的 MySQL 连接池直读。
- **读写分离**：写（爬虫执行）走子进程 + 全局串行队列；读（数据/配置/断点查询）走上层连接池，可并发。

## 文档索引

| 文档 | 内容 |
|---|---|
| [01-architecture.md](01-architecture.md) | 上层/下层架构、读写分离、配置注入方式、参数可覆盖矩阵、Job 状态模型、新增文件清单 |
| [02-mcp-tools-interface.md](02-mcp-tools-interface.md) | 10 个 MCP Tool 的完整 input/output 协议、(platform, data_type)→表名映射表 |
| [03-open-issues.md](03-open-issues.md) | 改造中遇到的痛点、设计约束、待用户确认的决策点 |

## 时间

- 首次整理：2026-07-03
