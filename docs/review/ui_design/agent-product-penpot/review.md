# Agent 产品 UI 设计阶段审查

日期：2026-10-15。状态：**Penpot 新项目设计已交付；不代表产品功能已完成。**

## 项目与访问

- 项目：Agent 产品 UI · v1.0；ID `e618892f-b98f-815a-8008-caa931107e70`。
- 文件：Agent 产品 UI · v1.0；ID `e618892f-b98f-815a-8008-caa95765b9e9`。
- 页面：Agent · 企业资源与完整使用旅程。
- [免登录只读原型](http://192.168.2.41:9001/#/view?file-id=e618892f-b98f-815a-8008-caa95765b9e9&page-id=e618892f-b98f-815a-8008-caa95765c5af&section=interactions&share-id=fae893f5-64ff-4116-ad88-7fd881ebb994&index=16)。
- [编辑入口](http://192.168.2.41:9001/#/workspace?team-id=e618892f-b98f-815a-8008-caa88b1bb5a6&file-id=e618892f-b98f-815a-8008-caa95765b9e9&page-id=e618892f-b98f-815a-8008-caa95765c5af)；需要专用设计账号权限。

使用 Penpot 正规注册功能创建专用设计账号，创建独立项目和文件，没有重置管理员或修改原 Workflow 项目。账号凭据仅保存在部署设计目录的0600隐藏文件，不进入本仓库、日志和原型。原型只读分享不发送邮件或邀请。

## 画板与旅程

共17张1440×900画板，109个导航/旅程热点：

1. 企业 Prompt 资源库。
2. Prompt 编辑。
3. Prompt 历史与回滚。
4. 资源共享、部门 viewer/editor 与双重授权。
5. 企业 Skill 资源库。
6. Git 仓库、分支、子目录及凭据导入。
7. Skill 文件树、在线编辑和草稿。
8. Skill 发布、历史与下载。
9. MCP 服务目录。
10. 企业 MCP 登记与连接。
11. Agent 配置及固定资源选择。
12. 用户显式更新确认。
13. Session 对话与工具权限确认。
14. 空态、失败、权限不足、冲突与窄屏行为。
15. Skill 压缩包上传。
16. Prompt 直接填写模式。
17. Prompt 不设置模式。

设计依据已确认的企业资源库和 Agent 配置契约。企业资源查询取企业上下文，配置写入额外要求 Workspace manage；不做扩缩容、隔离或 SRE 页面。沿用 Workflow v1.1/v1.2 及 `ui_workflow/src/index.css` 的暖浅背景、白面板、炭黑主按钮、黄色强调和字体体系。

## 文件与验证

部署源目录为 `/home/tao/deploy/penpot/agent-product-ui/`，包含17张可编辑 SVG、`index.html` 总览、`design.py`、`manifest.json`、原生导入备份和 `penpot-links.json`。画板采用 Penpot 正规 SVG 导入，并添加原生交互热点；没有把全部 SVG 文字宣称为 Penpot 原生文本。

已通过插件检查实际文件、17张画板尺寸和109个交互目标；通过独立匿名浏览器确认只读链接可打开，实际渲染检查企业 Prompt 列表及不设置 Prompt 画面。截图为部署目录的 `penpot-first-screen.png` 和 `penpot-readonly.png`。原型样例数据和按钮跳转不代表真实 API 已完成。

## 实现时的约束

管理界面接入现有 Shell/Slot，不另建宿主导航系统；使用后端真实权限和版本，秘密不回显或持久化到浏览器。后续实现将面向用户的说明统一为“系统默认指引”等产品用语，避免在操作流程中暴露内部模板或服务实现术语。完整产品的功能验收继续由 F1–F4 记录。
