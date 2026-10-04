# 2026-10-04 · OAuth凭据与旧ServiceToken工具解耦

Client Secret改用既有独立opaque生成器，保留旧Secret的哈希验证与全部grant行为。领域能力清单补齐已有管理员客户端CRUD，不把共享随机工具误判为ServiceToken依赖。
