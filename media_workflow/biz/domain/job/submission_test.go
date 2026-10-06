package job

import (
	"strings"
	"testing"
)

func TestSubmissionIdentityValidationAndScope(t *testing.T) {
	key := SubmissionKey{SubmittedBy: "user", OrganizationID: "org", WorkspaceID: "workspace", IdempotencyKey: "submit:one"}
	if key.Validate() != nil {
		t.Fatal("valid identity rejected")
	}
	for _, bad := range []string{"", "a b", "a\n", "中文", strings.Repeat("x", 161)} {
		copy := key
		copy.IdempotencyKey = bad
		if copy.Validate() == nil {
			t.Fatal("invalid key accepted")
		}
	}
	for _, other := range []SubmissionKey{
		{SubmittedBy: "other", OrganizationID: "org", WorkspaceID: "workspace", IdempotencyKey: "submit:one"},
		{SubmittedBy: "user", OrganizationID: "other", WorkspaceID: "workspace", IdempotencyKey: "submit:one"},
		{SubmittedBy: "user", OrganizationID: "org", WorkspaceID: "other", IdempotencyKey: "submit:one"},
		{SubmittedBy: "user", OrganizationID: "org", WorkspaceID: "workspace", IdempotencyKey: "other"},
	} {
		if other.LockID() == key.LockID() {
			t.Fatal("scope unexpectedly shared lock")
		}
	}
	receipt := Submission{SubmissionKey: key, JobID: "job", RequestDigest: strings.Repeat("a", 64)}
	if receipt.Validate() != nil {
		t.Fatal("valid receipt rejected")
	}
	receipt.RequestDigest = strings.Repeat("A", 64)
	if receipt.Validate() == nil {
		t.Fatal("noncanonical digest accepted")
	}
}
