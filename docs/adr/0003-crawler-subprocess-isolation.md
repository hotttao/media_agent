# media_crawler 以子进程隔离调用 Python 爬虫

media_crawler 不在进程内 import 爬虫代码，而是为每个 Crawl Job 起独立子进程 `python mcp_runner.py`，配置经 CLI 参数 + 环境变量注入。原因是爬虫的 `config.*` 是模块级全局可变状态（`cmd_arg.parse_cmd()` 解析后直接改写 `config.PLATFORM/KEYWORDS/CRAWLER_TYPE` 等），同一进程内并发跑两个爬虫会互相覆盖配置；子进程天然隔离这些全局状态，进程退出即清理，且零侵入不修改爬虫代码。备选的"进程内 + 全局锁串行"虽也能避免并发覆盖，但爬虫退出后的全局状态残留、`media_crawler_db_var` ContextVar 跨任务继承等仍是隐患，子进程最干净。
