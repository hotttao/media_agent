---
name: platform_token
description: 代表用户执行长期离线委托的加密托管凭证
dependencies:
    - auth
related_workflow:
    - agent_schedule_execution
    - platform_token_migration
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
- 不写入 River、Outbox、日志、Trace 或业务任务表。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 获取令牌 | 为允许组件按 purpose 解密返回 | `AcquirePlatformToken` |
| 内省令牌 | 验证长期委托并返回 Principal | `IntrospectPlatformToken` |
| 轮换令牌 | 原子替换当前 active Token | `RotatePlatformToken` |
| 撤销令牌 | 使用户长期委托失效 | `RevokePlatformToken` |

## 聚合与不变量

Platform Token 是独立聚合，不复用 API Token/Talos 或旧 Service Token 仓储。密文使用 AES-256-GCM，每次随机12字节nonce；AAD绑定本表用途、不可变Token ID、User ID及密钥版本，解密后常量时间核对SHA-256。调换归属、密文、nonce、Hash或版本必须失败，错误不包含Token或底层加密细节。密钥仅由Secret/KMS提供，数据库不保存密钥，进程不能自动生成替代密钥来绕过解密失败。

同一User至多一条未撤销记录；过期记录不能验证，但需先撤销再创建新当前记录。撤销不删除密文和历史，重复撤销幂等。用户/内部调用者授权与purpose校验由应用用例负责，仓储不将解密成功当作授权。

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

## 能力详细说明

### AcquirePlatformToken

1. 从 mTLS 证书取得 CallerService，校验 `(caller, purpose)` allowlist。
2. 加载用户 active Token；不存在时按策略创建。
3. 使用 key_version 对应密钥解密，只在响应内存返回并记录不含 Token 的审计事件。

### RotatePlatformToken

事务内撤销旧 Token、创建新 Token 与审计 Outbox；调用方不得假设旧 Token 在已开始请求中立即中断。

## 关联关系

- 依赖 auth 的当前User生命周期；不依赖 service_token。
- 旧表到加密存储的迁移须先逐条验证ID/owner/Hash与密文可恢复，再原子转移事实；迁移编排和调用方切换是删除旧认证的前置门禁，不能以新表存在视为迁移完成。
- [Platform Token Migration](../../workflow/0013-platform-token-migration/platform_token_migration.md)协调旧凭据到密文事实的同库原子转移，包含撤销历史，保留ID与时间，默认演练回滚。
