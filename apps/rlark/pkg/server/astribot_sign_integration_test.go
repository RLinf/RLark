package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

func TestSignAgentCertificateRequiresAdminMTLS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ca, err := cert.GenerateCA(cert.GenerateTemplateCA())
	if err != nil {
		t.Fatal(err)
	}
	admin, err := cert.Sign(ca, "x509", map[string]string{apis.MetaPermissionAdmin: "true"})
	if err != nil {
		t.Fatal(err)
	}
	nonadmin, err := cert.Sign(ca, "x509", map[string]string{apis.MetaCertRole: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Config{})
	srv.ca = []cert.Data{*ca}
	router := gin.New()
	router.POST("/api/sign", srv.handleCertCheck, srv.handleSignCertificate)
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	testServer := httptest.NewUnstartedServer(router)
	testServer.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool, MinVersion: tls.VersionTLS12}
	testServer.StartTLS()
	defer testServer.Close()
	request := func(identity *cert.Data) (int, []byte) {
		transport := testServer.Client().Transport.(*http.Transport).Clone()
		if identity != nil {
			pair, e := tls.X509KeyPair(identity.CertPEM, identity.KeyPEM)
			if e != nil {
				t.Fatal(e)
			}
			transport.TLSClientConfig.Certificates = []tls.Certificate{pair}
		}
		client := &http.Client{Transport: transport}
		defer transport.CloseIdleConnections()
		resp, e := client.Post(testServer.URL+"/api/sign", "application/json", bytes.NewBufferString(`{"role":"agent","client_id":"isolated"}`))
		if e != nil {
			if identity == nil {
				return 0, nil
			}
			t.Fatal(e)
		}
		defer func() { _ = resp.Body.Close() }()
		var body bytes.Buffer
		if _, e = body.ReadFrom(resp.Body); e != nil {
			t.Fatal(e)
		}
		return resp.StatusCode, body.Bytes()
	}
	if status, _ := request(nil); status != 0 {
		t.Fatalf("missing client certificate accepted: %d", status)
	}
	if status, _ := request(nonadmin); status != http.StatusForbidden {
		t.Fatalf("non-admin status %d", status)
	}
	status, body := request(admin)
	if status != http.StatusOK {
		t.Fatalf("admin status %d", status)
	}
	var issued SignResponse
	if err := json.Unmarshal(body, &issued); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair([]byte(issued.CertPEM), []byte(issued.KeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = leaf.CheckSignatureFrom(ca.Cert); err != nil {
		t.Fatal(err)
	}
	meta, ok := cert.GetX509CertMeta(leaf)
	if !ok {
		t.Fatal("issued certificate has no metadata")
	}
	if meta[apis.MetaAgentID] != "isolated" || meta[apis.MetaCertRole] != "agent" {
		t.Fatalf("unexpected identity metadata: %v", meta)
	}
}

func TestRevocationRequiresPersistentStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := NewServer(Config{})
	router := gin.New()
	router.POST("/revoke", func(c *gin.Context) {
		c.Set(ContextKeyCertMeta, map[string]string{apis.MetaPermissionAdmin: "true"})
		c.Next()
	}, s.handleRevokeCertificate)
	request := httptest.NewRequest(http.MethodPost, "/revoke", bytes.NewBufferString(`{"cert_type":"x509","serial_number":"123","subject_key_id":"abcd"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("revoke status %d", response.Code)
	}
	if revoked, err := s.checkCertRevoked(context.Background(), "x509", "123", "abcd"); err != nil || revoked {
		t.Fatal("unexpected in-memory revocation")
	}
}

type testRevocationStore struct {
	revoked bool
	err     error
	calls   int
	addErr  error
	added   int
	last    *db.RevokedCertificateModel
}

func (s *testRevocationStore) AddRevokedCertificate(_ context.Context, model *db.RevokedCertificateModel) error {
	s.added++
	if s.addErr == nil {
		s.revoked = true
		s.last = model
	}
	return s.addErr
}
func (s *testRevocationStore) IsCertificateRevoked(context.Context, string, string, string) (bool, error) {
	s.calls++
	return s.revoked, s.err
}

func TestRevocationStoreFailureAndRecovery(t *testing.T) {
	s := NewServer(Config{})
	store := &testRevocationStore{}
	s.rcStore = store
	revoked, err := s.checkCertRevoked(context.Background(), "x509", "123", "abc")
	if err != nil || revoked {
		t.Fatalf("initial lookup: %v %v", revoked, err)
	}
	store.err = errors.New("database unavailable")
	if _, err = s.checkCertRevoked(context.Background(), "x509", "123", "abc"); err == nil {
		t.Fatal("cached prior allow or ignored database failure")
	}
	store.err = nil
	store.revoked = true
	revoked, err = s.checkCertRevoked(context.Background(), "x509", "123", "abc")
	if err != nil || !revoked {
		t.Fatalf("recovery: %v %v", revoked, err)
	}
}

func TestCertificateCheckFailsClosedWhenStoreUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := NewServer(Config{})
	s.rcStore = &testRevocationStore{err: errors.New("database unavailable")}
	r := gin.New()
	r.GET("/", s.handleCertCheck, func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{SerialNumber: big.NewInt(1)}}}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", response.Code)
	}
}

func TestRevokeValidatesIdentityAndPersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := NewServer(Config{})
	store := &testRevocationStore{}
	s.rcStore = store
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		c.Set(ContextKeyCertMeta, map[string]string{apis.MetaPermissionAdmin: "true"})
		c.Next()
	}, s.handleRevokeCertificate)
	send := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	for _, body := range []string{`{}`, `{"cert_type":"x509","serial_number":"0123","subject_key_id":"abcd"}`, `{"cert_type":"x509","serial_number":"123","subject_key_id":"ABCD"}`, `{"cert_type":"ssh","serial_number":"-1","subject_key_id":"key"}`} {
		if status := send(body); status != http.StatusBadRequest {
			t.Fatalf("invalid request status %d", status)
		}
	}
	if store.added != 0 {
		t.Fatal("invalid request reached storage")
	}
	body := `{"cert_type":"x509","serial_number":"123","subject_key_id":"abcd"}`
	store.addErr = errors.New("unavailable")
	if status := send(body); status != http.StatusInternalServerError {
		t.Fatalf("storage failure status %d", status)
	}
	if revoked, _ := s.checkCertRevoked(context.Background(), "x509", "123", "abcd"); revoked {
		t.Fatal("failed write cached revocation")
	}
	store.addErr = nil
	if status := send(body); status != http.StatusOK {
		t.Fatalf("successful write status %d", status)
	}
	if revoked, err := s.checkCertRevoked(context.Background(), "x509", "123", "abcd"); err != nil || !revoked {
		t.Fatal("successful write not cached")
	}
}

func TestRevocationMatchesIssuedCertificateIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ca, err := cert.GenerateCA(cert.GenerateTemplateCA())
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"x509", "ssh"} {
		t.Run(kind, func(t *testing.T) {
			issued, err := cert.Sign(ca, kind, map[string]string{apis.MetaCertRole: "agent"})
			if err != nil {
				t.Fatal(err)
			}
			var serial, keyID string
			if kind == "x509" {
				serial = issued.Cert.SerialNumber.String()
				keyID = hex.EncodeToString(issued.Cert.SubjectKeyId)
			} else {
				serial = strconv.FormatUint(issued.SSHCert.Serial, 10)
				keyID = issued.SSHCert.KeyId
			}
			s := NewServer(Config{})
			store := &testRevocationStore{}
			s.rcStore = store
			r := gin.New()
			r.POST("/", func(c *gin.Context) {
				c.Set(ContextKeyCertMeta, map[string]string{apis.MetaPermissionAdmin: "true"})
				c.Next()
			}, s.handleRevokeCertificate)
			body, err := json.Marshal(RevokeCertRequest{CertType: kind, SerialNumber: serial, SubjectKeyID: keyID})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("revoke status %d", w.Code)
			}
			if store.last == nil || store.last.CertType != kind || store.last.SerialNumber != serial || store.last.SubjectKeyID != keyID {
				t.Fatal("stored identity does not match issued certificate")
			}
			other := NewServer(Config{})
			other.rcStore = store
			revoked, err := other.checkCertRevoked(context.Background(), kind, serial, keyID)
			if err != nil || !revoked {
				t.Fatalf("issued identity not recognized: %v %v", revoked, err)
			}
		})
	}
}

func TestRevocationOverMTLSAcrossServerInstances(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ca, err := cert.GenerateCA(cert.GenerateTemplateCA())
	if err != nil {
		t.Fatal(err)
	}
	admin, err := cert.Sign(ca, "x509", map[string]string{apis.MetaPermissionAdmin: "true"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := cert.Sign(ca, "x509", map[string]string{apis.MetaCertRole: "agent", apis.MetaAgentID: "isolated"})
	if err != nil {
		t.Fatal(err)
	}
	store := &testRevocationStore{}
	start := func() *httptest.Server {
		s := NewServer(Config{})
		s.rcStore = store
		r := gin.New()
		r.POST("/revoke", s.handleCertCheck, s.handleRevokeCertificate)
		r.GET("/check", s.handleCertCheck, func(c *gin.Context) { c.Status(http.StatusOK) })
		pool := x509.NewCertPool()
		pool.AddCert(ca.Cert)
		ts := httptest.NewUnstartedServer(r)
		ts.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool, MinVersion: tls.VersionTLS12}
		ts.StartTLS()
		t.Cleanup(ts.Close)
		return ts
	}
	first, second := start(), start()
	call := func(ts *httptest.Server, identity *cert.Data, method, path string, body []byte) int {
		transport := ts.Client().Transport.(*http.Transport).Clone()
		pair, e := tls.X509KeyPair(identity.CertPEM, identity.KeyPEM)
		if e != nil {
			t.Fatal(e)
		}
		transport.TLSClientConfig.Certificates = []tls.Certificate{pair}
		client := &http.Client{Transport: transport}
		defer transport.CloseIdleConnections()
		req, e := http.NewRequest(method, ts.URL+path, bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	if got := call(second, agent, http.MethodGet, "/check", nil); got != http.StatusOK {
		t.Fatalf("initial status %d", got)
	}
	body, err := json.Marshal(RevokeCertRequest{CertType: "x509", SerialNumber: agent.Cert.SerialNumber.String(), SubjectKeyID: hex.EncodeToString(agent.Cert.SubjectKeyId)})
	if err != nil {
		t.Fatal(err)
	}
	if got := call(first, admin, http.MethodPost, "/revoke", body); got != http.StatusOK {
		t.Fatalf("revoke status %d", got)
	}
	if got := call(second, agent, http.MethodGet, "/check", nil); got != http.StatusUnauthorized {
		t.Fatalf("revoked status %d", got)
	}
	store.revoked = false
	store.err = errors.New("store unavailable")
	third := start()
	if got := call(third, agent, http.MethodGet, "/check", nil); got != http.StatusServiceUnavailable {
		t.Fatalf("unavailable status %d", got)
	}
}
