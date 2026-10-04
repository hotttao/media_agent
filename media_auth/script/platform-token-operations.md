# Platform Token 本地切换与维护

仅适用于已接受的独立密文托管方案。普通 compose 默认关闭此监听器；启用必须显式叠加 `docker-compose.platform-token.yml`。私有 8890 不发布宿主端口、不接统一公共网关。API Token 的公开认证链路不变。

## 首次准备

从仓库根目录执行，目录必须不存在；脚本遇到已有或部分材料拒绝覆盖。仅本地测试使用该 CA，生产使用受控 PKI/Secret 管理。

```bash
bash media_auth/script/init-platform-token-test-materials.sh \
  /home/tao/code/github/media_agent/media_auth/.secrets/platform-token
go -C media_auth build -o /tmp/media-agent-platform-token-migrate ./tools/platform-token-migrate
```

目录 0700、文件 0600。将 `custody-v1.key` 备份至仓库外的受控目录，验证副本一致性。当前测试备份在 `/data/media_auth/platform-token-backup/custody-v1.key`。密钥丢失则密文不可恢复，不能通过重新生成密钥修复；恢复数据库时必须恢复匹配的版本密钥。

Auth 仅挂载服务端证书/私钥、可信 CA 和托管密钥，均只读且禁止自动创建缺失路径；不挂载 CA 签发私钥或客户端私钥。客户端证书约定身份分别为 `media_agent_gateway` 和 `media_control_schedule_worker`。证书 90 天有效；本地 CA 365 天。更新证书需受控签发和重启接线，不擅自生成替代托管密钥。

## 旧数据原子迁移

有旧数据的升级必须分阶段：先使用只包含 0015/0016 的过渡版本，`PLATFORM_TOKEN_ENABLED=false` 正常启动 Auth；再停止 Auth 及其他旧凭据写入者。维护窗口内先演练，再显式提交，命令不输出 Secret、Hash、密文或 DSN。包含 0017 的最终版本只能在旧 Platform 行全部转移后启动，不能拿它代替过渡版本；即使是已撤销的旧 Platform 行也会阻断 0017。新建空库可直接应用全部迁移。

```bash
/tmp/media-agent-platform-token-migrate \
  --config /home/tao/code/github/media_agent/media_auth/conf/local.yaml \
  --key-file /home/tao/code/github/media_agent/media_auth/.secrets/platform-token/custody-v1.key \
  --key-version v1
# 演练默认回滚。检查结果后使用完全相同参数，并追加 --apply。
```

`--apply` 同事务保留 ID、User、Hash、创建/过期/撤销历史，验证加密回读后才删除已验证旧 Platform 行，不改变普通旧 ServiceToken。迁移失败整批回滚，不跳过错误行；重跑已完成迁移返回零行。提交后不得回退到旧镜像或明文写入者。

0017 将剩余普通旧凭据保留为只读历史，不再是认证来源。语句级 `ENABLE ALWAYS` 触发器拒绝 INSERT/UPDATE/DELETE/TRUNCATE（包括影响零行的写入）；数据库管理员的 DDL 权限不在此保护边界内。该迁移不删除、不改写历史字段，down 明确拒绝自动回退，不能通过降级恢复旧认证。专用 Platform 维护投影仍可执行零行重跑，不注册旧业务 CRUD。

## 启动与验收

未跟踪的 `media_auth/.env.platform-token.local` 设置 `PLATFORM_TOKEN_ENABLED=true`；已有 Agent Gateway 专用凭据仍从其私有 env 文件读取。

```bash
INFRA=unit docker compose \
  --env-file media_auth/.env.agent-gateway.local \
  --env-file media_auth/.env.platform-token.local \
  -f media_auth/docker-compose.yml \
  -f media_auth/docker-compose.platform-token.yml \
  up -d --no-deps media_auth
```

启动检查版本、dirty、残留旧 Platform 行和全部历史密文完整性；任一失败即拒绝启动，无明文回退。审计仅写私有 0600 文件 `media_logs/media-auth/platform-audit/events.jsonl`，无需观测后端。容器以 root 写入时宿主普通用户不可读；通过授权审计身份运行验收，不 chmod 放宽读取。

真实 HTTP 门禁位于 `media-cli/e2e/platform_token_mtls_e2e_test.go`，四项环境变量见 `harness/contributing/e2e.md`。创建随机密码的独立测试 User，获取/内省/轮换仅操作该 User；轮换后凭据一小时过期。不会重置或轮换已有测试账号。用例读取实际落盘元数据，验证 no-store、purpose 限制、无证书握手拒绝、普通 HTTP 拒绝及旧凭据失效，不打印或保存 Secret。

此门禁不替代 API Token HTTP 回归及 CLI 跨进程门禁，也不代表未来 Gateway/Control 调用适配器已接线。旧路由退役和历史归档另有专用门禁；真实企微回调和真实用户删除仍需各自验收。
