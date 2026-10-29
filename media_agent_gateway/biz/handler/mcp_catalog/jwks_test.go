package mcp_catalog_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"media_agent/hertz_infra/authn/mcpdelegation"
	handler "media_agent/media_agent_gateway/biz/handler/mcp_catalog"
	router "media_agent/media_agent_gateway/biz/router/mcp_catalog"
	"testing"
)

func TestPublicVerificationKeyRouteDoesNotRequireServiceCredentials(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := mcpdelegation.NewSigner(key, "issuer", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.ConfigureDelegationSigner(signer)
	defer handler.ConfigureDelegationSigner(nil)
	h := server.New()
	router.Register(h)
	response := ut.PerformRequest(h.Engine, "GET", "/.well-known/mcp-jwks.json", nil)
	if response.Code != 200 {
		t.Fatalf("JWKS inaccessible to verifier: %d", response.Code)
	}
	var document struct {
		Keys []struct {
			Kid string `json:"kid"`
			D   string `json:"d"`
		} `json:"keys"`
	}
	if json.Unmarshal(response.Body.Bytes(), &document) != nil || len(document.Keys) != 1 || document.Keys[0].Kid != signer.PublicJWKS().Keys[0].Kid || document.Keys[0].D != "" {
		t.Fatal("invalid public key document")
	}
}
