# 时间线与剪辑

`tools/short_video.py` 使用 FFmpeg 做确定性剪辑，不需要 Remotion 服务或额外模型。路径相对 manifest 文件，所有路径需指向真实本地素材。产品目录 plans 保存时间线，finals 保存输出。脚本会归一化帧率、尺寸、像素比和音频；没有音轨的片段补静音；短片段不会通过冻结尾帧掩盖时长不足。

```json
{
  "version": 1,
  "max_generation_seconds": 10,
  "clips": [
    {"path":"../segments/presenter.mp4","role":"presenter","start":0,"duration":10,"generated":true,"semantic_beat":"完整说明购买痛点"},
    {"path":"../closeup-videos/product.mp4","role":"product","start":0,"duration":10,"generated":true,"semantic_beat":"展示已核实的产品细节"},
    {"path":"../usage-videos/use.mp4","role":"usage","start":0,"duration":10,"generated":true,"semantic_beat":"实际使用及行动引导"}
  ],
  "voiceover": "../segments/voice.wav",
  "music": "../segments/music.wav",
  "captions": "../segments/captions.srt"
}
```

voiceover、music、captions 可省略。没有 voiceover 时保留各片段原声；有 voiceover 时替换原声。音乐按0.12音量混入。字幕输入 UTF-8 SRT，需要 FFmpeg libass 与本机中文字体。语音不足时会补静音，因此必须验听，不能仅凭渲染成功就声称口播完整。

```bash
python3 tools/short_video.py validate short-video/products/<id>/plans/edit.json
python3 tools/short_video.py render short-video/products/<id>/plans/edit.json short-video/products/<id>/finals/v1.mp4
```

默认1080×1920/30fps，等比缩放并补边，防止裁掉产品。`--width` / `--height` 可调整。输出已存在时拒绝覆盖，另取版本名。render report 状态为 rendered_awaiting_visual_review。渲染后再 inspect 抽帧，并检查音频、字幕、商品字样和颜色；成片不得把产品错误细节当“AI创意”。

当前支持顺序切换，不包含自动交叉淡化或双轨画中画。需要动画字幕/复杂版式时，可另用 Remotion 官方 skills，但安装后应核查许可及与现有时间线的关系，不将其作为本流程运行前提。
