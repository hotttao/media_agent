## 问题
### 1. 当前架构问题
讨论一下 agent-platform 的架构问题，首先项目采用控制面和数据面分离的架构，control-plan 属于控制面。但是从微服务的架构上我不太能理解 control-plan。他好像作为了网关层，代理了所有服务的访问。但是并没有解耦认证和
鉴权，所有其他服务仍然需要自己解决认证和鉴权问题。browser 服务我能理解，如果需要依赖 cp 转发操作的视频，对于 cp 的压力会很大实现也更复杂。所以我应该怎么理解为什么采用这样的架构。如果想要优化，可以如何优化。请
给我画出优化后的架构图，以及其他服务如何处理认证和鉴权，如果我想增加一个plugin，需要开发后端和mcp 服务，可以更少侵入 cp 完成。

### 2. DDD
D:\Code\media\media_agent 是我为了重构新建的项目。其中 media_crawler media_gen  media_materrials media_workflow 是我想接入的新的 插件和 mcp 服务。media_agent 对应当前的 agents media_auth 对应认证鉴权和你说的
mcp service。我现在需要你做的是: 1. 首先将刚刚的输出的内容整理更新到 D:\Code\media\media_agent\docs\research\architecture.md 2. 依据刚刚你的输出，结合我当前的重构，进行具体的架构设计和选型。这一个部分，你可以选择 2-3 个方案，并进行对比选择最后。3. 进行服务划分并写清楚每个服务的职责。 4. 确定服务后，使用 D:\Code\media\media_agent\harness\tools\new_server 初始化服务 5. 使用 domain-modeling 进行领域建模，建模完成后，通过 idl 定义好每一个服务的接口，接口开发过程要严格遵守 harness\contributing\architecture.md 架构规范。当然如果你发现 architecture.md 存在问题，可以提问和修改。6. 验证，验证 control-plan 里所有的接口都有对应的服务实现。并在最后以列表的形式列出映射关系。

注意当前是设计阶段，不需要执行 plan 不进行具体业务的开发。所有内容更新到 D:\Code\media\media_agent\docs\research\architecture.md 文件内。

### 3. 认证和鉴权
我觉得你一直没有梳理清楚这个问题。我做了一个总结:
首先每个服务都支持两种认证方式:
1. oauth 认证获取用户信息，然后签发 cookie jwt token
2. service token 认证

系统里面需要如下几种认证:
1. Web 用户：
    - 当前是哪位登录用户，通过 Cookie JWT
    - 还能自己创建 service token，用于 cli 等非浏览器方式访问的认证
2. Agent Session：当前代表哪个用户、Workspace、Session，通过 session_token 认证

还有一种特殊的 platform token 它是一种特殊的 service token 本质上也是代表用户，适用于
1. 非用户直接发起的任务。比如定时任务
2. session token 只代表 session，browser service 不支持 session token 认证，所以也需要 platform token

那你现在核心要解决的问题是:
1. platform token 不能删除，因为定时任务等等都需要一个长期的 token 
2. 核心要解决的就是不要把 platform token 暴露出去。哪一个服务需要共享，就由那个服务自己生成一个短时受限 token 用户共享。

剩下的就还有一个问题，平台内自定义的服务和 mcp 被 agent 调用时候如何获取 platform token。其实本质上鉴权是一方面，平台内自定义的服务和 mcp 是需要获取请求的用户和 workerspace 信息。


现在做一个总结  ui_platform 发送过来的请求，网关先验证 cookie 然后，后面通过 mtls 验证服务信息，后端服务直接信任网管设置的 header 。如果是agent 发
  起的请求，走的就是 session_token 和 platform_token 这一套验证逻辑。这个总结对么