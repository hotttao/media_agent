# auth domain 对比 control-plane 差异清单

> 对比源：`agent-platform/control-plane/src/routes/auth.ts` + `services/auth.ts` + `services/db/users.ts`
> media_auth 实现：`media_auth/biz/{domain,policy,service,handler,middleware,shared}/auth*`
> 日期：2026-06-30

## 1. 路由接口对齐（method/path/请求/响应）

| 接口 | control-plane | media_auth | 一致性 |
|---|---|---|---|
| 登录 | `POST /api/auth/login` {username,password} → ApiUser + set-cookie | 同 | ✅ |
| 登出 | `POST /api/auth/logout` → {success} + delete-cookie | 同 | ✅ |
| 当前用户 | `GET /api/auth/me` → ApiUser | 同 | ✅（字段差异见 §2） |
| 更新设置 | `PATCH /api/auth/me` {auto_evolution?} → {success} | 同 | ⚠️（tri-state 丢失，见 §3） |
| 改密 | `PUT /api/auth/me/password` {current_password,new_password} → {success} | 同 | ✅ |
| 默认 prompt | `PUT /api/auth/me/default-prompt` {prompt_id:string\|null} → {success} | 同 | ⚠️（tri-state 丢失，见 §3） |

## 2. ApiUser 响应字段差异

| 字段 | control-plane | media_auth | 原因 |
|---|---|---|---|
| id | ✅ | ✅ | — |
| username | ✅ | ✅ | — |
| role | ✅（dbUser.role） | ✅ | — |
| auth_source | `password_hash ? 'password' : 'ldap'`（企微账号误报 ldap） | password_hash 非空→password；否则查 wecom identity→wecom，无→ldap | ✅ 修正（spec §5.1），见 §4 |
| default_prompt_id | ✅ | ✅ | — |
| **default_prompt_name** | ✅（查 prompts 表补 name） | ❌ 不返回 | **故意**：prompts 表留 control_plane，media_auth 只返 opaque id，name 由网关层补（spec/ADR-0001）。破坏性，P2 网关适配 |
| auto_evolution | ✅ | ✅ | — |

## 3. tri-state 丢失（去 proto3 optional 导致）

protoc-gen-hertz 不支持 proto3 optional，IDL 去掉了 `optional` 关键字，导致两类 tri-state 语义丢失：

| 接口 | 字段 | 原语义 | 现行为 | 影响 |
|---|---|---|---|---|
| `PATCH /me` | auto_evolution | optional 区分「未传」与「显式 false」 | 总是设置（bool 无法区分未传） | 客户端必须显式传 bool；未传时会被设为 false。可接受：PATCH /me 当前仅此字段，前端总是显式传 |
| `PUT /me/default-prompt` | prompt_id | optional string\|null（null=清除） | 空串=清除，非空=设置 | 无法区分「未传 prompt_id」与「显式传空串清除」。可接受：PUT 语义本就要求传值 |

**补救**：若后续需要严格 tri-state，方案：handler 层用 `*bool`/`*string` wrapper + 请求体 raw presence 判断（绕过 DTO 的 bool 零值），或升级 protoc-gen-hertz 支持 optional。

## 4. auth_source wecom 修正

control-plane `GET /me` 与 login 的 auth_source：`password_hash ? 'password' : 'ldap'`——企微派生账号（password_hash 空）误报 ldap。

media_auth 修正（spec §5.1）：无密码账号查 user_identities，有 wecom 绑定→`wecom`，否则→`ldap`。
- 实现：`User.AuthSource(resolver)`，resolver 由 identity domain 实现（`AuthSourceResolver` 接口）。
- **当前状态**：identity domain 未实现，resolver=nil，wecom 分支回退 ldap（与 control-plane 行为一致）。identity domain 完成后注入 resolver 即修正。

## 5. 行为差异

| 行为 | control-plane | media_auth | 说明 |
|---|---|---|---|
| login ensurePlatformToken | fire-and-forget 创建 platform service token | 日志占位（service_token domain 未实现） | TODO，service_token domain 完成后接入 |
| login last_login_at | upsertUser 内更新 | 本地登录成功后单独 UpdateLastLogin | 等价；LDAP 走 Upsert 内更新 |
| argon2 参数 | @node-rs/argon2 默认 | argon2id m=64MB t=3 p=2（OWASP） | hash 格式不同但都标准 argon2id；**注意**：control-plane 存量 password_hash 用 node 默认参数，media_auth Verify 需解析哈希串内的 m/t/p 参数（已实现：从 `$argon2id$v=19$m=..,t=..,p=..$` 解析），兼容存量 |
| session cookie | httpOnly, secure=false, sameSite=Lax, maxAge=24h, path=/ | 同 | ✅ |
| session 滑动续期 | exp 剩余 < 12h 重签 | 剩余 < 半周期（12h）重签 | ✅ 等价 |
| 鉴权失败 | API 401 / 页面 302 → /login | 统一 401 | media_auth 仅 API，无页面；P2 网关处理页面 |
| JWT 算法 | HS256 | HS256 | ✅ |
| JWT_SECRET | env JWT_SECRET | config.AuthConfig.jwt_secret | ✅ 同源（共享 env） |

## 6. 破坏性变更清单（P2 前端/网关适配）

1. **`default_prompt_name` 移除**：ApiUser 不再返回该字段，由 control_plane 网关层查 prompts 表补。前端若依赖需改。
2. **auth_source wecom 修正**：企微账号 auth_source 从 `ldap` 变 `wecom`。前端若据 auth_source 判断改密入口，行为更准确（wecom 账号本就不可改密），但值变化需前端兼容。
3. **tri-state**（§3）：PATCH /me 与 PUT /me/default-prompt 的未传语义弱化，前端需显式传值。

## 7. 未对齐项（待后续 domain 补齐，非 auth domain 缺陷）

- `ensurePlatformToken`：依赖 service_token domain（后续）。
- `auth_source` 的 wecom 精确判断：依赖 identity domain（后续）。
- Bearer token 鉴权（OAuth access token）：依赖 oauth domain（后续）；当前 session middleware 只验 cookie JWT。
