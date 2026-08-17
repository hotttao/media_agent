# Agent 事件分级持久化规格

## 范围

本规格覆盖 `media_agent_gateway` 对 Runtime SSE 的规范化、持久化、Session 广播与终态提交，以及 `ui_platform` 对持久化事件和临时帧的消费。HTTP 路径保持不变。

## 数据模型

- `agent_sessions.chat_status text NOT NULL DEFAULT 'human' CHECK (chat_status IN ('human','agent'))`。
- 创建 Turn 与 User Message 的事务同时写 `chat_status='agent'`。
- 完成/失败 Turn 的事务同时写 `chat_status='human'`。
- `SessionResponse` 增加 `chat_status`；`status` 继续表示 Session 生命周期。
- `universal_events.sequence` 只分配给持久化事件；临时帧不占 sequence。
- 分配持久 sequence 的事务先锁定所属 Session 行，再读取该 Session 的 `MAX(sequence) + 1`；所有 Gateway 实例因此按 Session 串行分配，唯一索引作为最终防线。

## Runtime 事件分类

| 输入 | 实时广播 | 数据库 | Message |
|:---|:---|:---|:---|
| `item.started` | 无 id 临时帧 | 不写 | 不写 |
| `item.delta` | 无 id 临时帧 | 不写 | 不写 |
| message `item.completed` | 提交后带 id | 完整事件 | 完整 assistant Message |
| tool_call `item.completed` | 提交后带 id | 单次追加 | 不写 |
| tool_result `item.completed` | 聚合后带 id | 首次立即、后续 300ms 追加快照 | 不写 |
| Session 生命周期、question、error | 提交后带 id | 追加 | 不写 |

不认识的事件类型采用持久化后广播，避免静默丢失新的权威事实。

## Tool result 聚合器

- 位于 `biz/workflow`，由单个 Turn execution 独占；不放入 handler、domain 或 shared utils。
- 以 `call_id` 为 key。首次值同步持久化并广播；窗口内后续值覆盖内存 pending；timer 到期持久化最新值并广播。
- 每次 flush 都通过现有 `PersistEvent` 追加新的 sequence，不 upsert 旧 sequence。
- `Flush` 停止 timer、写入仍 dirty 的最新值并等待完成；可重复调用，无新值时 no-op。
- Runtime 返回、error、timeout 或上游 EOF 后，必须先调用 `Flush`，再持久化错误/终态和 `FinishTurn`。
- timer 写失败保存在聚合器内；下一次 `Accept` 或 `Flush` 返回该错误，使 Turn 失败，不得提交成功终态。

## Tool result 规范化

- Runtime 可选实现 capability provider；Codex 返回 32 KiB 上限，未知 Runtime 使用 32 KiB 安全默认值。
- 按 UTF-8 字节保留头尾，输出中间插入截断说明；清除 `U+0000`。
- 在 item content part 上写入 `truncated`、`original_bytes`、`stored_bytes`。
- 规范化只在进入聚合/持久化前执行一次；同一规范化 payload 用于广播和数据库。

## SSE 与 UI

- `sessionevents.Event.Sequence == 0` 表示临时帧。
- handler 对临时帧写空 SSE id，不比较或更新 cursor；持久化事件保持现有去重规则。
- UI parser 对无 id frame 调用 `onEvent(event, currentCursor)`；仅有效且大于当前 cursor 的数字 id 更新 cursor。
- `SessionResponse.chat_status == 'agent'` 时，刷新后的 Chat App 进入 streaming；`session.ended` 后刷新 Message 和 Session 列表/状态。

## 架构映射

- `biz/domain/agent_session`：Chat Status 不变量、Runtime event 分类值对象。
- `biz/service/agent_session`：单个持久化事件与 Turn/Session 原子状态变更。
- `biz/workflow`：Runtime 调用、tool result 聚合、flush 和副作用顺序。
- `biz/dal/repo/agent_session`：数据库映射；不包含聚合策略。
- `biz/handler/chat`：DTO 与 SSE framing；不包含事件分类规则。
- `ui_platform/src/domains/chat/lib`：SSE 协议消费；页面只组合状态。

## 验证

- Domain/service：分类、仅 message completed 生成 Message、chat status 原子更新。
- Workflow：delta 不落库但实时到达；首次/窗口/终态 tool result flush；持久化失败阻止 completed。
- Shared/handler：临时帧不被 cursor 丢弃，持久化事件仍去重。
- UI lib：无 id frame 被消费但不推进 cursor；重复 id 被忽略。
- 执行 Go 单测、race（目标包）、arch-check、UI Vitest、TypeScript build 与 dependency check。
