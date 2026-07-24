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


## SessionStream queue 顺序问题

当前 Python `SessionStream` 的核心结构是：

```python
_queue: asyncio.Queue[tuple[str, str]]
_buffer: list[tuple[str, str]]
disconnected: bool
```

当前写入规则是：

```text
connected:    write(event) -> _queue.put(event)
disconnected: write(event) -> _buffer.append(event)
reconnect:    read() 先 flush _buffer，再继续读 _queue
```

这会产生一个顺序风险。示例：

```text
t0  SSE 连接正常，Agent 产生 old1
t1  write(old1) 写入 _queue，但 StreamingResponse 还没有从 _queue 读走或还没有完成发送
t2  客户端断线，router 调用 detach_stream，SessionStream 进入 disconnected=true
t3  Agent 继续产生 new1
t4  write(new1) 因为 disconnected=true，写入 _buffer
t5  客户端 reconnect
t6  read() 当前逻辑先 flush _buffer，所以先发送 new1
t7  read() 再读 _queue，才发送 old1
```

结果是：

```text
实际发送顺序: new1 -> old1
真实产生顺序: old1 -> new1
```

所以这个模型有顺序错误。问题根源不是无限队列本身，而是“未发送事件”被拆成了两个容器，并且 reconnect 时优先消费后产生的 `_buffer`。

### TS 版本如何处理

TS 版本没有独立的 producer queue。它维护的是一个可替换的 `SessionSink.write`：

```ts
const chatWriter = async (event, data) => {
  await stream.writeSSE({ event, data })
}

sink.write = chatWriter
```

正常连接时，事件直接调用当前 writer：

```text
Agent update -> sink.write(event) -> stream.writeSSE(event)
```

断线时：

```ts
switchToBufferMode(sessionId, chatWriter)
```

如果当前 writer 仍然是这个断开的 `/chat` writer，就切换成：

```ts
sink.write = async (event, data) => {
  sink.buffer.push({ event, data })
}
```

reconnect 时：

```text
1. 复制 sink.buffer
2. 清空 sink.buffer
3. 逐条 await stream.writeSSE(buffered event)
4. sink.write = reconnectWriter
5. 后续事件直接写 reconnectWriter
```

也就是说，TS 里的事件只有两种状态：

```text
已经交给旧 writer 的事件
断线后进入 buffer 的事件
```

它没有 Python 当前这种“旧事件还在 live queue，新事件已经在 reconnect buffer”的双队列重排问题。

### 如果第一个 TS event 还没发送完，后续 event 怎么办

TS 代码里有两类写法：

```ts
await sink.write(...)
```

和：

```ts
sink.write(...)
```

`session.started`、finalize、`session.ended` 等路径是 `await sink.write(...)`，这些事件会显式等待 `stream.writeSSE` 返回后再继续下一条。

但 `onUpdate(update)` 里是同步回调：

```ts
for (const evt of translator.translateUpdate(update)) {
  turnProducedOutput = true
  sink.write('message', JSON.stringify(evt))
}
```

这里没有 `await`，所以代码自身没有提供严格的 backpressure。它依赖的是：

```text
1. JS 单线程事件循环按调用顺序调用 sink.write
2. 每次 sink.write 立即调用同一个 stream.writeSSE
3. 底层 stream writer 通常按写入调用顺序处理 chunk
```

因此 TS 版本的顺序语义是“按调用顺序交给同一个 writer”，而不是“每条事件都确认客户端收到之后再发送下一条”。

如果 old1 已经调用了旧的 `chatWriter`，随后连接断开，TS 不会把 old1 重新放回 buffer。old1 的结果只有两种：

```text
1. 已经被旧连接发送出去
2. 旧连接发送失败，old1 丢失
```

然后断线后的 new1 会进入 `sink.buffer`，在 reconnect 时补发。

所以 TS 版本避免的是“old1 和 new1 在同一个可恢复流里反序”；它并不保证“已经交给旧 writer 但发送失败的 old1 一定可恢复”。这是一个丢失窗口，不是重排窗口。

### 当前 Python 版本和 TS 的关键差异

当前 Python 是：

```text
Agent update -> SessionStream.write -> _queue.put(event)
SSE generator -> _queue.get -> yield chunk
```

`write()` 只表示事件进入内存队列，不表示事件已经交给 ASGI，更不表示客户端已经收到。断线之后，新事件进入 `_buffer`；但旧 `_queue` 里可能还有未消费事件。由于 reconnect 先读 `_buffer`，顺序就可能反。

TS 是：

```text
Agent update -> sink.write -> stream.writeSSE
```

