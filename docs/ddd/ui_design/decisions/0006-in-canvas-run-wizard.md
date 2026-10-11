# 0006 · Workflow 画布内运行准备向导

状态：采纳（2026-10-10 用户要求规划、设计并实现）。

## 决策

运行从 Workflow 画布内进入准备模式，不切换到任务列表。按真实 Step 的 workflow_input 绑定定位需要外部输入的节点；step_output 与 const 不要求用户预先填写。DAG 使用稳定拓扑顺序，同一入口名称只在首次消费它的节点选择一次，其他节点复用该值。准备完成后明确确认并提交一次 Job；成功在同一 App 的工作流执行画布观察真实状态。

输入选择、准备游标、复核阶段及未确认提交意图归 App instance 内存，按 Organization、Workflow 和启动意图区分。保留现有 Header Portal、Navigation Port、Slot；不增加 Shell、不用 URL/localStorage 保存运行草稿或签名 URL。

准备状态不冒充 StepRun 的 pending/succeeded。图上只高亮当前需要用户操作的节点，不生成连接所有节点的输入节点。搜索/回退/改选不触发执行，人工审批仍发生在真实执行之后。

## Product 入口

商品详情展示真实可用、同 Scope、具有 PRODUCT 入口的已授权工作流。发起时通过原 Navigation Port 传递商品 ID，目标重新读取资源与 registered 回执，预填商品，用户确认后补其余输入。不新增永久绑定事实；后端尚无 Product—Workflow 持久关系，禁止使用 metadata 或浏览器配置伪造。

## 验证

拓扑、共享输入与输入覆盖规则用纯函数测试；选择器搜索、必填推进、返回保留及显式提交用组件测试；真实浏览器仅准备/取消及跨 App 商品预填，不执行旧历史任务、不启动 Worker、不留下新的测试执行。
