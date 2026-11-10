# 视频理解与语义分段

微信本地仓库已有完整工具。连接 `wx-channel` MCP 后：

1. get_runtime_status 确认媒体工具与页面能力；list_local_videos/get_local_video 定位记录。
2. download_video_for_analysis(record_id) 下载到 completed。需要文件时 get_local_media_metadata，source=download。
3. start_video_analysis(record_id=<download record id>, source=download, mode=hybrid, use_external_model=false)。保存 analysis_id。
4. get_video_analysis_status 直到 completed。此时只是预处理完成，review_status 仍可能 awaiting_codex_review。
5. get_video_analysis_shots(analysis_id,page,page_size=1,include_images=true,image_offset=0)，按 next_image_offset 读完所有帧，再翻下一镜。记录每张图的真实时间。
6. save_video_analysis_review 逐镜保存 description、evidence、confidence、shooting_plan、video_prompt；使用上游工具 schema 支持的 frame_analyses、transcript、storyboard 等字段。直到 review_status=completed。
7. get_video_analysis_report 获取摘要和 JSON/HTML 引用。复制可访问的报告到产品 analysis 目录，并保留来源 ID。

口播、OCR 和视觉推断分开。截图可证明该时间点存在字幕，不能证明整段字幕展示区间或音轨实际说了这些内容。ASR/OCR 后端缺失应报告 unavailable，不能填空后宣称没有语音或字幕。不要用响度推断口播文案。

其他本地视频：`python3 tools/short_video.py inspect <video> <analysis-dir>` 提取有时间戳的抽样证据；用 `scenedetect -i <video> detect-adaptive list-scenes`（安装后）获得候选切点。将候选切点与逐镜画面、可得语音证据结合。抽样未覆盖的部分需要进一步取帧，不把均匀抽样宣称为全片理解。

分析文件记录：reference 来源；原片时长；每镜 start/end；原文与卖点；完整语义单元；画面/声音证据；重写建议；不确定性。生成分段另行记录 target_duration、presenter、user、scene、product_refs、first_frame、last_frame、dialogue、camera、continuity。可合并原片多个镜头到一个卖点段，也可把一个长镜的两个完整卖点拆开；不截断一句口播或一次关键展示动作。
