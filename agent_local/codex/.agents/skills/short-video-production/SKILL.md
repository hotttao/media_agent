---
name: short-video-production
description: 制作约30秒的原创带货短视频；从抖音或微信视频号参考视频获取、逐镜理解、完整语义分段，到人物与商品场景素材、首尾帧、豆包分段生成和本地剪辑。用于本仓库 agent_local/codex 工作区。
---

# 带货短视频

先阅读工作区 AGENTS.md。所有业务素材写入 short-video；使用 `tools/short_video.py init <product-id>` 建立目录。不得把样片、分镜预处理或生成提示词当成成片。

1. 获取参考。微信先 `wx-channel.get_runtime_status`，然后解析分享链接、定位已捕获浏览记录。`download_video_for_analysis(record_id)` 返回 completed 后再分析。仅有分享链接时，让用户在运行 wx_channel 的微信中打开目标形成记录；不得把 share_reference 当 record_id。抖音用 `tools/short_video.py download <https-url> <references-dir>`；失败时保留失败状态，不能拿封面替代视频。
2. 逐镜理解。按 [analysis.md](references/analysis.md) 完成全部镜头的画面、字幕/口播、时长、卖点、证据、置信度和缺失信息。微信优先复用本地 wx_channel hybrid 审核流程。普通本地视频先 inspect 采样，可用 PySceneDetect 补充切点，必要时提取音轨并转写；切点不是完整语义边界。无转写工具时明确缺口，不能伪造已听到的口播。
3. 确立新品脚本。区分 buyer（付费决策者）、user（实际使用者）、presenter（带货达人），选择可复用的人物/场景资产。成人给孩子买商品时，母亲可做达人，使用场景必须呈现真实使用者。依据真实商品详情制作产品卡，未核实的功效不入脚本。wx_channel 无已确认商品详情/图片接口；不得把封面当商品图。缺少商品细节/图片时先完成分析和脚本草案，并请求必需素材，暂停依赖其真实性的生成。
4. 规划约30秒、27–33秒的完整故事。参考结构可为痛点5秒、展示8秒、使用验证9秒、行动引导8秒，但按语义调整。每段4–15秒且不超过当前 Worker 的 duration_seconds；默认按10秒规划。长句/动作先重写，避免机械均分截断。不要逐镜复制原视频，改变文案、构图、场景和镜头组织。
5. 素材与生成。保存产品近景图、实际使用人群的场景图、达人图及场景定位。每段记录首帧/尾帧提示词、人物/产品参考、对白、动作、镜头、准确时长、连续性约束。使用现有 GPT 图像能力批量生成并审核首尾帧；使用豆包多图能力传有序图像。多图参考不等于严格的尾帧约束，现有工具未明确支持时不要承诺尾帧锁定；在生成提示词说明目标并审核实际结果。生成后记录 job_id、对象路径和本地文件，保留各项幂等键，只重试失败项。
6. 编排。将达人片段、商品特写视频、使用场景视频纳入统一时间线；每个条目指定 role、start、duration、semantic_beat。按 [editing.md](references/editing.md) 编写 manifest、validate 后 render。配音/字幕优先于背景音乐，字幕必须匹配核实口播。不使用参考视频片段充当原创生成结果。
7. 验收。用 ffprobe 及抽帧检查成片时长、竖屏画幅、画面和首尾连续性，实际听取音轨检查截断/静音/音乐盖人声。检查人物/商品一致性及商品细节；无法实际听取时明确写入待验收。交付 finals 视频、render report、时间线、分镜、商品素材缺口和生成记录。发布不属于本技能的默认动作。

工具职责及调研来源见 [sources.md](references/sources.md)。不调用额外模型 API；默认由当前 Codex 会话完成理解与审核。
