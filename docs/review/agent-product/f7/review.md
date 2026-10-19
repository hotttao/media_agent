# F7 · 聊天长内容显示与当前范围收口

日期：2026-10-19。

## 完成

真实 Goose MCP 历史截图中，工具结果 JSON 的连续字段在窄 Chat Slot 内溢出气泡，并被聊天视口裁切。MessageBubble 现在允许 flex 内容收缩，长 URL、JSON 和连续正文在可用宽度内断行，保留全部原文和换行。用户、assistant 和 tool 使用同一规则；未修改消息解析、持久化或 Runtime。

这是现有产品显示缺陷修复，不新增 OAuth、Kubernetes、Docker 扩缩容或其他运维能力。完成审查同步 F6 的真实模型验收状态，并记录用户明确延期外部 MCP OAuth 的决定；原 2026-10-16 检查记录继续保留其日期，未冒充本轮重跑。

## 验证

- TypeScript / Vite 生产构建通过，UI 依赖边界检查通过。
- 直接渲染实际 MessageBubble，使用合成 URL、长文本、工具 JSON 做布局边界检查。480 / 320 / 240 / 358px 四个宽度，旧规则均复现横向溢出；修复后全部气泡无横向溢出、原文完整。见 geometry-fixtures.json；这是合成布局验证，不是新的真实模型调用。
- 正式入口以原用户重新登录，只读打开既有 Goose 验收 Session，1280×800 桌面与 390×844 手机视图均验证气泡、Chat 和整页没有横向溢出，原有工具结果身份文本完整；页面脚本错误 0。见 live-history.json、desktop.png 和 mobile.png。
- 未产生模型调用、消息写入、配置更新或凭据替换。纯布局变更未添加只检查 className 的单元测试。
- 新界面镜像已部署，入口 HTTP 200。

## 本次环境恢复

开始时本机刚重启，Control / Gateway / Edge 已停止，Docker 残留端口代理与分配状态，原容器启动和重新创建均报端口已分配。用户明确允许重启 Docker 后，恢复了重启前运行的 38 个容器及 Agent 三个入口服务；数据卷与正式 Workspace 凭据保留。k3s 仍 inactive。详见 deployment.json。本轮未提交 Docker 守护进程或运维机制修改。

## 剩余范围

当前已确认 Docker 功能清单没有新增未实现项。外部 MCP OAuth 实际提供商接入延期；Kubernetes 双 Pod / RWX 仍需独立可用环境；所有历史 media-cli E2E 未在本轮重新全跑，不能把历史计划的全量验收门禁全部标为本轮通过。后续新增业务功能需要明确需求，不能把这些已延期项当成本轮自动启动目标。
