# Material Library Editor Spec

## 范围

完善 `ui_workflow` 中 Material、Virtual IP、Product 的 create/edit/show。复用 `Library Object Upload` 和 `Library Object Resolution`，不增加前端直连 S3 上传，不恢复旧 URL 字段。

## 组件契约

- `uploadLibraryObject({organizationId, scope, purpose, file, idempotencyKey})` 使用 `FormData`，不得设置 JSON Content-Type。
- `LibraryObjectField` 管理单图的 local-preview/uploading/uploaded/error 状态，输出 `object_path`。
- `ProductImagesField` 复用 Library Object Upload，输出有序 `{object_path,is_main,sort_order}`，删除后重排，主图删除后第一张自动成为主图。
- `LibraryImage` 继续通过 resolve 获取短期 URL；表单已选本地文件优先显示本地预览。
- `LibraryGallery` 负责详情页主图与缩略图切换；无图片时显示一致的空态。
- `StructuredDetails` 将扩展对象递归展示为字段、布尔状态和标签集合，详情页禁止使用 JSON `<pre>`。

## 数据映射

- Material：`description`、`prompt`、`object_path`、`content`、`metadata`、`visibility`、`team_id`。
- Virtual IP：四个 `*_object_path` 与 `persona_profile`。Persona Profile 稳定字段为 age、gender、height、weight、bust、waist、hip、education、major、city、occupation、income_level、basic_setting、family_background、personality、catchphrase、small_habit、hobbies。
- Product：target_audience、product_details、display_actions、tags、external_commerce、metadata、images。
- Protobuf 中的 JSON string 字段由 domain API adapter 在请求前 `JSON.stringify`，响应在进入表单前安全解析；组件不处理协议字符串。

## 验证

- 单元测试覆盖 multipart 请求、purpose/idempotency header、Product 主图与排序纯规则。
- 组件/页面测试至少覆盖上传成功、失败重试、删除引用和详情图片渲染。
- `yarn lint`、`yarn test`、`yarn typecheck`、standalone/plugin build 必须通过。
