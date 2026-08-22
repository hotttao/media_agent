# Workspace Runtime Directory 落地计划

- [x] 更新 Runtime Environment、Workspace Lifecycle、UI 状态归属和实现规格。
- [x] 增加 RUNTIME_WORKSPACE_ID 配置与安全路径段校验。
- [x] 实现原子 mount identity 创建/读取与冲突拒绝。
- [x] 增加只读 Runtime Workspace API/OpenAPI。
- [x] 把 Agent Compose 从共享 named volume 改为 `/workspace/<workspace_id>` bind mount。
- [x] 增加 media-cli 命令、单元测试和 HTTP E2E。
- [x] 验证容器重启前后 storage_id 与宿主文件不变。
- [x] 运行门禁并独立提交。
