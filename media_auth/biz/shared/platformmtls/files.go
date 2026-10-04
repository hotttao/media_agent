package platformmtls

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"os"
	"path/filepath"
)

// LoadFiles rejects symlinks and unsafe key permissions, bounds reads, and
// never exposes parser errors or generates fallback trust/key material.
func LoadFiles(certPath, keyPath, caPath string, callers map[string]string) (*Verifier, *tls.Config, error) {
	certPEM, err := readFile(certPath, false)
	if err != nil {
		return nil, nil, ErrDenied
	}
	keyPEM, err := readFile(keyPath, true)
	if err != nil {
		return nil, nil, ErrDenied
	}
	defer func() {
		for i := range keyPEM {
			keyPEM[i] = 0
		}
	}()
	caPEM, err := readFile(caPath, false)
	if err != nil {
		return nil, nil, ErrDenied
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, nil, ErrDenied
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, nil, ErrDenied
	}
	v, err := NewVerifier(pool, callers, nil)
	if err != nil {
		return nil, nil, ErrDenied
	}
	config, err := v.TLSConfig(cert)
	if err != nil {
		return nil, nil, ErrDenied
	}
	return v, config, nil
}

func readFile(path string, private bool) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, ErrDenied
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0022 != 0 || private && before.Mode().Perm()&0077 != 0 || before.Size() < 1 || before.Size() > 65536 {
		return nil, ErrDenied
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrDenied
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || after.Mode() != before.Mode() || after.Size() != before.Size() {
		return nil, ErrDenied
	}
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	current, statErr := os.Lstat(path)
	if err != nil || statErr != nil || !os.SameFile(after, current) || current.Mode() != after.Mode() || len(data) != int(after.Size()) {
		return nil, ErrDenied
	}
	return data, nil
}
