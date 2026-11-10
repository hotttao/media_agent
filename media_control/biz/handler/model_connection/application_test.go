package model_connection

import (
	"context"
	"encoding/json"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"media_agent/hertz_infra/authn/appcredential"
	d "media_agent/media_control/biz/domain/model_connection"
	s "media_agent/media_control/biz/service/model_connection"
	"media_agent/media_control/biz/shared/secret"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type memoryConnections map[string]d.Connection

func (r memoryConnections) Get(_ context.Context, id string) (*d.Connection, error) {
	v, ok := r[id]
	if !ok {
		return nil, d.ErrNotFound
	}
	return &v, nil
}
func (r memoryConnections) List(_ context.Context, owner string) ([]d.Connection, error) {
	out := []d.Connection{}
	for _, v := range r {
		if v.OwnerUserID == owner {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r memoryConnections) Save(_ context.Context, v *d.Connection, expected int64) error {
	if r[v.ID].Version != expected {
		return d.ErrConflict
	}
	r[v.ID] = *v
	return nil
}
func TestApplicationConnectionOwnershipScopesAndRevocation(t *testing.T) {
	previous, authority := service, applicationAuthority
	defer func() { service, applicationAuthority = previous, authority }()
	owner := "app-user"
	allowed := true
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Scope string `json:"scope"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if !allowed {
			w.WriteHeader(403)
			return
		}
		json.NewEncoder(w).Encode(appcredential.Principal{ApplicationID: "app", UserID: owner, ExternalID: "external", TeamID: "team", OrganizationID: "org", Scopes: []string{req.Scope}, ExpiresAt: time.Now().Unix() + 60})
	}))
	defer auth.Close()
	repo := memoryConnections{}
	Configure(s.New(repo, secret.New("test")))
	ConfigureApplications(appcredential.New(auth.URL, "internal"))
	h := server.New()
	h.POST("/connections", CreateApplicationConnection)
	h.GET("/connections", ListApplicationConnections)
	h.PUT("/connections/:connection_id", UpdateApplicationConnection)
	h.POST("/connections/:connection_id/revoke", RevokeApplicationConnection)
	request := func(method, path, body string, want int) string {
		t.Helper()
		w := ut.PerformRequest(h.Engine, method, path, &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Authorization", Value: "Bearer mac_" + strings.Repeat("a", 64)}, ut.Header{Key: "Content-Type", Value: "application/json"})
		if w.Code != want {
			t.Fatalf("%s %s status %d: %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	raw := `{"tokens":{"access_token":"private-access","refresh_token":"private-refresh"}}`
	b, _ := json.Marshal(map[string]string{"name": "Codex", "auth_json": raw, "owner_user_id": "victim"})
	response := request("POST", "/connections", string(b), 201)
	if strings.Contains(response, "private-") {
		t.Fatal("credential leaked")
	}
	var v struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(response), &v)
	if repo[v.ID].OwnerUserID != "app-user" || strings.Contains(string(repo[v.ID].EncryptedAuth), "private-access") {
		t.Fatal("ownership or encryption failure")
	}
	owner = "other-app-user"
	var listed struct {
		Connections []json.RawMessage `json:"connections"`
	}
	json.Unmarshal([]byte(request("GET", "/connections", "", 200)), &listed)
	if len(listed.Connections) != 0 {
		t.Fatal("foreign connection listed")
	}
	request("PUT", "/connections/"+v.ID, `{"name":"Other","expected_version":1}`, 404)
	request("POST", "/connections/"+v.ID+"/revoke", `{"expected_version":1}`, 404)
	owner = "app-user"
	request("PUT", "/connections/"+v.ID, `{"name":"Renamed","expected_version":1}`, 200)
	request("POST", "/connections/"+v.ID+"/revoke", `{"expected_version":1}`, 409)
	allowed = false
	request("POST", "/connections/"+v.ID+"/revoke", `{"expected_version":2}`, 401)
	allowed = true
	request("POST", "/connections/"+v.ID+"/revoke", `{"expected_version":2}`, 200)
	if len(repo[v.ID].EncryptedAuth) != 0 {
		t.Fatal("revoked credential retained")
	}
	if _, err := service.Resolve(context.Background(), v.ID); err != d.ErrRevoked {
		t.Fatal("revoked credential resolved")
	}
}
