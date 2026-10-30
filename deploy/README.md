# 外部工具部署元数据

只维护原仓库缺少的工具安装/更新配置和 Penpot。已有 `docker-compose/`、模块 Compose 或 Dockerfile 的组件继续使用原入口，不在这里重复包装。

- 工具源码、二进制及运行 Compose：`/home/tao/deploy/<工具>`。
- 应用数据及私密配置：`/data/<工具>`。
- 仓库只放脚本、公开配置模板和版本信息；不放实际账号、Token、Cookie、安装产物或运行数据。
- Chrome profile、native-host manifest 和 CLI 官方登录文件保留平台规定的位置，不自动搬动现有登录态。

## 安装与更新

从仓库根执行。下列脚本仅在显式运行时安装或准备配置，本次没有实际部署。

| 工具 | 入口 | 当前基线 |
|---|---|---|
| [Doubao Studio](doubao-studio/README.md) | `bash deploy/doubao-studio/install.sh` | 用户 fork `15e2e7e`，包含多图与确认发送修复 |
| [chrome-use](chrome-use/README.md) | `bash deploy/chrome-use/install.sh` | v1.5.266，扩展授权由用户操作 |
| [image-use](image-use/README.md) | `python3 deploy/image-use/install.py` | bridge 固定 commit + SHA-256 |
| [Penpot](penpot/README.md) | `python3 deploy/penpot/prepare.py` | 2.18，含自身 PG/Valkey/Mailpit 与 MCP |
| [宿主前置依赖](system/README.md) | `bash deploy/system/install.sh` | Python、Git、Electron Linux 库；其他工具安装说明 |

安装脚本支持 `DEPLOY_ROOT`，数据路径遵循 `/data`。本目录不负责安装 Codex、Claude Code 或 OpenCode。
更新沿用各工具安装入口并显式选定版本；image-use 更新需同步 bridge 的固定哈希和兼容性。Studio 当前固定修复提交尚在本机，可按其 README 从本机 Git 仓库安装。

Penpot 配置准备支持 `DEPLOY_ROOT` / `DATA_ROOT`，生成0600私密 `.env` 并拒绝覆盖已有部署。更新时先在临时目录准备，review 后合入运行目录，保留现有密钥与数据库。现有 named volume 与 Worker 状态不会自动迁移。

## 依赖检查结果

检查了 Worker、GPT bridge、`agent_local` 配置、Agent Dockerfile、旧爬虫配置与现有 Compose：

- 豆包视频链路需要 Studio；GPT 成图链路需要 image-use、chrome-use、Chrome 与扩展。上述安装入口已补齐。
- PostgreSQL、Ory、Traefik、Conductor、SeaweedFS、观测栈与 RabbitMQ 已有 Compose，继续复用。Goose 和服务端 Codex/Claude adapter 已有 Dockerfile，也不重复增加入口。
- 旧爬虫配置指向的 MediaCrawlerPro-Python 目录当前不存在。没有对应授权源码，无法确认其完整上游依赖；这里记录缺口，不添加未经验证的安装方案。
- 当前真实 Worker 没有直接使用 ffmpeg、yt-dlp、MinIO 或 browser-use，无需额外安装。对象存储使用已有的 SeaweedFS。

## 校验

`python3 deploy/check.py` 检查脚本语法，以及 Penpot Compose 解析、私密配置权限和拒绝覆盖行为。只使用临时目录，不安装工具、不启动服务。
GPT bridge 14项回归测试已通过。第三方登录、扩展连接和真实生成仍需实际部署后验收。
