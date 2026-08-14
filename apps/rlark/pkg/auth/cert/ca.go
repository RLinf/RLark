package cert

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// GenerateTemplateCA generates the templateCA.
func GenerateTemplateCA() *x509.Certificate {
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	return &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "RLark CA",
			Organization: []string{"RLinf"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(20 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
}

// GenerateCAKeyPair 生成一对 CA 私钥和公钥的 PEM 数据（x509 自签名 CA 证书 + 私钥）。
func GenerateCAKeyPair(template *x509.Certificate) (*x509.Certificate, *rsa.PrivateKey, error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	template.IsCA = true
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	caCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}
	return caCert, caKey, nil
}

// GenerateCA generates the cA.
func GenerateCA(template *x509.Certificate) (*Data, error) {
	caCert, caKey, err := GenerateCAKeyPair(template)
	if err != nil {
		return nil, err
	}

	var certBuf bytes.Buffer
	if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}); err != nil {
		return nil, err
	}
	caCertPEM := certBuf.Bytes()

	var keyBuf bytes.Buffer
	if err := pem.Encode(&keyBuf, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caKey),
	}); err != nil {
		return nil, err
	}
	caKeyPEM := keyBuf.Bytes()

	return &Data{
		CertPEM: caCertPEM,
		Cert:    caCert,
		KeyPEM:  caKeyPEM,
		Key:     caKey,
	}, nil
}

// SignX509Certificate signs the x509Certificate.
func (ca Data) SignX509Certificate(template *x509.Certificate) ([]byte, error) {
	if template == nil {
		return nil, fmt.Errorf("certificate template is nil")
	}
	if template.PublicKey == nil {
		return nil, fmt.Errorf("certificate template must include a public key")
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.Cert, template.PublicKey, ca.Key)
	if err != nil {
		return nil, err
	}
	return EncodeCertificateToPEM(certDER)
}

// SignSSHCertificate signs the sSHCertificate.
func (ca Data) SignSSHCertificate(template *gossh.Certificate) ([]byte, error) {
	if template == nil {
		return nil, fmt.Errorf("SSH certificate template is nil")
	}
	if template.Key == nil {
		return nil, fmt.Errorf("SSH certificate template must include a public key")
	}
	caSigner, err := gossh.NewSignerFromKey(ca.Key)
	if err != nil {
		return nil, err
	}
	cert := &gossh.Certificate{
		Key:             template.Key,
		Serial:          template.Serial,
		CertType:        template.CertType,
		KeyId:           template.KeyId,
		ValidPrincipals: template.ValidPrincipals,
		ValidAfter:      template.ValidAfter,
		ValidBefore:     template.ValidBefore,
		Permissions:     template.Permissions,
		Reserved:        template.Reserved,
	}
	if err := cert.SignCert(rand.Reader, caSigner); err != nil {
		return nil, err
	}
	return EncodeSSHCertificateToPEM(cert.Marshal())
}
