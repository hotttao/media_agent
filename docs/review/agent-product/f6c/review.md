# F6c · Claude Code / Goose 真实产品验收

## 验收环境

本机 Docker 单容器 Agent，k3s 保持 inactive / disabled；Docker 扩缩容不在范围。使用用户提供的 SiliconFlow API Key 与真实模型 `deepseek-ai/DeepSeek-V4.1-Flash`，没有模型 mock。Claude Code 2.1.231 使用 Anthropic 兼容接口（基础地址 https://api.siliconflow.cn），Goose 1.52.0 使用 OpenAI 兼容接口（基础地址 https://api.siliconflow.cn/v1）。

从正式入口登录现有用户，在界面创建 `Claude · SiliconFlow` 和 `Goose · SiliconFlow`。模型配置在正式 Workspace 加密保存，不回显密钥；现有 Codex Workspace、个人连接及用户历史保持原样。

## 完成与证据

| 能力 | Claude Code | Goose |
| --- | --- | --- |
| 正式界面创建 Workspace / Controller 创建实际容器 | 通过 | 通过 |
| 实际模型流式回复 | 通过 | 通过 |
| 第二轮记住前轮标记、刷新历史 | 通过 | 通过 |
| 原生新进程恢复持久会话 | 通过 | 通过 |
| 界面权限选择、批准后实际写文件 | 通过 | 通过 |
| 选定 Skill 正文加载并生成仅存在于文件中的标记 | 通过 | 通过 |
| Gateway 内置 MCP 真实工具调用、返回正确 Session / Workspace | 通过 | 通过 |

浏览器验证两轮唯一标记和历史后刷新，无页面脚本错误。工具测试仅批准指定命令写入专用验收 Workspace 的 `f6-tool-check.txt`，随后检查容器内文件与预期完全一致；未批准未知工具。MCP 验证实际调用 `platform__session_context`，以持久化 Tool Result 中的 Session / Workspace 身份核对，不以模型宣称调用代替证据。Skill 原生验收使用固定文件产物，提示中不含预期正文标记；Prompt / Skill / MCP 资源固定与网关身份刷新另有真实 ACP 协议回归。

Claude 原生控制通道把工具确认映射为 permission 请求、AskUserQuestion 映射为 input 请求，只有有效用户选择才继续。问答映射与拒绝分支通过协议测试，未把模型自行生成问题当作已完成的真实用户问答验收。Goose questions 能力继续 false。

## 验收中修复

真实 Goose MCP 调用曾正确返回工具结果，但最终 assistant 消息仅保存 UUID 前半段。ACP SDK 默认把通知交给分离任务，prompt 响应可在文本通知及下游写入完成前返回。已改为通知顺序处理，并在 new/load/prompt 成功或 prompt 失败响应后等待先前通知完成，才重置历史回放状态、生成最终快照或提交终态；权限请求保持并发。没有把 delta 改为落库。

新增慢持久化观察器与原生连续通知 fixture：旧 SDK 默认分发方式可稳定使测试失败，修复后完整文本与终态顺序检查通过；失败响应同样不允许终态后晚到文本。修复后真实 Goose MCP 再次返回并保存完整 UUID。

Goose 原生选项标识转换成中英文许可标签，提交仍保留原始 option_id；Claude / Goose 能力描述与已接入的受管执行路径同步。操作说明补齐两个 Runtime 的镜像构建与模型地址规则。

## 检查结果

- Python 全量：120 passed、12 skipped（专用数据库 / RabbitMQ 环境变量未启用）；另有正式产品真实数据库、Broker 与模型链路验收。
- ACP / Claude / Goose 相关 40 项通过；新增顺序检查覆盖成功与失败响应。
- Python 分层约束 14 kept / 0 broken。
- 修改 Python 文件 Ruff 通过，ACP 3 文件 mypy 通过，Claude 4 文件 mypy 通过。
- Workspace 设置真实浏览器回读通过：模型名/地址保留、API key 输入为空、公开接口不含密钥；720px 高度弹窗可以滚动并操作底部按钮。
- UI 相关 2 文件 9 项通过；TypeScript / Vite 构建、依赖边界检查通过。
- Control 配置历史 / 发布事务 PostgreSQL 集成通过，专用临时数据库容器已删除。
- Control / Gateway、两个 Runtime 和正式 UI 镜像构建与部署完成；实际运行镜像 ID 与最终构建一致。

安全公开证据见同目录 acceptance.json 和截图。密钥、浏览器登录状态及内部令牌不进入审查产物。验收后删除本地明文密钥输入文件及浏览器登录状态副本；正式 Workspace 的加密凭据保留可用。

## 范围与限制

此阶段完成 Claude / Goose 接入；未恢复 k3s，未开发 Docker 扩缩容。Kubernetes 真实共享存储和真实外部 OAuth 服务验收仍后排，不能视为本阶段完成。SiliconFlow 服务的外网访问仍依赖当前有效网络 / 代理；原生预检出现过一次超时，重新启动复测及正式多轮验收通过，记录于 F6b。不同兼容服务和模型的工具遵循程度仍取决于服务实现。

参考：[SiliconFlow Claude Code 接入](https://docs.siliconflow.cn/docs/usercases/use-siliconcloud-in-ClaudeCode)、[Goose v1.52 Skill 规则](https://github.com/aaif-goose/goose/blob/v1.52.0/documentation/docs/guides/context-engineering/using-skills.md)。
