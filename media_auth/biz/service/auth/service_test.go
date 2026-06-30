// service_test.go：auth 用例 service 单测（fake repo/signer/ldap，不依赖 DB/网络）。
//
// 覆盖：Login 本地密码成功 / LDAP upsert / 凭证失败 / system 禁登；
//
//	GetMe；ChangePassword 成功 / 旧密码错 / 密码过短 / 无本地密码；
//	UpdateMe；SetDefaultPrompt。
package auth

import (
	"context"
	"testing"
	"time"

	domainauth "media_agent/media_auth/biz/domain/auth"
	"media_agent/media_auth/biz/shared/argon2"
	ldapclient "media_agent/media_auth/biz/shared/client/ldap"
)

// fakes

type fakeRepo struct {
	users            map[string]domainauth.User // by id
	byName           map[string]string          // username → id
	lastLoginUpdated string
	passwordSet      string
	autoEvolution    *bool
	defaultPrompt    string
	upserted         bool
}

func newFakeRepo(seed ...domainauth.User) *fakeRepo {
	r := &fakeRepo{users: map[string]domainauth.User{}, byName: map[string]string{}}
	for _, u := range seed {
		r.users[u.ID] = u
		r.byName[u.Username] = u.ID
	}
	return r
}

func (r *fakeRepo) FindByUsername(_ context.Context, username string) (domainauth.User, error) {
	id, ok := r.byName[username]
	if !ok {
		return domainauth.User{}, domainauth.ErrUserNotFound
	}
	return r.users[id], nil
}
func (r *fakeRepo) FindByID(_ context.Context, id string) (domainauth.User, error) {
	u, ok := r.users[id]
	if !ok {
		return domainauth.User{}, domainauth.ErrUserNotFound
	}
	return u, nil
}
func (r *fakeRepo) Upsert(_ context.Context, username, displayName, email string) (domainauth.User, error) {
	r.upserted = true
	if id, ok := r.byName[username]; ok {
		u := r.users[id]
		u.DisplayName = displayName
		u.LastLoginAt = time.Now()
		r.users[id] = u
		return u, nil
	}
	u := domainauth.User{ID: "ldap-" + username, Username: username, DisplayName: displayName, Email: email, Role: "user"}
	r.users[u.ID] = u
	r.byName[username] = u.ID
	return u, nil
}
func (r *fakeRepo) SetPassword(_ context.Context, id, hash string) error {
	r.passwordSet = hash
	return nil
}
func (r *fakeRepo) SetDefaultPrompt(_ context.Context, id, promptID string) error {
	r.defaultPrompt = promptID
	return nil
}
func (r *fakeRepo) SetAutoEvolution(_ context.Context, id string, v bool) error {
	r.autoEvolution = &v
	return nil
}
func (r *fakeRepo) UpdateLastLogin(_ context.Context, id string) error {
	r.lastLoginUpdated = id
	return nil
}

type fakeSigner struct{}

func (fakeSigner) Sign(s domainauth.Session) (string, error) { return "tok-" + s.Sub, nil }
func (fakeSigner) Verify(token string) (domainauth.Session, error) {
	return domainauth.Session{Sub: token[4:]}, nil
}
func (fakeSigner) ShouldRenew(domainauth.Session) bool        { return false }
func (fakeSigner) Renew(s domainauth.Session) (string, error) { return "tok-" + s.Sub, nil }

// fakeLdapClient 实现 LdapAuthenticator 接口，注入固定返回。
type fakeLdapClient struct{ user *ldapclient.LdapUser }

func (f fakeLdapClient) Authenticate(_ context.Context, _, _ string) (*ldapclient.LdapUser, error) {
	return f.user, nil
}

// newSvc 构造带 fake 的 service。ldap 为 nil 表示禁用 LDAP。
func newSvc(repo domainauth.UserRepo, ldap LdapAuthenticator) *Service {
	return NewService(repo, fakeSigner{}, ldap)
}

// --- Login ---

