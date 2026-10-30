# Penpot

包含 frontend/backend/exporter/admin-console/MCP、PostgreSQL 15、Valkey、Mailpit，沿用当前 2.18 部署系列和官方拓扑。
[官方部署说明](https://help.penpot.app/technical-guide/getting-started/) · [上游 Compose](https://github.com/penpot/penpot/blob/develop/docker/images/docker-compose.yaml)。

```bash
python3 deploy/penpot/prepare.py
# 随后核对 /home/tao/deploy/penpot/.env 的公开 URL 和监听地址。
cd /home/tao/deploy/penpot
docker compose config --quiet
docker compose pull
docker compose up -d
```

`prepare.py` 只在新目录生成 Compose 和0600随机密钥 `.env`，不创建数据目录，不启动容器。默认仅本机9001；需要 LAN 时显式设置 `PENPOT_BIND=0.0.0.0`，URL 与实际入口一致。
这是与当前开发环境对应的 HTTP 配置；公网部署需 HTTPS、secure cookies、邮件验证和真实 SMTP。
数据全部 bind mount 在 `/data/penpot/{postgres,assets,valkey,mailpit}`，部署前按镜像 UID 创建并赋权。多用户 MCP 使用镜像默认命令，不复制现有本机单用户命令覆盖。

**当前 `/home/tao/deploy/penpot` 已有部署，脚本会拒绝覆盖。** 更新先指定临时 `DEPLOY_ROOT` 准备并对比；保留已有 SECRET_KEY、数据库密码和用户/文件数据。
当前实例使用 Docker named volumes；改成 `/data` 必须先停写、备份、确认旧卷名，显式迁移后再切换挂载。此任务没有执行迁移，也没有将现有部署中的密钥、账号或设计文件复制到仓库。
升级时修改运行 `.env` 的 PENPOT_VERSION，备份数据库和 assets 后显式 pull/up；所有 app 镜像版本保持一致。数据库迁移后回滚需恢复配套备份。
