package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMediaControlEnvironmentTokenE2E(t *testing.T) {
	authAddress, controlAddress := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddress == "" || controlAddress == "" {
		t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required")
	}
	client := registerGatewayUser(t, authAddress, "environment-token")

	status, body := doJSON(t, client, http.MethodPost, controlAddress+"/api/v1/runtime-environments", map[string]string{
		"name":          fmt.Sprintf("token-env-%d", time.Now().UnixNano()),
		"endpoint":      "http://runtime-controller",
		"capacity_json": "{}",
	})
	if status != http.StatusCreated {
		t.Fatalf("create environment: %d %s", status, body)
	}
	var environment struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &environment); err != nil || environment.ID == "" {
		t.Fatalf("decode environment: %+v err=%v", environment, err)
	}

	type issuedResponse struct {
		Metadata struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"metadata"`
		Token string `json:"token"`
	}
	issue := func(name string) issuedResponse {
		status, body := doJSON(t, client, http.MethodPost, controlAddress+"/api/v1/runtime-environments/"+environment.ID+"/tokens", map[string]string{"name": name})
		if status != http.StatusCreated {
			t.Fatalf("issue %s: %d %s", name, status, body)
		}
		var response issuedResponse
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		if response.Metadata.ID == "" || response.Metadata.Name != name || len(response.Token) != 68 || !strings.HasPrefix(response.Token, "env_") {
			t.Fatalf("unexpected issued token: %+v", response)
		}
		return response
	}
	oldToken, newToken := issue("old-runner"), issue("new-runner")
	if oldToken.Token == newToken.Token {
		t.Fatal("two issued tokens must differ")
	}

	status, body = doJSON(t, client, http.MethodGet, controlAddress+"/api/v1/runtime-environments/"+environment.ID+"/tokens", nil)
	if status != http.StatusOK || strings.Contains(string(body), oldToken.Token) || strings.Contains(string(body), newToken.Token) || strings.Contains(string(body), "token_hash") {
		t.Fatalf("unsafe token listing: %d %s", status, body)
	}
	var listed struct {
		Tokens []struct {
			ID string `json:"id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(body, &listed); err != nil || len(listed.Tokens) != 2 {
		t.Fatalf("rotation overlap not listed: %+v err=%v", listed, err)
	}

	revokeURL := controlAddress + "/api/v1/runtime-environments/" + environment.ID + "/tokens/" + oldToken.Metadata.ID + "/revoke"
	for attempt := 0; attempt < 2; attempt++ {
		status, body = doJSON(t, client, http.MethodPost, revokeURL, map[string]any{})
		if status != http.StatusOK || !jsonContains(body, `"revoked_at":`) {
			t.Fatalf("revoke attempt %d: %d %s", attempt+1, status, body)
		}
	}
	status, body = doJSON(t, client, http.MethodGet, controlAddress+"/api/v1/runtime-environments/"+environment.ID+"/tokens", nil)
	if status != http.StatusOK || strings.Contains(string(body), oldToken.Metadata.ID) || !strings.Contains(string(body), newToken.Metadata.ID) {
		t.Fatalf("active token list after rotation: %d %s", status, body)
	}
}
