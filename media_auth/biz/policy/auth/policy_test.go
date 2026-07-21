// policy_test.go：auth 领域纯决策规则单测（不查 DB）。
//
// 覆盖：IsValidRole、ValidateCreateUser、CanDeleteUser。
package auth

import (
	"errors"
	"testing"
)

func TestIsValidRole(t *testing.T) {
	cases := []struct {
		role string
		want bool
	}{
		{"user", true}, {"admin", true}, {"system", false}, {"", false}, {"superadmin", false},
	}
	for _, c := range cases {
		if got := IsValidRole(c.role); got != c.want {
			t.Errorf("IsValidRole(%q) = %v, want %v", c.role, got, c.want)
		}
	}
}

func TestValidateCreateUser(t *testing.T) {
	if err := ValidateCreateUser("", "secret123"); !errors.Is(err, ErrUsernameRequired) {
		t.Errorf("empty username: want ErrUsernameRequired, got %v", err)
	}
	if err := ValidateCreateUser("alice", "123"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("short password: want ErrPasswordTooShort, got %v", err)
	}
	if err := ValidateCreateUser("alice", "secret123"); err != nil {
		t.Errorf("valid: want nil, got %v", err)
	}
}

func TestCanDeleteUser(t *testing.T) {
	if err := CanDeleteUser("u1", "u1"); !errors.Is(err, ErrCannotDeleteSelf) {
		t.Errorf("delete self: want ErrCannotDeleteSelf, got %v", err)
	}
	if err := CanDeleteUser("u2", "u1"); err != nil {
		t.Errorf("delete other: want nil, got %v", err)
	}
}
