// Package platformmtls authenticates internal callers from the TLS connection,
// never from request headers. URI names are local certificate conventions, not
// an implementation of SPIFFE workload attestation or federation.
package platformmtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/url"
	"strings"
	"time"
)

var ErrDenied = errors.New("platform caller authentication failed")

type Verifier struct {
	roots   *x509.CertPool
	callers map[string]string
	now     func() time.Time
}

type evidence struct {
	leaf          []byte
	intermediates [][]byte
}

// Each verifier instance is its own context key/trust boundary.
type evidenceKey struct{ verifier *Verifier }

func NewVerifier(roots *x509.CertPool, callers map[string]string, now func() time.Time) (*Verifier, error) {
	if roots == nil || len(roots.Subjects()) == 0 || len(callers) == 0 {
		return nil, ErrDenied
	}
	v := &Verifier{roots: roots.Clone(), callers: make(map[string]string), now: now}
	if v.now == nil {
		v.now = time.Now
	}
	for identity, caller := range callers {
		u, err := url.Parse(identity)
		if err != nil || u.Scheme != "spiffe" || u.Host != "media-agent" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.Opaque != "" || !validName(caller) || u.Path != "/services/"+caller || u.String() != identity {
			return nil, ErrDenied
		}
		v.callers[identity] = caller
	}
	return v, nil
}

func validName(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// TLSConfig is for the dedicated internal listener only. Its CA is independent
// of public Web/Agent credentials. Unknown identities fail during handshake.
func (v *Verifier) TLSConfig(server tls.Certificate) (*tls.Config, error) {
	if v == nil || server.PrivateKey == nil || len(server.Certificate) == 0 {
		return nil, ErrDenied
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{server}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: v.roots.Clone(), Time: v.now, SessionTicketsDisabled: true,
		VerifyConnection: func(state tls.ConnectionState) error { _, err := v.fromState(&state); return err },
	}, nil
}

func (v *Verifier) fromState(state *tls.ConnectionState) (evidence, error) {
	// VerifyConnection runs just before HandshakeComplete becomes true; the
	// request binding below separately requires a completed handshake.
	if v == nil || state == nil || state.Version < tls.VersionTLS13 || len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return evidence{}, ErrDenied
	}
	leaf := state.PeerCertificates[0]
	if leaf == nil || len(leaf.Raw) == 0 {
		return evidence{}, ErrDenied
	}
	e := evidence{leaf: append([]byte(nil), leaf.Raw...)}
	for _, cert := range state.PeerCertificates[1:] {
		if cert == nil {
			return evidence{}, ErrDenied
		}
		e.intermediates = append(e.intermediates, append([]byte(nil), cert.Raw...))
	}
	if _, err := v.verify(e); err != nil {
		return evidence{}, ErrDenied
	}
	return e, nil
}

// Bind must receive the server-side connection state, e.g. Request.TLS, not
// forwarded metadata. It snapshots DER and verifies against this instance's CA.
func (v *Verifier) Bind(ctx context.Context, state *tls.ConnectionState) (context.Context, error) {
	if ctx == nil || ctx.Err() != nil || state == nil || !state.HandshakeComplete {
		return nil, ErrDenied
	}
	e, err := v.fromState(state)
	if err != nil {
		return nil, ErrDenied
	}
	return context.WithValue(ctx, evidenceKey{v}, e), nil
}

func (v *Verifier) verify(e evidence) (string, error) {
	if v == nil {
		return "", ErrDenied
	}
	leaf, err := x509.ParseCertificate(e.leaf)
	if err != nil || leaf.IsCA || len(leaf.URIs) != 1 || leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return "", ErrDenied
	}
	clientAuth := false
	for _, usage := range leaf.ExtKeyUsage {
		if usage == x509.ExtKeyUsageClientAuth {
			clientAuth = true
		}
	}
	if !clientAuth {
		return "", ErrDenied
	}
	intermediates := x509.NewCertPool()
	for _, raw := range e.intermediates {
		cert, err := x509.ParseCertificate(raw)
		if err != nil {
			return "", ErrDenied
		}
		intermediates.AddCert(cert)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: v.roots, Intermediates: intermediates, CurrentTime: v.now(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return "", ErrDenied
	}
	identity := leaf.URIs[0].String()
	caller, ok := v.callers[identity]
	if !ok || strings.TrimSpace(identity) != identity {
		return "", ErrDenied
	}
	return caller, nil
}

// VerifyPlatformCaller implements the service port structurally. Reverify at
// use time: expiry during keep-alive or delayed work cannot extend the identity.
func (v *Verifier) VerifyPlatformCaller(ctx context.Context) (string, error) {
	if v == nil || ctx == nil || ctx.Err() != nil {
		return "", ErrDenied
	}
	e, ok := ctx.Value(evidenceKey{v}).(evidence)
	if !ok {
		return "", ErrDenied
	}
	return v.verify(e)
}
