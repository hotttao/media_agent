// Package servicetoken 存放 service_token（B2B API 令牌）领域的稳定业务核心：Token 聚合、
// 仓库接口、platform 唯一不变量、domain 错误。
//
// 禁止 import Hertz / hertz_gen / biz/dal / biz/infra / logger / config / ORM。
// 不变量：每用户至多一个 active platform token（DB 部分唯一索引保证）；
//
//	明文 token 列仅 platform 非 NULL（ADR-0002）；非 platform 仅存 token_hash。
package servicetoken

import (
	"context"
	"time"
)

// Token 是 service_tokens 行的领域形态（聚合根）。
//
// Token（明文）仅 platform token 持有且仅创建时返回一次；非 platform 的 Token 字段为空。
type Token struct {
	ID         string
	Name       string
	TokenHash  string // SHA-256(明文 token)，DB 唯一查找键
	Token      string // 明文，仅 platform 非空（getPlatformToken 用）
	CreatedBy  string // user id
	CreatedAt  time.Time
	RevokedAt  *time.Time // 非空 = 已撤销
	IsPlatform bool
}

// IsRevoked 是否已撤销。
func (t Token) IsRevoked() bool { return t.RevokedAt != nil }

// IsActive 是否有效（未撤销）。
func (t Token) IsActive() bool { return !t.IsRevoked() }

// TokenRepo 是 service_token 领域的仓储接口（由 dal/repo/service_token 实现）。
type TokenRepo interface {
	// Create 新建 token 行（非 platform）。返回落库后的 Token（含 id/created_at）。
	Create(ctx context.Context, name, tokenHash, createdBy string) (Token, error)
	// GetByHash 按 token_hash 查未撤销 token（VerifyToken 用）。不存在/已撤销返回 ErrNotFound。
	GetByHash(ctx context.Context, tokenHash string) (Token, error)
	// ListByUser 列 user 的全部未撤销 token（不含 hash/明文），按 is_platform ASC, created_at DESC。
	ListByUser(ctx context.Context, userID string) ([]Token, error)
	// Revoke 撤销：UPDATE revoked_at=now WHERE id=? AND revoked_at IS NULL AND is_platform=false。
	// 0 行返回 ErrNotFound（不属本人 / 已撤销 / 是 platform）。
	Revoke(ctx context.Context, id string) error
	// GetPlatformToken 取 user 的 active platform token 明文。不存在返回 ""。
	GetPlatformToken(ctx context.Context, userID string) (string, error)
	// EnsurePlatformToken 确保 user 拥有 active platform token：存在则返回其明文；不存在则建。
	// 并发冲突由 DB 部分唯一索引兜底（ON CONFLICT DO NOTHING 后回读）。
	EnsurePlatformToken(ctx context.Context, userID string) (string, error)
}

// domain 错误。
var (
	ErrNotFound = errSentinel("service token not found")
)

// errSentinel 是简单的哨兵错误类型，支持 errors.Is 比较。
type errSentinel string

func (e errSentinel) Error() string { return string(e) }
