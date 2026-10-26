# 产品详情子 Tab 与关联生成素材

保持顶部产品主视觉与操作；底部采用 Radix Tabs，默认“产品信息”，另一个为“相关图片”。原底部信息和管理内容放入信息面板；关联面板懒挂载，使用 Generated Asset domain 中的组件查询 `/generated-assets?organization_id=&source_product_id=&scope=&page=&page_size=12`。依旧由服务端做资源授权，不新增后端关联或查询协议。

关联组件仅接收企业、Product ID 和 Scope 标量，不导入 Product domain。查询键包含企业和筛选值；页码由组件内存保存，产品/企业改变时通过 key 重建。生成结果保留服务端分页，不在浏览器过滤全局列表。使用现有 LibraryMedia 解析 Object Path、识别图片/视频，并通过只读 Dialog 查看媒体/文本、类型和生成时间；不新增删除或执行入口。中英文文案同步，提供加载、无数据、失败重试以及窄屏状态。验证使用组件测试覆盖惰性查询、来源参数、分页、预览及切换产品隔离，配合类型/依赖检查和生产构建。
