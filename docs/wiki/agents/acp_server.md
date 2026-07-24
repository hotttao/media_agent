# ACP Server 连接与重连设计

本文记录 Python ACP adapter 中 `AcpServer`、`AcpSession`、SSE stream 之间的职责边界，重点解释 TS 版 `acp-server.ts` 中 `activeSinks`、`switchToBufferMode`、`reconnect` 在 FastAPI 架构下应该如何迁移。

## sessions 与 active_streams

`sessions` 和 `active_streams` 解决的是两类不同问题。

`sessions` 是会话生命周期索引：

```text
session_id -> AcpSession
```

它表示 ACP session 是否在内存中，内部持有 bridge、translator、busy_turns、last_active、pending_destroy 等状态。即使当前没有 SSE 连接，只要 session/bridge 仍然可复用，`sessions` 仍然应该保留。

`active_streams` 是当前 turn 的 SSE 传输状态索引：

```text
session_id -> SessionStream
```

它表示某个 session 当前是否有一个正在执行或可 reconnect 的输出流，内部关注 writer、buffer、disconnected、done 等状态。turn 结束且没有断线等待 reconnect 时，这个状态应该清理。

在当前 Python 设计里，一个 `AcpSession` 一对一绑定一个 `AcpBridge`，不像 TS 版一个 bridge 注册多个 session handler。因此不一定必须维护一个独立的 `active_streams` dict。更简单的实现可以把 stream 状态挂在 session 上：

```python
class AcpSession:
    stream: SessionStream | None
```

然后 `AcpServer.reconnect(session_id)` 通过 `sessions[session_id].stream` 查找当前可恢复的流。

结论是：不一定需要新的映射，但一定需要一个独立于 session 生命周期的 stream 状态对象。`sessions` 不能直接替代它，因为 session 存在不等于“当前还有一个可重连的 SSE turn”。

推荐边界：

```text
AcpServer
  - 管理 session/bridge 生命周期
  - 创建、查询、清理当前 turn 的 SessionStream
  - 提供 chat_stream / reconnect_stream 给 router

AcpSession
  - 接收 ACP session_update
  - 翻译为 UniversalEvent
  - 写入当前 SessionStream

SessionStream
  - 管理 writer 或 async iterator
  - 断线后 buffer
  - reconnect 后 flush buffer
  - turn done 后决定是否清理
```

## 为什么需要 buffer 模式

ACP turn 可能持续很久。浏览器、反向代理、网关或客户端刷新页面时，原来的 `/chat` SSE 连接可能断开，但 agent 子进程还在执行。如果断线后继续把事件写向已经失效的网络连接，这些事件会丢失，客户端也无法通过 `/reconnect` 恢复。

buffer 模式的目标是：

1. 客户端断线后，agent turn 继续运行。
2. 新产生的事件不丢弃，先写入内存 buffer。
3. 客户端调用 `/sessions/{id}/reconnect` 后，先补发 buffer，再接管后续事件。
4. 如果 reconnect 连接又断开，再切回 buffer。

## TS 版切换逻辑

TS 版 `SessionSink` 本质上是一个可替换 writer：

```ts
interface SessionSink {
  write(event, data): Promise<void>
  buffer: Array<{ event: string; data: string }>
  disconnected: boolean
  donePromise: Promise<void>
}
```

正常连接时：

```text
sink.write = chatWriter -> stream.writeSSE(...)
```

断线后：

```text
sink.write = bufferWriter -> sink.buffer.push(...)
sink.disconnected = true
```

reconnect 后：

```text
flush sink.buffer
sink.write = reconnectWriter -> stream.writeSSE(...)
sink.disconnected = false
```

## 关键竞态：expectedWriter

`switchToBufferMode(sessionId, expectedWriter)` 里的 `expectedWriter` 是为了防止旧连接的 abort handler 覆盖新连接。

示例：

```text
t0: /chat 建立连接
    sink.write = chatWriter

t1: 网络抖动，客户端发起 /reconnect
    reconnect 先执行成功
    sink.write = reconnectWriter

t2: 旧 /chat 的 abort 事件晚到
    如果不检查 writer，旧 abort 会把 sink.write 改成 bufferWriter
    结果 reconnectWriter 被覆盖，新连接虽然还活着，但收不到后续事件

t3: 有 expectedWriter 检查
    switchToBufferMode(sessionId, chatWriter)
    发现 sink.write !== chatWriter
    说明 writer 已被 reconnect 接管
    跳过切换
```

时序图：

```text
Client          /chat handler        SessionStream        /reconnect handler        Agent
  |                  |                     |                      |                  |
  | POST /chat       |                     |                      |                  |
  |----------------->|                     |                      |                  |
  |                  | attach chatWriter   |                      |                  |
  |                  |-------------------->| writer=chatWriter    |                  |
  |                  | run turn            |                      | prompt           |
  |                  |------------------------------------------------------------->|
  |                  |                     |                      | update           |
  |                  |                     |<--------------------------------------|
  |                  |                     | write via chatWriter |                  |
  |<-----------------|                     |                      |                  |
  | disconnect       |                     |                      |                  |
  |                  | abort pending       |                      |                  |
  | POST reconnect   |                     |                      |                  |
  |------------------------------------------------------------->|                  |
  |                  |                     |                      | flush buffer     |
  |                  |                     |<---------------------|                  |
  |                  |                     | writer=reconnectWriter                  |
  |                  | late abort          |                      |                  |
  |                  | switch(expected=chatWriter)                |                  |
  |                  |-------------------->| skip, writer changed |                  |
  |                  |                     |                      | update           |
  |                  |                     |<--------------------------------------|
  |                  |                     | write via reconnectWriter              |
  |<-------------------------------------------------------------|                  |
```

