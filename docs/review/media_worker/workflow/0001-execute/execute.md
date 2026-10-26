# Execute 结构迁移 review

日期：2026-10-25。依据：[执行流程](../../../../ddd/media_worker/workflow/0001-execute/execute.md)、[迁移计划](../../../../plan/media-worker-doubao.md)。

## 发现与修正

- CLI 原先直接协调注册、恢复、并发轮询、执行、心跳和交付。现由 `execute.Workflow.Run(Command)` 协调，通过 execution 端口注入；CLI 只负责参数、fixture 构造、具体适配器装配、调用、日志输出与退出。
- 原业务协程通过 log.Fatal 退出，绕过其他执行清理。现 Save/Finish/Pending/注册错误返回调用方，致命错误取消并等待所有能力协程；每次任务结束取消并等待其心跳退出。CLI 仍负责非零退出。
- 原 workflow 判断具体 HTTPError。现由 client 映射领域拒绝，policy 决定归档；workflow 不引用具体 client、fixture 或 repo。
- trace 工具独立迁入 shared/correlation，不依赖 Task 或任何业务层。覆盖有效/无效 trace 校验及跨任务上下文隔离。
- archcheck 对归一化目标路径的后续覆盖导致 monorepo 违规漏检，已删除覆盖赋值。真实临时多模块 workspace 回归测试验证 domain→client/repository 被拒绝、domain→shared 可通过。

## 行为证据

fake 端口测试覆盖：注册与恢复先于轮询、恢复不执行 Executor、Save 先于发送、原 Dispatch/Attempt/key 保留、transient/permanent 分类、注册/Pending/Save/Finish 错误返回、拒绝归档及未知回执保留。跨进程适配器测试位于 e2e，包含旧格式结果重启交付和 CLI 注册失败非零退出/响应与 Token 脱敏。

## 门禁

执行环境使用 `/tmp/media-worker-toolchain/go/bin` 的 Go 1.26.3；不修改项目 Go 版本。门禁范围为 media_worker 与修改的 archcheck 工具，不宣称其他服务已验证。

| 检查 | 结果 |
|:---|:---|
| gofmt / git diff --check | 通过 |
| go build ./...（media_worker） | 通过 |
| go test ./...（media_worker，含 e2e） | 通过 |
| go vet ./...（media_worker） | 通过 |
| go run ../harness/tools/archcheck（media_worker） | 通过 |
| go test ./tools/archcheck（harness） | 通过，真实 workspace 允许/拒绝依赖回归 |
| go run ../harness/tools/deadcheck -filter=media_agent/media_worker | 通过（完整 deadcode + alias 检查） |
| CGO_ENABLED=1 go test -race ./...（media_worker） | 通过，含跨进程 e2e |

初始 shell 无 Go 与 C 编译器；下载 Go 1.26.3 至 `/tmp/media-worker-toolchain`，并将 Debian C 工具链包校验 SHA-256 后解包至 `/tmp/media-worker-cc`，使用临时 wrapper 启用 cgo，完成 race；未安装系统包。

Go workspace 下 deadcheck 的默认 `go list -m` 会列出多个模块；本次显式指定 filter，避免依赖默认过滤器判定本模块。

## 确认边界

P1 代码和文档已完成；R1 在最终门禁记录后提交用户确认。P2–P5 和用户 fork 均保持未启动：未连接 studio，未消费真实队列，未执行生成或上传部署验收。用户确认后才能继续完整迁移；后续还必须验证同次不重复提交、恢复资格、确切会话与产物绑定。
