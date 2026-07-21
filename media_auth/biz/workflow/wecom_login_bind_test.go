// wecom_login_bind_test.go：企微登录/绑定 workflow 编排测试（fake wecom client + fake user repo +
// real identity service + real signer + fake ensurer）。
//
// 覆盖：login 全流程（未绑定→not_bound / 命中→签 session+ensure platform token）；
//
//	bind 全流程（未登录→auth_required / 已绑别人→already_bound / 成功→bindIdentity）；
//	缺 code → missing_code；企微 API 失败 → oauth_failed。
package workflow

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	configpb "media_agent/hertz_gen/config"
	domainauth "media_agent/media_auth/biz/domain/auth"
	domainidentity "media_agent/media_auth/biz/domain/identity"
	domainwecom "media_agent/media_auth/biz/domain/wecom"
	identityservice "media_agent/media_auth/biz/service/identity"
	"media_agent/media_auth/biz/shared/session"
)

// fakeIdentity + fakeIdentityRepo：workflow 测试用的 identity 仓储 fake（真实 identityservice 持有）。
type fakeIdentity struct {
	UserID      string
	Provider    string
	ExternalID  string
	DisplayName string
}

type fakeIdentityRepo struct {
	mu         sync.Mutex
	byExternal map[string]fakeIdentity // key: provider+":"+external_id
}

func newIdentityFakeRepo() *fakeIdentityRepo {
	return &fakeIdentityRepo{byExternal: map[string]fakeIdentity{}}
}

func (r *fakeIdentityRepo) ListByUser(_ context.Context, userID string) ([]domainidentity.Identity, error) {
	return nil, nil
}
func (r *fakeIdentityRepo) FindByExternal(_ context.Context, provider domainidentity.Provider, externalID string) (domainidentity.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byExternal[string(provider)+":"+externalID]
	if !ok {
		return domainidentity.Identity{}, domainidentity.ErrNotFound
	}
	return domainidentity.Identity{UserID: id.UserID, Provider: provider, ExternalID: id.ExternalID, DisplayName: id.DisplayName}, nil
}
func (r *fakeIdentityRepo) FindByUserProvider(_ context.Context, userID string, provider domainidentity.Provider) (domainidentity.Identity, error) {
	return domainidentity.Identity{}, domainidentity.ErrNotFound
}
func (r *fakeIdentityRepo) Create(_ context.Context, id domainidentity.Identity) (domainidentity.Identity, error) {
	return r.Upsert(nil, id)
}
func (r *fakeIdentityRepo) Upsert(_ context.Context, id domainidentity.Identity) (domainidentity.Identity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := string(id.Provider) + ":" + id.ExternalID
	if existing, ok := r.byExternal[k]; ok && existing.UserID != id.UserID {
		return domainidentity.Identity{}, domainidentity.ErrAlreadyBound
	}
	r.byExternal[k] = fakeIdentity{UserID: id.UserID, Provider: string(id.Provider), ExternalID: id.ExternalID, DisplayName: id.DisplayName}
	return id, nil
}
func (r *fakeIdentityRepo) Delete(_ context.Context, userID string, provider domainidentity.Provider) error {
	return domainidentity.ErrNotFound
}

// ── fakes ────────────────────────────────────────────────────────────

type fakeWeComClient struct {
	useridByCode map[string]string // code → userid
	userInfo     map[string]domainwecom.UserInfo
	codeErr      error
}

func (f *fakeWeComClient) GetUserIDByCode(_ context.Context, code string) (string, error) {
	if f.codeErr != nil {
		return "", f.codeErr
	}
	return f.useridByCode[code], nil
}
func (f *fakeWeComClient) GetUserInfo(_ context.Context, userid string) (domainwecom.UserInfo, error) {
	return f.userInfo[userid], nil
}

type fakeUserRepo struct {
	users map[string]domainauth.User
}

func (r *fakeUserRepo) FindByID(_ context.Context, id string) (domainauth.User, error) {
	u, ok := r.users[id]
	if !ok {
		return domainauth.User{}, domainauth.ErrUserNotFound
	}
	return u, nil
}

