# Workflow 静态部署登录恢复

2026-10-20，目标环境 `192.168.2.38`。

## 问题与修复

迁移镜像 `media-workflow-ui:migration` 的 Nginx 只有 SPA fallback，`/self-service/login/browser`、flow 查询及 `/api/auth/me` 均返回 HTML 200。认证页面导航后再次落入 SPA，无法显示登录表单，持续加载。

新增 `ui_workflow/nginx.conf`，把认证 endpoints、业务 API、Ory 页面及明确的静态资源转发到同网络的 `media-agent-gateway:80`。应用 bundle、登录及注册页面仍从静态目录读取。API 使用 `$host:18080`，与当前规范网关地址及 Oathkeeper 规则匹配；flow 保留浏览器 Host，保证 CSRF、action 与 cookie origin 正确。未改变认证校验或业务权限规则。

配置已复制到现有 `media-workflow-ui`，`nginx -t` 通过后 reload。原配置备份在私有迁移目录 `import/workflow-nginx-before-login-fix.conf`。已更新本地 `media-workflow-ui:migration` 镜像，镜像 ID `sha256:3e446dba1071be2b46a3fd48af07e56d9e00c9c86e3f62f0575712cb061dba26`，保留当前静态业务 bundle。新增 `ui_workflow/Dockerfile.static`，供后续构建时一并安装该配置。

## 验证

- 修复前：5173 的 flow 创建与 `/api/auth/me` 返回 HTML 200；修复后：flow 创建返回 303，未登录的业务 API 返回 JSON 401。
- Chrome 全新临时 profile，分别从 `http://192.168.2.38:5173/login` 和统一入口 `http://192.168.2.38:18080/login` 进入，均显示 flow 登录表单。使用已有测试账号完成真实密码登录，进入素材库，`/api/auth/me` 返回 JSON 200，浏览器异常为 0。证据见 `browser-check.json`（统一入口检查）。
- 密码恢复返回 303，Ory CSS 返回 200，Agent 入口返回 200；见 `http-check.json`。
- 未触发 Workflow 任务执行、审批或发布。浏览器临时 profile 已删除，无账号密码、cookie 或 token 提交入库。

本次只修改部署路由，没有重新编译业务前端；没有运行全量业务测试。现有机器迁移配置、分页开发及其他未提交工作保持原状。该 Nginx 配置要求与 `media-agent-gateway` 同网络，规范 API 入口端口为 18080。
