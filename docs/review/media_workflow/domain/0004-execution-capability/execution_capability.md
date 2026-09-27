# Execution Capability 实现一致性审查

审查基线：`docs/ddd/media_workflow/domain/0004-execution-capability/execution_capability.md` 与企业素材工作流 Spec。

## 结论

Execution Capability 当前为 `missing`。仓库内没有 capability 聚合、表、迁移、仓储、管理 API 或发布状态机。Template 仍保存并依赖自由字符串 `platform/gen_way/gen_type`，Worker 也用这三个字段 claim StepRun。

## 影响

- 无法把“即梦首帧+提示词”“即梦多图+提示词”“豆包首帧视频”等能力固化为可版本化契约。
- Workflow 发布时无法冻结 Worker 能力版本，也无法验证输入 schema 与唯一输出类别。
- Provider 重试、超时与 Task Definition 路由没有权威来源。
- Worker 可领取范围无法按 Runtime Environment、Workspace、Capability 精确授权。
- Conductor Step Fragment 无法安全解析到预注册 SIMPLE Task。

## 迁移要求

建立 `(capability_key, version)` 不可变聚合后，将现有 `platform/gen_way/gen_type` 数据映射到 Capability；新写 Template API 必须只接受 Capability 引用。旧字段仅用于历史 Job 收尾，不再参与新 Job 路由。
