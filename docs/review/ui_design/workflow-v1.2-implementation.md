# Workflow UI v1.2 实施审查

2026-10-08，首批范围为视觉基础、Slot/插件兼容、三类列表及认证字段。不是七个完整旅程的整体验收。

| 设计声明 | 实现位置 | 验证 | 状态 |
|:---|:---|:---|:---|
| 保留 AppDefinition/SlotProvider/AppWindow/Header Portal | 原注册 ID、Slot 状态格式与生命周期未改；两个 App 继续由 host-shim 取实例状态 | standalone 切换、plugin 两 panel 注册与 Header Portal 浏览器检查 | aligned |
| 不嵌套 Shell、不改宿主 CSS | UiBoundary、PortalContainer、plugin CSS 构建作用域、动画名称隔离、公共构建常量 | built-plugin host fixture 对比 body 配色、字体、:root Token，实际打开 Radix Select | aligned |
| 暖浅色、Plus Jakarta Sans、炭黑与黄 | index.css、本地字体包、Desktop、App 子导航 | 真实任务列表桌面/390px 截图；480px Slot 单列检查 | aligned |
| 工作流/模板/我的任务真实列表 | 三类 list，真实分页与既有筛选；模板状态和任务名称搜索明确标当前页 | API 状态归一化、列表异常/分页单测、真实读列表 | aligned |
| 跨 App 保留来源与列表状态 | workflow_launchId 与三个列表筛选/页码使用 instance state，不持久化表单秘密 | standalone 往返与宿主 instance hook | aligned |
| 密码显示/隐藏，不改 Kratos 字段 | PasswordField、OryFlowForm 动态字段、AuthPage | 密码值/表单提交不变、提交期间禁用单测；真实 browser flow 登录 | aligned |
| Workflow/Template 制作发布与权限交互 | 本轮只修状态解码及视觉，定义工作台仍需后续改造 | 未进行新建发布业务写入 | partial |
| 提交、审批、失败继续、整次重跑、历史 Resume | 仍是已有页面/协议，待按 v1.2 逐项对齐；冻结拓扑等查询缺口不伪造 | 本轮未启动或审批工作流 | partial |
| 素材详情、上传与兼容 Workflow 入口 | 已有功能保留，本轮更新导航与 Token；后续逐页对齐 | 未新增上传或资源删除 | partial |

## 测试边界与已发现问题

- `typecheck`、`dep-check`、中英 key 校验及 standalone/plugin 构建成功。插件交付为单 JS，FlowGram 样式和字体已内联；只复用宿主 React，Query 与 i18n 仍插件本地。
- 14 个原/新测试文件首轮 38 断言通过；加入密码测试后全套 15 文件、40 断言通过；列表恢复/分页再增加 2 断言，独立通过。最终以本轮后续命令结果为准。
- browser flow 认证与独立 App 往返成功。宿主 fixture 不是 media_platform 生产 loader 验收，也不冒充真实工作流执行验收。
- 首轮浏览器在模块/认证加载阶段超时；重跑使用显式诊断预算。并行构建期间独立版又在加载态超时，保留失败记录并串行复核，不把单次绿色包装成性能问题已解决。
- 插件浏览器初次载入报 `process is not defined`；Vite library 构建未自动替换依赖环境读取。改为编译期公开的 `{ NODE_ENV: 'production' }`，不注入本机 env，不新增浏览器全局 process，随后实际插件检查通过。
- standalone 没有 PopoutLayer，原控件会把唯一 App 移入不可见层。只隐藏该不支持的 UI 入口，Slot API/Profile 和未来宿主窗口能力不移除。
- 中文文件原被 .gitignore 排除。本轮显式允许 zh-CN.json 纳入版本管理，避免新环境只剩英文；用户提供的 DESIGN.md 未改。

最终复核：首批两个浏览器检查串行全绿；增强独立版用例后，搜索条件在 App 往返后仍保留，单独重跑通过；重建最新插件后，使用语义 label 定位的弹层用例重跑通过。构建目录只有 `media-workflow.js`，内嵌字体且没有未替换的 `process.env.NODE_ENV`。Vite 保持运行于 `http://192.168.2.41:5173/`，首页返回 200。没有提交或审批借用任务、创建业务资源、启动观测服务，也没有提交代码。

## 第二批：定义编辑安全与工作台

工作流：按需节点配置、容器宽度响应式面板、删除影响确认；节点改名维护依赖与最终输出，删除保留失效绑定。草稿沿用 instance state，读取不覆盖编辑；创建选择真实 Workspace，提交 Organization/Workspace 与 Scope 查询参数；创建成功进入草稿详情。旧模板引用按明确版本读取和选择。修正校验 reject_reasons/derived_edges，以及版本-only 更新回执的处理，注册中轮询真实状态。

