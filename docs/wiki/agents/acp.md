

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

所有逻辑都包含在 D:\Code\media\media_agent\media_agents\adapter\biz\domain\acp 内。