# media_crawler 的 MCP 走 HTTP transport 而非 stdio

media_crawler 的 MCP server 采用 Streamable HTTP transport（挂 `/mcp` endpoint），而非 MCP 默认的 stdio。原因是消费方 agent 为远程调用，stdio 仅支持本地 1:1，无法跨网络；HTTP 允许远程/多客户端连接，且与同进程的 REST 展示接口共用 hertz :8004。代价是 HTTP transport 必须自行处理认证——v1 因 media_auth 未就绪暂不认证，仅 bind localhost 不暴露公网，代码留 auth 中间件位待 media_auth 接入后走 OAuth2 或 service_token。备选 stdio 虽免认证但无法满足远程 agent 需求。
