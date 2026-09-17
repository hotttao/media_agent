# media_gen 媒体生成上下文

负责 Workspace 内图片、音频和视频生成、语音转写及视频发布。生成与转写任务是本服务持久化事实，River 只负责唤醒执行。

## 业务术语词典

**Workspace（工作空间）**:
Generation Task、Transcription Task 和 Publish 的业务资源隔离归属；对象路径必须位于对应 Workspace。
_禁用替代词_：Scope、Worker Identity

**Generation Task（生成任务）**:
一次图片、音频或视频生成请求的持久化事实，拥有独立状态、attempt 和候选产物路径。
_禁用替代词_：Step Run、River Job、Generation Execution

**Generation Worker（生成 Worker）**:
由 River 唤醒、按 Generation Task ID 执行 Provider 调用和对象写入的无状态执行组件。
_禁用替代词_：Scheduler、Generation Task

**Candidate（候选产物）**:
Generation Task 产生的一个可选结果，以 Storage Object Path 表达；多候选任务需确认一个最终结果。
_禁用替代词_：Generated Asset（Generated Asset 是 Materials 业务记录）、URL

**Primary Candidate（主候选）**:
单候选任务自动确定或多候选任务由用户确认的最终 Candidate。
_禁用替代词_：Generated Asset、Default URL

**Generation Review（生成结果确认）**:
用户对多候选 Generation Task 作出的选择或重新生成决定；选择确定唯一 Primary Candidate，重新生成保留旧候选并开始新的 attempt。
_禁用替代词_：Workflow Output、Retry、Candidate Status

**Transcription Task（转写任务）**:
把一个音频或视频对象转换为文本与时间片段的持久化任务。
_禁用替代词_：ASR Request、Generation Task

**Publish（视频发布）**:
把一个已确认视频资源发布到指定外部平台的一次持久化动作；任务和平台发布 Adapter 均归 media_gen。
_禁用替代词_：Generation、Upload

**Publish Adapter（发布适配器）**:
media_gen 内把统一 Publish 命令转换为目标平台 API 调用的基础设施适配器；不拥有 Publish Task 业务状态。
_禁用替代词_：Crawler、Publish Domain、Channel Service

**Publish Target（发布目标）**:
Publish 的外部平台，例如 xhs、douyin 或 bilibili。
_禁用替代词_：Channel、Storage

**Publish State（发布状态）**:
Publish 的生命周期：`pending -> publishing -> succeeded | failed | result_unknown | cancelled`。
_禁用替代词_：Generation Task State