没有额外 `_queue`，所以不会出现 `_buffer` 插队 `_queue` 的情况。

### 推荐修正方向

Python 版本应该避免 `_queue` 和 `_buffer` 两套未发送事件容器并存。推荐改为一个有序 backlog：

```text
write(event): append 到同一个 ordered backlog
reader: 从 backlog 头部按序取出并 yield
断线: 停止 reader，但 write 仍 append 到同一个 backlog
reconnect: 继续从 backlog 头部读取
```

如果短期保留 `_queue + _buffer`，则断线切换到 buffer 模式时必须先把 `_queue` 中尚未读取的事件 drain 到 `_buffer` 前部：

```text
detach_writer:
  drain _queue -> pending_old_events
  _buffer = pending_old_events + _buffer
  disconnected = true

write(new1): append 到 _buffer 尾部
reconnect read(): flush _buffer => old1 before new1
```

但这个方案仍然需要锁保护，避免 drain queue 时 `write()` 并发写入造成边界竞态。更干净的实现还是单一 ordered backlog。

### 推荐时序

```text
Client          SSE reader          SessionStream              Agent
  |                 |                     |                       |
  | /chat           |                     |                       |
  |---------------->| attach writer       |                       |
  |                 |-------------------->| writer=1              |
  |                 |                     | write old1            |
  |                 |                     |<----------------------|
  |                 | yield old1 or keep ordered backlog             |
  | disconnect      |                     |                       |
  |                 | detach writer       |                       |
  |                 |-------------------->| writer=None           |
  |                 |                     | write new1            |
  |                 |                     |<----------------------|
  | /reconnect      |                     |                       |
  |---------------->| attach writer       |                       |
  |                 |-------------------->| writer=2              |
  |                 | read from backlog head: old1 before new1       |
  |<----------------|                     |                       |
```

结论：当前 Python `SessionStream` 的双容器模型确实可能造成 reconnect 后事件反序。后续业务代码应该把 `SessionStream` 改成单一有序 backlog，或者在 detach 时把 `_queue` 未消费内容原子搬到 `_buffer` 前面。
## SSE writer 的底层 buffer 与发送失败

### 底层 writer 会不会 buffer

会，但这个 buffer 属于 HTTP/框架/内核发送链路，不属于业务可控的 reconnect buffer。

以 TS/Hono 的 `stream.writeSSE({ event, data })` 为例，可以把链路理解成：

```text
业务代码
  -> stream.writeSSE
  -> Hono/Node HTTP response writer
  -> Node stream internal buffer
  -> OS socket buffer
  -> TCP
  -> client
```

`await stream.writeSSE(...)` 成功，只能说明这一帧已经被框架接受并尝试写入响应流；它不等价于“浏览器/CP 已经处理了这条 SSE”。如果连接已经断开或写入过程中发现断开，`writeSSE` 可能 reject；如果数据已经进入 Node 或内核 buffer，应用层通常拿不到逐条消息的客户端确认。

所以 TS 版本的语义不是 exactly-once delivery，而是：

```text
连接正常时，尽量按调用顺序写入当前 SSE writer。
连接断开被感知后，后续新事件进入业务 buffer，等待 reconnect。
已经交给旧 writer 的事件，不再回收。
```

### JS 代码发送失败时如何处理

TS 版本有两个失败感知入口。

第一个是每次写 SSE 时的 promise：

```ts
const chatWriter: SessionSink['write'] = async (event, data) => {
  lastWriteAt = Date.now()
  await stream.writeSSE({ event, data })
}
```

如果调用方写的是：

```ts
await sink.write('message', data)
```

那么 `writeSSE` reject 会被外层 `/chat` handler 的 `try/catch` 捕获。当前代码会尝试再写：

```ts
await sink.write('message', JSON.stringify(translator.error(msg)))
await sink.write('message', JSON.stringify(translator.sessionEnded('error')))
```

如果连接已经坏了，这两条通常也会失败，最终 `streamSSE` handler 退出。之后连接的 abort 事件会触发，或者 handler 结束清理。

第二个是 HTTP request 的 abort signal：

```ts
c.req.raw.signal.addEventListener('abort', () => {
  switchToBufferMode(currentSessionId!, chatWriter)
  clearInterval(keepaliveTimer)
})
```

这表示服务端感知到客户端断开。`switchToBufferMode(sessionId, chatWriter)` 会检查当前 writer 归属：

```text
如果 sink.write 仍然是 chatWriter:
  disconnected = true
  sink.write = bufferWriter

如果 sink.write 已经被 reconnectWriter 替换:
  跳过，避免旧连接的 late abort 覆盖新连接
```

