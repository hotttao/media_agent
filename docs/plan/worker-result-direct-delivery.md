# Worker结果直接交付落地

依据[spec](../spec/worker-result-direct-delivery.md)，不涉及UI或历史迁移。

- [x] DDD/契约：原生确认、旧Inbox存档。
- [x] 同步代理与精确Task确认、移除生产发送器及新结果入库。
- [x] Personal私有结果文件/恢复交付；Managed同执行内重试发送。
- [x] 单测/race与真实子进程保存后退出恢复。
- [x] 真实Conductor、隔离PG、全量和架构回归。
- [x] 停旧版本、核对pending=0，部署后公开单样本复验。

不得用本地文件替代Provider幂等，不转移旧Task结果到新Task，不以历史202回执当原生确认。其余审批/取消/旧代码/输入覆盖/编译权限/trace/wiki继续由主plan跟踪。

2026-10-07证据：原生临时/永久失败6.14s PASS；实际Conductor已提交更新的响应丢失→精确查询确认/过期lease同结果重放仅一次POST，4.14s PASS；隔离PG授权27.27s、失败直接代理22.25s、真实Conductor+隔离PG Managed40.67s PASS。Workflow全量单测、关键race/Worker全量race、架构/diff检查通过。Personal真实子进程落盘退出后恢复发送不调用executor。旧运行库只读核对93条均submitted；未停旧版本/未部署本批，不替代部署切换检查与公开单样本复验。

2026-10-07部署复验：停止旧API/内部Worker后确认pending/failed=0、schema23/dirty=false。API镜像`sha256:a0c66bd90b004d5a552abff209464c1020ad188b8906e3c3fb213b9af664074a`、内部Worker`sha256:ec96ae06f3dcf82e4fb709d7a24aad950230b1caa92cb0cc23d23515b4157b8d`运行且重启0。CLI dry-run后唯一Submit键`direct-delivery-20261007-single`，Job `job_d6a47436-ed95-4c77-b5be-79e50d960a99`、原实例`ab7a6f19-1053-4aaf-a1ed-3a8984a2cd11`。配置预检14.94s、公开HTTP反馈/重放+独立CLI原生Retry/多候选确认+独立客户端mock执行33.28s PASS，3PNG+1MP4独立Range206子门禁2.15s。四节点succeeded，6个Dispatch交付记账；Inbox仍93条、无新增。客户端已停止，后端和5173/Vite常驻HTTP200。没有真实Provider调用、人工引擎修复、UI变更或历史迁移；其他全局missing不因本项通过而关闭。