## Python/FastAPI 下的差异

FastAPI `StreamingResponse` 通常消费一个 async iterator：

```python
async def event_generator():
    async for chunk in stream.read():
        yield chunk
```

这里和 TS 最大的差异是：业务代码通常没有直接拿到 ASGI `send` 回调。`yield chunk` 只是把数据交给 `StreamingResponse`，真正的网络发送发生在 Starlette/FastAPI 内部。

因此，Python 里的 `SessionStream.write()` 不应该假设“写入成功等于客户端收到”。如果当前实现是 queue：

```text
AcpSession.session_update -> SessionStream.write -> queue.put
StreamingResponse generator -> queue.get -> yield chunk
```

那么 `write()` 只证明事件进入了内存队列，不证明 SSE 发送成功。发送失败只能在 generator 所在任务被取消、抛异常，或 `request.is_disconnected()` 返回 true 时感知。

## SSE 发送失败如何处理

推荐做法是把异常边界放在 SSE generator，而不是放在 `session_update` 回调。

router 仍然保持薄，只负责把断线信号转发给 domain：

```python
async def sse():
    stream = acp_server.attach_chat_stream(session_id)
    writer_id = stream.writer_id
    try:
        async for chunk in stream.read():
            if await request.is_disconnected():
                await acp_server.detach_stream(session_id, writer_id)
                break
            yield chunk
    except asyncio.CancelledError:
        await acp_server.detach_stream(session_id, writer_id)
        raise
    except Exception:
        await acp_server.detach_stream(session_id, writer_id)
        raise
```

`detach_stream(session_id, writer_id)` 内部执行类似 TS 的：

```text
if current writer_id still equals expected writer_id:
    switch to buffer mode
else:
    skip because reconnect has already replaced writer
```

注意：`yield chunk` 后，如果 Starlette 在真正 `send` 时失败，generator 未必能通过普通 `try/except` 捕获到“这一条 chunk 发送失败”的异常；更常见的是 generator 被取消，并进入 `finally` 或 `CancelledError` 分支。因此 Python 版应按连接级别处理失败，而不是按单条消息确认。

## buffer flush 的确认策略

TS 中 `await stream.writeSSE(...)` 失败可以直接 catch，所以 flush buffer 后清空比较直观。

Python async iterator 下，flush buffer 时不要在“准备 yield”之前永久删除事件。更稳妥的策略是：

```text
1. reconnect 建立后，把 buffer 快照出来。
2. generator 逐条 yield。
3. 只有当 generator 继续运行到下一轮时，才认为上一条至少已经交给 ASGI send。
4. 如果 generator 被取消或异常退出，未确认的事件需要留在 buffer，等待下一次 reconnect。
```

简化实现也可以接受“flush 时先清 buffer，失败后依赖后续事件继续 buffer”，但这会在 reconnect 发送阶段断线时丢失已从 buffer 移除、但未真正发到客户端的事件。ACP 输出如果要求尽量不丢，应使用确认游标或 pending flush 队列。

## 推荐的 Python 模型

建议引入 `SessionStream`，而不是继续让 `SessionSink` 只做 queue。

```python
class SessionStream:
    session_id: str
    writer_id: str | None
    disconnected: bool
    done: asyncio.Event
    live_queue: asyncio.Queue[SseEvent]
    buffer: list[SseEvent]

    async def write(event):
        if self.disconnected:
            self.buffer.append(event)
        else:
            await self.live_queue.put(event)

    def detach(expected_writer_id):
        if self.writer_id != expected_writer_id:
            return
        self.disconnected = True
        self.writer_id = None

    def attach_new_writer():
        self.writer_id = new_id()
        self.disconnected = False
        return self.writer_id
```

如果不想维护 `active_streams` dict，可以让 `AcpSession` 持有：

```python
class AcpSession:
    current_stream: SessionStream | None
```

然后：

```text
AcpServer.chat()
  -> session = resolve/create/load
  -> stream = session.open_stream()
  -> return stream
  

AcpServer.reconnect(session_id)
  -> session = sessions[session_id]
  -> stream = session.current_stream
  -> if stream is None: 404
  -> stream.attach_new_writer()
  -> return stream.reconnect_reader()

AcpServer.run_turn()
  -> writes events through session.current_stream
  -> on completion stream.done.set()
  -> if stream.disconnected: keep stream for reconnect
  -> else session.current_stream = None
```

## router 应该保持的职责

router 可以保留：

```text
- 解析 HTTP body/path/query
- 创建 StreamingResponse
- 调用 request.is_disconnected()
- 捕获 CancelledError 并通知 AcpServer detach
- 把 domain exception 转成 HTTP status
```

router 不应该包含：

```text
- buffer flush 规则
- writer ownership 判断
- reconnect 后替换 writer
- turn done 后 active stream 清理
- reused bridge retry-once
- pending_destroy drain
```

