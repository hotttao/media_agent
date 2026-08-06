# 006 领域模型规范

## 1. 核心聚合

| 服务 | 聚合根 |
|---|---|
| `media_auth` | User、WebSession、ServiceToken、PlatformToken、Team、OAuthCredential |
| `media_control` | Workspace、RuntimeEnvironment、AgentConfig、Schedule、TeamworkTopology、Prompt、AgentTemplate、Skill、Provider、MemoryStore |
| `media_agent_gateway` | AgentSession、AgentTask、BatchRun、TeamworkSessionLink |
| `media_plugin` | MCPDefinition、WorkspacePluginBinding、ExternalMCPConnection |
| `media_storage` | StoredObject |
| `media_notification` | Notification、Delivery |
| `media_materials` | Material |
| `media_workflow` | Workflow、WorkflowRun |
| `media_gen` | GenerationTask、TranscriptionTask |
| `media_crawler` | CrawlTask、CrawlResult |

## 2. 不变量

- Workspace 是控制面租户边界；业务资源必须记录 `workspace_id`。
- 资源所属服务完成最终授权，不依赖 Edge 替其判断资源归属。
- AgentSession 固定关联一个用户和 Workspace；每个 Session 使用稳定的 `session_token`，撤销后立即失效。
- Platform Token 由 `media_auth` 加密保存，仅允许固定平台组件按用途获取。
- Material 只保存 Storage 对象路径；对象生命周期由 `media_materials` 决定。
- Plugin Binding 不构成业务接口授权依据。
- 跨聚合、跨服务引用只保存 ID，不建立跨库外键。

## 3. 关键流程

### 创建 Agent Session

`media_control` 校验 Workspace 配置，`media_agent_gateway` 创建 Session 与 Session Token，`media_agents` 启动 Runtime。运行期对话和 MCP 调用不再经过 Control。

### 定时执行

`media_control` 的 Schedule Worker 创建执行记录，按需获取 Platform Token，再向 `media_agent_gateway` 创建 Agent Task。

### 媒体工作流

`media_workflow` 编排 `media_crawler`、`media_gen` 和 `media_materials`。每个服务拥有自身任务状态，跨服务步骤通过明确命令和事件衔接。

### Teamwork

`media_control` 管理成员与拓扑；`media_agent_gateway` 管理运行中的 Session 关联。配置移除成员时不强杀正在执行的调用，允许自然结束，新任务使用新版本拓扑。

## 4. 外部模型

AFS 是第三方运行组件。平台的 AFS 配置、绑定和状态投影属于 `media_control`；AFS 内部对象不复制为平台领域聚合，也不修改第三方代码。