// 以下方法 workflow 不用，仅满足 UserRepo 接口。
func (r *fakeUserRepo) FindByUsername(context.Context, string) (domainauth.User, error) {
	return domainauth.User{}, domainauth.ErrUserNotFound
}
func (r *fakeUserRepo) Upsert(context.Context, string, string, string) (domainauth.User, error) {
	return domainauth.User{}, nil
}
func (r *fakeUserRepo) SetPassword(context.Context, string, string) error      { return nil }
func (r *fakeUserRepo) SetDefaultPrompt(context.Context, string, string) error { return nil }
func (r *fakeUserRepo) SetAutoEvolution(context.Context, string, bool) error   { return nil }
func (r *fakeUserRepo) UpdateLastLogin(context.Context, string) error          { return nil }
func (r *fakeUserRepo) CreateUser(_ context.Context, username, displayName, passwordHash, email, role string) (domainauth.User, error) {
	return domainauth.User{ID: "u-" + username, Username: username, DisplayName: displayName, Role: role}, nil
}
func (r *fakeUserRepo) ListUsers(context.Context, string, string, string, int, int) ([]domainauth.User, int, error) {
	return nil, 0, nil
}
func (r *fakeUserRepo) DeleteUser(_ context.Context, id string) error {
	delete(r.users, id)
	return nil
}

type fakeEnsurer struct {
	called bool
	mu     sync.Mutex
}

