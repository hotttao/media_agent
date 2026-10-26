# GPT 图片 Worker / image-use web

## 范围与契约

生产 profile `gpt_image_use`；能力 `gpt.t2i.text_prompt@1`（prompt）与 `gpt.i2i.multi_image_prompt@1`（prompt + 1～4 有序 images），输出单个 IMAGE，通过既有 Worker Gateway 注册/领取/Heartbeat/授权上传/Complete。Provider 为 gpt；模板仍选择自己的图片 Asset Kind。发布新增 capability migration；MultiImagePrompt 的 max_items 允许声明有限上限，旧 max_items=0 仍表示无限，不修改旧发布版本。

GPT 使用独立 domain/gpt、workflow/gpt_generate 与检查点仓储，不共用豆包生成状态机；原流程正名为 workflow/doubao_generate，保留原有行为与磁盘兼容性。只共用执行循环、网关上传与待交付结果日志。多图下载使用每个索引派生的独立下载 Key，防止覆盖 input-0。

## 浏览器适配与持久化

仅支持当前 Chrome 单账号串行，不提供 Studio 式多账号池。固定上游 image-use commit `7f97846e70e1b82b53baac3499f3272e9f5c1aa9`。Python bridge 只加载校验过 SHA-256 的本地上游脚本，入口固定 `--backend web --profile relay --no-style --keep-conversation --keep-tab`，关闭升级检查。提示词经 stdin 传入 Python 内存并保存在私密请求记录中，不进入系统 argv；Go 不透传上游 stdout/stderr 或网关 Token。

Adapter 在独立 ledger 内保存请求摘要与不可重复的 submit intent，文件/目录 fsync 后才允许浏览器动作。Lookup 只在 ledger 可验证时报告确切不存在；已有 intent 即使没有结果也不得再次生成。bridge 在上游唯一 Send 确认后保存绑定（确切 ChatGPT conversation、单一用户 turn 的稳定 ID），成功时 fsync 文件及单个 artifact manifest。恢复只调用上游 recover，在锁内复核原 conversation 与 user turn ID，再限定原会话的最后一轮，下载后再次复核身份；缺少绑定或不确定发送保留 needs_human，已知绑定的读取失败保留 pending，不回退到 Codex。恢复探测限频，文件校验成功前不上传。

所有 Worker 共享浏览器准入锁，并保持单槽；上游已有跨工具 ChatGPT 锁继续生效。不读 Cookie/Token，不新建临时浏览器替代已登录 Chrome，不自动处理平台挑战，不删除用户会话。运行时进程取消仅终止自己启动的进程组。Chrome 扩展的首次连接需要用户在浏览器完成。

## 配置与验证

配置走 config.proto → pb-config → 共享 loader，新增 gpt_image_use 子段，包含 Python/bridge 命令、上游 CLI 路径、浏览器锁目录、图片 size 提示、网页模型标签、可选 Project URL、超时。凭证继续使用既有环境变量，backend 不可配置。

测试覆盖 backend 固定、上游 hash、意图先于效果、响应丢失/崩溃无重发、会话绑定恢复、单图精确产物、文件篡改/不安全路径、全局串行、多图不覆盖及原 Doubao 回归。真实 Chrome 和公开网关验收单独记录，不把 fake CLI 当真实成图。
