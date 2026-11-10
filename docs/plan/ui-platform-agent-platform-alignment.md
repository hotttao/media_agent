# UI Platform 对齐 agent-platform

先对齐平台自身实现，再提取应用 UI SDK。源码差距与处理顺序见 [对照记录](../review/ui_design/agent-platform-alignment.md)。

- [x] 读取参考平台、UI SDK、reviewdeck 接线与本仓库实际 Chat 实现。
- [x] 记录已有能力、差距、协议差异和本仓库必须保留的规则。
- [ ] Review 对齐范围和 P1/P2 优先级。
- [ ] 在现有 ui_platform 内提取单一会话状态层并让平台实际使用。
- [ ] 对齐消息/工具呈现、受控输入与请求交互。
- [ ] 验证刷新、切换、后台 Turn、退出、权限拒绝和暂停恢复。
- [ ] 提取同一份核心为应用 UI SDK，并接入 Go Relay adapter。

不先建设第二套独立状态机；不为对齐参考平台而绕过当前 DDD、UI 分层、Session Event 与 execution_epoch 契约。
