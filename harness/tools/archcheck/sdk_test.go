package main

import "testing"

func TestSDKBoundaries(t *testing.T) {
	const root = "media_agent/packages/platform-go-sdk/"
	for _, v := range []struct {
		from, to string
		bad      bool
	}{
		{"biz/shared/client", "media_agent/hertz_gen/model/media_auth/application", false},
		{"biz/workflow/application_access", "media_agent/hertz_gen/model/media_auth/application", true},
		{"biz/workflow/application_access", root + "biz/shared/client", true},
		{"biz/shared/credential", root + "biz/workflow/application_access", true},
		{"biz/shared/client", "media_agent/media_auth/biz/service/application", true},
		{"biz/workflow/application_access", root + "biz/shared/credential", false},
	} {
		if got := len(checkSDKRules(GoPackage{ImportPath: root + v.from}, v.to)) > 0; got != v.bad {
			t.Errorf("%s -> %s: %v", v.from, v.to, got)
		}
	}
}