### 示例 1：事件已经交给旧 writer，随后连接失败

```text
t0  Agent 产生 old1
t1  onUpdate 调用 sink.write(old1)
t2  chatWriter 调用 stream.writeSSE(old1)
t3  old1 已经进入 Node/OS buffer，或者正在写 socket
t4  客户端断线
t5  abort signal 触发，switchToBufferMode 把后续写入切到 sink.buffer
t6  Agent 产生 new1
t7  new1 写入 sink.buffer
t8  客户端 reconnect
t9  reconnect flush buffer，只补发 new1
```

这里 old1 不会被重新放入 `sink.buffer`。结果可能是：

```text
情况 A: old1 在断线前已经被客户端收到
  客户端看到 old1，reconnect 后再看到 new1，顺序正常。

情况 B: old1 已经交给 writer，但实际没到客户端
  客户端 reconnect 后只看到 new1，old1 丢失。
```

这就是 TS 版本的丢失窗口。它不解决“已经写给旧连接但客户端没收到”的逐条确认问题。

### 示例 2：连接已经 abort，后续事件进入业务 buffer

```text
t0  客户端断线
t1  abort signal 触发
t2  switchToBufferMode 成功，sink.write 被替换成 bufferWriter
t3  Agent 产生 new1
t4  sink.write(new1) 实际执行 sink.buffer.push(new1)
t5  Agent 产生 new2
t6  sink.buffer.push(new2)
t7  客户端 reconnect
t8  reconnect handler 复制并清空 sink.buffer
t9  按 buffer 顺序 await stream.writeSSE(new1), await stream.writeSSE(new2)
t10 sink.write 替换成 reconnectWriter，后续事件直接写新连接
```

这段是 TS reconnect buffer 真正保护的范围：断线被服务端感知之后产生的事件。

### 示例 3：reconnect 过程中再次发送失败

```text
t0  sink.buffer = [new1, new2]
t1  客户端调用 /reconnect
t2  reconnect handler buffered = [...sink.buffer]
t3  sink.buffer = []
t4  await stream.writeSSE(new1) 成功或已交给 writer
t5  await stream.writeSSE(new2) 时连接失败
t6  abort signal 触发 switchToBufferMode(sessionId, reconnectWriter)
t7  后续 new3 写入 sink.buffer
```

注意：TS 代码在 flush 前已经清空了 `sink.buffer`。如果 reconnect 在 flush 中途失败，`new1/new2` 中尚未真正到达客户端的部分不会自动回到 buffer。后续 `new3` 会被 buffer，但 flush 批次里的事件可能丢失。

因此 TS 的 reconnect buffer 不是严格可靠队列。它只保证：

```text
断线被感知后、下一次 reconnect 成功 flush 前，后续产生的事件不会直接写坏连接。
```

它不保证：

```text
已经交给某个 SSE writer 的事件一定被客户端收到。
reconnect flush 中途失败时，已从 buffer 移除的事件一定可恢复。
```

### 丢失不处理会不会有问题

要看事件类型。

对于纯文本 delta，少量丢失可能表现为前端展示缺字、缺一段工具输出或状态跳变。后续如果 CP/前端会在 turn 完成后从持久化会话重新拉全量历史，影响可能可接受。

对于生命周期和状态事件，丢失风险更大：

```text
session.started 丢失:
  调用方可能拿不到新 session_id，后续无法正确关联会话。

item.created / tool_call.created 丢失:
  后续 delta 找不到父 item，前端可能无法拼接消息或工具调用。

session.ended 丢失:
  调用方可能认为 turn 仍在运行，状态卡住，直到上游超时或补偿任务修复。

error 丢失:
  用户只看到连接中断，看不到 agent 失败原因。
```

所以如果 ACP server 只追求“刷新页面后尽量不断流”，TS 模型基本够用；如果 Python 版本要更严格，需要引入业务层有序 backlog 和 ack/游标语义，例如：

```text
1. 每个 SSE event 分配递增 sequence。
2. 服务端按 sequence 保留 backlog。
3. 客户端 reconnect 时带 last_received_sequence。
4. 服务端从 last_received_sequence + 1 开始补发。
5. 只有确认过的 sequence 才能从 backlog 删除。
```

在没有 ack/游标之前，Python 版本至少不应该比 TS 更差：不能因为 `_queue + _buffer` 造成事件反序。短期应先修顺序问题；是否进一步处理丢失，要看 CP/前端是否依赖 SSE 作为唯一事实来源。
