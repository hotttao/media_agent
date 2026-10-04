package platformmtls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

type certificateAuthority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newCA(t *testing.T, now time.Time) certificateAuthority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("key generation failed")
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "isolated test CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal("CA generation failed")
	}
	cert, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal("CA parse failed")
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return certificateAuthority{cert, key, pool}
}

func (ca certificateAuthority) issue(t *testing.T, now time.Time, names []string, usage x509.ExtKeyUsage) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("key generation failed")
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "media_agent_gateway"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Minute), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}, DNSNames: []string{"localhost"}}
	for _, name := range names {
		u, err := url.Parse(name)
		if err != nil {
			t.Fatal("fixture URI invalid")
		}
		template.URIs = append(template.URIs, u)
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal("certificate generation failed")
	}
	return tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key}
}

const gatewayURI = "spiffe://media-agent/services/media_agent_gateway"
const scheduleURI = "spiffe://media-agent/services/media_control_schedule_worker"

func TestRealTLSOnlyAllowedClientCertificateReachesHandler(t *testing.T) {
	now := time.Now().UTC()
	ca, foreignCA := newCA(t, now), newCA(t, now)
	v, err := NewVerifier(ca.pool, map[string]string{gatewayURI: "media_agent_gateway", scheduleURI: "media_control_schedule_worker"}, nil)
	if err != nil {
		t.Fatal("verifier initialization failed")
	}
	serverConfig, err := v.TLSConfig(ca.issue(t, now, nil, x509.ExtKeyUsageServerAuth))
	if err != nil {
		t.Fatal("server TLS initialization failed")
	}
	var reached atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		ctx, err := v.Bind(r.Context(), r.TLS)
		if err != nil {
			http.Error(w, "denied", 403)
			return
		}
		caller, err := v.VerifyPlatformCaller(ctx)
		if err != nil {
			http.Error(w, "denied", 403)
			return
		}
		_, _ = io.WriteString(w, caller)
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.TLS = serverConfig
	srv.StartTLS()
	defer srv.Close()
	for _, tc := range []struct {
		name     string
		cert     *tls.Certificate
		expected string
		tls12    bool
	}{
		{"gateway", ptr(ca.issue(t, now, []string{gatewayURI}, x509.ExtKeyUsageClientAuth)), "media_agent_gateway", false},
		{"schedule", ptr(ca.issue(t, now, []string{scheduleURI}, x509.ExtKeyUsageClientAuth)), "media_control_schedule_worker", false},
		{"no certificate", nil, "", false},
		{"foreign CA", ptr(foreignCA.issue(t, now, []string{gatewayURI}, x509.ExtKeyUsageClientAuth)), "", false},
		{"common name only", ptr(ca.issue(t, now, nil, x509.ExtKeyUsageClientAuth)), "", false},
		{"unknown service", ptr(ca.issue(t, now, []string{"spiffe://media-agent/services/personal_worker"}, x509.ExtKeyUsageClientAuth)), "", false},
		{"multiple identities", ptr(ca.issue(t, now, []string{gatewayURI, scheduleURI}, x509.ExtKeyUsageClientAuth)), "", false},
		{"server usage only", ptr(ca.issue(t, now, []string{gatewayURI}, x509.ExtKeyUsageServerAuth)), "", false},
		{"unrestricted usage", ptr(ca.issue(t, now, []string{gatewayURI}, x509.ExtKeyUsageAny)), "", false},
		{"expired", ptr(ca.issue(t, now.Add(-2*time.Minute), []string{gatewayURI}, x509.ExtKeyUsageClientAuth)), "", false},
		{"old TLS", ptr(ca.issue(t, now, []string{gatewayURI}, x509.ExtKeyUsageClientAuth)), "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &tls.Config{RootCAs: ca.pool, ServerName: "localhost", MinVersion: tls.VersionTLS13}
			if tc.cert != nil {
				config.Certificates = []tls.Certificate{*tc.cert}
			}
			if tc.tls12 {
				config.MinVersion = tls.VersionTLS12
				config.MaxVersion = tls.VersionTLS12
			}
			transport := &http.Transport{TLSClientConfig: config}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			before := reached.Load()
			req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
			req.Header.Set("X-Caller-Service", "media_agent_gateway")
			req.Header.Set("X-Forwarded-Client-Cert", gatewayURI)
			resp, err := client.Do(req)
			if tc.expected == "" {
				if resp != nil {
					resp.Body.Close()
				}
				if err == nil || reached.Load() != before {
					t.Fatal("unauthorized peer reached handler")
				}
				return
			}
			if err != nil {
				t.Fatal("authorized peer rejected")
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != 200 || string(body) != tc.expected {
				t.Fatal("wrong authenticated identity")
			}
		})
	}
}

