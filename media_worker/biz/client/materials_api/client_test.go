package materials_api

import (
	"context"
	"encoding/json"
	"media_agent/hertz_infra/authn/mcpdelegation"
	"media_agent/media_worker/biz/client/business_http"
	c "media_agent/media_worker/biz/shared/media_management"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueriesUseBoundOrganizationAndMaterialsCredential(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer material-grant" || r.Header.Get(mcpdelegation.ServiceHeader) != strings.Repeat("s", 32) || r.URL.Query().Get("organization_id") != "bound-org" {
			t.Error("query identity was not bound")
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "total": 0})
	}))
	defer server.Close()
	h, err := business_http.New(server.URL, strings.Repeat("s", 32), mcpdelegation.Materials)
	if err != nil {
		t.Fatal(err)
	}
	ctx := mcpdelegation.WithPrincipal(context.Background(), mcpdelegation.Principal{OrganizationID: "bound-org"})
	ctx = c.WithCredentials(ctx, c.Credentials{Materials: "material-grant", Workflow: "must-not-forward"})
	api := Client{HTTP: h}
	for _, name := range []string{"virtual_ips_list", "virtual_ips_get", "materials_list", "materials_get", "generated_assets_list", "generated_assets_get"} {
		if _, err = api.Execute(ctx, name, c.Params{ID: "item", Page: 1, PageSize: 20}); err != nil {
			t.Fatal(name, err)
		}
	}
	if calls != 6 {
		t.Fatal(calls)
	}
	if _, err = api.Execute(ctx, "jobs_submit", c.Params{}); err == nil || calls != 6 {
		t.Fatal("unknown operation sent")
	}
}
