package cert

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestSigning(t *testing.T) {
	ca, err := GenerateCA(GenerateTemplateCA())
	if err != nil {
		t.Fatalf("failed to generate CA data: %v", err)
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate leaf key: %v", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	x509Template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    ca.Cert.NotBefore,
		NotAfter:     ca.Cert.NotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		PublicKey:    leafKey.Public(),
		SubjectKeyId: []byte("test-key-id"),
	}
	x509PEM, err := ca.SignX509Certificate(x509Template)
	if err != nil {
		t.Fatalf("failed to sign x509 certificate: %v", err)
	}
	x509Cert, err := DecodeCertificateFromPEM(x509PEM)
	if err != nil {
		t.Fatalf("failed to decode signed certificate: %v", err)
	}
	if err := x509Cert.CheckSignatureFrom(ca.Cert); err != nil {
		t.Fatalf("certificate signature verification failed: %v", err)
	}

	sshKey, err := gossh.NewPublicKey(leafKey.Public())
	if err != nil {
		t.Fatalf("failed to create SSH public key: %v", err)
	}
	sshTemplate := &gossh.Certificate{
		Key:             sshKey,
		Serial:          serial.Uint64(),
		CertType:        gossh.UserCert,
		KeyId:           "test",
		ValidPrincipals: []string{"test"},
		ValidAfter:      uint64(ca.Cert.NotBefore.Unix()),
		ValidBefore:     uint64(ca.Cert.NotAfter.Unix()),
		Permissions:     gossh.Permissions{},
	}
	SetSSHCertMeta(sshTemplate, map[string]string{"role": "test"})
	sshPEM, err := ca.SignSSHCertificate(sshTemplate)
	if err != nil {
		t.Fatalf("failed to sign SSH certificate: %v", err)
	}
	sshCert, err := DecodeSSHCertificateFromPEM(sshPEM)
	if err != nil {
		t.Fatalf("failed to decode signed SSH certificate: %v", err)
	}
	cc := &gossh.CertChecker{
		IsUserAuthority: func(auth gossh.PublicKey) bool {
			return bytes.Equal(auth.Marshal(), sshKey.Marshal())
		},
	}
	if err := cc.CheckCert("test", sshCert); err != nil {
		t.Fatalf("SSH certificate verification failed: %v", err)
	}
	if meta, ok := GetSSHCertMeta(sshCert); !ok || meta["role"] != "test" {
		t.Fatalf("SSH certificate metadata verification failed: got %v, ok=%v", meta, ok)
	}
}
