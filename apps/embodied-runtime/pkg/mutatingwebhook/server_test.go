package mutatingwebhook

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

// TestServeHTTP_Allow verifies that a POST with a valid AdmissionReview is
// dispatched to the Handler and the response carries the request UID and the
// handler's Allowed decision.
func TestServeHTTP_Allow(t *testing.T) {
	handler := HandlerFunc(func(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
		return &admissionv1.AdmissionResponse{Allowed: true}
	})
	s := &Server{cfg: ServerConfig{Path: "/mutate", Handler: handler}}

	body := mustMarshalAdmissionReview(t, "allow-me")
	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.serveAdmission(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp admissionv1.AdmissionReview
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (body: %s)", err, rec.Body.String())
	}
	if resp.Response == nil {
		t.Fatal("response is nil")
	}
	if resp.Response.UID != "allow-me" {
		t.Errorf("UID = %q, want allow-me", resp.Response.UID)
	}
	if !resp.Response.Allowed {
		t.Error("Allowed = false, want true")
	}
}

// TestServeHTTP_NilResponseDefaultsToDenied confirms that a handler returning
// nil yields a denied response (the API server requires an explicit decision).
func TestServeHTTP_NilResponseDefaultsToDenied(t *testing.T) {
	handler := HandlerFunc(func(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
		return nil
	})
	s := &Server{cfg: ServerConfig{Path: "/mutate", Handler: handler}}

	body := mustMarshalAdmissionReview(t, "deny-me")
	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	s.serveAdmission(rec, req)

	var resp admissionv1.AdmissionReview
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Response.Allowed {
		t.Error("Allowed = true, want false (nil response defaults to denied)")
	}
	if resp.Response.UID != "deny-me" {
		t.Errorf("UID = %q, want deny-me", resp.Response.UID)
	}
}

// TestServeHTTP_MethodNotAllowed confirms non-POST methods are rejected.
func TestServeHTTP_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: ServerConfig{Path: "/mutate", Handler: HandlerFunc(func(context.Context, *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse { return nil })}}
	req := httptest.NewRequest(http.MethodGet, "/mutate", nil)
	rec := httptest.NewRecorder()
	s.serveAdmission(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// TestServeHTTP_BadBody confirms an invalid JSON body is rejected with 400.
func TestServeHTTP_BadBody(t *testing.T) {
	s := &Server{cfg: ServerConfig{Path: "/mutate", Handler: HandlerFunc(func(context.Context, *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse { return nil })}}
	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()
	s.serveAdmission(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestServeHTTP_Patch confirms a handler that returns a JSON patch has it
// echoed back with the JSONPatch type.
func TestServeHTTP_Patch(t *testing.T) {
	patch := []byte(`[{"op":"add","path":"/spec/replicas","value":3}]`)
	handler := HandlerFunc(func(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
		return &admissionv1.AdmissionResponse{
			Allowed:   true,
			Patch:     patch,
			PatchType: (*admissionv1.PatchType)(ptr(string(admissionv1.PatchTypeJSONPatch))),
		}
	})
	s := &Server{cfg: ServerConfig{Path: "/mutate", Handler: handler}}

	body := mustMarshalAdmissionReview(t, "patch-me")
	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.serveAdmission(rec, req)

	var resp admissionv1.AdmissionReview
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !bytes.Equal(resp.Response.Patch, patch) {
		t.Errorf("patch = %q, want %q", resp.Response.Patch, patch)
	}
	if resp.Response.PatchType == nil || *resp.Response.PatchType != admissionv1.PatchTypeJSONPatch {
		t.Errorf("patchType = %v, want %s", resp.Response.PatchType, admissionv1.PatchTypeJSONPatch)
	}
}

// TestNewServer_Validation exercises the required-field checks so callers get
// clear errors before any k8s API call is attempted.
func TestNewServer_Validation(t *testing.T) {
	cs := fake.NewSimpleClientset()
	tests := []struct {
		name string
		cfg  ServerConfig
		want string
	}{
		{"no addr", ServerConfig{WebhookName: "w", Clientset: cs, Handler: HandlerFunc(nil)}, "Addr"},
		{"no webhook name", ServerConfig{Addr: ":8443", Clientset: cs, Handler: HandlerFunc(nil)}, "WebhookName"},
		{"no clientset", ServerConfig{Addr: ":8443", WebhookName: "w", Handler: HandlerFunc(nil)}, "Clientset"},
		{"no handler", ServerConfig{Addr: ":8443", WebhookName: "w", Clientset: cs}, "Handler"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewServer(tt.cfg); err == nil {
				t.Fatalf("expected error mentioning %q, got nil", tt.want)
			}
		})
	}
}

// TestNewServer_FullCAFlow wires NewServer against a fake clientset with a
// pre-existing empty MutatingWebhookConfiguration and asserts the caBundle is
// populated and the serving cert verifies against the CA. No Secret is
// configured, so the CA lives in memory.
func TestNewServer_FullCAFlow(t *testing.T) {
	wh := newEmptyWebhookConfig("webhook.example.com")
	cs := fake.NewSimpleClientset(wh)

	cfg := ServerConfig{
		Addr:        "127.0.0.1:0",
		Path:        "/mutate",
		WebhookName: "webhook.example.com",
		Clientset:   cs,
		Handler: HandlerFunc(func(context.Context, *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
			return &admissionv1.AdmissionResponse{Allowed: true}
		}),
		DNSNames: []string{"localhost"},
	}
	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if s.servingCert == nil || len(s.servingCert.certPEM) == 0 {
		t.Fatal("serving cert not generated")
	}

	// The webhook's caBundle must now carry our CA cert.
	got, _ := cs.AdmissionregistrationV1().MutatingWebhookConfigurations().
		Get(context.Background(), "webhook.example.com", metav1.GetOptions{})
	if len(got.Webhooks) == 0 || len(got.Webhooks[0].ClientConfig.CABundle) == 0 {
		t.Fatal("caBundle was not populated")
	}
	if !bytes.Contains(got.Webhooks[0].ClientConfig.CABundle, s.ca.certPEM()) {
		t.Error("caBundle does not contain the server's CA cert")
	}

	// The serving cert must verify against the CA.
	pool := x509.NewCertPool()
	pool.AddCert(s.ca.cert)
	leaf, err := parsePEMCert(s.servingCert.certPEM)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSName: "localhost"}); err != nil {
		t.Errorf("serving cert verify: %v", err)
	}
}

// mustMarshalAdmissionReview builds a minimal AdmissionReview JSON request with
// the given UID.
func mustMarshalAdmissionReview(t *testing.T, uid string) []byte {
	t.Helper()
	review := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request: &admissionv1.AdmissionRequest{
			UID:       types.UID(uid),
			Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Operation: admissionv1.Create,
			Object: runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"foo"}}`),
			},
		},
	}
	b, err := json.Marshal(review)
	if err != nil {
		t.Fatalf("marshal admission review: %v", err)
	}
	return b
}

// newEmptyWebhookConfig builds a minimal single-webhook MutatingWebhookConfiguration
// whose caBundle is empty (so the server will populate it on startup).
func newEmptyWebhookConfig(name string) *admissionregistrationv1.MutatingWebhookConfiguration {
	path := "/mutate"
	port := int32(443)
	return &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "mutate.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Name:      "webhook",
						Namespace: "rlark",
						Path:      &path,
						Port:      &port,
					},
				},
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
					},
				},
			},
		},
	}
}

// ptr returns a pointer to s; used to take the address of a PatchType literal.
func ptr(s string) *string { return &s }
