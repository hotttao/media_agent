# 素材列表与多语言

- Material 列表顶部以展开的复选表单选择零到多个已注册的图片 Material Asset Kind；候选查询固定为 `resource_type=MATERIAL`、`media_type=IMAGE`（枚举值 1/1）。零项等于全部类型，点击“全部类型”清空选择。多项编码为逗号分隔的 `asset_kind` 查询值，由服务端按集合过滤；空结果仍保留筛选入口。
- Product 列表按每页 20 条分页，显示总数、当前页和前后翻页；名称搜索由服务端执行且重置页码。
- 独立站点首次访问默认简体中文，可切换英文；选择保存到 `tos-language`。插件跟随宿主语言事件。Shell、认证入口、Material 与 Product 列表、创建和编辑表单提供中英文文案；其余旧详情页逐步清理硬编码。
- 不改变资源共享边界、对象存储 URL 或后端权限判定。
- 旧系统 `movement_materials` 是独立表，需作为 `asset_kind=movement` 的 Material 迁入；动作文本进入 `content`，参考视频进入 Storage 的 `object_path`，服装范围及通用性进入稳定 metadata 字段。旧姿势 ID 保存在迁移 metadata，兼容关系 API/表落地后再转为正式关联。
