package mutatingwebhook

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Handler processes a single admission request and returns the admission
// response. The response's UID is set by the server to match the request; a
// handler may leave it zero. Implementations are free to set Allowed/Patch
// and should return a non-nil response even when denying the request.
type Handler interface {
	Mutate(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse
}

// HandlerFunc is a function adapter for Handler.
type HandlerFunc func(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse

// Mutate calls f(ctx, req).
func (f HandlerFunc) Mutate(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	return f(ctx, req)
}

// ServerConfig holds configuration for the mutating webhook server.
type ServerConfig struct {
	// Addr is the TCP address (":port" or "host:port") the HTTPS server
	// listens on, e.g. ":8443".
	Addr string

	// Path is the HTTP path the admission endpoint is served at, e.g.
	// "/mutate". Defaults to "/mutate".
	Path string

	// WebhookName is the name of the MutatingWebhookConfiguration whose
	// caBundle this server manages. Required.
	WebhookName string

	// Clientset is the Kubernetes clientset used to read/update the webhook
	// caBundle and the CA Secret. Required.
	Clientset kubernetes.Interface

	// Handler processes admission requests. Required.
	Handler Handler

	// DNSNames are the DNS Subject Alternative Names for the serving
	// certificate. For a webhook exposed via a Service "webhook" in
	// namespace "rlark" this should be:
	//
	//	[]string{"webhook", "webhook.rlark", "webhook.rlark.svc",
	//	         "webhook.rlark.svc.cluster.local"}
	//
	// Defaults to []string{"localhost"} (useful for local development only).
	DNSNames []string

	// CASecretName is the name of the Secret where the CA certificate and
	// private key are persisted. When set (together with CASecretNamespace)
	// the CA survives restarts and is shared across webhook pods; when empty
	// a fresh in-memory CA is generated on every start.
	CASecretName string

	// CASecretNamespace is the namespace of the CA Secret. Ignored when
	// CASecretName is empty.
	CASecretNamespace string

	// OwnerReferences, when non-empty, are applied to the CA Secret on
	// creation so it is garbage-collected with the referenced owner.
	OwnerReferences []metav1.OwnerReference
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Addr:     ":8443",
		Path:     "/mutate",
		DNSNames: []string{"localhost"},
	}
}

// Server is a Kubernetes mutating admission webhook server with automatic CA
// management. On startup it ensures a CA exists (persisted in a Secret when
// configured), publishes the CA certificate into the MutatingWebhookConfiguration's
// caBundle when empty, signs a short-lived serving certificate, and serves
// admission reviews over HTTPS.
type Server struct {
	cfg         ServerConfig
	ca          *caKeyPair
	servingCert *servingCert
	httpSrv     *http.Server
}

// NewServer prepares a webhook server: it loads or generates the CA,
// syncs the caBundle into the MutatingWebhookConfiguration, and signs the
// serving certificate. The returned server is not started; call Run to start
// listening.
func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("webhook server: Addr is required")
	}
	if cfg.Path == "" {
		cfg.Path = "/mutate"
	}
	if cfg.WebhookName == "" {
		return nil, fmt.Errorf("webhook server: WebhookName is required")
	}
	if cfg.Clientset == nil {
		return nil, fmt.Errorf("webhook server: Clientset is required")
	}
	if cfg.Handler == nil {
		return nil, fmt.Errorf("webhook server: Handler is required")
	}
	if len(cfg.DNSNames) == 0 {
		cfg.DNSNames = []string{"localhost"}
	}

	ctx := context.Background()

	// 1. Obtain the CA: load from Secret when configured, otherwise generate
	// a fresh in-memory CA.
	ca, err := loadOrGenerateCA(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// 2. Publish the CA certificate into the webhook's caBundle for any
	// webhook that has an empty caBundle.
	if err := ensureWebhookCABundle(ctx, cfg.Clientset, cfg.WebhookName, ca.certPEM()); err != nil {
		return nil, fmt.Errorf("sync webhook caBundle: %w", err)
	}

	// 3. Sign a serving certificate with the CA.
	serving, err := ca.signServingCert(cfg.DNSNames)
	if err != nil {
		return nil, fmt.Errorf("sign serving cert: %w", err)
	}

	return &Server{cfg: cfg, ca: ca, servingCert: serving}, nil
}

