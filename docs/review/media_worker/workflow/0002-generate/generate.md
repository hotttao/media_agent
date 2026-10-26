# Generate 实现审查与真实验收

日期：2026-10-26。范围：P2–P6，承接已经过用户确认的 P1。对照 [spec](../../../../spec/media-worker-doubao.md)、[流程](../../../../ddd/media_worker/workflow/0002-generate/generate.md) 与 Harness 分层规范。

## 实现审查

| 能力 | 所有者与实现 | 状态 |
|:---|:---|:---|
| 注册、轮询、心跳、结果日志重放 | execution / workflow/execute；原网关 wire 和磁盘 DTO | covered |
| 生成资格、稳定意图、引用、下载、上传续接 | workflow/generate；domain 定义端口，client/repository 实现 | covered |
| Studio 专用实例与本机控制协议 | client/doubao/studio + 用户 fork，PID/token/readiness/规范目录哈希证明 | covered |
| 原生参数确认和精确会话观察 | 用户 fork 的 TaskService/Repository、既有 renderer；先保存单次动作意图 | external-owner：doubao-studio |
| Workflow 渲染、DAG、候选审批、原生 Retry | media_workflow；Worker 不推进业务图 | external-owner：media_workflow |
| 企业归属与幂等产物登记 | Finalize 传递可信 Job 企业，Materials 校验可选来源与对象 | external-owner：media_materials |

本次声明能力无 missing。CLI 仅配置与依赖构造；domain 不包含 HTTP/JSON/磁盘/Studio DTO，policy 决定能力与失败类别，跨边界副作用由 workflow 协调。旧 fixture 和日志恢复协议保留。

审查并修正的问题：

- Electron 会覆盖原 argv/environ，不能依赖 `/proc/PID/environ` 证明实例身份。改为私密发现记录与认证 health 的目录 SHA-256、活 PID 和自启动进程身份；错目录证明拒绝。
- renderer 重读 Task 时不能重置正在执行的槽与 busy 事实；新增纯投影刷新。
- 页面视频模式入口需要绑定所属 Doubao guest 的原生事件；修正注入模板的正则转义，保留权限边界。
- 参数确认绑定账号 partition 与确切 conversation URL，输入前后核验，只持久化并尝试一次，响应不明确不重复发送。
- 通用 prompt/images Job 没有 Product/VIP；内部登记增加可信 Job 企业字段，旧来源推导兼容，来源冲突拒绝，受控对象校验和结果回执事务不变。更新协议并重新生成 model/OpenAPI。
- 部署时 Materials 与 internal_worker 的存储公开 origin 不一致；统一为本机可访问地址，通过原生 retry 只恢复登记。

## 技术检查

- media_worker 全包 `go test -race`、`go vet`、构建、模块内 archcheck、deadcheck 通过。跨进程 e2e 覆盖四个检查点提交后的进程退出，恢复保留稳定 Key。
- Harness archcheck 临时 workspace 的允许/拒绝依赖测试通过。
- 修改的 Materials generated_asset service、handler、Workflow finalize_task/shared client 单测与 vet 通过；service、finalize_task、client 的 race 通过。Materials 模块 archcheck 通过。
- Studio `pnpm validate` 通过：60 文件、1156 测试，类型/工程与契约规则/构建通过；lint 为现有阈值内 148 warning、0 error。构建仍有上游 bundle-size 提示。
- media_workflow 模块全量 archcheck 有既有违例：service/job 依赖 dal/compiler/executable，service/template 与 service/workflow 依赖 dal。涉及文件未在本次改动，不把该门禁写成全量通过，也不扩大本次迁移修复范围。
- 生成代码由 hz/protoc 生成；未更改的共享 annotation 产物恢复，避免无关工具版本变化。依赖下载改用可访问镜像；临时 Go/C 工具链不进入仓库。

## 真实验收证据

独立账号 profile 完成扫码，首个独立 Studio smoke 视频自动生成；用户明确回复“没有手动操作，自动生成”。生产验收前核对 owner 的待执行队列为空，使用独立 Worker 身份、模板、工作流与 Job。

| 事实 | 值 |
|:---|:---|
| Job | `job_20938866-9ba7-4844-84a4-bf7a4d77f1fe`，SUCCEEDED |
| StepRun | `sr_df855bcb-23e2-4a2f-ad45-badd77bd834f`，SUCCEEDED |
| Conductor | `eea7da8a-93a4-405c-95a3-b16f52b924e7`，COMPLETED |
| Dispatch/Attempt | `dispatch_275d48a7-e110-462d-832f-2c22e106d8d4` / 1 |
| 稳定生成 Key | `ef20a91df1c3def6a0bca5e09492c329994c27bc21ac3de120ac946039ed8197` |
| Generated Asset | `gen_beb5adc8-aedb-4e2a-8c72-36d22c9953fc` |
| Studio Artifact | `artifact-7107d6ca` |
| 校验 | 288520 bytes；SHA-256 `457c94557918b7c48dcef216ee79e5c65416c8a1db6a19f66ec2dc60e2a92b84` |
| 媒体 | H.264，720×1280，5.041667 秒；AAC；容器 5.056009 秒 |

真实 Worker 在引用绑定后停止并以原 Worker ID/凭证/状态重启，复用同一 Studio Task；完成精确下载、网关授权上传与 Complete。随后经普通 User API 审批。初次 Finalize 因缺少来源推导企业失败；修复后首个 retry 暴露部署 origin 不一致，再统一配置并通过原生 retry 完成。最终 Conductor 中 Invoke 一次、HUMAN 一次、Finalize 三次；只有成功 Finalize 登记产物。没有再次领取生成任务或重发提示词。

经公开 GeneratedAsset API 读取并下载产物，Object Path 与检查点一致，返回视频的字节数和 SHA-256 与本地 manifest 完全一致；检查点 closed，待交付日志已清空；Studio 相同 Key 只有一个 done Task。私密运行证据位于本机 `~/.local/state/media-worker/gateway-smoke/acceptance-evidence.json`，含签名 URL 的响应、凭证和浏览器状态不提交。

验收结束停止专用 Worker，避免消费后续任务；Studio 保留已登录可见会话。诊断 CDP 已关闭。后端必要服务保持运行。Docker 上游镜像解析不可达，验收镜像使用本地构建的静态 Go 二进制覆盖既有运行镜像；保留 `before-studio-migration` 标签便于回退，未修改仓库 Dockerfile。

## Fork 与范围

用户仓库 [hotttao/doubao-studio](https://github.com/hotttao/doubao-studio)，分支 `codex/media-worker-studio`，提交 `840117a`，上游基线 `7eeb7b67a518e47ee4296c36e5c5f17942a9cff0`。仅源代码、测试和集成文档进入提交，未上传 profile/token/媒体。

结论覆盖单一 `doubao.i2v.first_frame_prompt@1` 的真实成功链路及同次进程恢复；平台挑战、额度不足和动作结果不确定保持人工/待恢复，不通过盲重发绕过。多图、生图、其他模型和不同平台页面版本需要单独验收。
