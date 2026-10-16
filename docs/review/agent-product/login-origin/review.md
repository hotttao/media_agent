# Agent 登录入口修复

## 问题与修改

从18080进入 Agent 登录时，Kratos 的 public base URL 仍为同主机 Workflow 的5173，返回的 ui.action 带5173端口。前端严格比较 origin 后抛出 `Kratos returned an invalid login action`，尚未向 Kratos 提交凭证。此前真实模型验收复用了已认证 Cookie，未覆盖新浏览器表单登录，这属于验收遗漏。

认证客户端现统一处理登录 action 和 logout_url：仅接受当前协议、同 hostname、预期 self-service 路径且无 URL 用户信息/fragment 的地址；把允许的端口差异归一到当前 origin，保留 flow/token 查询参数。凭证和 Cookie 始终通过当前 Edge 提交，保留 Kratos CSRF 校验。跨主机、跨协议和错误路径仍拒绝。未更改账号、密码、Kratos 配置或 Workflow 入口。

## 验证

- 8项认证单元测试通过并通过 TypeScript 检查：普通登录、5173端口别名登录、登出端口别名、跨主机/协议/错误路径拒绝；断言非法地址不会触发凭证 POST。
- 前端依赖边界检查通过，修改文件 Biome 与 diff 空白检查通过。
- 生产镜像构建完成并部署到5178/Edge18080。使用1556824234@qq.com，在无 Cookie 的真实 Chrome 中完成登录、业务身份查询、退出、刷新确认401、再次登录；两次凭证 POST 均留在18080，页面脚本错误0。全程未拦截 API、未预置登录 Cookie，见 [验收结果](browser-acceptance.json)。
