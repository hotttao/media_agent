# Koala Web 源码阅读指引

> 面向新人的源码学习路径。总分总结构：先看全局地图，再按主线抓手拆解，最后用端到端时序把所有章节串起来。
>
> 所有代码引用均带 `文件:行号`，可直接跳转。基于 `web/src` 当前 main 分支。

## 你会学到什么

读完这套文档，你应该能够：

1. 说清 `window.tos` 是什么、为什么必须共享 React 实例、内置插件与外部插件的区别
2. 画出从 `main.tsx` 到 React Provider 嵌套树的启动时序
3. 解释"全局 active-session-store + 每个 workspace 一个 agent-session-store"的双层架构为什么这么设计
4. 描述一条用户消息从输入框到 SSE 流到消息气泡渲染的完整数据流
5. 理解 Slot 模型如何用单一状态结构同时支撑布局列、弹出窗口、跨槽位移动
6. 读懂任何一段 web/src 代码所属的层与职责

## 阅读顺序

| 章节 | 模式 | 一句话 | 文件 |
|------|------|--------|------|
| [00 总体架构](./00-overview.md) | 总 | 代码地图 + 启动时序 + Provider 嵌套树 | `00-overview.md` |
| [01 主机层与插件三件套](./01-host-and-plugins.md) | 分 | `window.tos` + 注册三件套 + eager/lazy 加载 | `01-host-and-plugins.md` |
| [02 状态层与会话双层架构](./02-stores-and-session.md) | 分 | agent-session-store + active-session-store 桥接 | `02-stores-and-session.md` |
| [03 Shell/Slot 模型与聊天渲染管线](./03-shell-and-chat.md) | 分 | Desktop/SlotContext + Chat transcript + SSE | `03-shell-and-chat.md` |
| [04 端到端阅读路径](./04-reading-paths.md) | 总 | 三条主线把所有章节串起来 | `04-reading-paths.md` |
| [05 App Registry 与 Slot 装配链路](./05-slots-and-app-registry.md) | 分 | AppDefinition/AppInstance 模型 + App 来源 + per-instance state + SlotContext 与 instance-state-store 循环 import 分析 | `05-slots-and-app-registry.md` |
| [workspace-profile 字段参考](./workspace-profile.md) | 分 | 7 个 payload 字段（layout_id/selected_layout_id/slots/instances/popoutGeo/wallpaper/chatSendKey）+ ProfileBackend 双后端 + 400ms 去抖浅合并 + 加新字段流程 | `workspace-profile.md` |
| [06 React Hook 决策框架](../hook.md) | 分 | useState vs useMemo vs useRef vs Context 决策树 + deps 3 步走 + "重算是否行为等价"终极测试 + useWsApps 实战 + useState-as-ref callback + AppWindow header slot 案例 + 常见反模式 | `../hook.md` |
| [UI 页面骨架与布局搭建指南](./ui.md) | 总 | 全屏 ASCII 骨架图 + 每个区域对应哪个组件 + 从零搭骨架的 8 步顺序 + 规划新前端项目的阶段指南 + 布局设计决策记录 | `ui.md` |

## 怎么用这套文档

**第一次读**：按 00 → 01 → 02 → 03 → 04 顺序，每章先看"代码地图"建立坐标，再看"抓手拆解"理解细节，最后看"端到端时序"验证理解。

**复习时**：直接跳到 04，沿三条端到端路径回顾。卡住某个环节再回到对应分章。

**写代码时**：把分章当索引，遇到具体场景（加一个插件、加一个 store、加一个面板）直接查对应小节的 API 表与 file:line 引用。

**查 App 体系**：加新 App / 改 App 分组 / 调试 instance state 丢失 / 疑惑 SlotContext 与 instance-state-store 的循环 import 时，直接读 05。

**查 workspace_profile**：想知道 payload 里有什么字段 / 字段哪个层级（ws vs fleet）/ 加新字段怎么改 / 调试 400ms 去抖或浅合并问题时，直接读 workspace-profile.md。

**查 React Hook 决策**：纠结 useState vs useMemo vs useRef / 确定 useMemo deps 放对象还是字段 / 疑惑 useWsApps 为什么 biome-ignore exhaustive-deps / 创建 store 为什么必须用 useState + lazy init / DOM 节点要流到子组件时为什么用 useState 当 ref callback 而不是 useRef / 为什么用 Context 不用 props 时，直接读 `../hook.md`。

**查页面布局**：不知道屏幕某块区域是谁渲染的 / 想了解从零搭这个布局的步骤 / 规划新前端项目时，先读 ui.md 建立全局坐标，再按需深入对应分章。

## 前置知识

- React 18（hooks、Context、Suspense、lazy）
- Zustand（`create` 与 `createStore`/vanilla 两种用法，`useStore` selector）
- TanStack Query（queryKey、`invalidateQueries`）
- React Router v6（`Routes`/`Route`/`useParams`）
- TypeScript
- SSE 概念（不需要写过原生 EventSource，但要知道事件流是什么）

## 项目坐标速查

| 坐标 | 路径 |
|------|------|
| 包根 | `web/` |
| 源码根 | `web/src/` |
| 入口 | `web/src/main.tsx` |
| 路由 | `web/src/App.tsx` |
| 根 Shell | `web/src/components/shell/Desktop.tsx` |
| 主机 | `web/src/lib/host.ts` |
| API 客户端单例 | `web/src/lib/api/client.ts`（`export const api`，line 2147） |
| 会话状态上下文 | `web/src/stores/AgentSessionContext.tsx` |
| 会话状态实现 | `web/src/stores/agent-session-store.ts` |
| 插件注册表 | `web/src/plugins/registry.ts` |
| 面板注册表 | `web/src/lib/panel-registry.ts` |
| 外部插件加载器 | `web/src/lib/plugin-loader.ts` |
| SSE 消费 | `web/src/lib/api/sse.ts` |
| Slot 上下文 | `web/src/contexts/SlotContext.tsx` |
| 鉴权上下文 | `web/src/contexts/AuthContext.tsx` |

## 约定

- `file:line` 均相对 `web/src/`，例如 `lib/host.ts:81` = `web/src/lib/host.ts` 第 81 行。
- 章节内"代码地图"小节给坐标，"抓手拆解"小节给细节，"端到端时序"小节给串联。
- 不复制大段源码，只贴关键签名与一行注释。要全文请点 `file:line` 自己读。
