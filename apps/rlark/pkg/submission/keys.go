package submission

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// LoadPrivateKey reads a filesystem-protected PKCS#8 Ed25519 signing key.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("submission signing key permissions must be 0600 or stricter")
	}
	der, err := readPEM(path, "PRIVATE KEY")
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	private, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("submission signing key must be Ed25519")
	}
	return private, nil
}

// LoadPublicKey reads a PKIX Ed25519 verification key.
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	der, err := readPEM(path, "PUBLIC KEY")
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	public, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("submission verification key must be Ed25519")
	}
	return public, nil
}

func readPEM(path, expectedType string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != expectedType || len(rest) != 0 {
		return nil, fmt.Errorf("expected one PEM block of type %s", expectedType)
	}
	return block.Bytes, nil
}
