# 响应式布局

## 桌面

宽屏默认使用三栏：Sessions、工作 App、Chat。左右栏可拖动调整宽度；用户可切到一栏聚焦布局。Workspace 导航位于顶部 Menubar，App Instance 导航位于底部 Dock。

## 移动端

窄屏强制使用单栏 Canvas，同一时间只展示一个活动 App。用户通过同一个 Dock 激活 Sessions、Settings 或 Chat；选择 Session 后激活 Chat。Workspace 选择使用 Menubar popover，创建/更新使用 Dialog，不永久占据内容宽度。

响应式变化不得销毁 Profile 中另一个 Slot 的状态。键盘焦点必须跟随用户主动切换，不能因断点变化自动抢占输入焦点。

Settings 根据自身容器宽度切换左侧导航/紧凑选择器，不能仅按viewport判断。右侧滚动视口铺满剩余空间，表单max-width只作用于内部内容。

Chat 消息按 Slot 宽度收缩。用户正文、模型回复和工具结果中的长 URL、JSON 与连续字符串均可在气泡内换行；保留原始换行和完整文本，不能用截断或隐藏横向溢出代替可读展示。消息内容不能撑宽 Slot 或页面。
