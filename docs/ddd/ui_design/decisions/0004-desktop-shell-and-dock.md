# ADR-0004：以 Menubar、Layout 与 Segmented Dock 替代固定双栏

## 状态

Accepted，取代 ADR-0003 中“桌面固定双栏”和固定移动端 Sessions/Chat 导航部分。ADR-0003 关于“窄屏一次只显示一个 Slot、隐藏 Slot 状态保留”继续有效。

## 背景

最初实现把 Workspace 列表常驻左侧，并把 App Instance 标签放入 AppWindow header。对照 agent-platform 的真实交互后确认，这会混淆 Workspace、Layout、Slot、AppWindow 和实例导航职责，阻碍 App header portal、三栏工作区和统一移动端导航。

## 决策

采用三层 Shell：顶部 Menubar 管 Workspace 与用户入口，中央 Layout Canvas 承载 Slot，底部 Segmented Dock 管 App Instance。AppWindow header 左侧由 App portal 注入业务动作，右侧由 Shell 提供 fill/close。

桌面提供 1 栏和 3 栏，默认 3 栏；移动端强制单栏但复用 Dock。P0 使用 localStorage Profile Backend，后续可替换服务端 backend。

## 结果

Workspace 不再占用 Layout 列；App 实例导航与业务工具栏分离；内置 App 和未来 plugin 可复用相同宿主交互。代价是需要路由迁移、Dock、Layout 和 Profile schema 升级。
