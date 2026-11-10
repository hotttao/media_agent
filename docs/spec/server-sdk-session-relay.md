# Server SDK 与会话中继

第二阶段沿用Auth/Control/Gateway事实所有者，SDK属于客户端协议适配，不新增Domain或业务服务。SessionRelay接收Fetch Request，先通过应用提供的authenticate取得可信本地用户，再由resolve(ctx,key)确定客户端及固定Workspace。浏览器的key只用于业务选择，不直接作为平台Workspace ID。

固定路径：basePath/workspaces/:key/sessions，以及其下/:session/messages、turns、interrupt、events、requests、turns/:turn/requests/:request/resolve。列表/创建使用resolve得到的Workspace；所有Session操作先在该Workspace的当前会话列表中检查归属，平台仍作最终User/应用/资源授权。不同Workspace的Session返回404。

写操作要求同源Origin；应用登录与会话仍由authenticate实现。JSON读取设字节上限，未知方法/路径404，不接收浏览器提供的平台Authorization、Cookie或目标URL。出站只使用SDK凭证。SSE原样传输并保留恢复游标，响应不缓冲；请求取消和响应流取消向上游传播。断开不等于中断Turn，中断必须显式调用interrupt。

当前平台是创建Session、提交Turn、订阅事件三步协议，不伪造上游chat端点或session.started。创建结果不明通过列表恢复，Turn复用idempotency_key；不自动重试写请求。仅支持当前IDL具有的能力，图片/排队消息端点另行实现后才能暴露。


应用个人模型连接：Control 增加 /apps/v1/model-connections 的 POST/GET，以及 /:connection_id 的 PUT 和 /:connection_id/revoke 的 POST。请求响应复用现有 ModelConnection DTO，所有权仅来自在线验证的应用凭证；GET 要求 workspace.read，写操作要求 workspace.ensure。ensureAgent 增加 model_connection_id，与 api_key 互斥，仅支持 codex。SDK 提供 create/list/update/revokeModelConnection，更新与撤销必须传 expected_version。凭据仅在服务端导入，不经 Session Relay 暴露；禁止自动重试写请求。

Runtime 中断验收覆盖 native prepare 阶段（包括已有 Session 的恢复）：取消当前准备任务并清理 bridge，持久提交 interrupted；不提交新模型输入、不启动后续 queued Turn。正常停机取消不转换为用户中断。
