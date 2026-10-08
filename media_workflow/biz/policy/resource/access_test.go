package resource

import "testing"

func TestVisibilityAndDepartmentGrantPermissions(t *testing.T) {
	grants := []Grant{{TeamID: "dept", Permission: "viewer"}, {TeamID: "edit", Permission: "editor"}, {TeamID: "", Permission: "editor"}}
	for _, tc := range []struct {
		name, owner, visibility, caller string
		teams                           []string
		want                            Level
	}{
		{"owner", "owner", "private", "owner", nil, Owner},
		{"missing identity", "owner", "public", "", []string{"edit"}, None},
		{"reader", "owner", "team", "member", []string{"dept"}, Viewer},
		{"editor", "owner", "team", "member", []string{"dept", "edit"}, Editor},
		{"wrong department", "owner", "team", "member", []string{"other"}, None},
		{"private disables grants", "owner", "private", "member", []string{"edit"}, None},
		{"public reader", "owner", "public", "member", nil, Viewer},
		{"public editor", "owner", "public", "member", []string{"edit"}, Editor},
		{"empty department", "owner", "team", "member", []string{""}, None},
		{"unknown visibility", "owner", "unknown", "member", []string{"edit"}, None},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Effective(tc.owner, tc.visibility, tc.caller, tc.teams, grants); got != tc.want {
				t.Fatalf("permission=%v want=%v", got, tc.want)
			}
		})
	}
}
