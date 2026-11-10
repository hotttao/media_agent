# 应用认证本地部署验证

2026-11-08，platform分支。已部署到当前本地Docker环境，Auth、Control、Gateway均使用本次代码构建的独立镜像，验证结束时运行正常、重启次数均为0。

部署前备份三个业务数据库，并保留旧镜像。备份、原镜像ID、Compose配置和本次覆盖配置保存在 `~/.local/state/media-application-deploy/`，目录权限0700；数据库备份与配置权限0600。新镜像命名为 `media-application-<service>:20261108`。现有服务环境变量沿用，Caller增加独立0600 RSA私钥挂载和issuer配置。

部署验证发现并修复Traefik缺少应用入口路由的问题：补充应用管理、用户、Agent、聊天和Caller公钥端点，均经过Oathkeeper决策。重启Traefik及Oathkeeper后实际请求通过。仅添加Oathkeeper规则不足以使这些路径在现有Traefik中可达。

实际执行：

- Kratos创建专用验证管理员，浏览器密码登录取得真实Session Cookie。
- 经现有18080入口完成应用注册、应用用户映射、短期凭证签发与资料读取。
- profile凭证访问Control/Gateway业务入口分别返回401/403，即使同时带有管理员Cookie也不能绕过。
- 通过管理API创建专用验证组织、团队，添加托管用户成员与workspace.create权益，配置Application Team Grant。
- 应用业务凭证创建真实Workspace返回201，重复ensure返回200且ID不变，读取Workspace及Gateway会话列表返回200。
- 删除Team Grant后Control立即以401拒绝旧业务凭证；应用停用后资料访问401。
- Caller公钥端点经网关返回200，仅包含公钥；未执行真实业务MCP工具回调或模型推理。

验证管理员的Kratos Identity已删除；最后一次验证应用已停用，专用验证组织、团队和Workspace作为验收数据保留。早期失败尝试也可能留下deploy-*验证数据，不能将其当作用户业务数据或用广泛删除语句清理。

状态码记录见 [验证结果](application-platform-deployment.json)。私钥、密码和应用secret不写入仓库。SDK类型声明在部署核验时修正为实际响应的用户id字段，SDK四项测试通过。

重建容器应沿用manifest中原有Compose文件，并追加对应service的本次override文件；直接执行原Compose的up可能恢复旧image或丢失Caller密钥挂载。回退需结合新增schema和托管用户安全边界评估，数据库dump及旧镜像均已保留。

尚未验证真实模型推理、Agent工作负载创建及业务MCP Caller回调；本记录不将会话列表成功视为推理成功。


## 2026-11-10 受管理 Runtime 恢复与 SDK 验收

本机沿用 `/data/agent-product/product.env` 和 runtime-policy，以及已有 Gateway `.secrets` 的签名/Session 密钥。恢复现有 runtime-controller、product-broker、agent-model-proxy；Broker 的旧固定 IP 与当前 Gateway 冲突，通过保留原卷重新创建 Broker 恢复 Docker DNS。

Gateway 启用 AGENT_METADATA_ENABLED、AGENT_RUNTIME_ENABLED、AGENT_WORKLOAD_ENABLED；设置原 AGENT_ENVIRONMENT_ID、runtime 签名文件、Session key 文件、Docker identity URL/credential 和 product Broker URL。MEDIA_AUTH_AGENT_GATEWAY_TOKEN 从当前 Auth 配置复用，AGENT_METADATA_GATEWAY_CREDENTIAL 与 Control 保持一致。保持现有 Caller 和 MCP 配置。秘密只保存在本机私有 override，未写入本文。

原部署 override 的备份保存在 `~/.local/state/media-application-deploy/*-before-managed.json`；启用配置位于同目录原 override。回滚配置时须沿用原 Compose 文件列表，并以备份 override 最后覆盖。Codex 升级前镜像保留为 `media-agent-codex:pre-application-runtime`；当前 local 镜像包含本次 prepare 中断修复。正常完整构建入口仍为 `media_agents/codex/Dockerfile`。

实际验收通过连续两轮个人 Codex OAuth 推理、历史、SSE 游标重连和第三轮准备阶段中断。测试容器已停止；连接、Team Grant 和应用凭据已撤销。基础 Controller/Broker/代理及 Gateway 保持启用。证据见 server-sdk-runtime-smoke.json；可重复脚本见 packages/platform-server-sdk/examples/runtime-smoke.mjs。
