# Teamwork Runtime Context 能力覆盖矩阵

| 能力 | Harness 层 | 单元测试 | media-cli E2E | 状态 |
|:---|:---|:---|:---|:---|
| Workspace 公开读模型返回 `agent_configuration_id` | Handler/DTO | `TestRuntimeContextRejectsForeignConfiguration` | `TestMediaControlTeamworkRuntimeContextE2E` | covered |
| 创建和更新拓扑时校验成员配置属于同一 Workspace | Service + Repository | `TestRuntimeContextRejectsForeignConfiguration` | `TestMediaControlTeamworkRuntimeContextE2E` | covered |
| 内部服务身份读取 active topology 的当前版本投影 | Service + Handler | `TestRuntimeContext` | `TestMediaControlTeamworkRuntimeContextE2E` | covered |
| 普通用户不能调用内部 Runtime Context 接口 | Middleware/Handler | `TestRuntimeContextRequiresInternalPrincipal` | `TestMediaControlTeamworkRuntimeContextE2E` | covered |
| Runtime Context 不包含 Provider API Key | DTO | `TestRuntimeContext` | `TestMediaControlTeamworkRuntimeContextE2E` | covered |

## 接口契约

`GET /api/v1/teamwork-topologies/:topology_id/runtime-context`

- 仅接受由 `X-Control-Service-Token` 建立的内部 acting-user 身份。
- 按 acting user 校验 Topology 所属 Workspace；只返回 `active` topology。
- 返回 `topology_id`、`workspace_id`、`topology_version`、`entry_member_id`、`members_json` 和 `edges_json`。
- 投影只描述拓扑，不返回 Agent Configuration 密文或解密后的 Provider API Key；成员运行配置继续通过 Workspace Runtime Context 获取。
- 此切片不改变 Gateway 准入版本；拓扑变更事件和 Gateway Inbox 在后续独立切片实现。
