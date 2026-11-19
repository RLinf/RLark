package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestStoreAgentCertCreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	cl := fake.NewSimpleClientset()
	g := &Gateway{rawClient: cl}
	ns := g.managementNamespace()
	if err := g.storeAgentCertSecret(ctx, "test", []byte("ca"), []byte("cert"), []byte("key")); err != nil {
		t.Fatal(err)
	}
	s, err := cl.CoreV1().Secrets(ns).Get(ctx, "rlark-agent-cert-test", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s.Data["other"] = []byte("preserved")
	if _, err = cl.CoreV1().Secrets(ns).Update(ctx, s, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	cl.PrependReactor("update", "secrets", func(a ktesting.Action) (bool, runtime.Object, error) {
		attempts++
		if attempts == 1 {
			return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, "test", errors.New("conflict"))
		}
		return false, nil, nil
	})
	if err = g.storeAgentCertSecret(ctx, "test", []byte("new-ca"), []byte("new-cert"), []byte("new-key")); err != nil {
		t.Fatal(err)
	}
	s, err = cl.CoreV1().Secrets(ns).Get(ctx, s.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(s.Data["tls.crt"]) != "new-cert" || string(s.Data["tls.key"]) != "new-key" || string(s.Data["other"]) != "preserved" || attempts != 2 {
		t.Fatalf("unexpected stored data or retries: %d", attempts)
	}
}

func TestRevokeRequiresDurableServerReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name           string
		upstreamStatus int
		response       string
		want           int
	}{
		{"persisted", 200, `{"persisted":true}`, 200},
		{"legacy empty success", 200, ``, 502},
		{"unconfirmed", 200, `{"persisted":false}`, 502},
		{"invalid identity", 400, `{}`, 400},
		{"store unavailable", 503, `{}`, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/revoke" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tc.upstreamStatus)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer upstream.Close()
			g := &Gateway{config: Config{ServerAddress: upstream.URL}, serverTransport: http.DefaultTransport.(*http.Transport)}
			router := gin.New()
			router.POST("/", g.handleRevokeCertificate)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"cert_type":"x509","serial_number":"1","subject_key_id":"ab"}`)))
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestStoreAgentCertRejectsForeignAndCreateFailure(t *testing.T) {
	ctx := context.Background()
	cl := fake.NewSimpleClientset()
	g := &Gateway{rawClient: cl}
	ns := g.managementNamespace()
	foreign := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "rlark-agent-cert-test", Namespace: ns, Labels: map[string]string{common.AgentCertLabelKey: common.AgentCertLabelValue}, Annotations: map[string]string{"rlark.io/cluster-id": "different"}}, Data: map[string][]byte{"tls.key": []byte("unchanged")}}
	if _, err := cl.CoreV1().Secrets(ns).Create(ctx, foreign, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := g.storeAgentCertSecret(ctx, "test", nil, nil, []byte("new")); err == nil {
		t.Fatal("accepted foreign secret")
	}
	s, err := cl.CoreV1().Secrets(ns).Get(ctx, foreign.Name, metav1.GetOptions{})
	if err != nil || string(s.Data["tls.key"]) != "unchanged" {
		t.Fatal("foreign secret changed", err)
	}
	denied := fake.NewSimpleClientset()
	denied.PrependReactor("create", "secrets", func(a ktesting.Action) (bool, runtime.Object, error) { return true, nil, errors.New("unavailable") })
	g.rawClient = denied
	if err := g.storeAgentCertSecret(ctx, "test", nil, nil, nil); err == nil {
		t.Fatal("ignored create failure")
	}
	for _, a := range denied.Actions() {
		if a.GetVerb() == "update" {
			t.Fatal("updated after unrelated create error")
		}
	}
}

func TestSignAgentCertificateDoesNotReturnCredentialsWhenPersistenceFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cert_pem":"test-certificate","key_pem":"test-private-key"}`))
	}))
	defer upstream.Close()
	cl := fake.NewSimpleClientset()
	g := &Gateway{rawClient: cl, config: Config{ServerAddress: upstream.URL}, serverTransport: http.DefaultTransport.(*http.Transport)}
	ns := g.managementNamespace()
	for _, s := range []*corev1.Secret{{ObjectMeta: metav1.ObjectMeta{Name: common.AdminCertSecretName, Namespace: ns}, Data: map[string][]byte{"client.crt": []byte("cert"), "client.key": []byte("key")}}, {ObjectMeta: metav1.ObjectMeta{Name: common.TLSCASecretName, Namespace: ns}, Data: map[string][]byte{"ca.crt": []byte("ca")}}} {
		if _, err := cl.CoreV1().Secrets(ns).Create(context.Background(), s, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	cl.PrependReactor("create", "secrets", func(a ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("storage unavailable")
	})
	router := gin.New()
	router.POST("/", g.handleSignAgentCert)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"cluster_id":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "test-private-key") || strings.Contains(rec.Body.String(), "test-certificate") {
		t.Fatal("returned credentials on persistence failure")
	}
}
