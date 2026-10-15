# F4a · 企业资源库与 Workspace 配置界面

## 完成范围

ui_platform 新增企业资源库：Prompt 创建、编辑、固定历史读取与回滚；Skill 在线创建、归档包上传、Git 来源和手动同步、文件增删改与执行权限、草稿发布、历史下载；资源私有/企业/部门授权、归档及旧资源一次性认领企业。MCP 目录按企业管理员/平台管理员显示管理能力，支持无需认证、Bearer、API Key、OAuth 四种类型。

Workspace 设置接入 Prompt 三种来源、已发布 Skill 和 MCP Binding 的固定版本选择，先展示确认再发布；保留未显式修改的历史版本和重试幂等键。MCP 连接支持凭据只写、保持/清空和 OAuth 用户连接/断开。秘密不写入 URL、localStorage 或配置 Release。只读 Workspace 不提供保存能力，权限来自新增 Control permissions 端口。原有会话继续使用固定元数据，只有 Agent 类型变化需要新会话。

Workspace 创建补齐真实企业/部门选择。Gateway 聊天入口复用当前 Auth/签名 Edge 身份，拒绝后不退回旧 Cookie。公开 MCP 历史查询校验当前企业可见性后读取固定版本。Edge 增加 Prompt/Skill/Plugin/聊天/OAuth 路由，预留 /agent 前端入口并保留原 Workflow 根入口。

更新 UI 旅程和状态归属文档，新增内容提供中英文翻译和语言选择。部门授权选择暂使用现有“我所属部门”接口；扩大企业部门名称可见范围仍待用户确认，不擅自扩大权限。

## 验证

- 前端 14 个测试文件、32 项测试通过；覆盖配置固定版本、互斥来源、幂等重试、只读状态、创建 Workspace 企业归属及旧会话兼容。
- TypeScript、Biome、依赖分层、翻译键检查和正式构建通过。翻译 tracker 已更新。
- 三条 Chrome 浏览器流程通过：Prompt 编辑/历史/回滚/分享及 390px 窄屏无横向溢出；Skill 编辑/发布/固定历史下载；MCP 凭据不回显、不落浏览器持久存储，配置明确确认及旧会话继续发送。浏览器 API 为明确 fixture，真实服务行为由独立后端联调验证，尚不代表真实模型通过。
- Control/Gateway 模块 Go 测试、两个服务架构检查通过。PostgreSQL 独立数据库 + 真实 HTTP 验证 Workspace owner/editor/viewer/outsider 权限端口及企业 MCP 历史可见性通过。
- 验证记录：/tmp/agent-product-f4-{ui-tests-final,browser-final,backend-live,go-tests,build}.txt。界面截图 /tmp/agent-product-{prompt-desktop,prompt-mobile,skill-desktop,configuration-desktop}.png。

## 后续

F4b 接通 Docker 单实例正式部署、真实身份和模型会话，验证流式消息、历史与工具使用。当前未宣称产品整体完成，k3s 保持关闭，未增加扩缩容或 SRE 专项。
