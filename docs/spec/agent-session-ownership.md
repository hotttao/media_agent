# P3 · Session 身份、实例登记与执行接管

依据 [Agent Runtime 规格](agent-runtime.md)。核心不变量：同 Session 一个 running，不同 Session 可并行；执行提交与当前 owner/epoch 绑定；配置与 queued 顺序不因迁移变化。

## 实例和通信

实例注册先核验稳定 Agent 委托、Control 环境授权和原生创建资源。资源定位、UID、运行 generation 和 endpoint 来自环境 adapter；请求不能任意指定 endpoint。运行 generation 固定本次容器启动，Agent 在启动时解析一次并在重试中保持；同 generation 的不同 incarnation 冲突，阻止晚到注册覆盖已登记代次。K8s 使用 Pod-bound TokenReview 和 Pod 身份；Docker 使用创建服务核验的启动身份。具体资源创建属于 P5，注册 adapter 只读原生事实。

注册响应交付独立实例凭证，Gateway 仅存摘要。重复注册以配置、资源 generation、incarnation 和 expected_version 幂等，保留 draining；签发使用持久密钥版本，响应丢失可重建同一凭证。控制链路短期 Ed25519 请求签名绑定 HTTP method/path/body hash、目标 Workspace/config/replica/incarnation 与 epoch；Agent 只有公钥，不能伪造 Gateway 控制请求。托管模式的 Session API 校验签名，关闭不受执行所有权约束的旧 ACP 执行入口；独立模式仍兼容。

## Session 凭据与绑定

Session Token 使用独立持久 MAC 密钥环和 key_id，opaque、确定性重建，DB 只保存 hash 与 key_id；不使用浏览器 Cookie 或 Agent API Token。旧 Session 在首次已授权访问时分配真实新 Token，不从旧 Cookie 回填。

Gateway 用 binding_version CAS 固定 environment/replica/incarnation/epoch；扩大副本不移动有效绑定。转移先 CAS 保存 phase=transferring、transfer_id、原 owner/epoch 与目标，再在事务外操作 Agent；查询 Agent 事实完成 active 绑定，丢响应重试同意图。候选和源终止证据来自环境读端口，NotReady/NotFound 不自动证明旧执行终止。

## Agent 事实和屏障

runtime_sessions 增加 workspace/config、owner replica/incarnation、execution_epoch、last_transfer_id、handoff_id；Turn 保存 claim 时 epoch。旧行保持未归属，不能由多副本启动扫描猜测归属。持久 admission 以 workspace/config/replica 定位当前 incarnation、active/draining、drain operation/version。

所有执行写入先验证当前代次与 Session owner/epoch；claim 与 drain 使用同一 admission 事务屏障。running 可继续完成和 flush，draining 不再 claim queued。单 Session 迁移使用 Quiesce 命令持久化 handoff_id：只暂停该 Session 领取 queued，其他 Session 正常运行；当前 Turn 完成不领取下一条。重复同 transfer_id 幂等，不同意图冲突；Transfer 清除 handoff_id，新 owner 再 Adopt。整实例 drain 留给缩容协调。GetDrain 同时检查数据库 running 与本机任务/缓冲完成，返回全部 owned Session IDs；只剩 committed idle 的 Session 可直接迁移。

Transfer 在目标实例锁 Session；同 transfer_id 重复返回既有 owner/epoch，不产生第二次转移。无 running 时保留状态、handle、固定配置与 FIFO，owner 切换且 epoch+1。有 running 时仅接受 Gateway 已核验的终止证据，原 Turn 标记 failed/outcome_unknown，Session paused，queued 保留且不自动重放。正常 Adopt 只恢复原来 active 的 queued，原 paused 不恢复。显式 Resume 必须是用户请求并匹配 epoch，只 claim 原 queued。

执行回调、持久事件、请求响应、finalize 校验 owner/epoch；旧部署对已归属行的执行写也由数据库屏障拒绝。启动恢复只处理本 Workspace/config 当前副本/代次的 running；新代次不批量终结旧代次或其他副本。实时 delta 无持久 sequence，但携带 owner/epoch 并在 Gateway 按当前绑定过滤。

## 凭据与恢复

托管执行每个 Turn 开始前经 Gateway 的 execution-context 接口重取固定版本 Provider 凭据和 Session Token，不依赖原进程缓存；秘密不入 Turn input、表或日志。Gateway 核验当前实例凭证、active 绑定、epoch 和运行 Turn，并重新执行 Control 授权。网络失败和结果不明不能自动重放 running。

迁移控制请求、查询与执行均在数据库事务外。两个 Gateway 争同一绑定只可有一个意图；未完成意图由后续请求/后台协调重读收敛，不另外随机挑选目标。实例登记/排空/转移/接管/显式恢复接口同步维护 Go IDL 和 Python YAML/Pydantic。
