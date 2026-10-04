package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	configpb "media_agent/hertz_gen/config"
	platformhandler "media_agent/media_auth/biz/handler/platform_token"
	platformrouter "media_agent/media_auth/biz/router/platform_token"
	platformservice "media_agent/media_auth/biz/service/platform_token"
	"media_agent/media_auth/biz/shared/platformmtls"
)

type platformHTTPFake struct {
	mu       sync.Mutex
	calls    int
	verifier *platformmtls.Verifier
}

func (f *platformHTTPFake) check(ctx context.Context, purpose string) error {
	if _, err := f.verifier.VerifyPlatformCaller(ctx); err != nil {
		return platformservice.ErrDenied
	}
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if purpose == "failure" {
		return errors.New("private diagnostic must not be returned")
	}
	if purpose != "offline-task" {
		return platformservice.ErrDenied
	}
	return nil
}
func (f *platformHTTPFake) AcquirePlatformToken(ctx context.Context, user, purpose string) (string, error) {
	if err := f.check(ctx, purpose); err != nil {
		return "", err
	}
	return "fixture-secret", nil
}
func (f *platformHTTPFake) IntrospectPlatformToken(ctx context.Context, token, purpose string) (platformservice.Principal, error) {
	if err := f.check(ctx, purpose); err != nil {
		return platformservice.Principal{}, err
	}
	return platformservice.Principal{Active: token == "fixture-secret", UserID: "fixture-user", CredentialID: "fixture-credential"}, nil
}
func (f *platformHTTPFake) RotatePlatformToken(ctx context.Context, user, purpose string, expiry *time.Time) (string, error) {
	if err := f.check(ctx, purpose); err != nil {
		return "", err
	}
	return "fixture-rotated", nil
}

func platformTLSFiles(t *testing.T) (string, string, string, *x509.CertPool, tls.Certificate) {
	t.Helper()
	now := time.Now()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("fixture key failed")
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	caRaw, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &key.PublicKey, key)
	if err != nil {
		t.Fatal("fixture CA failed")
	}
	ca, _ := x509.ParseCertificate(caRaw)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	issue := func(usage x509.ExtKeyUsage, client bool) tls.Certificate {
		leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal("fixture key failed")
		}
		tpl := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}, DNSNames: []string{"localhost"}}
		if client {
			identity, _ := url.Parse("spiffe://media-agent/services/media_agent_gateway")
			tpl.URIs = append(tpl.URIs, identity)
		}
		raw, err := x509.CreateCertificate(rand.Reader, tpl, ca, &leafKey.PublicKey, key)
		if err != nil {
			t.Fatal("fixture cert failed")
		}
		return tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: leafKey}
	}
	serverCert, clientCert := issue(x509.ExtKeyUsageServerAuth, false), issue(x509.ExtKeyUsageClientAuth, true)
	dir := t.TempDir()
	certPath, keyPath, caPath := filepath.Join(dir, "server.pem"), filepath.Join(dir, "server-key.pem"), filepath.Join(dir, "ca.pem")
	serverKey, err := x509.MarshalPKCS8PrivateKey(serverCert.PrivateKey)
	if err != nil {
		t.Fatal("fixture encode failed")
	}
	for path, data := range map[string][]byte{certPath: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Certificate[0]}), keyPath: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: serverKey}), caPath: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caRaw})} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal("fixture file failed")
		}
	}
	return certPath, keyPath, caPath, pool, clientCert
}

