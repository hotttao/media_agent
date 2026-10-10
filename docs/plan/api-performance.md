# API 性能优化计划

- [x] 提交已有功能：a7fc7a7；用户 DESIGN.md 不包含在内。
- [x] 添加只读测量工具 harness/tools/api_latency.mjs，测 Token/Cookie 基线。
- [x] 商品图片批量读取与隔离数据库查询预算测试（1/20/200 页固定 3 次查询，空页 2 次）。
- [x] 生成素材有界并行解析，历史对象单次 HEAD 校验及签名，保留完整性检查。
- [x] JWT 明确配置不同 issuer 时提前拒绝，保留空配置兼容、完整签名/claims 验证。
- [x] 任务读取去掉无用 StepRun 查询、已成功节点无效投影事务及相同状态写入；实时 Conductor 查询和审批/轮次保护保留。
- [x] 实时权限事实有界并行，原顺序组装交集，不跨请求缓存授权。
- [x] Materials 部署、Token/Cookie 只读复测、真实网关分页/读取权限回归、相关单元/竞态与架构检查。
- [x] 最新 Workflow 镜像完成构建/部署、任务/模板/Workflow Token/Cookie 同口径性能复测。
- [x] 部署后的 HTTP/独立 CLI 只读门禁、4 个产物 Range 206、原队列 119 条 awaiting_resume 不变。
- [x] Workflow 隔离事务完整回归；专用临时库及默认 Testcontainers 两条路径均通过 `-race`，商品固定查询预算两条路径亦通过。

2026-10-10：宿主机 Docker ContainerStart 长期不返回，Workflow 隔离事务测试在 Ryuk 启动阶段超时（没有执行业务断言）；新 Go 进程观察到 D 状态、`netlink_dump_start` 等待。Workflow 最后构建停在 managed-worker 构建阶段，未替换常驻 Workflow。不修改外部服务/内核配置，不以 Materials 结果冒充整体性能验收。结果和后续命令见 [性能记录](../wiki/api-performance.md)。

后续：网络等待短暂解除后部署完成，镜像 14c61749594e，迁移入口仍关闭，功能只读/CLI 门禁与两种身份的完整性能复测通过。随后 Docker 再次阻塞，两项 SQL 复跑在 ContainerStart 超时；追加显式专用 PostgreSQL 临时库路径（默认 Testcontainers 不变），其安全校验单测通过，但实际 Workflow 测试又在 Go 初始化的内核网络等待卡住，测试开始前超时。该备用路径没有被当成 SQL 验收通过；最终事务回归仍未勾选，需要先处理宿主机网络阻塞。

不启动生成 Worker，不恢复原 119 条历史任务，不开启观测后端，不修改权限模型或编排语义。

## 维护后最终验收

用户明确授权宿主机 Docker/k3s 维护后，读取内核日志确认多个进程和 IPv6 地址工作队列等待网络锁；发现旧 k3s 工作负载有高重启计数/探针失败。仅临时停止 k3s 控制进程完成隔离，不删除集群对象/数据卷、不重启 Docker、不调整用户或内核限制，然后恢复 k3s 为 active/running，节点 Ready=true。

专用临时 PostgreSQL：Workflow 事务 23.45s、商品查询预算 8.64s，通过；默认 Testcontainers：Workflow 51.74s、商品 33.31s，通过。临时数据库和自身测试容器已清理。恢复后的 Materials 网关、Workflow HTTP/CLI、四个产物 Range 206 再通过；5173、9001 为 200；队列仍 271=145 succeeded+7 cancelled+119 awaiting_resume，active=0。

本轮接口优化和验收完成。k3s 停止期间阻塞缓解只是关联证据，不宣称旧工作负载的重启问题或内核根因已永久修复；长期治理独立于本轮 API 优化。
