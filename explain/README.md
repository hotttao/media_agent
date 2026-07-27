# Agent System Explain

这是一个用于理解 control-plane、agent 启动、MCP、Skills、Credentials、OAuth 登录链路的 React 资料站点。

## 运行

```bash
cd docs/wiki/agents/explain
npm install
npm run dev
```

## 当前模块

- 数据表关系浏览器：按业务域过滤表和字段，右侧可切换是否显示全部字段。
- 数据示例：用同一组示例 ID 串联 workspace、config、skills、credentials、MCP、usage。
- OAuth 登录说明：基于 `D:/Code/koala/agent-platform/docs/wiki/auth/oauth.md` 的登录过程，按步骤说明请求、数据库写入和关键表。

后续可以继续增加源码阅读路径、文件跳转、调用链图和按阶段的阅读任务。
