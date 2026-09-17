# team_invite_accept 变更记录

## 2026-09-17：邀请限制在同一企业内

- Team 明确为 Organization 内部门后，Team Invite 只允许该 Organization 的现有成员接受。
- 部门邀请不再隐式赋予企业成员资格；需要加入企业时必须走独立的 Organization 邀请或管理流程。
- 接受流程新增 Organization Membership 校验，同时保留 Team Membership 的幂等写入。
