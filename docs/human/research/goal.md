
# agent chat 可用

请你使用 domain-modeling 了解当前项目的设计。项目很庞大，当前我希望实现最基本的 chat 功能。
1. 用户登陆(ui_platform)后，展示 chat agent 对话页面
2. 对话框可以正常对话
3. media_agent 使用 codex。在制作镜像的时候，直接使用 /home/tao/.codex/auth.json 中的配置。
4. media_agent 中依赖的 media_control 接口可以全部mock 返回空。代码用户完全不配置
5. agent 生成的对话内容，需要完整保存在数据库。

当你实现遇到困难可以参考 /home/tao/code/github/agent-platform 的是，特别是其 web UI 代码。当前项目就是这个项目的重构。UI 代码可以完全参考。

整个开发请严格遵守 harness 目录约定的开发规范。