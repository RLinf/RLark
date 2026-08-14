package cert

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
)

// Bundle is an exported type.
type Bundle struct {
	CACertPEM []byte
	CAKeyPEM  []byte
	CertPEM   []byte
	KeyPEM    []byte
}

func resolveCertData(field string, label string) ([]byte, error) {
	if field == "" {
		return nil, fmt.Errorf("%s is empty", label)
	}
	if _, err := os.Stat(field); err == nil {
		data, err := os.ReadFile(field)
		if err != nil {
			return nil, fmt.Errorf("read %s file: %w", label, err)
		}
		return data, nil
	}
	if filepath.IsAbs(field) || strings.HasPrefix(field, ".") || strings.HasPrefix(field, "~") {
		return nil, fmt.Errorf("%s file not found: %s", label, field)
	}
	return []byte(field), nil
}

// GenerateBundle generates the bundle.
func GenerateBundle(cfg *types.CertConfig) (*Bundle, error) {
	caCertPEM, err := resolveCertData(cfg.CACert, "ca-cert")
	if err != nil {
		return nil, err
	}

	certPEM, err := resolveCertData(cfg.AgentCert, "agent-cert")
	if err != nil {
		return nil, err
	}
	keyPEM, err := resolveCertData(cfg.AgentKey, "agent-key")
	if err != nil {
		return nil, err
	}

	return &Bundle{
		CACertPEM: caCertPEM,
		CertPEM:   certPEM,
		KeyPEM:    keyPEM,
	}, nil
}

// WriteToDir writes the toDir.
func (b *Bundle) WriteToDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create cert dir: %w", err)
	}
	files := map[string][]byte{
		"ca.crt":  b.CACertPEM,
		"ca.key":  b.CAKeyPEM,
		"tls.crt": b.CertPEM,
		"tls.key": b.KeyPEM,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}