// Start begins serving admission reviews over HTTPS in the background and
// returns as soon as the TLS listener is bound. The server runs until Stop
// is called or the listener fails; a listener failure is logged. Use Start
// when embedding the webhook in a process that owns its own signal handling
// (e.g. the device plugin). For standalone use, prefer Run.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.Path, s.serveAdmission)
	mux.HandleFunc("/", s.serveAdmission)

	tlsCert, err := tls.X509KeyPair(s.servingCert.certPEM, s.servingCert.keyPEM)
	if err != nil {
		return fmt.Errorf("load serving key pair: %w", err)
	}

	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.Addr, err)
	}

	s.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			MinVersion:   tls.VersionTLS12,
		},
	}

	go func() {
		log.Printf("[mutating-webhook] serving on https://%s%s", ln.Addr(), s.cfg.Path)
		if err := s.httpSrv.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
			log.Printf("[mutating-webhook] server exited: %v", err)
		}
	}()
	return nil
}

// Run starts the HTTPS webhook server and blocks until a shutdown signal is
// received or the server fails. It is a convenience for standalone use;
// embedded callers should use Start + Stop and manage signals themselves.
func (s *Server) Run() error {
	if err := s.Start(); err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	log.Printf("[mutating-webhook] received %v, shutting down...", sig)

	s.Stop()
	return nil
}

// Stop gracefully shuts down the HTTPS server, waiting up to 5 seconds for
// in-flight admission requests to finish.
func (s *Server) Stop() {
	if s.httpSrv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.httpSrv.Shutdown(ctx); err != nil {
		log.Printf("[mutating-webhook] shutdown: %v", err)
	}
}

// serveAdmission decodes an AdmissionReview from the request body, dispatches
// it to the configured Handler, and writes the AdmissionReview response back.
func (s *Server) serveAdmission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[mutating-webhook] read body: %v", err)
		http.Error(w, "read request body", http.StatusBadRequest)
		return
	}

	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(body, &review); err != nil {
		log.Printf("[mutating-webhook] decode admission review: %v", err)
		http.Error(w, "invalid admission review", http.StatusBadRequest)
		return
	}
	if review.Request == nil {
		http.Error(w, "admission review missing request", http.StatusBadRequest)
		return
	}

	resp := s.cfg.Handler.Mutate(r.Context(), review.Request)
	if resp == nil {
		resp = &admissionv1.AdmissionResponse{Allowed: false}
	}
	resp.UID = review.Request.UID

	out := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: resp,
	}

	respBytes, err := json.Marshal(out)
	if err != nil {
		log.Printf("[mutating-webhook] encode response: %v", err)
		http.Error(w, "encode response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(respBytes)
}

// loadOrGenerateCA returns the CA, loading it from the configured Secret when
// available and generating (then persisting) a fresh one otherwise.
func loadOrGenerateCA(ctx context.Context, cfg ServerConfig) (*caKeyPair, error) {
	if cfg.CASecretName == "" || cfg.CASecretNamespace == "" {
		ca, err := generateCA()
		if err != nil {
			return nil, fmt.Errorf("generate CA: %w", err)
		}
		log.Printf("[mutating-webhook] using in-memory CA (no Secret persistence)")
		return ca, nil
	}

	secret, err := cfg.Clientset.CoreV1().Secrets(cfg.CASecretNamespace).Get(ctx, cfg.CASecretName, metav1.GetOptions{})
	if err == nil {
		ca, err := loadCAFromSecret(secret)
		if err != nil {
			return nil, fmt.Errorf("load CA from secret %s/%s: %w", cfg.CASecretNamespace, cfg.CASecretName, err)
		}
		log.Printf("[mutating-webhook] loaded CA from secret %s/%s", cfg.CASecretNamespace, cfg.CASecretName)
		return ca, nil
	}

	log.Printf("[mutating-webhook] CA secret %s/%s not found, generating a new CA", cfg.CASecretNamespace, cfg.CASecretName)
	ca, err := generateCA()
	if err != nil {
		return nil, fmt.Errorf("generate CA: %w", err)
	}
	if err := saveCAToSecret(ctx, cfg.Clientset, cfg.CASecretNamespace, cfg.CASecretName, ca, cfg.OwnerReferences); err != nil {
		return nil, err
	}
	return ca, nil
}
