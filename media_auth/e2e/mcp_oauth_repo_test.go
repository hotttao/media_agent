//go:build e2e

// mcp_oauth_repo_test.go：mcp_oauth 仓储集成测试（testcontainers postgres）。
//
// 覆盖关键不变量：UpsertToken 重置失败计数 + COALESCE 保留旧 refresh_token、
//
//	RecordRefreshFailure 计数 +1 并盖章（首次时间不随后续失败移动）。
package e2e

import (
	"context"
	"testing"

	dalmcp "media_agent/media_auth/biz/dal/repo/mcp_oauth"
	domainmcp "media_agent/media_auth/biz/domain/mcp_oauth"
)

func newMcpRepo(t *testing.T) domainmcp.McpRepo {
	db := newDB(t)
	seedUser(t, db, "u1", "alice")
	if err := db.Gorm.Exec(`INSERT INTO media_auth.mcp_oauth_clients (server_origin, client_id) VALUES ('https://mcp.example.com', 'cid')`).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	return dalmcp.NewRepo(db.Gorm)
}

func TestMcpRepo_UpsertToken_ResetsFailCounter(t *testing.T) {
	repo := newMcpRepo(t)
	repo.UpsertToken(context.Background(), "u1", "https://mcp.example.com", domainmcp.TokenResponse{
		AccessToken: "at1", RefreshToken: "rt1", TokenType: "Bearer", ExpiresIn: 1800,
	}, 1800)
	tok, _ := repo.GetToken(context.Background(), "u1", "https://mcp.example.com")
	if tok.AccessToken != "at1" || tok.RefreshFailCount != 0 {
		t.Errorf("first upsert: got tok=%+v", tok)
	}

	// 模拟两次刷新失败。
	repo.RecordRefreshFailure(context.Background(), "u1", "https://mcp.example.com")
	repo.RecordRefreshFailure(context.Background(), "u1", "https://mcp.example.com")
	tok, _ = repo.GetToken(context.Background(), "u1", "https://mcp.example.com")
	if tok.RefreshFailCount != 2 {
		t.Errorf("after 2 fails: want count=2, got %d", tok.RefreshFailCount)
	}

	// 再次 upsert（成功 refresh）→ 计数重置。
	repo.UpsertToken(context.Background(), "u1", "https://mcp.example.com", domainmcp.TokenResponse{
		AccessToken: "at2", RefreshToken: "rt2", TokenType: "Bearer", ExpiresIn: 1800,
	}, 1800)
	tok, _ = repo.GetToken(context.Background(), "u1", "https://mcp.example.com")
	if tok.AccessToken != "at2" || tok.RefreshFailCount != 0 || tok.RefreshFailFirstAt != nil {
		t.Errorf("after re-upsert: want count=0 + firstFailAt=nil, got tok=%+v", tok)
	}
}

func TestMcpRepo_UpsertToken_COALESCE_RefreshToken(t *testing.T) {
	repo := newMcpRepo(t)
	// 首次 upsert 带 refresh_token。
	repo.UpsertToken(context.Background(), "u1", "https://mcp.example.com", domainmcp.TokenResponse{
		AccessToken: "at1", RefreshToken: "rt1", ExpiresIn: 1800,
	}, 1800)
	// 再次 upsert 不带 refresh_token → 应保留旧 rt1（COALESCE）。
	repo.UpsertToken(context.Background(), "u1", "https://mcp.example.com", domainmcp.TokenResponse{
		AccessToken: "at2", ExpiresIn: 1800,
	}, 1800)
	tok, _ := repo.GetToken(context.Background(), "u1", "https://mcp.example.com")
	if tok.AccessToken != "at2" {
		t.Errorf("access: want at2, got %q", tok.AccessToken)
	}
	if tok.RefreshToken != "rt1" {
		t.Errorf("refresh should be preserved (COALESCE): want rt1, got %q", tok.RefreshToken)
	}
}

func TestMcpRepo_RecordRefreshFailure_StampsFirstFailAt(t *testing.T) {
	repo := newMcpRepo(t)
	repo.UpsertToken(context.Background(), "u1", "https://mcp.example.com", domainmcp.TokenResponse{
		AccessToken: "at1", RefreshToken: "rt1", ExpiresIn: 1800,
	}, 1800)

	count, first, err := repo.RecordRefreshFailure(context.Background(), "u1", "https://mcp.example.com")
	if err != nil || count != 1 || first == nil {
		t.Fatalf("first fail: count=%d first=%v err=%v", count, first, err)
	}
	firstStamp := *first
	// 第二次失败 → firstFailAt 不变（COALESCE 保留首次）。
	count, first, _ = repo.RecordRefreshFailure(context.Background(), "u1", "https://mcp.example.com")
	if count != 2 || first == nil || !first.Equal(firstStamp) {
		t.Errorf("second fail: count=%d first moved (want same), got first=%v", count, first)
	}
}

func TestMcpRepo_DeleteToken(t *testing.T) {
	repo := newMcpRepo(t)
	repo.UpsertToken(context.Background(), "u1", "https://mcp.example.com", domainmcp.TokenResponse{
		AccessToken: "at1", ExpiresIn: 1800,
	}, 1800)
	if err := repo.DeleteToken(context.Background(), "u1", "https://mcp.example.com"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	_, err := repo.GetToken(context.Background(), "u1", "https://mcp.example.com")
	if err == nil {
		t.Errorf("after delete: want error, got nil")
	}
}

func TestMcpRepo_GetClient(t *testing.T) {
	repo := newMcpRepo(t)
	c, err := repo.GetClient(context.Background(), "https://mcp.example.com")
	if err != nil || c.ClientID != "cid" {
		t.Errorf("GetClient: got c=%+v err=%v", c, err)
	}
	_, err = repo.GetClient(context.Background(), "https://nope.com")
	if err == nil {
		t.Errorf("missing client: want error")
	}
}
