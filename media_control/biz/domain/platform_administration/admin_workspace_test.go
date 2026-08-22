package platform_administration

import "testing"

func TestAdminWorkspaceFilterDefaultsAndValidation(t *testing.T) {
	filter, err := ValidateAdminWorkspaceFilter(" owner-1 ", "running", "claude-code", 0, 0)
	if err != nil || filter.OwnerUserID != "owner-1" || filter.PageSize != 50 {
		t.Fatalf("filter=%+v err=%v", filter, err)
	}
	for _, test := range []struct {
		state, agent string
		size, offset int32
	}{
		{state: "paused"},
		{agent: "unknown"},
		{size: 201},
		{size: 10, offset: -1},
	} {
		if _, err := ValidateAdminWorkspaceFilter("", test.state, test.agent, test.size, test.offset); err == nil {
			t.Fatalf("accepted invalid filter: %+v", test)
		}
	}
}
