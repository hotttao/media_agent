# Workflow UI v1.2 实施规格

2026-10-08 用户确认 Penpot 文件 `e618892f-b98f-815a-8008-c1c303e0740f`，明确保留 Slot 以接入 media_platform。本规格不授权重做调度器、迁移历史数据或扩展 Agent/Worker 身份。

## 架构与视觉

- 复用现有 React / Tailwind 3 / Radix / FlowGram；不更换框架。
- MaterialsApp、WorkflowApp 保持 `instanceId` 与原注册 ID；App 不直接控制宿主 Router 或全局导航。
- standalone 继续 SlotProvider → SlotContainer → AppWindow → App；plugin 继续只注册两个 panel，使用宿主 Header Portal 与实例状态，不嵌套 Desktop。
- App 资源、列表/详情/编辑视图、当前资源 ID 与运行入口按 instance 隔离；切 App 再返回不得丢失当前视图和运行来源。关闭实例仍清理状态。
- v1.1 的 Plus Jakarta Sans、暖浅色渐变、白色导航、炭黑按钮、黄色强调为视觉基线；以确认后的 v1.2 交互修订覆盖 DESIGN.md 中过时的业务陈述，不改用户 DESIGN.md。
- plugin CSS 限定在本插件 UI 及其 portal，不能改宿主 body、:root、全局字体/颜色/按钮。字体随 bundle 提供，不依赖 plugin public 目录。
- 图编辑占满 App 内容区；元数据折叠，节点点击/键盘选择后显示配置；图仅表示 step_output 依赖。
- 定义工作台工具栏复用 Header Portal；左侧工具箱打开已发布模板选择浮层，读取所选版本输入契约成功后创建节点。模板读取失败不修改草稿；配置输入按需展开，审批/复用独立选择。
- 详情以全高画布为主体，元数据和执行中记录按需展开；不在图上方堆叠统计卡片。归档确认并等待服务端成功。
- 初始布局仅是 UI 投影：线性流程可用紧凑双列错位，分支保留层级布局。两种布局必须生成相同业务依赖，不写入 DSL。

## 页面和真实数据

- 工作流为默认子入口，模板、我的任务为同级页签。创建入口清楚，生成物无手工创建。
- 列表使用真实分页、搜索及已有服务端筛选。仅当前页的派生数量/筛选必须写明覆盖范围，不伪造全库 KPI、趋势或引用次数。
- 工作流区分 draft/publishing/publish_failed/published/archived，只有真实注册就绪才可运行。保存新版本不等于发布。
- 任务区分 pending/running/need_confirm/paused/awaiting_resume/终态；不能把未知状态默认为成功/等待或把历史待继续误报取消。
- 观察节点仅对实际 GetJob 快照计算进度。冻结拓扑未暴露且当前版本不匹配时使用真实步骤列表，不画当前新版本边。
- 审批仅针对当前候选位置；异步回执与最终产物区分。原生失败继续、反馈重新生成、修改输入整次重跑、历史 Resume 分开。
- 素材上传、四类 VIP 图片、有序商品多图及纯文本动作保留已实现字段/接口，不退回 URL/JSON 编辑。
- owner 专属动作不由 orgadmin 推断；尚无权限读模型时清楚解释限制，服务端仍是授权权威。

## 查询缺口策略

本轮先使用既有 API 并明确降级：Capability 不提供伪造动态目录；Grant 无读取时不显示假列表；旧版本拓扑无读取时不伪造依赖。需新增只读接口时单列最小契约及验收，不能以 UI 名义扩大业务范围。

## 状态与验证

- 中文默认，新增/改造文案提供中英翻译；密码、Token 不进 URL、本地存储或日志。
- loading/empty/error/403/409、请求中与未知响应有可恢复状态；操作错误就地呈现。
- 窄 Slot 根据实际容器宽度折叠筛选及配置，不只按浏览器宽度判断；组件能在宿主三栏使用。
- 单测：状态枚举、过滤分页、图变更、防丢引用、实例恢复。组件测试：列表异常、导航切换、密码及 portal。
- Playwright：真实认证后的素材查看、模板/工作流保存发布、提交、审批/原生失败继续/历史继续；隔离测试数据并清理。设计/host fixture 浏览器检查与真实 API E2E 分开记录，不相互替代。
- 门禁：typecheck、unit、dep-check、i18n keys、standalone/plugin build；浏览器核对桌面、窄 Slot、截图及宿主 CSS 隔离。
