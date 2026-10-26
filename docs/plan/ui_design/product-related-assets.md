# 产品详情关联生成素材落地计划

对应 [spec](../../spec/ui_design/product-related-assets.md)。

- [x] 底部信息迁入默认产品信息 Tab，相关图片 Tab 懒加载关联素材。
- [x] 实现服务端来源过滤与分页、只读媒体预览、加载/空/错误及中英文。
- [x] 组件行为测试、类型/分层检查与 standalone/plugin 构建。
- [x] 更新当前部署并验证可访问，记录实际验证范围。

验证：新增 3 个产品详情行为测试通过，连同邻近产品/媒体测试共 6 项通过；TypeScript、修改文件 lint、dependency-cruiser、standalone/plugin 构建通过。隔离浏览器场景验证了惰性来源查询、预览/ESC、桌面和 390px 窄屏；无页面异常。真实登录场景在登录后的 Shell 加载处超时，未把它记为真实账号验收通过。

部署：运行镜像 `media-workflow-ui:product-related`，现有入口保持 `http://mini-001.local:5173/`（网关 18080 同样代理此页面），返回 HTTP 200。旧容器保留为 `media-workflow-ui-before-related-1793022253`。测试数据只在临时浏览器网络拦截中存在，未写入后端；临时入口与测试服务已删除/停止。
