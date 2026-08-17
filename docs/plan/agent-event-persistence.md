# Agent 事件分级持久化实施计划

## P1：持久化契约与 Chat Status

- [x] 增加 `chat_status` migration、model、domain/repository/service 映射。
- [x] 更新 Chat IDL 并通过生成命令刷新 Hertz model/swagger（路由未变化）。
- [x] 提取 Runtime event 分类与 tool result 规范化规则，补单元测试。
- [x] 让 message `item.completed` 成为唯一 assistant Message 写入口。

验收：Session API 返回 human/agent；Turn 创建/终止事务同步更新；delta 不进入数据库。

## P2：聚合、flush 与 SSE

- [x] 在 workflow 实现每 Turn tool result 聚合器。
- [x] 首次立即持久化，后续 300ms 追加最新快照，全部终止路径强制 flush。
- [x] broadcaster/handler 支持 sequence=0 临时帧，不推进恢复 cursor。
- [x] 补 workflow、broadcaster 和协议行为测试。

验收：高频 tool result 的数据库写入被窗口限制；最终快照总在 Turn 终态之前提交。

## P3：UI 消费与质量门禁

- [x] UI SSE parser 消费无 id 临时帧但只用持久化 id 更新 cursor。
- [x] 刷新时使用 `chat_status` 恢复 streaming 状态。
- [x] 补纯协议测试。
- [x] 运行格式化、Go/UI 单测、架构与依赖检查、构建。

风险：timer 与终态 flush 竞争。对策：聚合器以 mutex 串行化 dirty/version 状态，timer 与显式 Flush 共用唯一写路径；测试覆盖同一窗口内重复更新和并发终止。

验证结果：Go 全量测试、Gateway arch-check、UI 全量 Vitest、TypeScript/Vite build、Biome 与 dependency-cruiser 均通过。Go race 构建需要本机 C 编译器，当前 Windows 环境未安装 `gcc`，因此未执行成功。
