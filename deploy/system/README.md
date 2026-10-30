# 宿主机前置依赖

`bash deploy/system/install.sh` 仅适用于 Debian/Ubuntu，显式执行后安装 Git、curl、Python3/venv 和 Electron 所需 Linux 库。不会在本次任务运行。

| 依赖 | 用途 | 安装/更新方式 |
|---|---|---|
| Node.js 24 / npm | Studio 构建 | 使用当前 nvm 安装：`nvm install 24.22.0`，进入任务终端 `nvm use 24.22.0`；不替换现有 Node |
| pnpm 9.15.9 | Studio lockfile v9 | Studio 脚本装在自身 toolchain 目录，不改全局 Corepack |
| Python3（建议3.12） | GPT bridge | 本目录 apt 脚本 |
| Google Chrome | 已登录 ChatGPT + native extension | 保持当前官方 Chrome 安装，使用发行版已配置的 Google 软件源更新；不复制现有 profile |
| Docker Engine + Compose v2 | 基础设施/Penpot | 按 [Docker 官方 Ubuntu 指南](https://docs.docker.com/engine/install/ubuntu/) 配置官方 apt 源后 `sudo apt-get install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin` |
| Go | 编译 Worker | 使用根 `go.work` 与 `media_worker/go.mod` 的 go/toolchain 要求，保留现有 Go 工具链 |

桌面 Studio/Chrome 必须在同一桌面用户的图形会话运行。Docker 数据仍由 Engine 的既有配置管理，本任务不迁移 Docker root；应用持久化目录统一 `/data`。
目前真实 Worker 没有调用 ffmpeg、yt-dlp、MinIO 或 browser-use，不因名称相近额外安装。未来渲染能力引入它们时再登记。
