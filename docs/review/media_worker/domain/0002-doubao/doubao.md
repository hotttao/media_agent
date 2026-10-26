> 历史审查记录。2026-10-26 起 execution 已归回公共技术运行层，generate 更名 doubao_generate；当前结构见 `docs/ddd/media_worker/runtime.md`。

# Doubao 接口 review

日期：2026-10-25。依据：[doubao 模型](../../../../ddd/media_worker/domain/0002-doubao/doubao.md)、[迁移 spec](../../../../spec/media-worker-doubao.md)。

P1 端口定义符合范围：Generator.Submit/Lookup/Observe/FetchArtifacts，Request 包含稳定 Key、类别、已渲染 Prompt、有序本地图片与显式生成参数；Reference 不透明；Lookup 区分 Found/Absent/Uncertain；Observation 表达五种状态；Download 按 ArtifactID 绑定本地文件。模型无 CSV、Electron、CDP、studio batch/task 或其他层依赖。

`biz/client/doubao/studio/doc.go` 仅声明实现边界，无 Generator 实现、成功占位或生产 profile 注册。没有真实生成入口，因此本批不虚构运行测试；后续真实适配器需验证稳定 Key 查询和提交不确定恢复、精确产物关联、专用实例所有权/readiness 与全局串行容量。

首个生产能力仍限定 `doubao.i2v.first_frame_prompt@1`：prompt 与恰好一张图，输出 VIDEO。多图与生图须分别真实验证和登记能力。本次不进入实现与公开队列验收。
