# authorization_projection 变更记录

## 2026-09-20：增加受控的在线授权判定

- 新增内部 Team 授权入口，permission 使用固定闭集合，不开放任意 Keto namespace/relation 代理。
- 在线判定同时计算事实源与 Keto 决定；shadow 记录差异，enforce 要求二者同时允许。
- 内部 Team 查询与判定统一要求 `X-Media-Auth-Internal-Token`，配置缺失时 fail closed。
- Keto read/write 端口仅绑定宿主机 loopback，禁止经局域网统一网关暴露。
