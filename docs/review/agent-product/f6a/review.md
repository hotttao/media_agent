# F6a · Claude Code / Goose 固定模型调用设置

## 交付

Workspace支持模型API协议、模型名称、基础地址的创建/编辑/读取，并随Agent配置历史固定。Claude Code为Anthropic兼容；Goose为OpenAI/Anthropic兼容。新字段更新与同内容元数据Release在同一事务发布，旧Session保留旧目标和认证。地址拒绝嵌入凭据/query/fragment与控制字符；新字段不回显API Key。Codex原个人连接保持原语义。

Control→Gateway→Agent的固定执行上下文透传调用目标，Claude子进程环境接收选定模型/地址，Goose每个bridge独立配置provider/key/host/path，不用进程全局状态串用凭据。修复Goose原生路径：GOOSE_PATH_ROOT下配置应位于config/config.yaml；选定Skill复制到私有.agents/skills，Gateway仍是唯一MCP入口。

Settings编辑/创建Workspace新增非Codex模型服务字段，Goose要求明确模型名；切换Agent/协议清空API Key草稿，已有配置的空密钥仍按原规则保留。IDL模型由hz生成，Control/Gateway OpenAPI由protoc重新生成，补齐新字段及此前模型连接接口描述。

## 验证

- Control/Gateway全模块Go测试通过。
- 真实PostgreSQL新增模型设置集成测试通过（0.732秒）：历史固定、保留API Key、原子Release、无版本写入拒绝、非法地址不部分提交。
- Python相关23项通过，8文件mypy通过，相关Ruff检查通过。
- UI完整14文件41项测试通过，补充自定义Claude模型/地址的提交断言复测4项通过；TypeScript通过。
- 原生Claude Code镜像构建成功，版本2.1.231。使用用户明确提供的SiliconFlow凭据和指定模型，真实原生CLI返回唯一回复标记，退出0。本项是原生预检，完整产品会话/工具链由F6b/F6c继续。
- Goose原生镜像下载遇GitHub TLS EOF，尚未完成原生启动/真实模型验收，不在本切片宣称通过。Docker安装改动留给后续切片。

真实服务确认模型列表包含 deepseek-ai/DeepSeek-V4.1-Flash。凭据仅保存在忽略提交的0600私有验收配置及后续加密Workspace配置，不进入review、公开API、命令参数或提交。

参考：[SiliconFlow Claude Code接入](https://docs.siliconflow.cn/docs/usercases/use-siliconcloud-in-ClaudeCode)、[Goose v1.52路径规则](https://github.com/aaif-goose/goose/blob/v1.52.0/crates/goose/src/config/paths.rs)。

## 后续

F6b补齐原生流式/历史/工具交互及Docker实际创建；F6c从正式界面用SiliconFlow完成真实会话验收。Docker扩缩容不开发，k3s保持关闭。
