---
name: stored_object
description: 域名无关对象路径、对象元数据与物理回收
dependencies: []
related_workflow:
    - media_workflow_execution
    - object_retention_gc
---

## 一句话职责

Stored Object 负责按业务服务指定的 Object Path 保存、读取和物理清理二进制内容。

## 边界

### 做什么
- Put、Open、Stat、Delete 和 ResolveURL。
- 校验 mTLS CallerService 是否允许访问路径 Namespace。
- 通过 Driver 适配 LocalFile 与 AliyunOSS。

### 不做什么
- 不决定业务 owner、ACL、分享或何时删除。
- 不生成业务路径，不保存完整 URL，不管理派生资源。

## 能力清单

| 能力 | 说明 | 对外接口/函数 |
|:---|:---|:---|
| 写入对象 | 直接接收流并写入指定路径 | `StorageInternalService.PutObject` |
| 读取对象 | 返回对象流或元数据 | `OpenObject/StatObject` |
| 删除对象 | 进入回收期并最终物理清理 | `DeleteObject` |
| 解析地址 | 按路径返回临时访问地址 | `ResolveURL` |

## 数据模型

### `stored_objects`

| 字段 | 类型 | 含义 |
|:---|:---|:---|
| object_path | text PK | 业务服务生成的域名无关稳定路径 |
| namespace | text | 路径首段，用于调用服务访问控制 |
| driver | text | local_file/aliyun_oss |
| backend_key | text | Driver 内部物理对象键 |
| content_type | text | MIME 类型 |
| size_bytes | bigint | 对象字节数 |
| checksum | text | 内容校验值 |
| state | text | active/pending_delete/deleted |
| delete_after | timestamptz nullable | 回收期结束时间 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 最后更新时间 |

### UUID 生成规则

Storage 不生成业务 ID；`object_path` 由调用业务服务生成并作为稳定标识。

## 能力详细说明

### PutObject

1. 从 mTLS 获取 CallerService，校验 Namespace allowlist 和路径合法性。
2. 流式写 Driver 临时对象并计算 checksum，成功后原子完成 backend key。
3. upsert 元数据；同路径不同内容返回冲突。

### DeleteObject

1. 调用方已经完成业务删除决策，Storage 只验证服务和 Namespace。
2. 幂等标记 `pending_delete` 与 `delete_after`。
3. River 到期物理删除；失败重试，元数据保留审计状态。
