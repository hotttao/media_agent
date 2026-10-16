# Session 首次认证失败与重复发送500修复

## 事实与修复

用户 Session dcfa20e7-3a81-4af2-8bb4-39b050f64607 位于旧验收 Workspace d41d11cf-dba2-48d8-94c2-74f81a65a9fb，固定配置版本1，模型 API Key 为空且原生登录文件不存在。原生 ACP new_session 实测返回标准 Authentication required；首轮失败后 Agent 将 Session paused。后续提交被 Agent 以400拒绝，Gateway 原先包装成500。

- 标准 ACP 认证错误转换为 model_authentication_required，错误分类不包含原生响应或凭据。
- Chat Session 查询区分登记 status、Agent runtime_status 与 execution_epoch。暂停时提交在 Gateway 明确返回409/session_paused；其他 Agent400/422保留为400，服务失败返回502，不透传私有响应体。
- Chat 显示暂停状态，禁止直接发送，使用既有有授权和 epoch 校验的 Resume API。用户明确恢复，使用稳定 operation_id；刷新服务端事实，不重发失败输入。Edge 精确开放已有 resume 路径并复用 Oathkeeper。
- turn.failed/turn.interrupted 查询当前 Session 事实并提示失败；历史失败不覆盖当前已恢复状态。同步 POST 失败移除未持久化的乐观消息。

## 验证

Gateway handler/workflow/client 测试通过（包括暂停提交无副作用、错误状态码及私有体不泄漏），架构检查通过。Python执行回归10项通过，覆盖 new/load/prompt 三条原生认证错误路径及失败后不执行下一 queued；5文件 mypy 通过。前端40项测试、类型检查通过；浏览器验证暂停前不可发送，显式恢复后显示输入，并确认未自动发起 Turn。生产界面和 Gateway 镜像已构建部署。服务重启完成后，真实用户 Session 查询返回 paused/epoch=1，重复提交返回409/session_paused，未创建新 Turn。

用户后续要求将 auth.json 升级为正式个人模型连接，本切片不以手工复制登录宣布模型功能完成；正式导入、Workspace 选择、Agent 凭据加载及真实模型复验在下一切片实施。已撤回临时登录及临时代理策略，原始本机登录不修改。
