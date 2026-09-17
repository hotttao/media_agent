就像你之前所说的：ui_design 是前端平台架构，采用 Slot/App 模型和 L0–L3 分层

我现在需要先使用 media_matrials media_workflow。我希望你先实现这一部分，包括使用新的 ory 组件实现登陆认证和健全以及，组织和用户的管理。

但是需要你需要考虑 ui_design 设计，后面我会将这一部分页面作为 app 放到 slot 的插槽里。

我能想到的一个注意点是，对于顶部的导航栏，你需要使用类似 ui_design 里 context 节点注入的方式，这样，你在迁移的时候就可以直接使用 ui_design 里通过 context 注入的导航栏组件。这样结构上保持一致，可以直接迁移。