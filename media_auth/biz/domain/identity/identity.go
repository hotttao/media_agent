// Package identity 存放 identity（外部身份绑定）领域的稳定业务核心：Identity 值对象、
// Provider 枚举、仓库接口、双唯一不变量、domain 错误。
//
// 禁止 import Hertz / hertz_gen / biz/dal / biz/infra / logger / config / ORM。
// 不变量：(provider, external_id) 全局唯一；(user_id, provider) 每用户每 provider 至多一条。
package identity

import (
	"context"
	"encoding/json"
	"time"
)

// Provider 是外部身份提供方标识。
type Provider string

const (
	ProviderWeCom Provider = "wecom"
)

// Identity 是一条外部身份绑定值对象（user_identities 行的领域形态）。
//
// Metadata 为 provider-specific 字段（企微 avatar/email 等），明文 jsonb。
type Identity struct {
	UserID      string
	Provider    Provider
	ExternalID  string
	DisplayName string          // 空串 = NULL
	Metadata    json.RawMessage // 可空
	CreatedAt   time.Time
}

// IdentityRepo 是 identity 领域的仓储接口（由 dal/repo/identity 实现）。
type IdentityRepo interface {
	// ListByUser 列 user 的全部绑定，按 created_at ASC。
	ListByUser(ctx context.Context, userID string) ([]Identity, error)
	// FindByExternal 按 (provider, external_id) 查（PK 查询）。不存在返回 ErrNotFound。
	FindByExternal(ctx context.Context, provider Provider, externalID string) (Identity, error)
	// FindByUserProvider 按 (user_id, provider) 查（唯一索引）。不存在返回 ErrNotFound。
	// 供 AuthSourceResolver 与 bindIdentity 复查。
	FindByUserProvider(ctx context.Context, userID string, provider Provider) (Identity, error)
	// Create 新建绑定。违反 (provider,external_id) 唯一 → ErrAlreadyBound。
	Create(ctx context.Context, id Identity) (Identity, error)
	// Upsert 按 (user_id, provider) upsert（ON CONFLICT DO UPDATE external_id/display_name/metadata）。
	// 返回落库后的 Identity。
	Upsert(ctx context.Context, id Identity) (Identity, error)
	// Delete 按 (user_id, provider) 删。0 行返回 ErrNotFound。
	Delete(ctx context.Context, userID string, provider Provider) error
}

// domain 错误。
var (
	ErrNotFound     = errSentinel("identity not found")
	ErrAlreadyBound = errSentinel("external identity already bound to another user")
)

// errSentinel 是简单的哨兵错误类型，支持 errors.Is 比较。
type errSentinel string

func (e errSentinel) Error() string { return string(e) }
