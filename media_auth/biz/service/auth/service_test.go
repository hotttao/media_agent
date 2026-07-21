// service_test.go：auth 用例 service 单测（fake repo/signer/ldap，不依赖 DB/网络）。
//
// 覆盖：Login 本地密码成功 / LDAP upsert / 凭证失败 / system 禁登；
//
//	GetMe；ChangePassword 成功 / 旧密码错 / 密码过短 / 无本地密码；
//	UpdateMe；SetDefaultPrompt。
package auth

import (
	"context"
	"errors"
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
func (r *fakeRepo) CreateUser(_ context.Context, username, displayName, passwordHash, email, role string) (domainauth.User, error) {
	if _, ok := r.byName[username]; ok {
		return domainauth.User{}, domainauth.ErrUsernameTaken
	}
	u := domainauth.User{ID: "u-" + username, Username: username, DisplayName: displayName, PasswordHash: passwordHash, Email: email, Role: role}
	r.users[u.ID] = u
	r.byName[username] = u.ID
	return u, nil
}
func (r *fakeRepo) ListUsers(_ context.Context, q, sort, order string, page, pageSize int) ([]domainauth.User, int, error) {
	return nil, 0, nil
}
func (r *fakeRepo) DeleteUser(_ context.Context, id string) error {
	if u, ok := r.users[id]; ok {
		delete(r.users, id)
		delete(r.byName, u.Username)
		return nil
	}
	return domainauth.ErrUserNotFound
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

// --- Register ---

func TestRegister_Success(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, nil)
	user, token, err := svc.Register(context.Background(), RegisterCommand{
		Username: "newuser", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Username != "newuser" || user.AuthSource != "password" {
		t.Errorf("got user=%+v", user)
	}
	if token == "" {
		t.Errorf("token empty (should auto-login)")
	}
	// 用户落库。
	got, err := repo.FindByUsername(context.Background(), "newuser")
	if err != nil || got.Role != "user" || got.DisplayName != "newuser" {
		t.Errorf("user not persisted as role=user: got=%+v err=%v", got, err)
	}
}

func TestRegister_DisplayNameDefaultsToUsername(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, nil)
	_, _, err := svc.Register(context.Background(), RegisterCommand{Username: "bob", Password: "secret123"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, _ := repo.FindByUsername(context.Background(), "bob")
	if got.DisplayName != "bob" {
		t.Errorf("display_name should default to username, got %q", got.DisplayName)
	}
}

func TestRegister_ValidationErrors(t *testing.T) {
	svc := newSvc(newFakeRepo(), nil)
	cases := []struct {
		name string
		cmd  RegisterCommand
		err  error
	}{
		{"empty username", RegisterCommand{Password: "secret123"}, ErrUsernameRequired},
		{"short password", RegisterCommand{Username: "x", Password: "123"}, ErrPasswordTooShort},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := svc.Register(context.Background(), c.cmd)
			if !errors.Is(err, c.err) {
				t.Errorf("want %v, got %v", c.err, err)
			}
		})
	}
}

func TestRegister_UsernameTaken(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(repo, nil)
	// 先注册一个。
	svc.Register(context.Background(), RegisterCommand{Username: "dup", Password: "secret123"})
	// 再注册同名 -> ErrUsernameTaken。
	_, _, err := svc.Register(context.Background(), RegisterCommand{Username: "dup", Password: "secret123"})
	if !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("want ErrUsernameTaken, got %v", err)
	}
}
