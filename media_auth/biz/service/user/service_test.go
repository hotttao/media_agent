// service_test.go：用户管理用例 service 单测（fake repo）。
//
// 覆盖：ListUsers/CreateUser/DeleteUser/ChangeUserPassword 的 admin 校验 + 业务规则。
package user

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domainauth "media_agent/media_auth/biz/domain/auth"
	"media_agent/media_auth/biz/shared/argon2"
)

// fakeRepo 实现 domainauth.UserRepo（内存版）。
type fakeRepo struct {
	mu       sync.Mutex
	users    map[string]domainauth.User // by id
	byName   map[string]string          // username -> id
	password string                     // last SetPassword hash
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
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byName[username]
	if !ok {
		return domainauth.User{}, domainauth.ErrUserNotFound
	}
	return r.users[id], nil
}
func (r *fakeRepo) FindByID(_ context.Context, id string) (domainauth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return domainauth.User{}, domainauth.ErrUserNotFound
	}
	return u, nil
}
func (r *fakeRepo) Upsert(_ context.Context, username, displayName, email string) (domainauth.User, error) {
	return domainauth.User{}, nil
}
func (r *fakeRepo) CreateUser(_ context.Context, username, displayName, passwordHash, email, role string) (domainauth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[username]; ok {
		return domainauth.User{}, domainauth.ErrUsernameTaken
	}
	u := domainauth.User{ID: "u-" + username, Username: username, DisplayName: displayName, PasswordHash: passwordHash, Email: email, Role: role, CreatedAt: time.Now()}
	r.users[u.ID] = u
	r.byName[username] = u.ID
	return u, nil
}
func (r *fakeRepo) ListUsers(_ context.Context, q, sort, order string, page, pageSize int) ([]domainauth.User, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domainauth.User, 0, len(r.users))
	for _, u := range r.users {
		if u.Role != "system" {
			out = append(out, u)
		}
	}
	return out, len(out), nil
}
func (r *fakeRepo) SoftDeleteUser(_ context.Context, id, deletedBy string) (domainauth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return domainauth.User{}, domainauth.ErrUserNotFound
	}
	u.Status, u.AuthVersion, u.DeletedBy = domainauth.UserStatusDeleted, u.AuthVersion+1, deletedBy
	r.users[id] = u
	return u, nil
}
func (r *fakeRepo) CountActiveAdmins(context.Context) (int, error) { return 2, nil }

type fakeDeleter struct{ repo *fakeRepo }

func (d fakeDeleter) Delete(ctx context.Context, currentUserID, targetID string) error {
	_, err := d.repo.SoftDeleteUser(ctx, targetID, currentUserID)
	return err
}
func (r *fakeRepo) SetPassword(_ context.Context, id, passwordHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.password = passwordHash
	if u, ok := r.users[id]; ok {
		u.PasswordHash = passwordHash
		r.users[id] = u
	}
	return nil
}
func (r *fakeRepo) SetDefaultPrompt(_ context.Context, id, promptID string) error { return nil }
func (r *fakeRepo) SetAutoEvolution(_ context.Context, id string, v bool) error   { return nil }
func (r *fakeRepo) UpdateLastLogin(_ context.Context, id string) error            { return nil }

// ── 用例 ──────────────────────────────────────────────────────────────

func TestListUsers_AdminOnly(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, _, err := svc.ListUsers(context.Background(), "user", "", "", "", 1, 10)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("non-admin: want ErrForbidden, got %v", err)
	}
}

func TestListUsers_Success(t *testing.T) {
	repo := newFakeRepo(
		domainauth.User{ID: "u1", Username: "alice", Role: "user"},
		domainauth.User{ID: "u2", Username: "bob", Role: "admin"},
		domainauth.User{ID: "sys", Username: "sys", Role: "system"}, // 排除
	)
	svc := NewService(repo)
	users, total, err := svc.ListUsers(context.Background(), "admin", "", "", "", 1, 10)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if total != 2 || len(users) != 2 {
		t.Errorf("want 2 users (excl system), got total=%d len=%d", total, len(users))
	}
}

func TestCreateUser_AdminOnly(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.CreateUser(context.Background(), "user", CreateUserCommand{Username: "x", Password: "secret123"})
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("non-admin: want ErrForbidden, got %v", err)
	}
}