func TestLogin_LocalPasswordSuccess(t *testing.T) {
	hash, _ := argon2.Hash("secret123")
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", PasswordHash: hash, Role: "user"})
	svc := newSvc(repo, nil)

	user, token, err := svc.Login(context.Background(), LoginCommand{Username: "alice", Password: "secret123"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if user.Username != "alice" || user.AuthSource != "password" {
		t.Errorf("got user=%+v", user)
	}
	if token != "tok-u1" {
		t.Errorf("token: want tok-u1, got %s", token)
	}
	if repo.lastLoginUpdated != "u1" {
		t.Errorf("last_login not updated")
	}
}

func TestLogin_LdapUpsert(t *testing.T) {
	repo := newFakeRepo()
	ldap := fakeLdapClient{user: &ldapclient.LdapUser{Username: "bob", Name: "Bob", Email: "bob@x.com"}}
	svc := newSvc(repo, ldap)

	user, _, err := svc.Login(context.Background(), LoginCommand{Username: "bob", Password: "pw"})
	if err != nil {
		t.Fatalf("Login ldap: %v", err)
	}
	if !repo.upserted {
		t.Errorf("expected upsert")
	}
	if user.Username != "bob" || user.AuthSource != "ldap" {
		t.Errorf("got user=%+v", user)
	}
}

func TestLogin_LdapDisabled_BadCredentials(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, nil) // LDAP 禁用
	_, _, err := svc.Login(context.Background(), LoginCommand{Username: "bob", Password: "pw"})
	if err != ErrInvalidCredentials {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_SystemBlocked(t *testing.T) {
	hash, _ := argon2.Hash("secret123")
	repo := newFakeRepo(domainauth.User{ID: "sys", Username: "root", PasswordHash: hash, Role: "system"})
	svc := newSvc(repo, nil)

	_, _, err := svc.Login(context.Background(), LoginCommand{Username: "root", Password: "secret123"})
	if err != ErrAccountDisabled {
		t.Errorf("want ErrAccountDisabled, got %v", err)
	}
}

func TestLogin_BadCredentials(t *testing.T) {
	hash, _ := argon2.Hash("secret123")
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", PasswordHash: hash, Role: "user"})
	svc := newSvc(repo, nil)

	_, _, err := svc.Login(context.Background(), LoginCommand{Username: "alice", Password: "wrong"})
	if err != ErrInvalidCredentials {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

// --- GetMe ---

func TestGetMe(t *testing.T) {
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", Role: "admin"})
	svc := newSvc(repo, nil)

	user, err := svc.GetMe(context.Background(), domainauth.Session{Sub: "u1"})
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if user.Role != "admin" || user.AuthSource != "ldap" {
		t.Errorf("got user=%+v", user)
	}
}

// --- ChangePassword ---

func TestChangePassword_Success(t *testing.T) {
	hash, _ := argon2.Hash("oldpass1")
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", PasswordHash: hash, Role: "user"})
	svc := newSvc(repo, nil)

	err := svc.ChangePassword(context.Background(), domainauth.Session{Sub: "u1"}, ChangePasswordCommand{
		CurrentPassword: "oldpass1", NewPassword: "newpass1",
	})
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if repo.passwordSet == "" {
		t.Errorf("password not set")
	}
	// 新密码可验
	if err := argon2.Verify("newpass1", repo.passwordSet); err != nil {
		t.Errorf("new hash verify: %v", err)
	}
}

func TestChangePassword_WrongCurrent(t *testing.T) {
	hash, _ := argon2.Hash("oldpass1")
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", PasswordHash: hash, Role: "user"})
	svc := newSvc(repo, nil)

	err := svc.ChangePassword(context.Background(), domainauth.Session{Sub: "u1"}, ChangePasswordCommand{
		CurrentPassword: "wrong", NewPassword: "newpass1",
	})
	if err != ErrInvalidCredentials {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestChangePassword_TooShort(t *testing.T) {
	hash, _ := argon2.Hash("oldpass1")
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", PasswordHash: hash, Role: "user"})
	svc := newSvc(repo, nil)

	err := svc.ChangePassword(context.Background(), domainauth.Session{Sub: "u1"}, ChangePasswordCommand{
		CurrentPassword: "oldpass1", NewPassword: "123",
	})
	if err == nil {
		t.Errorf("want error for short password")
	}
}

func TestChangePassword_NoLocalPassword(t *testing.T) {
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", Role: "user"}) // 无 password_hash
	svc := newSvc(repo, nil)

	err := svc.ChangePassword(context.Background(), domainauth.Session{Sub: "u1"}, ChangePasswordCommand{
		CurrentPassword: "x", NewPassword: "newpass1",
	})
	if err != ErrPasswordNotSet {
		t.Errorf("want ErrPasswordNotSet, got %v", err)
	}
}

// --- UpdateMe / SetDefaultPrompt ---

func TestUpdateMe(t *testing.T) {
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", Role: "user"})
	svc := newSvc(repo, nil)
	v := true
	if err := svc.UpdateMe(context.Background(), domainauth.Session{Sub: "u1"}, UpdateMeCommand{AutoEvolution: &v}); err != nil {
		t.Fatalf("UpdateMe: %v", err)
	}
	if repo.autoEvolution == nil || !*repo.autoEvolution {
		t.Errorf("autoEvolution not set true")
	}
}

func TestSetDefaultPrompt(t *testing.T) {
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", Role: "user"})
	svc := newSvc(repo, nil)
	pid := "p1"
	if err := svc.SetDefaultPrompt(context.Background(), domainauth.Session{Sub: "u1"}, SetDefaultPromptCommand{PromptID: &pid}); err != nil {
		t.Fatalf("SetDefaultPrompt: %v", err)
	}
	if repo.defaultPrompt != "p1" {
		t.Errorf("defaultPrompt: want p1, got %q", repo.defaultPrompt)
	}
}
