---
name: platform_token
description: 代表用户执行长期离线委托的加密托管凭证
dependencies:
    - auth
related_workflow:
    - agent_schedule_execution
    - platform_token_migration
    - platform_audit_delivery
---

## 一句话职责

Platform Token 负责少数平台组件代表用户执行离线任务或调用旧式服务时的长期凭据托管。

## 边界

### 做什么
- 保证每个用户至多一个 active Platform Token。
- 加密保存可恢复明文并支持密钥轮换。
- 按 mTLS CallerService 和固定 purpose 发放、内省、轮换与撤销。

### 不做什么
- 不向 Agent、Plugin、自定义 MCP、公共 Edge 或普通业务 Worker 发放。
- 不把Secret写入 River、Outbox、日志、Trace 或业务任务表。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 获取令牌 | 为允许组件按 purpose 解密返回 | `AcquirePlatformToken` |
| 内省令牌 | 验证长期委托并返回 Principal | `IntrospectPlatformToken` |
| 轮换令牌 | 原子替换当前 active Token | `RotatePlatformToken` |
| 撤销令牌 | 使用户长期委托失效 | `RevokePlatformToken` |
| 登录确保 | 内部确保当前密文存在，不返回Secret | `EnsurePlatformToken` |

## 聚合与不变量

Platform Token 是独立聚合，不复用 API Token/Talos 或旧 Service Token 仓储。密文使用 AES-256-GCM，每次随机12字节nonce；AAD绑定本表用途、不可变Token ID、User ID及密钥版本，解密后常量时间核对SHA-256。调换归属、密文、nonce、Hash或版本必须失败，错误不包含Token或底层加密细节。密钥仅由Secret/KMS提供，数据库不保存密钥，进程不能自动生成替代密钥来绕过解密失败。

同一User至多一条未撤销记录；过期记录不能验证，但需先撤销再创建新当前记录。撤销不删除密文和历史，重复撤销幂等。用户/内部调用者授权与purpose校验由应用用例负责，仓储不将解密成功当作授权。

写用例与校验均锁定当前User行，序列化同用户确保/轮换/撤销与账号状态更新。不存在或非active User不得确保、获取、轮换或验证；底层查询故障不能当作无Token创建。现有密文不可解密时禁止自动替换；只能显式轮换。取消或审计写入失败导致整笔事务回滚，不返回尚未提交的Secret。

## 数据模型

### `platform_tokens`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | Platform Token ID |
| user_id | text | 被代表用户 ID |
| token_hash | bytea | Token 的 SHA-256 Hash |
| ciphertext | bytea | AEAD 加密后的 Token 密文 |
| nonce | bytea | 本次加密唯一 nonce |
| key_version | text | KMS/Secret 密钥版本 |
| created_at | timestamptz | 创建时间 |
| expires_at | timestamptz nullable | 可选过期时间 |
| revoked_at | timestamptz nullable | 撤销时间；每个用户至多一个未撤销记录 |

### UUID 生成规则

新ID使用仓库统一的32位随机十六进制字符串，迁移保留既有ID；Token使用 `tos_` + 32字节随机数的hex表示；每次加密使用唯一nonce。

#### 约束与索引

- ID主键、User同库逻辑引用；token_hash为32字节且唯一，ciphertext至少带GCM认证标签，nonce为12字节，key_version非空有界。
- `user_id WHERE revoked_at IS NULL` 部分唯一索引维护当前凭据不变量；过期判定不写入部分索引。
- 不含明文列；有记录时down migration拒绝删表，不把密文回写裸明文。

### `platform_token_audit_outbox`

这是同事务的审计Outbox，不保存待交付Secret，也不是API Token/Talos事件。

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| id | text PK | 审计ID |
| user_id | text | 被代表User |
| credential_id | text | 本次当前或被撤销凭据ID |
| operation | text | issued/acquired/introspected/rotated/revoked |
| caller_service | text | 可信接入层解析的服务身份或本地login/user-revocation用例 |
| purpose | text | 固定offline-task/login-bootstrap/user-revocation |
| occurred_at | timestamptz | 事实时间 |
| delivered_at | timestamptz nullable | 后台确认投递时间，未投递为NULL |
| lease_id | text nullable | 当前领取尝试ID |
| lease_until | timestamptz nullable | 当前领取过期时间 |
| attempts | integer | 已领取次数 |
| next_attempt_at | timestamptz | 下次可领取时间 |

