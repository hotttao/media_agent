# Agent Runtime 阶段审查索引

P0–P4 的既有审查与 P5 代码一起形成基线提交。后续每阶段完成审查和提交后自动继续，最终由用户统一 review。

- [P1 RabbitMQ](p1-rabbitmq.md)
- [P2 元数据](p2-metadata.md)
- [P3 所有权](p3-ownership.md)
- [P4 MCP](p4-mcp.md)
- [P5 资源创建](p5/review.md)
- [P6 主链路验收与环境限制](p6/review.md)
- [P7 Docker 最小服务与最终审查](p7/review.md)
- [P8 Docker 原生链路联调](p8/review.md)

核心开发已完成。P6 双 Gateway/双 Agent、116 项真实 Python 测试，以及 P7 单实例 Docker 验证通过；Kubernetes 双 Ready/RWX 环境门禁仍未通过，原因为隔离 NFS provisioner 镜像无法拉取，不把资源 Pending 当作成功。Kubernetes 部署前须补齐这一项。用户已于2026-10-15接受当前结果，并要求后续先以 Docker 测试、本机关闭 k3s；P8 真实 Docker-native 联调通过，Kubernetes 存储实测延期。
