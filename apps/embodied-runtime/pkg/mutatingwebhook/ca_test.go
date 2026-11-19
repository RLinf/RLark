package mutatingwebhook

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// TestGenerateCA verifies the generated CA is a valid self-signed CA: it has
// IsCA set, the certificate is within its validity window, and it verifies
// against its own public key as the pool root.
func TestGenerateCA(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	if !ca.cert.IsCA {
		t.Error("cert.IsCA = false, want true")
	}
	if ca.key == nil {
		t.Fatal("CA key is nil")
	}
	// Validity window: NotBefore in the past, NotAfter in the future.
	now := time.Now()
	if now.Before(ca.cert.NotBefore) {
		t.Errorf("NotBefore %v is in the future", ca.cert.NotBefore)
	}
	if now.After(ca.cert.NotAfter) {
		t.Errorf("NotAfter %v is in the past", ca.cert.NotAfter)
	}
	// The self-signed cert must verify against a pool containing itself.
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	if _, err := ca.cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		t.Errorf("CA self-verify: %v", err)
	}
}

// TestSignServingCert_VerifiesAgainstCA signs a serving cert with the CA and
// confirms it chains to the CA under server-auth usage with the expected DNS
// names.
func TestSignServingCert_VerifiesAgainstCA(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	dns := []string{"webhook.rlark.svc", "webhook.rlark.svc.cluster.local"}
	serving, err := ca.signServingCert(dns)
	if err != nil {
		t.Fatalf("signServingCert: %v", err)
	}
	if len(serving.certPEM) == 0 || len(serving.keyPEM) == 0 {
		t.Fatal("serving cert or key PEM is empty")
	}

	// The key pair must be loadable as a TLS key pair.
	tlsCert, err := tls.X509KeyPair(serving.certPEM, serving.keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}

	// The leaf cert must chain to the CA under server-auth.
	leaf := tlsCert.Leaf
	if leaf == nil {
		// tls.X509KeyPair doesn't populate Leaf; parse it ourselves.
		leaf, err = parsePEMCert(serving.certPEM)
		if err != nil {
			t.Fatalf("parse leaf: %v", err)
		}
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSName: "webhook.rlark.svc"}); err != nil {
		t.Errorf("serving cert verify against CA: %v", err)
	}
	for _, want := range dns {
		found := false
		for _, got := range leaf.DNSNames {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DNSNames missing %q (got %v)", want, leaf.DNSNames)
		}
	}
}

// TestLoadCAFromSecret_RoundTrip saves a CA to a Secret via the fake clientset
// and loads it back, confirming the certificate round-trips unchanged.
func TestLoadCAFromSecret_RoundTrip(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	clientset := fake.NewSimpleClientset()

	if err := saveCAToSecret(context.Background(), clientset, "rlark", "webhook-ca", ca, nil); err != nil {
		t.Fatalf("saveCAToSecret: %v", err)
	}

	got, err := clientset.CoreV1().Secrets("rlark").Get(context.Background(), "webhook-ca", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	loaded, err := loadCAFromSecret(got)
	if err != nil {
		t.Fatalf("loadCAFromSecret: %v", err)
	}
	if !loaded.cert.Equal(ca.cert) {
		t.Error("loaded cert does not equal original cert")
	}
	// Compare the marshaled private keys — *big.Int fields can't be compared
	// with == (pointer equality) so compare the canonical DER form.
	origDER, _ := x509.MarshalECPrivateKey(ca.key)
	loadedDER, _ := x509.MarshalECPrivateKey(loaded.key)
	if !bytes.Equal(origDER, loadedDER) {
		t.Error("loaded key does not equal original key")
	}
}

// TestEnsureWebhookCABundle_PatchesEmpty creates a MutatingWebhookConfiguration
// with an empty caBundle and confirms ensureWebhookCABundle patches it with
// the CA certificate.
func TestEnsureWebhookCABundle_PatchesEmpty(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	caPEM := ca.certPEM()

	wh := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "webhook.example.com"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{Name: "mutate.example.com"},
			{Name: "mutate2.example.com"},
		},
	}
	clientset := fake.NewSimpleClientset(wh)

	if err := ensureWebhookCABundle(context.Background(), clientset, "webhook.example.com", caPEM); err != nil {
		t.Fatalf("ensureWebhookCABundle: %v", err)
	}

	got, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().
		Get(context.Background(), "webhook.example.com", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get webhook: %v", err)
	}
	for i := range got.Webhooks {
		cb := got.Webhooks[i].ClientConfig.CABundle
		if !bytes.Equal(cb, caPEM) {
			t.Errorf("webhook %d caBundle = %q, want CA cert PEM", i, string(cb))
		}
	}

	// A Patch action must have been recorded.
	patched := false
	for _, a := range clientset.Actions() {
		if a.GetVerb() == "patch" && a.GetResource().Resource == "mutatingwebhookconfigurations" {
			patched = true
		}
	}
	if !patched {
		t.Error("expected a Patch action, found none")
	}
}

