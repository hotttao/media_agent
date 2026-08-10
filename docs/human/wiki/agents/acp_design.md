# ACP Adapter Design Notes

## SessionStream event ordering

Python 版本的 `SessionStream` 采用单一 `asyncio.Queue` 作为当前 turn 的待发送事件队列：

```text
AcpSession.session_update
  -> translator.translate(...)
  -> SessionStream.write(event)
  -> queue.put(event)

FastAPI StreamingResponse
  -> async for chunk in SessionStream.read(...)
  -> yield chunk
```

这个设计的短期目标是保证 reconnect 场景下的事件顺序不被 Python 自己打乱。

之前的双容器模型是：

```text
connected:    write -> live queue
disconnected: write -> buffer
reconnect:    flush buffer, then read live queue
```

它存在反序风险：如果 `old1` 已经进入 live queue 但还没有被 reader 消费，断线后 `new1` 进入 buffer，reconnect 时先 flush buffer，就会变成：

```text
实际发送: new1 -> old1
真实产生: old1 -> new1
```

现在改为单一 queue 后，disconnect 不会改变写入容器：

```text
connected:    write -> queue
disconnected: write -> same queue
reconnect:    continue reading same queue
```

因此同一个 `SessionStream` 内部可以保证：

```text
事件进入 SessionStream.write 的顺序 == SessionStream.read 取出的顺序
```

## Writer ownership

`SessionStream` 仍然维护 `writer_id`：

```text
attach_writer() -> 分配新的 writer_id
detach_writer(expected_writer_id) -> 只有当前 writer 仍是 expected 时才 detach
```

这个机制对应 TS 版本里的 `expectedWriter`。它解决的是 late abort 问题：旧 `/chat` 连接的 abort 如果晚于 `/reconnect` 到达，不能把新 reconnect writer 切回 disconnected 状态。

```text
t0 /chat writer=1
t1 /reconnect writer=2
t2 旧 /chat abort 到达
t3 detach(expected=1) 发现当前 writer=2，跳过
```

## Close semantics

`close()` 会把 sentinel 放入同一个 queue：

```text
queue: [event1, event2]
close()
queue: [event1, event2, SENTINEL]
```

reader 会先把已排队事件发完，再遇到 sentinel 退出。这保证 turn 结束时不会因为 close 直接丢掉已经进入 queue 的事件。

## Heartbeat

`read(keepalive_interval)` 在 queue 没有事件时使用 `asyncio.wait_for(queue.get(), timeout=...)`：

```text
有事件: 立即返回事件
无事件且超时: yield ': \n\n' SSE comment heartbeat
close: sentinel 唤醒 reader 并结束
```

这里没有 `Condition.notify()` 的配对问题。`asyncio.Queue` 自己负责生产者/消费者唤醒；多次连续 `put()` 会排队，reader 后续按 FIFO 顺序消费。

## Known reliability gap

当前设计不提供逐条 SSE 消息确认。事件从 queue 中取出后，在 `yield chunk` 之前就已经离开 queue：

```text
event = await queue.get()
yield format(event)
```

如果 `yield` 之后 ASGI/网络发送失败，这条事件不会自动回到 queue。也就是说当前保证的是顺序，不是可靠投递。

这与 TS 版本的可靠性边界一致：TS 里一旦事件被交给 `stream.writeSSE(...)`，如果随后连接失败，业务层也不会把这条事件重新放回 `sink.buffer`。TS 的 buffer 只保护“服务端已经感知断线之后产生的新事件”。

可能影响：

```text
text delta 丢失:
  前端展示缺字或缺一段输出。

item/tool created 丢失:
  后续 delta 找不到父对象，前端拼接失败。

session.started 丢失:
  调用方可能拿不到新 session_id。

session.ended 丢失:
  调用方可能认为 turn 仍在运行，直到超时或补偿逻辑修复。
```

如果后续需要更强可靠性，需要引入 sequence/ack 或 reconnect 游标：

```text
1. 每个 SSE event 分配递增 sequence。
2. 服务端保留按 sequence 排序的 backlog。
3. 客户端 reconnect 时携带 last_received_sequence。
4. 服务端从 last_received_sequence + 1 开始补发。
5. 只有确认过的事件才能从 backlog 删除。
```

短期版本不做 ack，只保证 Python adapter 自身不会因为 queue/buffer 分裂造成事件反序。

## SSE delivery boundary

SSE 本身没有业务级 ack。服务端能确认的通常只是 chunk 已经交给 ASGI server 或底层写入链路，不等于客户端已经解析并处理了 event。

典型传递流程：

```text
SessionStream.read()
  -> yield "event: message\ndata: ...\n\n"
  -> Starlette StreamingResponse
  -> ASGI send({type: "http.response.body", body: bytes, more_body: true})
  -> Uvicorn/Hypercorn
  -> asyncio transport.write(...)
  -> OS socket buffer
  -> TCP
  -> client HTTP parser
  -> SSE/EventSource parser
  -> client business handler
```

在 Python generator 中：

```python
async for chunk in stream.read():
    yield chunk
    # 能走到这里，只能说明上一条 chunk 已经交给 StreamingResponse/ASGI 发送链路
```

这不是客户端确认。它不能证明：

```text
客户端已经收到 bytes
客户端已经按 SSE frame 解析完成
客户端业务代码已经处理 event
```

它只能近似说明：

```text
服务端应用层已经把 chunk 提交给响应发送链路
```

发送失败常见表现：

```text
1. generator 被取消，抛 asyncio.CancelledError
2. request.is_disconnected() 返回 true
3. ASGI send 或底层 transport 报错，StreamingResponse 停止继续消费 generator
```

但失败可能发生在业务队列出队之后：

```text
t0 event 从 SessionStream queue 取出
t1 generator yield chunk
t2 chunk 进入 ASGI/uvicorn/socket buffer
t3 网络断开或客户端未处理成功
```

此时服务端很难精确判断 event 是：

```text
已经被客户端处理
已经到达客户端但未处理
还在 socket buffer
已经丢失
```

因此当前短期设计只能保证：

```text
SessionStream 内部按 write 顺序出队
reconnect 不会因为 queue/buffer 分裂造成反序
```

不能保证：

```text
某个已经 yield 的 event 一定到达客户端
某个已经 yield 的 event 失败后会自动回队
```

如果要实现“当前 event 发送成功后才进入下一条”的严格语义，需要把“发送成功”定义到业务协议层，而不是依赖 ASGI `send()` 返回。推荐模型：

```text
1. 服务端给每个 SSE event 分配递增 sequence。
2. 客户端收到并处理 event 后，上报 last_received_sequence。
3. 服务端只删除已经被 ack 的 sequence。
4. 客户端 reconnect 时携带 last_received_sequence。
5. 服务端从 last_received_sequence + 1 开始补发。
```

没有 sequence/ack/reconnect cursor 时，SSE 只是 best-effort streaming；服务端可以控制顺序和断线后的后续缓存，但不能做 per-event delivery confirmation。