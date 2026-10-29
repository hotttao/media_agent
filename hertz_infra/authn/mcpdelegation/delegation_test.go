package mcpdelegation

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"media_agent/hertz_infra/authn/internaljwt"
)

func TestAudienceAndInvocationIsolation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner(key, "urn:test:mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(signer.PublicJWKS()) }))
	defer server.Close()
	principal := Principal{Kind: "user", UserID: "u1", ActorID: "User:u1", CredentialID: "c1", OrganizationID: "o1"}
	digest, _ := ArgumentsDigest([]byte(`{"items":[{"item_id":"a","params":{}}]}`))
	backend := Grant{Principal: principal, Audience: Backend, CallID: "call1", SourceID: "media-workflow", Method: "tools/call", Operation: "jobs_submit", ArgumentsDigest: digest}
	raw, err := signer.Sign(backend)
	if err != nil {
		t.Fatal(err)
	}
	v := internaljwt.NewVerifier(server.URL, "urn:test:mcp", Backend, server.Client())
	verified, err := Verify(context.Background(), v, raw, Backend)
	if err != nil {
		t.Fatal(err)
	}
	api := backend
	api.Audience = Workflow
	api.Method = ""
	api.ArgumentsDigest = ""
	api.Presenter = Backend
	api.ExpiresAt = verified.ExpiresAt
	apiRaw, err := signer.Sign(api)
	if err != nil {
		t.Fatal(err)
	}
	target := internaljwt.NewVerifier(server.URL, "urn:test:mcp", Workflow, server.Client())
	grant, err := Verify(context.Background(), target, apiRaw, Workflow)
	if err != nil || !SameInvocation(verified, grant) {
		t.Fatalf("valid pair rejected: %v", err)
	}
	if _, err = Verify(context.Background(), target, raw, Workflow); err == nil {
		t.Fatal("backend credential accepted at business API")
	}
	if _, err = Verify(context.Background(), v, apiRaw, Backend); err == nil {
		t.Fatal("business credential accepted as backend request")
	}
	grant.UserID = "other"
	if SameInvocation(verified, grant) {
		t.Fatal("mixed identity accepted")
	}
	for _, mutate := range []func(*Grant){func(g *Grant) { g.ActorID = "Worker:w1" }, func(g *Grant) { g.Kind = "managed_agent" }, func(g *Grant) { g.Audience = "internal-api" }, func(g *Grant) { g.ExpiresAt = time.Now().Add(-time.Second) }, func(g *Grant) { g.OrganizationID = "" }} {
		bad := backend
		mutate(&bad)
		if _, err = signer.Sign(bad); err == nil {
			t.Fatal("invalid grant signed")
		}
	}
}
func TestDigestCoversWholeBatchAndRejectsAmbiguity(t *testing.T) {
	a, err := ArgumentsDigest([]byte(`{"items":[{"item_id":"a","params":{"x":1,"y":2}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ArgumentsDigest([]byte(`{"items":[{"params":{"y":2,"x":1},"item_id":"a"}]}`))
	if a != b {
		t.Fatal("key order changed digest")
	}
	c, _ := ArgumentsDigest([]byte(`{"items":[{"item_id":"b","params":{"x":1,"y":2}}]}`))
	if a == c {
		t.Fatal("item identity not bound")
	}
	for _, raw := range []string{`{"items":[],"items":[]}`, `{"items":[{"params":{"x":1,"x":2}}]}`, `{} {}`, `[]`, `null`} {
		if _, err := ArgumentsDigest([]byte(raw)); err == nil {
			t.Fatalf("ambiguous input accepted: %s", raw)
		}
	}
}
