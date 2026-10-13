# 核心接口的伪代码验证

2026-10-12。验证程序：harness/tools/agent_runtime_model/verify.py；执行 python3 harness/tools/agent_runtime_model/verify.py。

结果：22 个业务不变量场景通过。这是接口/状态顺序的模型验证，不是 RabbitMQ、Kubernetes、数据库或原生 Runtime 的集成测试。

## 广播与恢复

```text
openSSE(session, cursor):
  authorize(session)
  localReceiver = broadcaster.register(session)
  ensure instance queue bound(session)   # ready barrier, not a queue per SSE
  replay committed events > cursor
  merge receiver events by contiguous sequence; fill gaps from Agent API
  on broker reconnect: restore active bindings; periodic replay remains
  on one slow receiver: close only that receiver
```

重复/乱序及 commit 后漏投通过 Agent Session Sequence 收敛。delta 无 sequence、不回放。

## Session 接管

```text
beginTransfer(session, target, operation):
  Gateway TX: CAS binding_version; persist transferring intent/operation
  Agent TX: lock session; verify expected execution_epoch and target identity
    if running and no valid termination evidence: return unavailable
    if running with confirmed old executor stopped:
      finalize failed(outcome_unknown); keep queue paused; never rerun that Turn
    change owner/incarnation; increment execution_epoch
  Gateway: query actual owner/epoch even if response lost
  Gateway TX: CAS transferring -> active(actual committed owner)
  target.adopt(session, epoch): load original handle; recover credentials through Gateway
  if queue not paused: claim original FIFO head
```

排空先关闭新执行领取，完成 running 后不 claim queued。控制器只处理 CR/StatefulSet 等资源，不读取业务 Turn。两个 Gateway 竞争由 binding CAS 与 Agent epoch 分别解决，不能靠进程锁。

## 明确确认的配置更新

```text
confirm(selection):
  authorize manage and exact resource versions
  prepare immutable artifacts outside DB transaction
  Control TX: check source versions; append release + selected revision + outbox

consume(notification):
  validate envelope/type/data version; ignore unknown advisory changes
  serialize one instance/config scope
  pull confirmed target through Gateway    # no configuration cache
  prepare and verify whole immutable bundle
  query current confirmed selection again
  if superseded: discard candidate; converge to newly confirmed selection
  publish instance marker without decreasing selection_revision
  keep old Session version directories intact
  persist stable report; report through Gateway -> Control
  ACK notification; retry report independently if reporting fails
```

用户资源回滚形成新的选择序号。partial failure 不发布半成品，不以 ACK 声称全部实例成功。旧 incarnation 结果不能覆盖新代次。

## MCP

```text
listCapabilities(session):
  resolve stable Session Token
  query current authorized definitions/bindings
  list upstream capabilities; map each name/URI to a stable definition namespace
  return deterministic paginated view with scope-bound cursor

invoke(session, exposedName, input):
  recheck current authorization and definition status
  reverse-map to registered upstream, never client-provided endpoint
  forward verified identity using that upstream's auth adapter
  return result; no automatic retry of outcome-unknown write
```

同名 search 必须有不同来源前缀，禁用后即使模型仍记得旧工具名也不能调用。

## 模型发现并修正的设计缺口

- 跨库迁移需要持久 transferring 意图与实际 owner 查询恢复。
- 旧 epoch 的完成/事件/领取都要拒绝；只改 Gateway 路由不足以防旧执行写入。
- 配置发布前需要再核对选择，且必须承认跨服务最终一致窗口。
- 通知乱序与用户显式回滚必须以选择序号区分。
- queued 接管需重取凭据，不能沿用旧进程内缓存。
- 恢复范围、结果代次与共享文件隔离必须作为实现门禁。

## 后续验证

真实两 Gateway/两 Agent 与 PostgreSQL/RabbitMQ 验证广播、broker 重连、HTTP 超时和接管竞争；Kubernetes fake API/envtest 验证创建资源而不探活 Agent；Docker API fake 与真实单容器创建验证幂等。Native Runtime fixture 验证旧 Session 的固定 Prompt/Skill。模型通过不能替代这些 checks。
