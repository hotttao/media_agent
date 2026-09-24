# Library Object 16 MiB 上传上限

- [x] 在 Library Object Upload workflow 与 CRUD spec 中将单对象上限固化为 16 MiB。
- [x] 同步 UI 的 413 错误提示契约。
- [x] 调整上传 workflow 的限制并保留超限测试。
- [x] 将 Hertz multipart 请求体上限提高到 18 MiB，避免在业务校验前拒绝合法的 16 MiB 文件。
- [x] 调整 `media_ai` 迁移工具的源图片读取限制。
- [x] 通过幂等重跑迁入旧系统 9 MiB Virtual IP 全身图并校验四类图片字段。

验收：8 个 Material、1 个 Virtual IP、99 个 Product 全部迁移；源库 110 个非空图片引用均在目标对象存储中有对应对象。
