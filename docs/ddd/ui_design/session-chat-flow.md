# Session 与 Chat 旅程

Sessions App 加载当前 Workspace 的 Session，支持创建和选择。选择后把 Session ID 写入 URL query；移动端通过 Dock 激活 Chat。URL 指定无效 Session 时回退第一个有效 Session，并 replace URL。

Chat App 为活动 Session 建立独立事件订阅，先加载 Message 历史和 Session `chat_status`，再以最后 Universal Event sequence 回放并持续接收广播。发送 turn 只提交命令，不承载响应流；同一 Session 在多个页面打开时，每个页面都通过自己的订阅实时合并相同事件。带 SSE id 的 Universal Event 推进 cursor；无 id 的 Live Event Frame 只更新内存流式视图。流式响应按 item 标识合并，`item.completed` 用完整内容替换 delta，终态后从服务端刷新历史。

切换 Session 或 Workspace 时必须关闭旧订阅；旧订阅不得把消息、错误、结束状态或 streaming 状态写入新会话视图。网络断开采用有限退避重连，并携带最后已处理 sequence；重复 sequence 必须忽略，临时帧不得改变 sequence。页面关闭、刷新或订阅失败不得取消 Turn。刷新后若 `chat_status=agent`，Chat App 保持 streaming/busy，直到收到终态或刷新后的 Session 状态变为 human。

空态：无 Workspace 时引导创建；无 Session 时引导创建 Session；无消息时展示输入提示。请求失败在对应 App 内显示可读错误，不导致整个 Shell 崩溃。