func TestPlatformDedicatedHertzTLSHTTPBoundary(t *testing.T) {
	// No request bodies, certificate diagnostics or credentials in test output.
	hlog.SetOutput(io.Discard)
	certPath, keyPath, caPath, roots, clientCert := platformTLSFiles(t)
	v, tlsConfig, err := platformmtls.LoadFiles(certPath, keyPath, caPath, map[string]string{"spiffe://media-agent/services/media_agent_gateway": "media_agent_gateway"})
	if err != nil {
		t.Fatal("TLS files rejected")
	}
	fake := &platformHTTPFake{verifier: v}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("test listener failed")
	}
	h := newPlatformServer(listener, tlsConfig, fake, v)
	done := make(chan error, 1)
	go func() { done <- h.Run() }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = h.Shutdown(ctx)
		_ = listener.Close()
		select {
		case <-done:
		case <-ctx.Done():
			t.Error("listener did not stop")
		}
		platformhandler.PlatformSvc, platformhandler.PlatformVerifier, platformrouter.CallerVerifier = nil, nil, nil
	}()
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{clientCert}}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	base := "https://" + listener.Addr().String()
	for _, tc := range []struct {
		path, body string
		status     int
		expected   string
	}{
		{"acquire", `{"user_id":"fixture-user","purpose":"offline-task"}`, 200, `"token":"fixture-secret"`},
		{"introspect", `{"token":"fixture-secret","purpose":"offline-task"}`, 200, `"active":true`},
		{"rotate", `{"user_id":"fixture-user","purpose":"offline-task"}`, 200, `"token":"fixture-rotated"`},
		{"rotate", `{"user_id":"fixture-user","purpose":"offline-task","expires_at":-1}`, 400, "invalid platform token request"},
		{"acquire", `{"purpose":`, 400, "invalid platform token request"},
		{"acquire", `{"user_id":"fixture-user","purpose":"not-allowed"}`, 403, "platform token access denied"},
		{"acquire", `{"user_id":"fixture-user","purpose":"failure"}`, 503, "platform token service unavailable"},
	} {
		req, _ := http.NewRequest(http.MethodPost, base+"/internal/platform-token/"+tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal("TLS request failed")
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != tc.status || !strings.Contains(string(body), tc.expected) || strings.Contains(string(body), "private diagnostic") || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("wrong status, response or cache/error boundary")
		}
	}
	// The listener deliberately does not expose public auth or operational routes.
	resp, err := client.Get(base + "/api/auth/me")
	if err != nil {
		t.Fatal("route exclusion request failed")
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("public route exposed by internal listener")
	}
	// Generated routes accidentally mounted on public plaintext still deny.
	plainListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("plaintext fixture listener failed")
	}
	plain := server.New(server.WithListener(plainListener), server.WithDisablePrintRoute(true))
	platformrouter.Register(plain)
	plainDone := make(chan error, 1)
	go func() { plainDone <- plain.Run() }()
	fake.mu.Lock()
	before := fake.calls
	fake.mu.Unlock()
	request, _ := http.NewRequest(http.MethodPost, "http://"+plainListener.Addr().String()+"/internal/platform-token/acquire", strings.NewReader(`{"user_id":"fixture-user","purpose":"offline-task"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Caller-Service", "media_agent_gateway")
	request.Header.Set("X-Forwarded-Client-Cert", "spiffe://media-agent/services/media_agent_gateway")
	plainClient := &http.Client{Timeout: 3 * time.Second}
	plainResponse, plainErr := plainClient.Do(request)
	if plainResponse != nil {
		plainResponse.Body.Close()
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = plain.Shutdown(shutdownCtx)
	_ = plainListener.Close()
	select {
	case <-plainDone:
	case <-shutdownCtx.Done():
		t.Error("plaintext listener did not stop")
	}
	shutdownCancel()
	fake.mu.Lock()
	after := fake.calls
	fake.mu.Unlock()
	if plainErr != nil || plainResponse.StatusCode != 403 || plainResponse.Header.Get("Cache-Control") != "no-store" || before != after {
		t.Fatal("plaintext forwarded identity reached use case")
	}
	unauthTransport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS13}}
	defer unauthTransport.CloseIdleConnections()
	unauth := &http.Client{Transport: unauthTransport, Timeout: 3 * time.Second}
	if resp, err := unauth.Post(base+"/internal/platform-token/acquire", "application/json", strings.NewReader(`{"purpose":"offline-task"}`)); err == nil {
		resp.Body.Close()
		t.Fatal("missing client certificate admitted")
	}
}

func TestPlatformRuntimeDisabledAndMissingPrerequisites(t *testing.T) {
	for _, cfg := range []*configpb.PlatformTokenConfig{nil, {}, {Enabled: false, KeyFile: "/does-not-exist"}} {
		if runtime, err := newPlatformRuntime(nil, cfg); err != nil || runtime != nil {
			t.Fatal("disabled runtime touched prerequisites")
		}
	}
	if runtime, err := newPlatformRuntime(nil, &configpb.PlatformTokenConfig{Enabled: true}); err == nil || runtime != nil {
		t.Fatal("unprepared enabled runtime accepted")
	}
}
