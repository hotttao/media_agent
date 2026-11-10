# 已核实的来源与选择（2026-11-10）

- [wx_channel](https://github.com/nobiyou/wx_channel)：本地版本5.7.15，commit 41ce178f8e1a27a3a67e6953c6be71d2933b3014。已读取 `.agents/skills/wechat-channel-insight` 和 `wechat-channel-video-analysis`；复用其下载、hybrid 预处理、逐镜图像审核与报告能力，不复制微信协议。上游当前没有已确认商品详情/图片工具。
- [PySceneDetect](https://github.com/Breakthrough/PySceneDetect)：候选镜头切点检测，不能代替语义理解。作为普通本地视频的可选分析辅助；不成为微信分析工具的重复前置依赖。
- [Remotion 官方 Skills](https://github.com/remotion-dev/skills)：调研了 remotion-best-practices、remotion-create、remotion-render、remotion-captions。复杂排版可选用，本次普通片段编排由已安装 FFmpeg 实现，未安装或承诺 Remotion 运行时。
- [yt-dlp](https://github.com/yt-dlp/yt-dlp)：参考视频下载的提取器。平台支持、登录和风控随来源变化，下载失败需明确报告并接受用户提供的视频文件。
- [OpenAI 配置文档](https://learn.chatgpt.com/docs/config-file/config-reference)：项目配置使用 MCP bearer_token_env_var 和 env_http_headers；不硬编码 Token，不修改模型或审批偏好。

这是为本项目编写的编排技能；未原样复制第三方技能。上游能力或版本变化时，先查 tools/list 和公开 schema，再更新对应适配。
