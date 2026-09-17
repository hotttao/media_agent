# App Shell

Shell 由顶部 Menubar、中央 Canvas 和底部 Dock 构成。Menubar 负责品牌入口、当前 Organization/Team/Workspace、层级切换/创建入口和用户操作；管理员的企业、部门和 User Management 入口也归 Menubar。Dock 负责 Layout 与 App Instance 导航。层级切换器和管理页都不是 Workspace App，不占用 Slot。

Workspace 切换器使用 popover 展示 Organization/Team 下的 Workspace 搜索、选择与“新建 Workspace”入口。创建和更新使用 Dialog，不在 Canvas 中常驻表单。选择 Workspace 通过 Navigation Port 导航到 `/o/:organizationId/t/:teamId/w/:workspaceId`；根路由展示层级选择空态。

切换 Workspace 时：

1. 清除当前消息的瞬态显示；
2. 加载目标 Workspace 的 Session；
3. 恢复目标 Workspace 的 UI Profile；
4. URL 指定有效 `?session=` 时优先选择，否则选择第一个 Session；
5. Workspace 不存在 Session 时展示 Sessions/Chat 空态。

Shell 不保存 API Key，不负责发起 Chat turn，也不直接修改 Slot 内业务状态。移动端仍使用同一 Dock 导航，不能维护一套写死 Sessions/Chat 的平行导航。

Shell Context 是 App 与宿主的唯一导航边界。App 可以读取当前层级 ID、请求 route intent、向 App Header Portal 注入局部动作；App 不得渲染或直接控制全局 Menubar。插件模式下全局导航由 Agent Platform Host 渲染，standalone 模式由本仓库 Shell 渲染。
