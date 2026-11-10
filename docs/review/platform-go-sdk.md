# platform-go-sdk 实现与验证

2026-11-10，分支 `platform`。

## 实现

采用经确认的 `packages/platform-go-sdk` 独立 Go module，保留 JavaScript SDK。根入口只做公共门面与实例装配；工作流分别承载应用访问、Agent 访问和会话中继；生成 client、DTO 映射收口在 `biz/shared/client`，凭证缓存和 Caller 验证分别在 `shared/credential`、`shared/caller`。

普通接口复用现有 Auth、Control、Gateway 的 `hertz_gen` 代码与 `hertz_infra/clienthertz`。SSE 因生成 JSON client 不能持续读取响应体，作为单独流式适配器实现。此次未修改 IDL 或生成代码。

架构文档明确 SDK 目录和出站 DTO 转换边界，修正旧的“SDK 内重复生成 Go 协议”说明；archcheck 增加 SDK 禁止依赖服务实现、workflow 禁止依赖具体 client、shared 禁止反向依赖业务层的检查。

## 验证

- `go test ./packages/platform-go-sdk/...` 通过。
- `go test ./harness/tools/archcheck` 通过。
- SDK module 的 `go run ../../harness/tools/archcheck` 通过。
- `go vet ./...`（SDK module）通过。
- 修改的 Go 文件经 gofmt。
- 测试使用真实本地 HTTP 传输连接生成 client，覆盖凭证交换/隔离边界、并发交换与取消、过期刷新、401 不重放、重定向不转发凭证、路径输入、非 JSON 成功响应、会话归属、Origin、JSON 限额机制、SSE 游标及上游取消；Caller 使用平台 signer 验证正常与伪造/过期/错 audience/错 Workspace 等场景。
- 安装 gcc 14.2.0 后，`CGO_ENABLED=1 go test -race ./packages/platform-go-sdk/... ./harness/tools/archcheck` 通过，未发现数据竞争。

## 真实部署

见 [验收记录](platform-go-sdk-runtime-smoke.json)。Go SDK 通过实际 Ory、Auth、Control、Gateway 和受管理 Codex Runtime，完成个人 OAuth 模型连接创建/查询/更新/撤销、幂等 Agent 绑定、Session 创建、真实模型回复 `GO_SDK_OAUTH_OK`、历史读取、SSE 断点续传和下一轮中断。验收程序为 `packages/platform-go-sdk/examples/runtime-smoke`。

本机 Go 纯 Go resolver 不支持当前 `.local` 解析；验收使用临时 loopback 反向代理保持部署 Host 并转发至实际 Gateway IP，无身份逻辑或模型响应 mock。中继 handler 的安全与流式行为由本地真实 HTTP 组合测试验证；这次真实推理通过 Go SDK 直接客户端执行。

验收连接已撤销，Team Grant 已撤销，应用已禁用，临时管理员已删除。对应 Runtime 容器已停止，测试 Session 的 OAuth 文件与模型连接材料已清理，保留不含密钥的验收 ID 和事件类型。

## 使用边界

当前依赖仓库 `go.work` 解析内部 module path；尚未发布为外部可 `go get` 的 SDK。宿主提供应用登录验证、Workspace 映射和业务 MCP 的资源授权。
