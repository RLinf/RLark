package cert

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	gossh "golang.org/x/crypto/ssh"
)

// EncodePrivateKeyToPEM encodes the privateKeyToPEM.
func EncodePrivateKeyToPEM(key *rsa.PrivateKey) ([]byte, error) {
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodePrivateKeyFromPEM decodes the privateKeyFromPEM.
func DecodePrivateKeyFromPEM(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return nil, fmt.Errorf("decode PEM block containing RSA private key")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// EncodeCertificateToPEM encodes the certificateToPEM.
func EncodeCertificateToPEM(cert []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeCertificateFromPEM decodes the certificateFromPEM.
func DecodeCertificateFromPEM(pemData []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("decode PEM block containing certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

// EncodeSSHCertificateToPEM encodes the sSHCertificateToPEM.
func EncodeSSHCertificateToPEM(cert []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{
		Type:  "SSH CERTIFICATE",
		Bytes: cert,
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeSSHCertificateFromPEM decodes the sSHCertificateFromPEM.
func DecodeSSHCertificateFromPEM(pemData []byte) (*gossh.Certificate, error) {
	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "SSH CERTIFICATE" {
		return nil, fmt.Errorf("decode PEM block containing SSH certificate")
	}
	cert, err := gossh.ParsePublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse SSH certificate: %w", err)
	}
	typedCert, ok := cert.(*gossh.Certificate)
	if !ok {
		return nil, fmt.Errorf("parsed SSH certificate has wrong type: %T", cert)
	}
	return typedCert, nil
}
