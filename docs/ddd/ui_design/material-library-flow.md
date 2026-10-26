# 企业素材库编辑流程

## 目标

Materials App 为 Organization 成员提供 Material、Virtual IP、Product 的可视化创建、查看和编辑入口。业务表只保存 `object_path`；用户不直接编辑对象路径或 Provider URL，而是选择本地图片，经 Library Object Upload 得到路径后再提交业务表单。

## 状态所有权

| 状态 | 所有者 | 生命周期 |
|:---|:---|:---|
| Organization、Scope | Shell Context / 当前表单 | Organization 由宿主注入；Scope 在创建时选择，编辑时只读 |
| 已保存的 Object Path 与 Persona/Product 字段 | `media_materials` | 服务端事实；React Query 负责失效与刷新 |
| 本地文件、预览 URL、上传进度和错误 | 图片上传字段 | 仅当前页面内存；离开页面释放本地预览 URL |
| Resolved URL | Storage Provider / `LibraryImage` | 最多五分钟；只用于显示，不写入表单或浏览器持久化 |
| Idempotency Key | 单次文件选择 | 选择文件时生成；同一文件的显式重试复用，更换文件时重建 |

## 用户旅程

1. 用户先确认 Scope，再选择图片；没有 Organization 或 Scope 时上传入口禁用并解释原因。
2. UI 立即显示本地预览并上传到 `POST /api/v1/library-objects:upload`。
3. 上传成功后字段进入 `uploaded`，只在内存中保存返回的 Object Path；业务表单仍未提交。
4. 用户填写其余字段并提交 Material、Virtual IP 或 Product CRUD；服务端在一个聚合事务内保存 Object Path。
5. 业务提交失败时已上传对象保留，用户可以修正表单后重试；UI 不擅自删除对象。
6. 替换图片会产生新 Object Path；移除只清除当前表单引用，旧对象由 Retention/GC 回收。

### 图片用途

| 页面字段 | Upload purpose | 数量/主图规则 |
|:---|:---|:---|
| Material 媒体 | `material` | 0 或 1；Material 必须在媒体与文本内容之间至少有一个 |
| Virtual IP 头像/全身/三视图/九视图 | `virtual_ip_avatar` / `virtual_ip_full_body` / `virtual_ip_three_view` / `virtual_ip_nine_view` | 每类 0 或 1，分别预览和替换 |
| Product Images | `product_image` | 多张有序；最多一张主图，首张默认主图，可设置主图和删除 |

## 页面要求

- Material 列表顶部提供常驻展开的 Asset Kind 多选表单，不使用下拉菜单；候选项来自 Asset Kind 注册表且固定查询 `resource_type=MATERIAL`、`media_type=IMAGE`（对应枚举值 1/1），“全部类型”清空已选项。选中的多个类型以同一次服务端查询提交，切换类型后保留筛选控件并显示空结果。
- Product 列表使用服务端总数与页码分页；搜索名称时回到第一页，上一页、下一页及当前页状态与服务端结果一致。
- Virtual IP 列表使用服务端名称查询与分页，预览依次使用头像、全身照、三视图和九宫格，不因缺头像而隐藏已有图片。
- Generated Asset 列表使用真实分页；媒体优先解析 object_path，只有旧记录才使用 url。纯文本产物直接显示正文，不产生虚假的媒体加载失败。没有筛选选项的页面不保留空侧栏。
- 旧动作素材显示为 Material 的“动作素材”类型；文字动作显示 `content`，参考视频使用 Storage Object Path 播放，服装适用范围和通用性使用稳定 metadata 字段；未落地的姿势兼容关系只提示追溯状态，不提供虚假的关联编辑入口。

- 创建和编辑页使用分组表单，不显示 URL 或 Object Path 文本输入。
- 详情页必须以图片区展示图片，并展示稳定业务字段；Object Path 只可在诊断信息中折叠展示，本期默认不展示。
- 详情页按工具栏、主视觉与摘要、业务信息卡片、审计或权限、危险操作分层；多图使用主图与缩略图切换，不把全部大图纵向堆叠。
- Metadata、External Commerce 等扩展对象使用可读键值、标签和嵌套卡片展示，不向业务用户直接输出 JSON 文本。
- 扩展对象编辑保留嵌套对象、数组、布尔和数字类型；普通文本字段修改不重写整个对象。创建/编辑草稿归属 App instance，查询刷新不覆盖尚未保存的修改。
- 素材类别注册管理是管理员入口，不混进普通素材主导航；系统类别禁删，类别编辑只修改说明和状态，不能展示会被 API 忽略的名称/资源类型/媒体类型编辑。
- Virtual IP 表单覆盖 Persona Profile 的基本属性、身体数据、教育职业、人物设定、表达兴趣全部稳定字段。
- Product 表单覆盖 target audience、product details、display actions、tags、external commerce、metadata 和有序图片。
- 桌面宽屏允许表单双栏；窄屏自动单栏。上传中、上传失败、空图片和只读状态都有明确视觉反馈。

## 失败与权限

- 401 复用全局登录处理；403/404 显示 Organization 或对象不可用；409 提示重新选择文件；413 明确最大 16 MiB；其他错误允许原文件重试。
- 上传中禁止提交，避免保存尚未取得 Object Path 的表单。
- 图片 accept 限制为 JPEG、PNG、WebP、GIF；前端提示不替代服务端校验。

## 产品详情的底部分页

产品主图、名称、摘要和工具栏保持在详情顶部；其下使用“产品信息 / 相关图片”两个子 Tab，默认产品信息。产品信息容纳目标人群、详情、展示动作、商城扩展、审计与管理。相关图片按当前企业、Product ID 与 Scope 服务端分页查询 Generated Asset，不按名称猜测关联，不把 Product Image 当生成结果；已有图片、视频和文本结果按媒体形态呈现，并提供只读大图/视频预览。只有切到该 Tab 才发起查询。

切换产品重置 Tab、分页和预览；切回产品信息不影响产品数据。Tab 与分页仅属当前详情视图内存，生成素材关联和权限仍以 Materials 为准。加载显示骨架，空结果明确提示没有关联生成素材，失败显示错误与重试，401 复用登录处理。窄屏 Tab 适应容器宽度，素材网格降为单列，预览弹窗可滚动并支持键盘关闭；媒体短期 URL 不持久化。
