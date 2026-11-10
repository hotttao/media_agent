# Reference Import

客户端 Worker 获取参考视频，不生成新媒体。wx_channel 是微信登录、浏览记录、下载、解密及下载状态的唯一事实来源；不复制其数据库或微信协议。

```mermaid
sequenceDiagram
  participant W as media_workflow
  participant C as Client Worker
  participant X as Local wx_channel MCP
  participant S as Object Storage
  W->>C: Dispatch(record_id, attempt)
  C->>W: Heartbeat / verify lease
  C->>X: download_video_for_analysis(record_id)
  X-->>C: in_progress or completed + confined relative path
  Note over C,X: in_progress retains the same attempt; poll after 5s
  C->>C: Atomically snapshot completed MP4 in private state directory
  C->>W: Recheck lease
  C->>W: Request authorized upload grant
  C->>S: Upload immutable snapshot
  C->>W: Persist result locally, then Complete(object_path)
  W-->>C: Result receipt
```

`wx_channel.download.record@1` 的 provider 为 wx_channel，mode 为 reference_import，method 为 download，type 为 record，唯一必需输入为字符串 record_id，输出 VIDEO。仅 personal_worker 可以执行。不把下载伪装为 t2v/i2v。模板仍选择一个已启用的 VIDEO Asset Kind；record_id 可用 render_template 映射，其 prompt_body 只渲染浏览记录 ID。

客户端和 wx_channel 必须位于同一台机器、可访问同一下载目录。默认 MCP 2027，仅回环 HTTP，令牌从本地文件读取；不发送平台 Token 给 wx_channel，不转发本地 Token 到重定向/环境代理。用 os.Root 读取下载目录内的常规文件；限制 256 MiB、MP4、拒绝越界和逃逸符号链接。

幂等边界为 wx_channel 浏览记录 ID。后台下载不依附 MCP 连接。已完成下载由上游复用；同 Dispatch/Attempt/record_id 固定本地快照，上传重试复用相同字节及候选 index=0。未确认上传返回 pending；本地结果仓库保证交付重放。进程重启后由现有网关重新领取同一有效 Dispatch，再按记录及快照恢复，无独立生成任务恢复扫描。

每次执行及上传前检查执行租约。被拒绝即 abandoned，无结果完成回执；租约检查暂不可用保持 pending。非法输入、下载失败、路径/文件异常返回失败。持久化下载失败需要用户在 wx_channel 显式重试；不把一次失败当成空视频。超时由能力的 2400 秒限制及网关执行资格决定。取消不取消 wx_channel 已启动的下载，也不删除其文件。

未发现商品详情/商品图片的公开 MCP 契约，输出 provenance.product_information_status=unsupported；封面不是商品详情图。商品素材从用户或独立商品来源补充，缺失时暂停依赖真实商品图的生成。

当前依据本地 wx_channel commit `41ce178f8e1a27a3a67e6953c6be71d2933b3014`。Linux/macOS 不能承诺实时微信捕获；Windows 微信就绪测试留待用户环境验收。
