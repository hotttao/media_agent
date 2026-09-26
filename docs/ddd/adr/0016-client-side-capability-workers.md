# Capability Worker 运行在客户端

Execution Capability 对应的 Worker 统一运行在用户客户端，通过 Conductor Task API 拉取任务并回报结果；服务端 `media_workflow` 只运行 `media_render_template_v1`、结果收口等 Internal Execution Task。客户端以 `media_worker` 工具或常驻服务承载多个 Capability Worker，可访问本地模型、GPU、CLI 与 Provider 凭证，凭证不上传服务端。

该边界支持企业客户把昂贵计算、私有模型和 Provider 凭证留在自己的环境，同时保持服务端 Workflow、Template 和调度模型一致。代价是客户端必须处理注册、心跳、重试、版本兼容与离线恢复；Conductor 中的 Task Definition 和 Capability Version 因而成为双方稳定契约，服务端不得根据在线 Worker 临时改变已发布 Workflow。

联调允许 `media_worker` 使用显式 Mock Profile，根据 capability task 返回从旧系统选定样本冻结的结果。Mock Profile 只能由启动参数开启，输出必须包含来源标识，不能静默替代生产执行。

**状态：accepted**
