# F6b · 原生 Runtime 与 Docker 使用

## 完成

Claude Code 通过原生 stream-json stdin 控制通道接入持久 AgentRequest，工具确认默认拒绝、用户问答无隐式答案；支持真实 partial text delta，最终快照去重，原生会话恢复、取消和稳定错误分类。模型服务错误仅输出通用错误，不把原始服务响应或 stderr 回传界面。保留原生 CLI，不引入第二套模型代理或模拟回复。

Goose 镜像直接下载固定版本 1.52.0 的官方二进制，增加下载重试并正确解包；Claude Code 镜像固定 2.1.231。产品创建服务按 Workspace 类型启动单个 Docker Agent，未开发 Docker 扩缩容。创建/编辑表单限制弹窗高度并内部滚动，适配新增模型字段。

## 检查

- Python Claude/Goose/固定 MCP 恢复相关 16 项通过，Claude 改动 mypy 4 文件与 Ruff 通过。
- 两个正式 Docker 镜像构建成功；正式 Control/Gateway 与 UI 构建部署成功。
- 用户指定 SiliconFlow 真实服务：两个原生 Runtime 单轮回复及记忆恢复通过；Goose 在重启子进程后恢复成功，Claude 新进程 --resume 成功。
- 正式浏览器新建两个 Workspace，由 Controller 创建对应 Agent 容器；两轮真实回复和刷新历史通过，页面脚本错误 0。工具确认完整浏览器验收继续在 F6c 记录。
- 一轮 Claude 原生预检遇到 180 秒超时，清理该预检容器后新进程复测通过。未以该次超时作为成功证据；正式两轮调用通过。

## 边界

未更改现有 Codex Workspace、个人模型连接及历史。凭据仅由忽略提交的私有材料和正式加密配置持有，review 不含密钥。Kubernetes / 第三方 OAuth 服务验收后排。
