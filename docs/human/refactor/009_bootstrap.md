# 009 服务初始化与验证规范

## 1. 端口规划

| 服务 | 端口 |
|---|---:|
| `media_auth` | 8001 |
| `media_workflow` | 8002 |
| `media_materials` | 8003 |
| `media_crawler` | 8004 |
| `media_gen` | 8005 |
| `media_control` | 8006 |
| `media_agent_gateway` | 8007 |
| `media_plugin` | 8008 |
| `media_storage` | 8009 |
| `media_notification` | 8010 |

## 2. Linux 初始化命令

设计评审通过后，在 Linux 开发机的仓库根目录执行：

```bash
git status --short
go run ./harness/tools/new_server media_control media-control 8006
go run ./harness/tools/new_server media_agent_gateway media-agent-gateway 8007
go run ./harness/tools/new_server media_plugin media-plugin 8008
go run ./harness/tools/new_server media_storage media-storage 8009
go run ./harness/tools/new_server media_notification media-notification 8010
```

执行前先检查现有 `idl/media_control/health.proto`。如果生成器会覆盖已有 IDL，先保留并合并该文件，不重复初始化同名定义。

## 3. 生成与验证

```bash
make hz-model
make hz-client
make swagger-gen
make arch-check
go test ./...
```

初始化后必须检查：

- 新增文件均位于生成器约定目录，未改动无关服务。
- 服务名、模块名和端口与本规范一致。
- 生成代码无手工修改。
- IDL 与 `007_idl.md` 的边界一致。
- `008_migration.md` 中每项能力都有对应 IDL 或明确的外部服务所有者。

当前阶段只记录命令，不执行初始化和业务开发。
