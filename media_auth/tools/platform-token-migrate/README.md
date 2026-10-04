# Platform Token 离线迁移

仅用于旧 `service_tokens` 中Platform记录向独立密文表转移，不迁移普通ServiceToken或Talos API Token，不提供公开HTTP接口。包含已撤销历史，保留ID、用户、Hash与生命周期时间。

执行前须应用0015 schema、准备私有32字节密钥文件和对应版本，并停止旧Platform确保/读取/撤销调用方；提交后须切换到新链路，否则旧服务会重新创建明文。工具不替你停机、迁移schema、生成密钥或切换服务。当前运行时切换尚未完成，不要对运行中的旧链路执行`--apply`。

从仓库根目录执行演练（环境变量须先加载；`--config`不自动读取.env）：

```bash
go -C media_auth run ./tools/platform-token-migrate \
  --config conf/local.yaml \
  --key-file /run/secrets/media-auth/platform-token.key \
  --key-version platform-v1
```

默认完整执行校验/写入/回读/删除，最终回滚。只有加`--apply`才提交；默认超时2分钟，可用`--timeout`调整至最多30分钟。两侧表写入在事务内锁定，读取不禁止。失败整批回滚，不跳过坏记录，不覆盖冲突目标。提交结果不明确时重新演练并核对计数，不恢复明文。

密钥必须为regular文件，32字节原始密钥且无group/other权限；不支持符号链接，不自动生成替代密钥。现有目标必须可由提供的版本解密且事实完全一致，否则拒绝。密钥须独立备份并保留，不提交Git。输出只有`verified/created/reused/committed`计数，演练计数不表示实际写入；失败只输出固定安全错误。不要把Secret或DSN作为命令行参数。

隔离测试包含独立二进制的默认演练、显式提交、重跑和安全错误输出：

```bash
go -C media_auth test -tags=e2e -race ./e2e -run '^TestPlatformTokenMigration' -count=1 -v
```
