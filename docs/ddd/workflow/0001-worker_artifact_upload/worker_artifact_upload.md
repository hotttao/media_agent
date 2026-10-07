---
name: worker_artifact_upload
description: 将当前能力任务产物直传对象存储，再由原生执行完成审批和资源登记。
participants:
  - media_worker
  - media_workflow
  - media_materials
  - S3 Storage Provider
domains:
  - media_workflow/execution
  - media_materials/asset
---

## 触发与结果

Personal Worker为活动Dispatch的某个候选上传本地图片/视频；Managed内部运行时复用相同任务校验与存储端口，不借用User Token。成功只返回稳定Object Path，Generated Asset仍由Conductor调度的内部Finalize登记。既有受控存储URL mock保持兼容；不接入真实模型、不改UI、不扩展部门/角色ACL。

## 执行时间线

```mermaid
sequenceDiagram
    participant W as media_worker
    participant G as 统一网关
    participant F as media_workflow
    participant M as media_materials
    participant S as S3 Provider
    W->>G: Worker API Token + Dispatch/attempt/候选位置/文件元数据
    G->>F: 已验证Worker Internal JWT
    F->>F: 复核绑定身份、原生精确活动Task、冻结Capability媒体类型
    F->>M: 内部服务凭证 + 企业/Scope/不透明上传键
    M->>S: HEAD固定Object Path
    alt 对象已存在且元数据匹配
        M-->>W: 经F/G返回already_uploaded + Object Path
    else 对象不存在
        M->>M: 签发短期、条件写入及校验和绑定的PUT地址
        M-->>W: 经F/G返回URL、必要Headers、Object Path
        W->>S: 直接PUT文件，不转发Worker Token
    end
    W->>F: 经网关Complete(candidates[].object_path)
    F->>F: 复核对象企业/Scope、媒体类型和大小
    F->>F: 代理精确Conductor Task结果，不写结果Inbox
    Note over F,S: 审批/循环由Conductor；Finalize幂等登记所选Generated Asset
```

## 一致性与生命周期

上传键由服务端原Task ID、attempt与候选数组位置派生，客户端不能提供企业、Scope或任意对象路径。完整路径还包含类型/长度/SHA256/MD5清单摘要，同一任务位置与内容稳定重放，改换文件使用不同路径而不覆盖旧引用；候选终态唯一性仍由原生Task决定。S3条件写与摘要签名限制重复写入，不用上传准备器创建第二套决定状态。不同轮和技术重试有不同Task身份。地址最多有效5分钟；取消不撤销已签URL，但晚到对象不能完成任务或登记资源。

上传准备和PUT不持有数据库事务，不新建上传队列/任务调度器。对象已写入但响应丢失时，以原元数据重新申请，HEAD相同对象确认后跳过PUT；不同内容冲突，不重执行模型以重发文件。上传后的Object Path持久回写，签名地址不进入业务快照。未被接受/引用的对象保留，不自动删除历史或用户文件；生命周期回收不在本轮范围。

## 边界与失败

无身份、错Dispatch/attempt、替代/取消Task不签发；新Capability输出媒体类型决定可上传图片还是视频。图片最多16MiB，视频最多256MiB；候选位置0–7。媒体格式只接受已有图片/视频白名单，客户端先检查实际文件内容并计算摘要，Storage校验传输摘要和条件写。签名过期/上传未知可重申请原地址对应对象，存储故障不标任务成功。源码、存储协议和公开完整流程门禁必须分别验收，存在missing不宣称完成。
