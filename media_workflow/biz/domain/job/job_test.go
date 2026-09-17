package job

import "testing"

func TestNeedsApproval(t *testing.T) {
	cases := []struct {
		name     string
		policy   ApprovalPolicy
		count    int
		approval bool
	}{
		{"no candidates", ApprovalAlways, 0, false},
		{"one default candidate", ApprovalWhenMultipleCandidates, 1, false},
		{"one candidate always", ApprovalAlways, 1, true},
		{"multiple default candidates", ApprovalWhenMultipleCandidates, 2, true},
		{"multiple always candidates", ApprovalAlways, 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NeedsApproval(tc.policy, tc.count); got != tc.approval {
				t.Fatalf("NeedsApproval(%q, %d) = %v, want %v", tc.policy, tc.count, got, tc.approval)
			}
		})
	}
}
