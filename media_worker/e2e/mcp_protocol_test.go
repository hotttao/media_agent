package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/authn/mcpdelegation"
	"media_agent/hertz_infra/mcpupstream"
	handler "media_agent/media_worker/biz/handler/mcp"
	contract "media_agent/media_worker/biz/shared/media_management"
	flow "media_agent/media_worker/biz/workflow/media_management"
)

type mcpReceiptAPI struct{ calls atomic.Int32 }

func (a *mcpReceiptAPI) Execute(_ context.Context, operation string, p contract.Params) (map[string]any, error) {
	a.calls.Add(1)
	return map[string]any{"job_id": "job-" + p.IdempotencyKey, "status": "PENDING"}, nil
}

// Uses both real SDK transports, request-specific assertions, private-server
// admission and schema validation. No generation browser or business DB needed.
func TestMCPTransportBatchSubmission(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := mcpdelegation.NewSigner(key, "gateway-test", nil)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(signer.PublicJWKS()) }))
	defer jwks.Close()
	verifier := func(aud string) *internaljwt.Verifier {
		return internaljwt.NewVerifier(jwks.URL, "gateway-test", aud, nil)
	}
	api := &mcpReceiptAPI{}
	serverHandler, err := handler.New(flow.Management{API: api}, verifier(mcpdelegation.Backend), verifier(mcpdelegation.Workflow), verifier(mcpdelegation.Materials))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(serverHandler)
	defer server.Close()
	principal := mcpdelegation.Principal{Kind: "user", UserID: "u", ActorID: "User:u", CredentialID: "c", OrganizationID: "o", WorkspaceID: "w"}
	backend := mcpupstream.MCPBackend{Endpoint: server.URL, Transport: "streamable_http", RequestHeaders: func(ctx context.Context, method string, raw json.RawMessage) (map[string]string, error) {
		grant := mcpdelegation.Grant{Principal: principal, Audience: mcpdelegation.Backend, CallID: "call", SourceID: "media-workflow", Method: method, ExpiresAt: time.Now().Add(time.Minute)}
		if method == "tools/call" {
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			grant.Operation = p.Name
			grant.ArgumentsDigest, err = mcpdelegation.ArgumentsDigest(p.Arguments)
			if err != nil {
				return nil, err
			}
		}
		token, err := signer.Sign(grant)
		if err != nil {
			return nil, err
		}
		headers := map[string]string{"Authorization": "Bearer " + token}
		if method == "tools/call" {
			grant.Audience = mcpdelegation.Workflow
			grant.Presenter = mcpdelegation.Backend
			grant.Method = ""
			grant.ArgumentsDigest = ""
			headers[mcpdelegation.WorkflowHeader], err = signer.Sign(grant)
		}
		return headers, err
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	peer, err := (mcpupstream.MCPUpstream{}).Open(ctx, backend)
	if err != nil {
		t.Fatal("SDK connection", err)
	}
	defer peer.Close()
	listed, err := peer.Call(ctx, "tools/list", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var tools struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if json.Unmarshal(listed, &tools) != nil || len(tools.Tools) != 13 {
		t.Fatalf("bad discovery: %s", listed)
	}
	raw, err := peer.Call(ctx, "tools/call", []byte(`{"name":"jobs_submit","arguments":{"items":[{"item_id":"first","params":{"workflow_id":"wf","workflow_version":1,"idempotency_key":"first","input":{}}},{"item_id":"second","params":{"workflow_id":"wf","workflow_version":1,"idempotency_key":"second","input":{}}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Structured struct {
			Results []struct {
				Data struct {
					ID string `json:"job_id"`
				} `json:"data"`
			} `json:"results"`
		} `json:"structuredContent"`
	}
	if json.Unmarshal(raw, &receipt) != nil || len(receipt.Structured.Results) != 2 || receipt.Structured.Results[0].Data.ID != "job-first" || receipt.Structured.Results[1].Data.ID != "job-second" || api.calls.Load() != 2 {
		t.Fatalf("invalid async batch receipt: %s", raw)
	}
	_, err = peer.Call(ctx, "tools/call", []byte(`{"name":"jobs_submit","arguments":{"items":[]}}`))
	var rejected *mcpupstream.MCPRemoteError
	if !errors.As(err, &rejected) || rejected.Code != -32602 || api.calls.Load() != 2 {
		t.Fatalf("invalid batch must be rejected before dispatch: %v, calls=%d", err, api.calls.Load())
	}

}
