package cert

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// Data holds certificate data.
type Data struct {
	CertPEM []byte
	Cert    *x509.Certificate
	SSHCert *gossh.Certificate
	KeyPEM  []byte
	Key     *rsa.PrivateKey
}

// IsValid reports whether valid.
func (data *Data) IsValid() bool {
	if data.Cert == nil && data.SSHCert == nil {
		return false
	}
	if data.Key == nil {
		return false
	}
	if data.Cert != nil {
		if data.Cert.NotAfter.Before(time.Now()) {
			return false
		}
		if data.Cert.NotBefore.After(time.Now()) {
			return false
		}
	}
	if data.SSHCert != nil {
		if data.SSHCert.ValidBefore < uint64(time.Now().Unix()) {
			return false
		}
		if data.SSHCert.ValidAfter > uint64(time.Now().Unix()) {
			return false
		}
	}
	return true
}

// LoadData loads the data.
func LoadData(certPEM, keyPEM []byte) (*Data, error) {
	data := &Data{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}
	data.Cert, _ = DecodeCertificateFromPEM(certPEM)
	data.SSHCert, _ = DecodeSSHCertificateFromPEM(certPEM)
	if data.Cert == nil && data.SSHCert == nil {
		return nil, fmt.Errorf("failed to parse certificate")
	}
	var err error
	data.Key, err = DecodePrivateKeyFromPEM(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	return data, nil
}