约束：只允许固定操作/caller/purpose，无自由JSON载荷；无Secret/Hash/密文列。有审计历史时down拒绝删除。投递协调需要Workflow，不能在本用例事务内调用外部系统；本地Outbox写入不代表投递完成。消费者按event ID去重，至少一次投递保留重试/恢复门禁。

领取只选pending且到期事件，`FOR UPDATE SKIP LOCKED`后同事务写新lease并递增attempts（饱和而不溢出）。确认/退避仅匹配未投递且未过期的当前event/lease，失败CAS不覆盖其他投递器；delivered终态不重置。仓储不自行开事务，所有claim/确认/失败安排都必须传上层事务context。

过期检查至少使用数据库实际时钟，不能只用等待数据库前截取的进程时间，让已经过期的旧lease再次确认。

## 能力详细说明

### AcquirePlatformToken

1. 从 mTLS 证书取得 CallerService，校验 `(caller, purpose)` allowlist。
2. 加载用户 active Token；不存在时按策略创建。
3. 使用 key_version 对应密钥解密，只在响应内存返回并记录不含 Token 的审计事件。

调用者只从注入的可信CallerVerifier取得，命令不接收caller字段；固定`offline-task`只允许`media_agent_gateway`或`media_control_schedule_worker`。CallerVerifier缺失/故障、未授权caller或purpose一律拒绝；不能使用普通Internal Token、请求Header或JWT claim自行声称服务身份。mTLS接入必须实现该Verifier，未接入前不能开放端点。

内部监听器使用独立Client CA、TLS 1.3及强制客户端证书验证；叶证书仅有一个URI SAN，按精确URI `spiffe://media-agent/services/media_agent_gateway` 或 `spiffe://media-agent/services/media_control_schedule_worker` 映射服务身份。不使用CN、DNS SAN、转发Header或公共Session推导CallerService；此URI是本地证书命名约定，不表示已实现SPIFFE工作负载证明/联邦。证书必须具有ClientAuth用途，当前时间链验证成功；请求进入及用例读取身份时重新验证，keep-alive或后台延迟不能延长过期证书的权限。

可信接入层只把服务器连接的TLS状态绑定至context；凭据证据是不可变DER快照，绑定特定Verifier/CA实例，不能在另一信任配置复用。普通HTTP连接、未完成握手、未知服务、多个URI身份及校验故障一律拒绝。实例配置启动固化，换证/CA更新须按服务重启流程，不把用户Token或普通Internal Token作为降级认证。

### EnsurePlatformToken / IntrospectPlatformToken / RevokePlatformToken

登录内部确保只返回error，不交付Secret；当前过期时先撤销再创建，当前有效时须验证密文完整性。内省先校验caller/purpose，再按Hash找记录、锁User、重读凭据检查当前事实与密文完整性，只返回User/credential元数据；未知/撤销/过期/停用返回inactive，查询或加密故障返回固定错误。用户撤销内部命令允许停用/已删除User，幂等撤销当前凭据，不创造替代凭据。

### RotatePlatformToken

事务内撤销旧 Token、创建新 Token 与审计Outbox；调用方不得假设旧 Token 在已开始请求中立即中断。显式轮换可以恢复不可解密当前凭据，不尝试输出其Secret。轮换响应丢失不得自动重试轮换，应先内省/查询当前事实。

## 关联关系

- 依赖 auth 的当前User生命周期；不依赖 service_token。
- 旧表到加密存储的迁移须先逐条验证ID/owner/Hash与密文可恢复，再原子转移事实；迁移编排和调用方切换是删除旧认证的前置门禁，不能以新表存在视为迁移完成。
- [Platform Token Migration](../../workflow/0013-platform-token-migration/platform_token_migration.md)协调旧凭据到密文事实的同库原子转移，包含撤销历史，保留ID与时间，默认演练回滚。
- [Platform Audit Delivery](../../workflow/0014-platform-audit-delivery/platform_audit_delivery.md)协调事务外审计投递、lease恢复及event ID幂等，不拥有或解密凭据。
