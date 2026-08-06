# 002 服务边界规范

## 1. 划分原则

- 服务按业务数据所有权划分，不按接口数量划分。
- 每项业务数据只有一个权威写入方；服务不共享数据库，不建立跨库外键。
- `media_control` 只管理控制面资源，不代理媒体、Browser 或 Agent 热路径流量。
- 跨服务读取优先使用明确的内部接口；需要可靠传播的状态使用集成事件。

## 2. 平台服务

| 服务 | 职责与数据所有权 | 明确不负责 |
|---|---|---|
| `media_auth` | 用户、登录身份、Web Session、Cookie JWT、Service Token、Platform Token、团队成员关系、MCP OAuth 凭据 | 业务资源 ACL、业务请求代理 |
| `media_control` | Workspace、运行环境与放置、Agent 配置、Prompt、模板、Skill、Provider、凭据引用、Memory Store、Schedule、Teamwork 拓扑、平台侧 AFS 记录、Tag、管理配置 | Agent 对话热路径、媒体数据、AFS 实现 |
| `media_agent_gateway` | Agent Session、Session Token、Turn、Message、Event、Agent Request、Agent Task、Batch Run、Teamwork Session Link、Platform MCP | Workspace 配置所有权、通用业务代理 |
| `media_plugin` | 受控 MCP 定义、Workspace 启用关系、外部 OAuth MCP 连接状态与令牌托管 | Agent 请求中转、任意代码上传、业务 ACL |
| `media_storage` | 对象读写、元数据、后端驱动、URL 暴露、来源服务校验 | 业务归属、分享策略、业务删除规则 |
| `media_notification` | 通知、偏好、投递、已读状态、重试与死信 | 业务事件所有权 |

## 3. 业务服务

| 服务 | 职责 |
|---|---|
| `media_agents` | Agent Runtime、模型调用、工具执行与运行时状态 |
| `media_workflow` | 媒体工作流定义、执行与同进程 MCP Adapter |
| `media_materials` | 素材元数据、素材归属、引用与生命周期 |
| `media_gen` | 图片、音视频生成，以及 ASR/转写能力 |
| `media_crawler` | 抓取任务、抓取结果和来源记录 |

## 4. 外部依赖

- Browser、Sandbox、Channel 保持独立服务；所属业务服务通过内部接口调用。
- `afs-controller` 与 `afs-sidecar` 来自 `D:\Code\github\afs`，视为第三方依赖，不修改其代码。
- AFS 的平台配置和绑定记录归 `media_control`，AFS 运行机制仍沿用原 `control-plane` 的使用方式。

## 5. Edge Gateway

Edge 是基础设施入口，不是业务服务。它负责路由、限流、公共凭证终止、请求追踪和第一跳可信上下文注入；不保存业务状态，也不替目标服务做资源级授权。
