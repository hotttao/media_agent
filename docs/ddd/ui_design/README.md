# UI Platform Design

本目录维护 UI 平台的交互与扩展契约。业务术语、不变量和服务工作流仍以相应服务的 DDD 文档为准。

## 当前范围

当前 UI Platform 提供 Workspace Shell，以及 Sessions、Chat、Workspace Settings 三个内置 App；管理员还可从 Menubar 进入平台级 User Management 页面。User Management 不属于某个 Workspace，因此不占用 Workspace Slot。暂不实现 plugin loader、内置工具 App、复杂 Workspace 配置、拖拽布局和弹出窗口。

## 阅读顺序

1. [glossary.md](glossary.md)
2. [architecture.md](architecture.md)
3. [app-shell.md](app-shell.md)
4. [slot-model.md](slot-model.md)
5. [state-ownership.md](state-ownership.md)
6. [responsive-layout.md](responsive-layout.md)
7. [extension-model.md](extension-model.md)
8. 需求涉及的用户旅程与 `decisions/`

Agent Request 交互见 [agent-request-flow.md](agent-request-flow.md)。
浏览器认证、恢复与登出见 [authentication-flow.md](authentication-flow.md)。
企业素材创建、图片上传与详情维护见 [material-library-flow.md](material-library-flow.md)。
Workflow 选择输入并创建执行见 [workflow-execution-flow.md](workflow-execution-flow.md)。
画布内逐节点运行准备的新交互见 [Spec](../../spec/ui_design/workflow-run-wizard.md)、[Plan](../../plan/ui_design/workflow-run-wizard.md) 与 [ADR 0006](decisions/0006-in-canvas-run-wizard.md)；替代独立的整页输入选择流程，保留真实审批和 Slot 接入。
Workflow 可视化定义与编辑见 [workflow-authoring-flow.md](workflow-authoring-flow.md)。

Penpot v1.2 的七个核心用户旅程及交互修订见 [workflow-v1.2-flow.md](workflow-v1.2-flow.md)。用户已确认设计，视觉沿用 v1.1，实施保留 Slot 接入契约。后端查询支持边界见 [对齐审查](../../review/ui_design/penpot-v1.2-backend-alignment.md)，实施见 [spec](../../spec/ui_design/workflow-v1.2.md) 与 [plan](../../plan/ui_design/workflow-v1.2.md)。

## 设计与验证

| 设计对象 | 主要验证 |
|:---|:---|
| Slot 状态转换、Profile 归一化 | 单元测试 |
| App Window、空态、表单状态 | 组件测试 |
| 创建/更新 Workspace、切换 Session、Chat | Playwright E2E |
| 用户列表、创建与软删除确认 | Playwright E2E |
| HTTP 接口和凭证安全 | `media-cli` HTTP E2E |

计划、实现规格和审查产物分别保留在 `docs/plan`、`docs/spec`、`docs/review`。

Agent Prompt/Skill/MCP 管理、Workspace 固定配置及明确更新见 [agent-product-flow.md](agent-product-flow.md)。企业资源库属于企业管理入口，Workspace Settings 继续使用现有 Slot App。