func (e *fakeEnsurer) EnsurePlatformToken(_ context.Context, userID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.called = true
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────

func newTestSigner(t *testing.T) *session.Signer {
	t.Helper()
	s, err := session.NewSigner(&configpb.AuthConfig{JwtSecret: "test-secret", JwtExpiresInSeconds: 3600})
	if err != nil {
		t.Fatalf("newSigner: %v", err)
	}
	return s
}

// newWecomFlow 构造 workflow，identity 用真实 service + fake repo，wecom/users/ensurer 用 fake。
func newWecomFlow(t *testing.T, wecom *fakeWeComClient, users *fakeUserRepo, identity *identityservice.Service, signer *session.Signer, ensurer domainauth.PlatformTokenEnsurer) *WecomLoginBind {
	t.Helper()
	return NewWecomLoginBind(wecom, identity, users, signer, ensurer)
}

// ── login flow ───────────────────────────────────────────────────────

func TestWecomLogin_MissingCode(t *testing.T) {
	w := newWecomFlow(t, &fakeWeComClient{}, &fakeUserRepo{}, identityservice.NewService(newIdentityFakeRepo()), newTestSigner(t), &fakeEnsurer{})
	res, err := w.HandleCallback(context.Background(), "", "login", "")
	if err != nil || res.RedirectURL != "/login?error=missing_code" {
		t.Errorf("missing code: got res=%+v err=%v", res, err)
	}
}

func TestWecomLogin_NotBound(t *testing.T) {
	wecom := &fakeWeComClient{useridByCode: map[string]string{"code1": "wecom-userid"}}
	w := newWecomFlow(t, wecom, &fakeUserRepo{}, identityservice.NewService(newIdentityFakeRepo()), newTestSigner(t), &fakeEnsurer{})
	res, _ := w.HandleCallback(context.Background(), "code1", "login", "")
	if res.RedirectURL != "/login?error=not_bound" {
		t.Errorf("not bound: got %q", res.RedirectURL)
	}
}

func TestWecomLogin_Success(t *testing.T) {
	// identity 已绑 wecom-userid → u1；user u1 存在（role=user）。
	identityRepo := newIdentityFakeRepo()
	identityRepo.byExternal["wecom:wecom-userid"] = fakeIdentity{UserID: "u1", Provider: "wecom", ExternalID: "wecom-userid"}
	users := &fakeUserRepo{users: map[string]domainauth.User{
		"u1": {ID: "u1", Username: "alice", DisplayName: "Alice", Role: "user"},
	}}
	ensurer := &fakeEnsurer{}
	wecom := &fakeWeComClient{useridByCode: map[string]string{"code1": "wecom-userid"}}
	signer := newTestSigner(t)
	w := newWecomFlow(t, wecom, users, identityservice.NewService(identityRepo), signer, ensurer)

	res, err := w.HandleCallback(context.Background(), "code1", "login", "")
	if err != nil {
		t.Fatalf("login success: %v", err)
	}
	if res.RedirectURL != "/" || !res.SetCookie || res.SessionToken == "" {
		t.Errorf("login success: got res=%+v", res)
	}
	// session token 可验签。
	sess, err := signer.Verify(res.SessionToken)
	if err != nil || sess.Sub != "u1" {
		t.Errorf("verify session: got sess=%+v err=%v", sess, err)
	}
	// ensure platform token 被调（fire-and-forget，等一下 goroutine）。
	time.Sleep(50 * time.Millisecond)
	if !ensurer.called {
		t.Errorf("ensurePlatformToken not called")
	}
}

func TestWecomLogin_SystemAccount(t *testing.T) {
	identityRepo := newIdentityFakeRepo()
	identityRepo.byExternal["wecom:sys"] = fakeIdentity{UserID: "u_sys", Provider: "wecom", ExternalID: "sys"}
	users := &fakeUserRepo{users: map[string]domainauth.User{
		"u_sys": {ID: "u_sys", Username: "sys", Role: "system"},
	}}
	wecom := &fakeWeComClient{useridByCode: map[string]string{"code1": "sys"}}
	w := newWecomFlow(t, wecom, users, identityservice.NewService(identityRepo), newTestSigner(t), &fakeEnsurer{})
	res, _ := w.HandleCallback(context.Background(), "code1", "login", "")
	if res.RedirectURL != "/login?error=system_account" {
		t.Errorf("system account: got %q", res.RedirectURL)
	}
}

func TestWecomLogin_OAuthFailed(t *testing.T) {
	wecom := &fakeWeComClient{codeErr: errors.New("wecom api down")}
	w := newWecomFlow(t, wecom, &fakeUserRepo{}, identityservice.NewService(newIdentityFakeRepo()), newTestSigner(t), &fakeEnsurer{})
	res, _ := w.HandleCallback(context.Background(), "code1", "login", "")
	if res.RedirectURL != "/login?error=oauth_failed" {
		t.Errorf("oauth failed: got %q", res.RedirectURL)
	}
}

// ── bind flow ────────────────────────────────────────────────────────

func TestWecomBind_AuthRequired(t *testing.T) {
	// 无 cookie token → auth_required。
	wecom := &fakeWeComClient{useridByCode: map[string]string{"code1": "wecom-userid"}}
	w := newWecomFlow(t, wecom, &fakeUserRepo{}, identityservice.NewService(newIdentityFakeRepo()), newTestSigner(t), &fakeEnsurer{})
	res, _ := w.HandleCallback(context.Background(), "code1", "bind", "")
	if res.RedirectURL != "/login?error=auth_required" {
		t.Errorf("bind no session: got %q", res.RedirectURL)
	}
}

func TestWecomBind_AlreadyBound(t *testing.T) {
	// wecom-userid 已绑 u_other；当前 user u1 试图 bind → already_bound。
	identityRepo := newIdentityFakeRepo()
	identityRepo.byExternal["wecom:wecom-userid"] = fakeIdentity{UserID: "u_other", Provider: "wecom", ExternalID: "wecom-userid"}
	signer := newTestSigner(t)
	cookieToken, _ := signer.Sign(domainauth.Session{Sub: "u1", Username: "alice", Name: "Alice", Role: "user"})
	wecom := &fakeWeComClient{
		useridByCode: map[string]string{"code1": "wecom-userid"},
		userInfo:     map[string]domainwecom.UserInfo{"wecom-userid": {UserID: "wecom-userid", Name: "Wecom User"}},
	}
	w := newWecomFlow(t, wecom, &fakeUserRepo{}, identityservice.NewService(identityRepo), signer, &fakeEnsurer{})
	res, _ := w.HandleCallback(context.Background(), "code1", "bind", cookieToken)
	if res.RedirectURL != "/?wecom_bind=already_bound" {
		t.Errorf("already bound: got %q", res.RedirectURL)
	}
}

func TestWecomBind_Success(t *testing.T) {
	identityRepo := newIdentityFakeRepo()
	signer := newTestSigner(t)
	cookieToken, _ := signer.Sign(domainauth.Session{Sub: "u1", Username: "alice", Name: "Alice", Role: "user"})
	wecom := &fakeWeComClient{
		useridByCode: map[string]string{"code1": "wecom-userid"},
		userInfo:     map[string]domainwecom.UserInfo{"wecom-userid": {UserID: "wecom-userid", Name: "Wecom User", Avatar: "http://x/a.png"}},
	}
	w := newWecomFlow(t, wecom, &fakeUserRepo{}, identityservice.NewService(identityRepo), signer, &fakeEnsurer{})
	res, _ := w.HandleCallback(context.Background(), "code1", "bind", cookieToken)
	if res.RedirectURL != "/?wecom_bind=success" {
		t.Errorf("bind success: got %q", res.RedirectURL)
	}
	// 验证 identity 落库（u1 绑 wecom-userid）。
	svc := identityservice.NewService(identityRepo)
	uid, _ := svc.FindUserIDentity(context.Background(), "wecom", "wecom-userid")
	if uid != "u1" {
		t.Errorf("after bind: identity should map wecom-userid → u1, got %q", uid)
	}
}
