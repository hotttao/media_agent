# mcp_runner.py — media_crawler 上层调用的垫片（不修改 Python 爬虫代码）。
#
# 放在 media_agent/media_crawler/script/，运行时由 CRAWLER_WORK_DIR 指向
# MediaCrawlerPro-Python 项目根，加进 sys.path 以 import 其 config / main。
#
# 原 main.py 的 cmd_arg.parse_cmd() 通过 typer 解析 sys.argv 改写 config.* 全局变量。
# 本垫片：
#   1. 重组 sys.argv（从 MCP_CLI_ARGS），让原 typer 正常解析 CLI 参数；
#   2. 从 MCP_* env 注入 CRAWLER_MAX_NOTES_COUNT / SAVE_DATA_OPTION / START_PAGE
#      （这几项既不在 CLI 也不读 env，硬编码在 base_config.py）到 config 全局变量；
#   3. 调 main.main()。
#
# 用法（由 service/crawl_job/runner.go 以 subprocess 调用）：
#   CRAWLER_WORK_DIR=/path/to/MediaCrawlerPro-Python \
#   MCP_CLI_ARGS="--platform xhs --type search --keywords deepseek" \
#   MCP_MAX_NOTES_COUNT=50 \
#     python3 script/mcp_runner.py
import os
import sys
import asyncio

# 1. 把爬虫项目根加进 sys.path，使 import config / import main 命中爬虫代码
work_dir = os.getenv("CRAWLER_WORK_DIR", ".")
sys.path.insert(0, work_dir)

# 清除 proxy 环境变量（subprocess 继承了系统代理如 clash 的 SOCKS，会导致爬虫 httpx
# 报 ImportError: socksio 未装；爬虫直连目标平台，不应走系统代理）。
for _k in [
    "HTTP_PROXY",
    "HTTPS_PROXY",
    "ALL_PROXY",
    "http_proxy",
    "https_proxy",
    "all_proxy",
]:
    os.environ.pop(_k, None)

# 2. 重组 sys.argv，让 cmd_arg.parse_cmd() 的 typer 正常解析
sys.argv = ["main.py"] + os.getenv("MCP_CLI_ARGS", "").split()

import config  # noqa: E402

# 3. 注入硬编码项（不在 CLI、不读 env 的几项）
for key, env in [
    ("CRAWLER_MAX_NOTES_COUNT", "MCP_MAX_NOTES_COUNT"),
    ("SAVE_DATA_OPTION", "MCP_SAVE_DATA_OPTION"),
    ("START_PAGE", "MCP_START_PAGE"),
]:
    v = os.getenv(env)
    if v is not None:
        setattr(config, key, int(v) if v.isdigit() else v)

# 关闭日志文件输出（爬虫 logs/ 目录可能无写权限；日志走 stdout 由 media_crawler 捕获）。
config.ENABLE_LOG_FILE = False

import main  # noqa: E402

# 4. 执行爬虫（main.main 是 async）
asyncio.get_event_loop().run_until_complete(main.main())
