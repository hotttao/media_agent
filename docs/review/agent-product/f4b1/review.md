# F4b1 · 本机 Docker 产品启动与默认配置

## 结果与范围

补齐正式服务启动到真实 Session 注册的必要功能；单容器 Docker，不增加扩缩容能力。沿用现有 Auth、Control、Gateway、Edge、数据库，增加 Plugin、私有 Broker、Docker Controller 和独立 Agent 界面。k3s 已停止，Workflow 原入口保留。

- 新 Workspace 与初始固定 Release、配置版本、通知 Outbox 在同库事务内一起创建；默认 Prompt none、空资源和系统模板2，返回版本2。初始配置失败则整体回滚。
- Gateway 从 Control 校验当前身份与 owner 授权，经 Auth 自动签发单 Workspace Agent 凭据；私有 volume 按作用域加密存储，不写入公开 Spec/响应。每次取用仍检查当前授权。Controller 仅接收创建用私有参数，不决定身份或配置事实。
- 首次启动最多等待45秒完成同一 Session 注册，超时返回503与重试提示并保留 starting。界面禁止重复创建，忽略 Workspace 切换后的旧结果。
- 修复 Gateway 可执行程序同时导入两个同名 protobuf 文件导致启动 panic；重命名源 IDL 并重新生成，新增真实 main 导入图回归。
- Docker 构建不复制 .env；准备工具不读取模型登录，生成私有部署材料。Controller 有可选 HTTP/HTTPS/NO_PROXY 部署设置，支持需要宿主代理的模型访问。

## 验证

- Gateway、Control、Runtime Controller 全模块 Go 测试通过；Gateway/Control 架构检查通过。
- PostgreSQL 集成测试验证初始 Release 可供 Session 固定，以及注入初始化失败后 Workspace、配置、Release、Outbox 四表整体回滚。
- 加密凭据测试覆盖并发只签发一次、内容保密、身份隔离、篡改拒绝和授权撤销；创建协议检查私有参数不进入公开 Spec。
- UI 14文件32测试、TypeScript、依赖边界、翻译键检查通过；生产界面镜像构建成功。
- 真实 Kratos 登录经 Edge 创建验收 Workspace，自动创建的 Codex 容器 healthy，Session active/context_version=2。真实接口创建 Prompt、编辑发布 Skill、明确确认到版本3；已有 Session 仍固定版本2。
- 浏览器使用独立 API fixture，与真实 Docker/数据库检查分开。开发服务器冷编译在本机负载下出现白屏超时，改用生产构建预览执行回归；MCP 写入式凭据、明确确认与旧会话发送通过。新增首启等待用例通过，检查重复创建禁用及恢复；修正 fixture 对带查询参数 Session 列表的匹配，使新会话在重读后仍存在。共4个场景通过（目录3项及首启1项）。

## 使用与未完成项

入口：`http://192.168.2.41:18080/agent/`；启动说明见 `docker-compose/AGENT_PRODUCT.md`。私有 env、策略、密钥及验收状态不提交；保留 Gateway runtime_bootstrap volume 和密钥以支持重启。

本切片证明真实容器及 Session 注册、资源管理与配置固定。真实模型回复、流式内容、工具权限确认和历史恢复属于 F4b2，不能用 fixture 或 running Turn 替代。用户已授权临时复制本机 Codex 登录到专用验收 Workspace；模型验收后删除该 Workspace 中所有登录副本。
