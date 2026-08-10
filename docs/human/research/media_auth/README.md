# media_auth

> media 平台统一的认证 / 鉴权服务。当前为**设计产物阶段**:Protobuf IDL + 初始化 SQL + 设计文档已落盘,后续 hz 工具链生成的 Go handler/router 骨架见 P1。
>
> 配套领域模型与实现 spec 在 `docs/media_auth/`(CONTEXT / 6 domain / 4 workflow / spec);架构决策在 `docs/adr/`。

## 目录

```
docs/prd/media_auth/
├── README.md       # 本文件,顶层索引
├── design.md       # 设计文档(范围、边界、收敛路径、风险)
├── idl/            # Protobuf IDL(hz 工具链输入)
│   └── media_auth/
│       ├── auth.proto          # service: auth        本地账号 / 会话 / 密码
│       ├── identity.proto      # service: identity    外部身份绑定(generic)
│       ├── wecom.proto         # service: wecom       企微 OAuth 流程
│       ├── service_token.proto # service: service_token B2B API token
│       ├── oauth.proto         # service: oauth       OAuth2 provider(RFC 6749 + PKCE + RFC 8693)
│       ├── mcp_oauth.proto     # service: mcp_oauth   MCP 客户端 OAuth
│       └── token.proto         # service: token       internal VerifyToken(mTLS,供 control_plane 验 Bearer service token)
└── sql/
    └── 0001_init.sql           # media_auth schema 唯一源
```

## service 一句话

| service | 一句话 |
|---|---|
| `auth` | 本地账号登录 / LDAP / JWT cookie / 当前用户信息 / 改密 / 默认 prompt / auto_evolution 开关 |
| `identity` | 外部身份绑定列表与解绑(generic,目前承载企微等 provider) |
| `wecom` | 企微 corp 凭证获取 + 扫码登录 URL + OAuth callback |
| `service_token` | `tos_xxx` B2B API token 的创建 / 列表 / 撤销 |
| `oauth` | 第三方 OAuth2 客户端注册 + authorization_code / refresh_token / token-exchange 三种 grant + userinfo |
| `mcp_oauth` | 反向连接第三方 MCP 服务器用的 OAuth client(发现 / 连接状态 / 发起授权 / 回调 / 断开) |
| `token` | internal VerifyToken RPC(mTLS),供 control_plane 网关验 opaque Bearer service token |

## 后续动作

详见 `design.md` §6 收敛路径,以及 `docs/media_auth/spec/spec.md`。