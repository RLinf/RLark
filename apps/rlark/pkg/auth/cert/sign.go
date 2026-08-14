package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	gossh "golang.org/x/crypto/ssh"
)

// Sign signs the input.
func Sign(ca *Data, signType string, meta map[string]string) (*Data, error) {
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate private key: %w", err)
	}
	keyPEM, err := EncodePrivateKeyToPEM(leafKey)
	if err != nil {
		return nil, fmt.Errorf("encode private key: %w", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	var certPEM []byte
	switch signType {
	case "x509", "":
		template := &x509.Certificate{
			SerialNumber: serial,
			Subject:      pkix.Name{CommonName: "client"},
			NotBefore:    time.Now().Add(-1 * time.Hour),
			NotAfter:     time.Now().Add(365 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			PublicKey:    leafKey.Public(),
			SubjectKeyId: []byte(uuid.NewString()),
		}
		SetX509CertMeta(template, meta)
		certPEM, err = ca.SignX509Certificate(template)
		if err != nil {
			return nil, fmt.Errorf("sign x509 certificate: %w", err)
		}

	case "ssh":
		sshKey, err := gossh.NewPublicKey(leafKey.Public())
		if err != nil {
			return nil, fmt.Errorf("create SSH public key: %w", err)
		}
		template := &gossh.Certificate{
			Key:             sshKey,
			Serial:          serial.Uint64(),
			CertType:        gossh.UserCert,
			KeyId:           uuid.NewString(),
			ValidPrincipals: []string{"client"},
			ValidAfter:      uint64(time.Now().Add(-1 * time.Hour).Unix()),
			ValidBefore:     uint64(time.Now().Add(365 * 24 * time.Hour).Unix()),
			Permissions:     gossh.Permissions{},
		}
		SetSSHCertMeta(template, meta)
		certPEM, err = ca.SignSSHCertificate(template)
		if err != nil {
			return nil, fmt.Errorf("sign SSH certificate: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported certificate type: %s", signType)
	}
	return LoadData(certPEM, keyPEM)
}
