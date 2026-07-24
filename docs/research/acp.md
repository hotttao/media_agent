
## agents python 重构

agents 代码位于:
1. D:\Code\koala\agent-platform\internal\acp-adapter
2. D:\Code\koala\agent-platform\agents

这个是基于 js 实现的 agents 代码，需要重构为 python 实现。agents 支持多种 agent core，一来 ACP 协议。

## ACP 协议架构

```python
class AcpBridge:
    """
    ACP 协议的桥接层(以 codex 为例)
    1. child: 
        - 启动 codex-acp, 这是经过 acp 协议包装的 codex, 内部实现了 acp 要求的 Agent 接口
        - input, output: 分别是 codex-acp 的输入输出通道
    2. ClientSideConnect:
        1. Client: 用户层接受 agent-core 传递过来的消息，自定义处理逻辑
        2. ndJsonStream(input, output)
        3. ClientSideConnect 要求启动的 child, 必须实现了其 Agent 接口
    3. AcpBridge: 包装了 ClientSideConnect, 核心方法内部都是委托给 ClientSideConnect
    4. ClientSideConnect: 将消息的管理交给了 AcpSessionHandler
    5. AcpSessionHandler: 将 llm event 的解析交给 translateUpdate 转换为 UniversalEvent
    """


class AcpAgentApp:
    """
    基于 acp 协议，对外暴露的 Agent 应用接口
    # 1. acp 进程管理
    # 2. sse 链接管理
    # 3. /chat 与 /reconnect sse.writer 的切换
    # 4. /chat 响应传递过程:
        acp_bridge.prompt -> 
            ClientSideConnect.prompt -> ClientSideConnection.Client.sessionUpdate(新消息)
                AcpSessionHandler(currentBridge!.registerHandler)
                    sse.write(新消息)
    """

```

## 协议桥阶层

#### bridge -> ClientSideConnection -> sessionHandlers

```ts
    const client: Client = {
      async sessionUpdate(params: SessionNotification) {
        // Notifications carry native ids and the handler map is keyed by
        // native ids (registerHandler decodes) — look up directly. Keying by
        // platform ids would break for legacy sessions persisted before a
        // codec landed: decode() tolerates their unprefixed form, but
        // encode() can't reproduce it.
        const handler = self.sessionHandlers.get(params.sessionId)
        if (handler) {
          handler.onUpdate(params.update)
        } else {
          console.warn(`[acp-bridge] session/update for unknown session: ${params.sessionId}`)
        }
      },

      async requestPermission(params: RequestPermissionRequest) {
        const handler = self.sessionHandlers.get(params.sessionId)
        if (handler) {
          return handler.onPermissionRequest(params)
        }
        // Auto-approve with first option
        const optionId = params.options?.[0]?.optionId ?? 'allow-once'
        return { outcome: { outcome: 'selected' as const, optionId } }
      },

      async extNotification(method: string, params: Record<string, unknown>) {
        try {
          self.options.onExtNotification?.(method, params)
        } catch (e) {
          console.error(`[acp-bridge] onExtNotification failed (${method}):`, e)
        }
      },
    }

    this.connection = new ClientSideConnection(() => client, ndJsonStream(input, output))
```

#### sessionHandler -> translator -> sink(消息发出)

```ts
const sessionHandler: AcpSessionHandler = {
    onUpdate(update) {
    for (const evt of translator.translateUpdate(update)) {
        turnProducedOutput = true
        sink.write('message', JSON.stringify(evt))
    }
    },
    async onPermissionRequest(req) {
    const optionId = req.options?.[0]?.optionId ?? 'allow-once'
    return { outcome: { outcome: 'selected' as const, optionId } }
    },
}
currentBridge!.registerHandler(currentSessionId, sessionHandler)
```


## python 实现

python 实现上对上面的抽象做一些改动:
1. 删除 sessionHandler 添加 AcpSession 
2. AcpSeesion 实现 ClientSideConnection 所需的 client 接口
3. 设计上 AcpSession 面相 sse 连接层，处理与数据发送相关的逻辑，AcpBridge 面相 agent core 层
4. 顶层添加 AcpServer，全局唯一实例，管理所有 AcpSession 和 bridge，将原来 /chat /reconnect 的逻辑都交给了 AcpServer

所有逻辑都包含在 D:\Code\media\media_agent\media_agents\adapter\biz\domain\acp 内。我已经实现了核心部分，你需要对代码进行 review。

## 回答一
1. active_streams 需要维护这样一个新的映射么，因为代码里面已经维护了 sessions 的映射了，这里也是我的一个疑问，js 代码里一个 bridge 对应多个 session handler，这个是必须的么？现在的代码逻辑是一对一关系。
2. disconnect 后切换到 buffer 模式，这个是比较复杂，我看代码就没有看懂，能不能给我总结一下，以示例和时序图的方式给我解释切换逻辑更新到 docs\wiki\agents\acp_server.md
3. 3-7 没问题

我还有个疑问，fastapi 架构下，如何处理某个 sse 消息发送失败的问题，js 里面通过回调的方式，每次调用 stream 是可以直接 try-catch 的，但是 python 里面没有回调的方式，如何处理？对于异常处理也更新到 docs\wiki\agents\acp_server.md

## 配置文件
接下来是每一个具体 agent 的实现，包括 D:\Code\koala\agent-platform\agents。在 js 版本的实现里，虽然大多数逻辑 已经复用了 acp-adpater。但是每个 agent 里面还有重复的代码。主要集中在每个 agent 环境的准备上。我希望在 python 版本里也能避免重复的代码。

这一部分主要跟配置文件有关，现在的配置文件管理使用的 omegaconf，他一大有优点是可以在 yaml 同时处理 环境变量和默认值。所以我需要你设计一下 yaml 配置文件的结构。公共的配置文件定义在 adapter/config 下。差异化配置定义在各自的 agent 目录下。对应的 omegaconf load 成 Pydantic 对象之后，每一个 Pydantic 都应该有配置文件检查逻辑。load 配置文件，需要执行配置文件检查。请你先完成配置文件结构设计，并生成各个 agent core 的 配置文件和 Pydantic model 并 load 配置文件。

## SessionStream queue 的问题

queue 是无限队列，并且写入和读取是异步的。比如 读取的时候失败了，此时写入会缓存，但是此时 queue 里面其实是有缓存的 event 的，称为 old1，重连之前写入 buffer 的是 new1，当 reconnect 重连的时候，是先读取的 buffer，是不是new1 就排到了 old1 之前了。顺序错误了。你可以对比一下原 js 版本(D:\Code\koala\agent-platform\internal\acp-adapter\acp-server.ts)。没有 queue，直接顺序调用的 stream.writeSSE({ event, data }) 。顺序调用如果第一个 event 没发送完。后续是如何处理的。请把这段处理逻辑解释清楚，更新到 docs\wiki\agents\acp_server.md