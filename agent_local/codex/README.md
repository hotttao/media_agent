# 本地带货视频工作区

在 Codex Desktop 中打开此目录，或安装 Codex CLI 后运行 `python3 tools/launch.py`。目前执行环境已有 FFmpeg、ffprobe、yt-dlp，未安装 Codex CLI/PySceneDetect；Desktop 可直接使用目录中的 AGENTS.md 和 `.agents/skills`，PySceneDetect 仅为可选工具。

```bash
python3 tools/setup.py --wx-downloads /absolute/path/to/wx_channel/downloads
python3 tools/short_video.py doctor
python3 tools/short_video.py init <product-id>
```

本机已配置下载目录 `/home/tao/code/github/wx_channel/downloads`，并保留原有 media MCP 配置。当前该目录还没有 MCP token；需启动 wx_channel 使其生成 `.wx_channel_mcp_token`。项目配置的 http_headers_helper 会从该文件读取 Token，Desktop 重新打开工作区后即可使用；不会把 Token 写入配置。`launch.py` 也可为 CLI 加载此 Token。真实微信页面捕获优先在 Windows 上验证；当前下载的 `.exe` 不能在本 Linux 会话直接运行。

带货流程、目录约定和缺失商品素材时的行为见 [AGENTS.md](AGENTS.md)；分析与剪辑细节见 [.agents/skills/short-video-production/SKILL.md](.agents/skills/short-video-production/SKILL.md)。默认复用 wx_channel 的分镜和 Codex 审核，不需要第三方模型 API。

普通本地参考视频可先 `inspect`，编辑完分镜和 edit.json 后 `validate` / `render`；`short_video.py --help` 列出入口。真实带货生成通过现有 media MCP 执行，工具离线时不伪造结果。wx_channel 当前没有已确认的商品详情/图片工具，需另行提供真实商品资料。

验证：`python3 -m unittest discover -s tools -p 'test_*.py'`。测试用临时合成素材验证30秒编排、带/不带音轨的混排、抽帧、时长校验和拒绝覆盖；完整配音/背景音乐/SRT的30秒渲染也已本机验证。真实微信下载、平台上传和 GPT/Doubao 生成需登录后的用户环境验收。
