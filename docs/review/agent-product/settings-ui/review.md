# Settings UI 统一

## 问题与参考

对照本机 agent-platform 的 `web/src/components/workspace/WorkspaceSettingsPanel.tsx`：Settings 使用固定左侧导航与右侧独立滚动，General/Model/Prompt/MCP/Skills 分区展示。原实现把 `overflow-y-auto` 与居中的 `max-w-xl` 放在同一容器，导致滚动条随内容宽度停在Slot中央；资源库、模型连接又各自建立顶层管理页，导航边界不一致。

## 完成内容

Settings 左侧统一组织工作空间概览、Agent配置、企业Prompt/Skill/MCP目录、个人模型连接。顶层只保留统一Settings入口；配置中的“打开资源库”在同一Settings切换。企业和个人分组明确显示范围，不改变原ACL或绑定模型连接的显式选择规则。

滚动视口铺满右侧剩余宽高，仅内部阅读区域设置max-width。资源列表/详情按容器宽度切换；窄Slot使用紧凑分区选择器。修复首次Slot初始化覆盖URL打开Settings焦点的问题，切换分区不重新加载Chat；URL保留当前Session，刷新恢复分区。手机顶部缩短Workspace名称并收起Settings文字，保留可访问的按钮名称和登出操作。

旧 `/library/:organizationId?` 和 `/model-connections` 进入同一Settings结构，无Workspace时提供同样的企业/个人管理视图。工作空间编辑表单中的模型连接管理入口也指向Settings。管理组件下移到各自L2领域，页面在L3组合，未引入跨领域依赖或新框架。

## 验证

- 全部14文件41项前端测试通过，TypeScript通过。
- 完整10项浏览器场景通过（23.2秒）：Prompt创建/编辑/历史/授权与窄屏、Skill编辑发布下载、MCP写入式凭据与固定配置、首启、防重复创建、权限交互、暂停/旧配置恢复、个人模型连接管理及统一Settings布局。
- 布局回归检查滚动容器右边界与Slot一致、导航不随正文滚动、分区切换/刷新保留Session、窄屏导航和水平溢出。
- 依赖分层、翻译键和修改文件格式检查通过；生产构建并部署Docker Agent UI（8c0cef0f78e1）。最后的手机顶栏和文件选择器样式调整后，Settings/个人模型连接两项关键浏览器场景复测通过（12.2秒）。
- 真实账号、真实Auth/Control/Gateway服务验证桌面三栏、填满Slot、资源库和正式“本机Codex”连接、手机宽度与旧模型链接，脚本错误0。仅进行界面只读验证，不修改用户连接和资源，不提交模型对话。

截图：[桌面Settings](desktop.png)、[填满Slot的模型连接](models-filled.png)、[企业资源](resources-filled.png)、[手机宽度](narrow.png)。机器验收结果见 [live-acceptance.json](live-acceptance.json)。

本次只对齐参考产品的Settings交互结构；保留本项目已确认的企业资源ACL、个人模型连接和固定Session配置语义。SRE及扩缩容不在本次范围。
