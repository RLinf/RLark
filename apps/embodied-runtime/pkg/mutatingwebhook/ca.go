package mutatingwebhook

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
)

// Secret data keys for the persisted CA.
const (
	secretCertKey = "tls.crt"
	secretKeyKey  = "tls.key"
)

// caKeyPair is a self-signed CA certificate and its ECDSA private key. It is
// persisted in a Secret so the same CA survives restarts and is shared across
// webhook pods. The certificate (without the key) is also published in the
// MutatingWebhookConfiguration's caBundle so the API server trusts serving
// certificates signed by this CA.
type caKeyPair struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

// servingCert is a leaf certificate (with its private key) signed by the CA,
// used to serve the webhook over TLS.
type servingCert struct {
	certPEM []byte
	keyPEM  []byte
}

// generateCA creates a self-signed CA certificate and ECDSA P-256 private key.
// The CA is valid for 10 years and only has the CertSign/CRLSign key usages.
func generateCA() (*caKeyPair, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate CA serial: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "rlark-mutating-webhook-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create CA certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse CA certificate: %w", err)
	}
	return &caKeyPair{cert: cert, key: key}, nil
}

// signServingCert signs a leaf certificate for the given DNS names using the
// CA. The returned PEM blocks are suitable for crypto/tls.X509KeyPair.
func (c *caKeyPair) signServingCert(dnsNames []string) (*servingCert, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate serving key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serving serial: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "rlark-mutating-webhook"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		return nil, fmt.Errorf("sign serving certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal serving key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &servingCert{certPEM: certPEM, keyPEM: keyPEM}, nil
}

// certPEM returns the PEM-encoded CA certificate (no private key).
func (c *caKeyPair) certPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.cert.Raw})
}

// keyPEM returns the PEM-encoded CA private key.
func (c *caKeyPair) keyPEM() []byte {
	keyDER, err := x509.MarshalECPrivateKey(c.key)
	if err != nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

// loadCAFromSecret reads the CA certificate and private key from the given
// Secret. Returns an error if the Secret is missing the required data keys
// or the data is malformed.
func loadCAFromSecret(secret *corev1.Secret) (*caKeyPair, error) {
	certPEM, ok := secret.Data[secretCertKey]
	if !ok {
		return nil, fmt.Errorf("secret %s/%s missing key %q", secret.Namespace, secret.Name, secretCertKey)
	}
	keyPEM, ok := secret.Data[secretKeyKey]
	if !ok {
		return nil, fmt.Errorf("secret %s/%s missing key %q", secret.Namespace, secret.Name, secretKeyKey)
	}

	cert, err := parsePEMCert(certPEM)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}
	key, err := parsePEMECKey(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse CA key: %w", err)
	}
	return &caKeyPair{cert: cert, key: key}, nil
}

// saveCAToSecret creates or updates the Secret with the CA certificate and
// private key. ownerRefs, if non-empty, makes the Secret garbage-collected
// with the referenced owner.
func saveCAToSecret(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace, name string,
	ca *caKeyPair,
	ownerRefs []metav1.OwnerReference,
) error {
	data := map[string][]byte{
		secretCertKey: ca.certPEM(),
		secretKeyKey:  ca.keyPEM(),
	}

	existing, err := clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		existing.Data = data
		if _, err := clientset.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update CA secret %s/%s: %w", namespace, name, err)
		}
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: ownerRefs,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
	if _, err := clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create CA secret %s/%s: %w", namespace, name, err)
	}
	return nil
}

// ensureWebhookCABundle reads the MutatingWebhookConfiguration and, for each
// webhook whose caBundle is empty, patches it with the CA certificate. Webhooks
// that already have a caBundle are left untouched (assumed to be managed
// externally or by a previous run); a warning is logged when an existing
// caBundle does not match this CA, since the API server would not trust a
// serving certificate signed by our CA in that case.
//
// The patch is a strategic merge patch keyed on the webhook name, so only the
// webhooks listed in the patch body are affected.
func ensureWebhookCABundle(
	ctx context.Context,
	clientset kubernetes.Interface,
	name string,
	caCertPEM []byte,
) error {
	wh, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get mutating webhook config %q: %w", name, err)
	}

	// Collect webhook names that need a caBundle. Also warn when a non-empty
	// caBundle differs from our CA — the operator should clear it so the
	// webhook manager can own the trust anchor.
	type webhookPatch struct {
		Name         string `json:"name"`
		ClientConfig struct {
			CABundle []byte `json:"caBundle"`
		} `json:"clientConfig"`
	}

	var toPatch []webhookPatch
	for i := range wh.Webhooks {
		cb := wh.Webhooks[i].ClientConfig.CABundle
		if len(cb) == 0 {
			wp := webhookPatch{Name: wh.Webhooks[i].Name}
			wp.ClientConfig.CABundle = caCertPEM
			toPatch = append(toPatch, wp)
			continue
		}
		if !bytes.Contains(cb, caCertPEM) {
			log.Printf("[mutating-webhook] WARNING: webhook %q has a non-empty caBundle that does not contain this CA's certificate; "+
				"serving cert signed by this CA will not be trusted. Clear the caBundle to let this webhook manage it.", wh.Webhooks[i].Name)
		}
	}

	if len(toPatch) == 0 {
		return nil
	}

	patch := struct {
		Webhooks []webhookPatch `json:"webhooks"`
	}{Webhooks: toPatch}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal caBundle patch: %w", err)
	}

	if _, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().
		Patch(ctx, name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("patch caBundle on webhook %q: %w", name, err)
	}

	log.Printf("[mutating-webhook] updated caBundle for %d webhook(s) on %q", len(toPatch), name)
	return nil
}

// parsePEMCert returns the first certificate parsed from a PEM-encoded byte
// slice.
func parsePEMCert(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParseCertificate(block.Bytes)
}

// parsePEMECKey parses a PEM-encoded ECDSA private key.
func parsePEMECKey(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}