模板：移除错误的业务用途 Capability 选项，目录缺口明确提示；手工填写已发布 Provider Key/版本，推导只读平台/生成方式/生成类型；槽位编辑维护嵌套引用，不覆盖映射，显式动作才重建 prompt/images 的有序调用映射。不同 Resource Type 各自筛选注册类型，输出只列 Generated Asset Kind；布局改为分组双栏并支持窄 Slot。

验证：编辑纯规则、校验协议、版本回执及 Provider/调用映射单测。真实浏览器只修改内存草稿，验证选择、删除取消和跨 App 恢复，未保存、归档或审批借用资源。首轮用 List 的 steps 查样本失败：List 不一定带拓扑；改为 Get 读取实际定义后通过。完整创建/发布、权限、审批及恢复的业务写入验收仍未完成，不将此只读浏览器门禁冒充完整闭环。

本批全量单测 18 文件、54 断言通过，typecheck/dep-check/中英 key 检查和双构建成功。截图复核发现展开面板后图幅变化导致边缘节点裁切，已订阅 FlowGram 视口尺寸变化重新 fit，并提供“适应画布”；再次执行真实编辑浏览器门禁通过。历史模板快照没有名称时，仅用可读头记录名称作显示标签，槽位仍取指定历史版本。

## 第三批：工作流编辑器与详情画布

按已确认 v1.2 继续改造，而非另起一套视觉：左侧炭黑工具箱、已发布模板选择浮层、暖浅色全高画布、按需右侧配置。名称/Scope/Workspace 移入设置弹窗；业务工具栏注入宿主 Header Portal，仍保留 Slot 和实例内草稿。输入类型为解释性提示，不会创建全局输入汇聚节点。节点定位放左侧，不在右侧重复列所有 Step。

选择模板先读取明确版本的输入契约；失败/错版本不改变草稿。替换模板仅保留同名绑定，异步返回时使用最新草稿，不覆盖期间的其他修改。输入配置按需展开，审批和复用使用独立选择，节点可直接设为最终输出。校验绑定草稿指纹，旧结果不会冒充当前草稿已通过。

详情移除画布上方的统计卡片堆叠；元信息与执行中列表在弹窗内查看，节点点击后才展示配置。归档须确认，服务端成功后才返回，失败保持上下文。可见性由独立授权接口负责，本批设置只读显示，不冒充保存草稿会改变权限。

截图对比确实发现并修复了画布只占约 320px 的问题：原自定义 relative 样式覆盖 absolute 工具类，导致节点缩小、下半页留白。现在工作台画布明确绝对定位并铺满父容器；浏览器断言图高超过 500px。线性图仅调整视觉初始位置，分支仍保留层级投影；单测验证两种布局依赖完全一致。

单元/组件：19 文件、59 断言通过，含模板选择、读取失败/错版本与投影一致性；typecheck、dep-check、中英 key 检查通过。真实浏览器的编辑/删除取消/跨 App 草稿恢复及独立列表恢复通过；没有保存、发布、审批或删除借用样本。生成桌面详情、编辑器、模板选择浮层与 480px 面板截图，在本机 `/tmp/media-agent-v12-*-live.png`；截图由 E2E 可重跑生成，不作为已完成所有页面的证据。

两次中途浏览器失败与开发期间 Vite full reload 对应，已记录失败而非忽略；隔离测试文件和 e2e 输出的 watch 后重新冻结代码复核。服务仍通过 Vite 提供测试，不切换 nginx 部署。模板/执行/素材与权限交互的整体验收仍未完成。

最终复核：standalone 与最新 plugin 构建成功；工作流编辑器浏览器用例 59.9s、独立 App 状态恢复 24.6s，通过。插件单独复核首轮因旧用例在已打开 modal 时寻找背景设置按钮超时（Radix 正确将背景移出可访问树），修改为直接定位新建时的必填设置弹窗后 12.3s 通过。宿主配色/字体/根 Token 不变，480px Slot 和 scoped portal 检查成功。没有把 host fixture 当成真实创建/发布闭环。standalone 大块构建仍有 >1024kB 提示，不宣称已完成加载性能优化。测试地址 `http://192.168.2.41:5173/` 返回 200；未提交代码。

## 后续范围

按 [plan](../../plan/ui_design/workflow-v1.2.md) 继续：素材详情、模板完整工作台及发布观察、执行审批/恢复与权限共享。Capability 目录、有效权限/Grant 和冻结拓扑仍按设计审查明确降级或单列最小只读支持；不扩展 Agent Token、Provider 实现或历史迁移。
