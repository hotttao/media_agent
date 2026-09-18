## agent 架构设计

当下 media_agents 里实现了各种 agents，media_aget_gateway 作为 agent 的网关。

### sse 有状态导致 agent_gateway 单例
但是由于 sse 是有状态的，所以当下 media_aget_gateway 只能作为单例。

经过调研和分析，决定引入 rabbitmq，核心目的有两个:
1. 首先 rabbitmq 作为消息队列，agent 不在通过 sse 将消息传递给 agent_gateway。而是将消息先保存到 mysql，在投递到 rabbitmq。
2. agent_gateway 每个实例都可以接受前端的 sse 链接，并像现在这样同时维护多个 sse 链接，并广播
3. agent_gateway 读取 agent 消息时，先消费 rabbitmq 中消息，在读取 mysql 中的历史消息，过程跟现在的 sse 链接一样，只不过通过消息队列解耦了 sse 单例的状态
4. 每一个 Gateway 都可以接受用户发送的 chat，都可以直接发送给 agent，chat 消息在 agent 内排队
5. agent 状态也由 agent 自己维护，并负责更新到数据库


### 数据面变更优化
引入 rabbitmq 还有另一层目的，agent 当前是数据面，控制面在 media_control 等其他子服务中。按照现在的模式，控制面数据的更新，需要有 media_control 请求 agents。

我觉得随着系统的扩展，会导致很多子服务请求 agent，导致调用逻辑会变得越发复杂。我觉得可以做如下优化，下面内容以系统提示词更新为例:

1. media_control 首先知道哪些 workspace 对应的 agent 正在使用被修改的系统提示词
2. 每个 agent 启动后，都会监听一个专属于自己的任务队列，这个任务队列，专门用于接受，其他子服务发送的元数据变更通知
3. media_control 在变更系统提示词后，读取所有正在使用该系统提示词的 agent 实例，将变更通知发送给这些 agent 实例
4. agent 实例收到变更通知后，发送请求拉取最新的系统提示词


### agent_gateway 定位

从我的理解，agent 本质上不属于系统的子服务，他应该代表客户端。所以 agent_gateway 就是类似微服务网管的角色，负责代理agent 的所有请求。

所以 agent 只跟 agent_gateway 交互。除了代理请求外，agent_gateway 应该还有两个重要的职责:
1. mcp 动态发现。比如说系统增加子服务，子服务给 agent 提供了新的 mcp 工具。agent_gateway 是 agent mcp 的入口，而 agent_gateway 本身需要提供 mcp 注册和发现功能
2. agent 现在还包含 platform_prompt 的组装功能。platform_prompt 的生成，其实是跟 agent 的选线和支持的 skills 以及 mcp 关联的，这一部分应该由 agent_gateway 来负责。
3. 除了 platform_prompt 外，其他一些元数据的组装和处理也应该放在 agent_gateway 中。

为什么这么处理，根本原因是:
1. agent 是面向用户的，有状态的。所以 agent 的升级对用户来说是有感的
2. agent_gateway 是无状态的，可以滚动升级，可以做到无感。

所以业务内部逻辑，应该尽可能的放在 agent_way 处理，agent 本身应该足够简单。