func ptr(c tls.Certificate) *tls.Certificate { return &c }

func TestCallerEvidenceIsBoundToVerifierAndRechecksExpiry(t *testing.T) {
	now := time.Now().UTC()
	ca := newCA(t, now)
	var clock atomic.Int64
	clock.Store(now.UnixNano())
	v, _ := NewVerifier(ca.pool, map[string]string{gatewayURI: "media_agent_gateway"}, func() time.Time { return time.Unix(0, clock.Load()) })
	cert := ca.issue(t, now, []string{gatewayURI}, x509.ExtKeyUsageClientAuth)
	leaf, _ := x509.ParseCertificate(cert.Certificate[0])
	state := &tls.ConnectionState{HandshakeComplete: true, Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf, ca.cert}}}
	ctx, err := v.Bind(context.Background(), state)
	if err != nil {
		t.Fatal("valid state rejected")
	}
	if caller, err := v.VerifyPlatformCaller(ctx); err != nil || caller != "media_agent_gateway" {
		t.Fatal("valid evidence rejected")
	}
	other, _ := NewVerifier(ca.pool, map[string]string{gatewayURI: "media_agent_gateway"}, nil)
	if _, err := other.VerifyPlatformCaller(ctx); err == nil {
		t.Fatal("evidence crossed verifier boundary")
	}
	// Mutating the source certificate cannot change the captured DER evidence.
	leaf.URIs = nil
	if _, err := v.VerifyPlatformCaller(ctx); err != nil {
		t.Fatal("source mutation changed evidence")
	}
	clock.Store(now.Add(2 * time.Minute).UnixNano())
	if _, err := v.VerifyPlatformCaller(ctx); err == nil {
		t.Fatal("expired keep-alive evidence accepted")
	}
	if _, err := v.VerifyPlatformCaller(context.Background()); err == nil {
		t.Fatal("missing evidence accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := v.VerifyPlatformCaller(cancelled); err == nil {
		t.Fatal("cancelled evidence accepted")
	}
	state.HandshakeComplete = false
	if _, err := v.Bind(context.Background(), state); err == nil {
		t.Fatal("incomplete handshake accepted")
	}
}

func TestInvalidCallerMappingRejected(t *testing.T) {
	ca := newCA(t, time.Now())
	for _, identity := range []string{"", "https://media-agent/services/media_agent_gateway", gatewayURI + "?caller=x", gatewayURI + "#x", "spiffe://media-agent/services/%6dedia_agent_gateway", "spiffe://other/services/media_agent_gateway"} {
		if _, err := NewVerifier(ca.pool, map[string]string{identity: "media_agent_gateway"}, nil); err == nil {
			t.Fatal("noncanonical identity accepted")
		}
	}
	if _, err := NewVerifier(nil, map[string]string{gatewayURI: "media_agent_gateway"}, nil); err == nil {
		t.Fatal("missing CA accepted")
	}
}

func TestUnverifiedStateAndLaterTrustMutationCannotAuthorizeCaller(t *testing.T) {
	now := time.Now().UTC()
	ca, foreign := newCA(t, now), newCA(t, now)
	callers := map[string]string{gatewayURI: "media_agent_gateway"}
	v, err := NewVerifier(ca.pool, callers, nil)
	if err != nil {
		t.Fatal("verifier initialization failed")
	}
	// Caller-owned configuration changes cannot expand an existing verifier.
	ca.pool.AddCert(foreign.cert)
	callers[scheduleURI] = "media_control_schedule_worker"
	for _, tc := range []struct {
		cert     tls.Certificate
		verified bool
	}{
		{ca.issue(t, now, []string{gatewayURI}, x509.ExtKeyUsageClientAuth), false},
		{foreign.issue(t, now, []string{gatewayURI}, x509.ExtKeyUsageClientAuth), true},
		{ca.issue(t, now, []string{scheduleURI}, x509.ExtKeyUsageClientAuth), true},
	} {
		leaf, err := x509.ParseCertificate(tc.cert.Certificate[0])
		if err != nil {
			t.Fatal("fixture parse failed")
		}
		state := &tls.ConnectionState{Version: tls.VersionTLS13, HandshakeComplete: true, PeerCertificates: []*x509.Certificate{leaf}}
		if tc.verified {
			state.VerifiedChains = [][]*x509.Certificate{{leaf, ca.cert}}
		}
		if _, err := v.Bind(context.Background(), state); err == nil {
			t.Fatal("untrusted state or changed configuration authorized")
		}
	}
}