// TestEnsureWebhookCABundle_LeavesNonEmpty confirms that a webhook whose
// caBundle already contains our CA certificate is not patched again
// (idempotent).
func TestEnsureWebhookCABundle_LeavesNonEmpty(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	caPEM := ca.certPEM()

	wh := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "webhook.example.com"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{Name: "mutate.example.com"},
		},
	}
	wh.Webhooks[0].ClientConfig.CABundle = caPEM
	clientset := fake.NewSimpleClientset(wh)

	if err := ensureWebhookCABundle(context.Background(), clientset, "webhook.example.com", caPEM); err != nil {
		t.Fatalf("ensureWebhookCABundle: %v", err)
	}

	// No Patch action should have been recorded.
	for _, a := range clientset.Actions() {
		if a.GetVerb() == "patch" {
			t.Errorf("unexpected Patch action: %#v", a)
		}
	}
}

// TestEnsureWebhookCABundle_MissingConfig returns an error when the webhook
// configuration does not exist.
func TestEnsureWebhookCABundle_MissingConfig(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	clientset := fake.NewSimpleClientset()

	err = ensureWebhookCABundle(context.Background(), clientset, "nope.example.com", ca.certPEM())
	if err == nil {
		t.Fatal("expected error for missing webhook config, got nil")
	}
}

// TestSaveCAToSecret_UpdateExisting pre-creates a Secret and confirms
// saveCAToSecret updates it in place rather than failing on conflict.
func TestSaveCAToSecret_UpdateExisting(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "webhook-ca", Namespace: "rlark"},
		Data:       map[string][]byte{"tls.crt": []byte("old"), "tls.key": []byte("old")},
	}
	clientset := fake.NewSimpleClientset(existing)

	if err := saveCAToSecret(context.Background(), clientset, "rlark", "webhook-ca", ca, nil); err != nil {
		t.Fatalf("saveCAToSecret: %v", err)
	}
	got, err := clientset.CoreV1().Secrets("rlark").Get(context.Background(), "webhook-ca", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if string(got.Data["tls.crt"]) == "old" {
		t.Error("secret data was not updated")
	}
}

// TestEnsureWebhookCABundle_StrategicMergePatchShape is a whitebox test that
// the strategic merge patch only references webhooks with an empty caBundle,
// keyed by name, so non-empty siblings are untouched.
func TestEnsureWebhookCABundle_StrategicMergePatchShape(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	caPEM := ca.certPEM()

	wh := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "webhook.example.com"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{Name: "empty.example.com"},
			{Name: "full.example.com"},
		},
	}
	wh.Webhooks[1].ClientConfig.CABundle = caPEM
	clientset := fake.NewSimpleClientset(wh)

	if err := ensureWebhookCABundle(context.Background(), clientset, "webhook.example.com", caPEM); err != nil {
		t.Fatalf("ensureWebhookCABundle: %v", err)
	}

	// Find the patch action and confirm its payload only targets the empty
	// webhook (by name).
	var patch clienttesting.PatchAction
	for _, a := range clientset.Actions() {
		if pa, ok := a.(clienttesting.PatchAction); ok {
			patch = pa
			break
		}
	}
	if patch == nil {
		t.Fatal("expected a Patch action, found none")
	}
	body := string(patch.GetPatch())
	if !bytes.Contains(patch.GetPatch(), []byte(`"name":"empty.example.com"`)) {
		t.Errorf("patch does not reference empty webhook: %s", body)
	}
	if bytes.Contains(patch.GetPatch(), []byte(`"name":"full.example.com"`)) {
		t.Errorf("patch should not reference the non-empty webhook: %s", body)
	}

	// After patching, both webhooks must end up with our CA cert.
	got, _ := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().
		Get(context.Background(), "webhook.example.com", metav1.GetOptions{})
	for i := range got.Webhooks {
		if !bytes.Equal(got.Webhooks[i].ClientConfig.CABundle, caPEM) {
			t.Errorf("webhook %d caBundle mismatch", i)
		}
	}
}
