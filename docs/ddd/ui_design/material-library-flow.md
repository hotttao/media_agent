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

- 创建和编辑页使用分组表单，不显示 URL 或 Object Path 文本输入。
- 详情页必须以图片区展示图片，并展示稳定业务字段；Object Path 只可在诊断信息中折叠展示，本期默认不展示。
- 详情页按工具栏、主视觉与摘要、业务信息卡片、审计或权限、危险操作分层；多图使用主图与缩略图切换，不把全部大图纵向堆叠。
- Metadata、External Commerce 等扩展对象使用可读键值、标签和嵌套卡片展示，不向业务用户直接输出 JSON 文本。
- Virtual IP 表单覆盖 Persona Profile 的基本属性、身体数据、教育职业、人物设定、表达兴趣全部稳定字段。
- Product 表单覆盖 target audience、product details、display actions、tags、external commerce、metadata 和有序图片。
- 桌面宽屏允许表单双栏；窄屏自动单栏。上传中、上传失败、空图片和只读状态都有明确视觉反馈。

## 失败与权限

- 401 复用全局登录处理；403/404 显示 Organization 或对象不可用；409 提示重新选择文件；413 明确最大 4 MiB；其他错误允许原文件重试。
- 上传中禁止提交，避免保存尚未取得 Object Path 的表单。
- 图片 accept 限制为 JPEG、PNG、WebP、GIF；前端提示不替代服务端校验。
