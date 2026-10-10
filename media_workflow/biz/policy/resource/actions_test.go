package resource

import (
	"reflect"
	"testing"
)

func TestActions(t *testing.T) {
	for _, tc := range []struct {
		name                                     string
		level                                    Level
		archived, authoring, shareable, runnable bool
		permission                               string
		actions                                  []string
	}{
		{"none", None, false, true, true, true, "", []string{}},
		{"viewer", Viewer, false, true, true, true, "viewer", []string{"read", "run"}},
		{"editor", Editor, false, true, true, true, "editor", []string{"read", "update", "archive", "run"}},
		{"owner", Owner, false, true, true, true, "owner", []string{"read", "update", "archive", "publish", "change_visibility", "grant", "revoke", "run"}},
		{"revoked feature", Owner, false, false, true, true, "owner", []string{"read", "archive", "change_visibility", "grant", "revoke"}},
		{"archived", Owner, true, true, true, true, "owner", []string{"read", "change_visibility", "grant", "revoke"}},
		{"unbound", Owner, false, true, false, false, "owner", []string{"read", "update", "archive", "publish", "revoke"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Actions(tc.level, tc.archived, tc.authoring, tc.shareable, tc.runnable)
			if got.MyPermission != tc.permission || !reflect.DeepEqual(got.AllowedActions, tc.actions) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
