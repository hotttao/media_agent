# media_ai 历史生成链迁移前只读核对

2026-10-02 对旧 MySQL、旧媒体服务和测试企业 `8afeb26b9df643952f4f816576eb2504` 的目标公开 API 执行 `media_ai_history_migrate --audit-readiness`。未写入旧库或目标库，也未导入其他 Job。脚本和复跑方法见 `media_workflow/tools/media_ai_history_migrate/README.md`。

| 核对项 | 需要 | 找到对应记录 | 来源媒体可读取 |
|:---|---:|---:|---:|
| Product | 97 | 97 | 本工具不重复探测已独立迁移的商品图片 |
| Virtual IP | 1 | 1（按唯一名称匹配） | 本工具不重复探测已独立迁移的形象图片 |
| pose / scene | 2 / 4 | 2 / 4 | 6 / 6 |
| movement | 8 | 8 | 1 / 8 |
| ModelImage / StyleImage / FirstFrame | 97 / 119 / 83 | 来源记录均存在 | 299 / 299 |
| Video | 125 | 来源记录均存在 | 125 / 125 |

合计探测了 438 个候选输入/产物媒体引用，431 个 HTTP Range GET 取得了实际数据。另 7 个 movement 在源库的 `url` 为空，目标 Material 虽有来源 ID 和文本内容，但 `object_path` 也为空：

`178b8b1e-1b5b-43f2-9bab-172e196607af`、`3097d950-768f-4e10-9e6d-d3a6adf15e51`、`5f100242-7a2a-4761-9dd3-8f844c7d2992`、`6a9e92fa-9e22-4dff-8484-e12d57806bf2`、`756d3c36-5391-4d2c-8c81-987162223c2b`、`cf5f9900-80ca-4526-932f-564a63f35793`、`ebcc3490-110f-46e2-adc4-f41c4b2c7aaa`。

182 个候选分组中，123 个关联的来源媒体齐全，59 个引用上述 7 个无视频文件的 movement。这里的“媒体齐全”不是“已迁移”：现有 Resume 要求 movement 对象 URL，不能把文本当成视频文件；但已经完成的视频产物本身可读取，不能仅凭 movement URL 缺失就把其整条历史链误判为断链。全量导入前须明确这 7 个 movement 的语义：若它们是纯文本动作，应为该输入制定显式的内容型契约；若确实需要视频对象，则须找回文件或将对应继续执行分组隔离。不能自动伪造对象路径。

另有 50 条真正断链或无视频任务记录已按源关系列入排除清单，不在 182 个候选分组内。异常回归覆盖：断链排除与合法图片中间结果保留、重复导入与 Resume 幂等、真实网关跨账户 GET 404 / Resume 403、存储暂时失败时 Job 保持待续及 Worker 不交付无效输入、Conductor 失败后 Outbox 释放并在隔离 PostgreSQL 中重新领取。没有为测试停止运行中的 Conductor 或对象存储。
