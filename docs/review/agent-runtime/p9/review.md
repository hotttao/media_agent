# P9 · 可直接使用的 Agent 功能收口

P9 的交付由后续产品功能阶段实际完成；旧Runtime计划未同步勾选，本次依据代码、提交及真实验收记录补齐，未重复开发或伪造先前独立提交。

| P9要求 | 实际交付与提交 | 证据 |
|---|---|---|
| UI/Gateway/正式Agent入口、配置接通 | F4a f3969c8、F4b1 b4b6585 | agent-product/f4a、f4b1/review.md |
| 创建会话、消息、流式输出、历史 | F4b2 ab87a89 | 真实Codex/SSE/刷新历史验收与live-acceptance.json |
| 工具权限请求可见、确认后继续、MCP | F3b 0ddac58、F4b2 ab87a89 | 真实Gateway→Plugin→MCP调用、已解决权限请求 |
| Docker启动、正式模型配置与使用 | F4b1/F4b2、F5 59b3b68 | AGENT_PRODUCT.md、正式本机Codex连接与两轮模型回复 |
| 实际使用发现的修复 | 16247a2、ed7b5eb、75372ad、db0c824 | 登录、暂停/固定版本、统一Settings的独立review |

P9功能完成；Kubernetes RWX与双Ready实测、其他Runtime正式模型和第三方OAuth生产账号验收没有被本项替代。当前完整结论与检查见 [总审查](../../agent-product/completion/review.md)。
