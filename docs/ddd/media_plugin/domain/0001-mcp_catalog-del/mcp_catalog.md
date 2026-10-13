# MCP Catalog（已迁移）

MCP 服务注册目录由 media_agent_gateway 统一拥有，原 media_plugin 目录职责废弃。

目标设计：[Gateway MCP Catalog](../../../media_agent_gateway/domain/0006-mcp_catalog/mcp_catalog.md)。Workspace Plugin Binding 与 External MCP Connection 继续属于 media_plugin，通过 Gateway 服务接口引用 Definition ID，不共享数据库表。