func TestCreateUser_Success(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	view, err := svc.CreateUser(context.Background(), "admin", CreateUserCommand{
		Username: "newuser", DisplayName: "New", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if view.Username != "newuser" || view.Role != "user" || view.AuthSource != "password" {
		t.Errorf("got view=%+v", view)
	}
	// 密码 hash 落库。
	u, _ := repo.FindByUsername(context.Background(), "newuser")
	if u.PasswordHash == "" {
		t.Errorf("password_hash not set")
	}
	if argon2.Verify("secret123", u.PasswordHash) != nil {
		t.Errorf("hash verify failed")
	}
}

func TestCreateUser_AdminRole(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	role := "admin"
	view, err := svc.CreateUser(context.Background(), "admin", CreateUserCommand{
		Username: "admin2", Password: "secret123", Role: &role,
	})
	if err != nil || view.Role != "admin" {
		t.Errorf("create admin: got view=%+v err=%v", view, err)
	}
}

func TestCreateUser_Validation(t *testing.T) {
	svc := NewService(newFakeRepo())
	// 缺 username。
	_, err := svc.CreateUser(context.Background(), "admin", CreateUserCommand{Password: "secret123"})
	if err == nil {
		t.Errorf("missing username: want error")
	}
	// 短密码。
	_, err = svc.CreateUser(context.Background(), "admin", CreateUserCommand{Username: "x", Password: "123"})
	if err == nil {
		t.Errorf("short password: want error")
	}
	// 非法 role。
	bad := "superadmin"
	_, err = svc.CreateUser(context.Background(), "admin", CreateUserCommand{Username: "x", Password: "secret123", Role: &bad})
	if err == nil {
		t.Errorf("invalid role: want error")
	}
}

func TestCreateUser_UsernameTaken(t *testing.T) {
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "dup", Role: "user"})
	svc := NewService(repo)
	_, err := svc.CreateUser(context.Background(), "admin", CreateUserCommand{Username: "dup", Password: "secret123"})
	if !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("want ErrUsernameTaken, got %v", err)
	}
}

func TestDeleteUser_CannotDeleteSelf(t *testing.T) {
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", Role: "admin"})
	svc := NewService(repo, fakeDeleter{repo})
	err := svc.DeleteUser(context.Background(), "admin", "u1", "u1")
	if err == nil {
		t.Errorf("delete self: want error")
	}
}

func TestDeleteUser_Success(t *testing.T) {
	repo := newFakeRepo(
		domainauth.User{ID: "u1", Username: "alice", Role: "admin"},
		domainauth.User{ID: "u2", Username: "bob", Role: "user"},
	)
	svc := NewService(repo, fakeDeleter{repo})
	// u1 (admin) 删 u2（非自己）。
	if err := svc.DeleteUser(context.Background(), "admin", "u1", "u2"); err != nil {
		t.Errorf("delete: %v", err)
	}
	// u2 应保留且进入 deleted 终态。
	u2, err := repo.FindByID(context.Background(), "u2")
	if err != nil || u2.Status != domainauth.UserStatusDeleted {
		t.Errorf("after delete: got user=%+v err=%v", u2, err)
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, fakeDeleter{repo})
	err := svc.DeleteUser(context.Background(), "admin", "u1", "nope")
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("want ErrUserNotFound, got %v", err)
	}
}

func TestChangeUserPassword_AdminOnly(t *testing.T) {
	svc := NewService(newFakeRepo(domainauth.User{ID: "u1", Username: "alice", Role: "user", PasswordHash: "x"}))
	err := svc.ChangeUserPassword(context.Background(), "user", "u1", "newpass1")
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("non-admin: want ErrForbidden, got %v", err)
	}
}

func TestChangeUserPassword_Success(t *testing.T) {
	hash, _ := argon2.Hash("oldpass1")
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", Role: "user", PasswordHash: hash})
	svc := NewService(repo)
	if err := svc.ChangeUserPassword(context.Background(), "admin", "u1", "newpass1"); err != nil {
		t.Fatalf("ChangeUserPassword: %v", err)
	}
	if repo.password == "" {
		t.Errorf("password not set")
	}
}

func TestChangeUserPassword_LDAPRejected(t *testing.T) {
	// 无 password_hash（LDAP 账号）-> 不可改密。
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "ldapuser", Role: "user"})
	svc := NewService(repo)
	err := svc.ChangeUserPassword(context.Background(), "admin", "u1", "newpass1")
	if !errors.Is(err, ErrPasswordNotSet) {
		t.Errorf("LDAP user: want ErrPasswordNotSet, got %v", err)
	}
}

func TestChangeUserPassword_ShortPassword(t *testing.T) {
	hash, _ := argon2.Hash("oldpass1")
	repo := newFakeRepo(domainauth.User{ID: "u1", Username: "alice", Role: "user", PasswordHash: hash})
	svc := NewService(repo)
	err := svc.ChangeUserPassword(context.Background(), "admin", "u1", "123")
	if err == nil {
		t.Errorf("short password: want error")
	}
}
