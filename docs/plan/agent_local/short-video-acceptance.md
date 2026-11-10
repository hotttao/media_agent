# 带货 Agent 本地验收记录

日期：2026-11-10。对应需求：docs/human/research/010_agent_local.md。

## 已验证

- media_worker 与 media_workflow 全部 Go 测试通过，包含真实 MCP HTTP 传输认证、异步下载、路径穿越/符号链接逃逸拒绝、不可变快照重放，以及租约失效/未确认上传不完成任务的测试。
- media_worker arch-check 通过。media_workflow arch-check 未通过：原有 service/job 依赖 dal/compiler/executable、service/template 和 service/workflow 依赖 dal；此次改动没有添加这些依赖。未把既有违规当成本次通过。
- 临时隔离 PostgreSQL16容器执行全部29个 up 迁移成功；查询得到 wx_channel.download.record / reference_import / personal_worker。数据库拒绝 managed_worker 下载契约。测试容器已删除，无线上数据库变更。
- 本地 FFmpeg 测试：三种角色、带/不带音轨混排，生成30秒；另验证外部配音、循环背景音乐和SRT字幕的30秒完整渲染。输出160×284为快速集成测试尺寸，默认交付配置1080×1920。
- Python集成测试2项通过：完整渲染/抽帧/时长与非法计划拒绝，目录创建与路径限制。技能 quick_validate 通过，配置 TOML 解析和 setup 重复执行保持现有配置通过。
- 本机已有 ffmpeg/ffprobe/yt-dlp；PySceneDetect 可选且未安装。当前 PATH 中无 Codex CLI，Desktop 可直接打开工作区，CLI launcher 需先安装 CLI。

## 用户环境验收

1. 在运行微信的客户端启动本地 wx_channel 5.7.15，打开参考视频，使浏览记录与可用下载来源建立。Linux 中下载的Windows exe 不能直接运行；需要 Windows 客户端或已支持本地数据模式的运行时。
2. 确认 downloads/.wx_channel_mcp_token 出现；运行 tools/setup.py 指向真实下载目录。在 Desktop 打开 agent_local/codex 并确认 wx-channel 的 get_runtime_status 可用。项目 http_headers_helper 读取令牌，不写入 Git 或聊天。
3. 获取浏览记录ID，下载到 completed；验证 start_video_analysis 和逐镜保存审核，直到 review_status=completed。确认ASR/OCR后端状态及缺失。
4. 服务器执行000029迁移并重启服务；客户端按 media_worker/README.md 启动 wx_channel profile。创建对应能力模板、工作流与任务，确认视频经授权上传可获取、进程重启后可恢复同一任务。现有 media MCP Token/组织/工作区配置保持原样。
5. 真实商品详情与图片由用户或独立商品来源提供；上游未确认商品详情工具，因此商品信息抓取仍不可用，不以封面替代。
6. 输入实际参考和商品，运行原创脚本、首尾帧图、GPT/Doubao生成、剪辑和验听。未执行真实生成或发布，没有可交付的真实商品成片。

## 分支

先将已有 workflow 提交快进到 main/agent；微信集成在 workflow 提交后同步三个分支。然后从 main 新建 agent_local，保存本地创作工具和技能。platform 的独立两项改动没有被覆盖。远端同步使用普通快进推送，不改写历史。
