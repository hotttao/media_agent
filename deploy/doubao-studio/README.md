# Doubao Studio

`bash deploy/doubao-studio/install.sh` 安装/重建固定 fork 提交 `15e2e7e8e7820626ed1f8433f80ed01d9e047ce1`。
更新时显式指定 `STUDIO_REVISION=<完整提交>` 再执行同一脚本；拒绝覆盖 dirty checkout。当前固定提交包含多图、确认发送与恢复修复；上游 main 不能直接替换。
该提交目前是本地提交；从远端新装前须先将它推送至 fork。也可从现有本机 Git 仓库安装：

```bash
STUDIO_REPOSITORY=/home/tao/code/github/doubao-studio bash deploy/doubao-studio/install.sh
```

脚本仍会校验固定提交并复制到约定部署目录，不修改原工作仓库。取不到提交会失败，不会退回旧版本。

- 源码/构建：`/home/tao/deploy/doubao-studio/source`。
- 专用用户数据/登录态：`/data/doubao-studio/user-data`（0700）。
- 输入与运行结果：由 Worker 的 `/data/media-worker/doubao` 状态命名空间管理。
- 前置：Node.js 24、npm、桌面图形会话、Electron Linux 系统库，见 system。

构建完成后配置 Worker（不要同时手工启动第二个相同用户目录的 Studio）：

```bash
export MEDIA_WORKER_STUDIO_DIRECTORY=/home/tao/deploy/doubao-studio/source
export MEDIA_WORKER_STUDIO_ELECTRON=/home/tao/deploy/doubao-studio/source/node_modules/electron/dist/electron
export MEDIA_WORKER_STUDIO_USER_DATA=/data/doubao-studio/user-data
# MEDIA_WORKER_STUDIO_ACCOUNT_ID 由用户扫码后选择，写入私密运行配置。
```

Worker 使用 `--local-control` 启动 Studio，不需要 CDP 端口或 Docker。
配置模板见 [Worker 公开配置模板](../../media_worker/conf/local.example.yaml)。本次没有搬动旧用户目录；已有账号迁移需停进程后保留完整目录及权限，不能直接让运行中实例切换路径。
