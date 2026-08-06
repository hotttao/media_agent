---
name: platform_token
description: 代表用户执行长期离线委托的加密托管凭证
dependencies:
    - auth
related_workflow:
    - agent_schedule_execution
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

ID 使用 ULID；Token 使用 `tos_` + 至少 256 位随机数；每次加密使用唯一 nonce。

## 能力详细说明

### AcquirePlatformToken

1. 从 mTLS 证书取得 CallerService，校验 `(caller, purpose)` allowlist。
2. 加载用户 active Token；不存在时按策略创建。
3. 使用 key_version 对应密钥解密，只在响应内存返回并记录不含 Token 的审计事件。

### RotatePlatformToken

事务内撤销旧 Token、创建新 Token 与审计 Outbox；调用方不得假设旧 Token 在已开始请求中立即中断。
