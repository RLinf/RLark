package iam

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type memoryStore struct {
	mu       sync.Mutex
	users    map[string]User
	byName   map[string]string
	sessions map[string]Session
	events   []AuditEvent
}

func newMemoryStore() *memoryStore {
	hash, _ := bcrypt.GenerateFromPassword([]byte("astribot"), 12)
	return &memoryStore{users: map[string]User{"u_admin": {ID: "u_admin", Username: "admin", DisplayName: "Admin", Role: RoleAdmin, PasswordHash: string(hash)}}, byName: map[string]string{"admin": "u_admin"}, sessions: map[string]Session{}}
}
func (m *memoryStore) Authenticate(_ context.Context, name string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[m.byName[name]]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}
func (m *memoryStore) GetUser(_ context.Context, id string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}
func (m *memoryStore) ListUsers(context.Context) ([]User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []User{}
	for _, u := range m.users {
		out = append(out, u)
	}
	return out, nil
}
func (m *memoryStore) CreateUser(_ context.Context, u User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byName[u.Username]; ok {
		return ErrConflict
	}
	m.users[u.ID] = u
	m.byName[u.Username] = u.ID
	return nil
}
func (m *memoryStore) UpdateUser(_ context.Context, u User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.ID]; !ok {
		return ErrNotFound
	}
	m.users[u.ID] = u
	if u.Disabled {
		for id, s := range m.sessions {
			if s.UserID == u.ID {
				delete(m.sessions, id)
			}
		}
	}
	return nil
}
func (m *memoryStore) DeleteUser(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return ErrNotFound
	}
	delete(m.users, id)
	delete(m.byName, u.Username)
	for sid, s := range m.sessions {
		if s.UserID == id {
			delete(m.sessions, sid)
		}
	}
	return nil
}
func (m *memoryStore) SetPassword(_ context.Context, id, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return ErrNotFound
	}
	u.PasswordHash = hash
	m.users[id] = u
	for sid, s := range m.sessions {
		if s.UserID == id {
			delete(m.sessions, sid)
		}
	}
	return nil
}
func (m *memoryStore) TouchLogin(context.Context, string, time.Time) error { return nil }
func (m *memoryStore) CreateSession(_ context.Context, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	return nil
}
func (m *memoryStore) GetSession(_ context.Context, id string, now time.Time) (Session, User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || !now.Before(s.ExpiresAt) {
		delete(m.sessions, id)
		return Session{}, User{}, ErrNotFound
	}
	u, ok := m.users[s.UserID]
	if !ok {
		return Session{}, User{}, ErrNotFound
	}
	return s, u, nil
}
func (m *memoryStore) DeleteSession(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	return nil
}
func (m *memoryStore) DeleteUserSessions(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for sid, s := range m.sessions {
		if s.UserID == id {
			delete(m.sessions, sid)
		}
	}
	return nil
}
func (m *memoryStore) ListSessions(_ context.Context, userID, currentID string) ([]SessionView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []SessionView{}
	for _, s := range m.sessions {
		if s.UserID == userID {
			out = append(out, SessionView{ID: s.ID, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt, LastSeenAt: s.LastSeenAt, UserAgent: s.UserAgent, IP: s.IP, Current: s.ID == currentID})
		}
	}
	return out, nil
}
func (m *memoryStore) DeleteOtherSessions(_ context.Context, userID, currentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		if s.UserID == userID && id != currentID {
			delete(m.sessions, id)
		}
	}
	return nil
}
func (m *memoryStore) DeleteExpiredSessions(context.Context, time.Time) error { return nil }
func (m *memoryStore) InsertAudit(_ context.Context, e AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, e)
	return nil
}
func (m *memoryStore) ListAudit(context.Context, string, string, string, int) ([]AuditEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]AuditEvent(nil), m.events...), nil
}

func TestLoginOpaqueSessionAndSessionEndpoints(t *testing.T) {
	store := newMemoryStore()
	handler := NewServer(store, time.Hour, false, map[Role]string{RoleAdmin: "upstream-secret"}).Handler()
	login := httptest.NewRequest(http.MethodPost, "/api/iam/login", strings.NewReader(`{"username":"ADMIN","password":"astribot"}`))
	login.RemoteAddr = "192.0.2.5:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, login)
	if response.Code != 200 {
		t.Fatalf("login status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Token string         `json:"token"`
		User  map[string]any `json:"user"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Token) != 64 {
		t.Fatalf("opaque token has length %d", len(body.Token))
	}
	if _, ok := body.User["passwordHash"]; ok {
		t.Fatal("user response exposed password hash")
	}
	if strings.Contains(response.Body.String(), "upstream-secret") {
		t.Fatal("login response exposed upstream token")
	}
	if got := response.Result().Cookies()[0]; got.Name != cookieName || !got.HttpOnly || got.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie attributes: %#v", got)
	}
	call := func(path string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+body.Token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}
	if call("/api/iam/sessions") != 200 {
		t.Fatal("sessions endpoint rejected current user")
	}
	revoke := httptest.NewRequest(http.MethodPost, "/api/iam/sessions/revoke-others", nil)
	revoke.Header.Set("Authorization", "Bearer "+body.Token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, revoke)
	if rr.Code != 204 {
		t.Fatalf("revoke others status=%d", rr.Code)
	}
	if _, _, err := store.GetSession(context.Background(), tokenHash(body.Token), time.Now()); err != nil {
		t.Fatalf("revoke-others removed current session: %v", err)
	}
}

func TestLoginFailureAndExpiredSession(t *testing.T) {
	store := newMemoryStore()
	handler := NewServer(store, time.Hour, false, nil).Handler()
	bad := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, bad)
	if rr.Code != 401 {
		t.Fatalf("bad login status=%d", rr.Code)
	}
	store.sessions["expired"] = Session{ID: "expired", UserID: "u_admin", ExpiresAt: time.Now().Add(-time.Minute)}
	req := httptest.NewRequest(http.MethodGet, "/api/iam/me", nil)
	req.Header.Set("Authorization", "Bearer expired")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("expired session status=%d", rr.Code)
	}
}

func TestDisableAndPasswordChangeRevokeSessions(t *testing.T) {
	store := newMemoryStore()
	store.sessions["sid"] = Session{ID: "sid", UserID: "u_admin", ExpiresAt: time.Now().Add(time.Hour)}
	hash, _ := bcrypt.GenerateFromPassword([]byte("newpassword"), 12)
	if err := store.SetPassword(context.Background(), "u_admin", string(hash)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.GetSession(context.Background(), "sid", time.Now()); err == nil {
		t.Fatal("password change left session valid")
	}
	store.sessions["sid2"] = Session{ID: "sid2", UserID: "u_admin", ExpiresAt: time.Now().Add(time.Hour)}
	u, _ := store.GetUser(context.Background(), "u_admin")
	u.Disabled = true
	if err := store.UpdateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.GetSession(context.Background(), "sid2", time.Now()); err == nil {
		t.Fatal("disable left session valid")
	}
}

func TestUsernameUniqueness(t *testing.T) {
	store := newMemoryStore()
	u := User{ID: "u_two", Username: "admin"}
	if err := store.CreateUser(context.Background(), u); err != ErrConflict {
		t.Fatalf("duplicate username error=%v", err)
	}
	if validUsername("Upper") || validUsername("x") || !validUsername("user_2") {
		t.Fatal("username validation did not enforce canonical lowercase form")
	}
}

func TestAuthorizeHeadersFailClosedAndMissingTokens(t *testing.T) {
	store := newMemoryStore()
	operatorHash, _ := bcrypt.GenerateFromPassword([]byte("operator-password"), 12)
	store.users["u_operator"] = User{ID: "u_operator", Username: "operator", DisplayName: "Operator", Role: RoleOperator, PasswordHash: string(operatorHash)}
	store.byName["operator"] = "u_operator"
	server := NewServer(store, time.Hour, false, map[Role]string{RoleAdmin: "gateway-secret"})
	handler := server.Handler()
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"astribot"}`))
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	var loginBody struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &loginBody); err != nil {
		t.Fatal(err)
	}
	operatorLogin := httptest.NewRequest(http.MethodPost, "/api/iam/login", strings.NewReader(`{"username":"operator","password":"operator-password"}`))
	operatorLoginResponse := httptest.NewRecorder()
	handler.ServeHTTP(operatorLoginResponse, operatorLogin)
	var operatorBody struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(operatorLoginResponse.Body.Bytes(), &operatorBody); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/internal/authorize", nil)
	request.Header.Set("X-Original-Method", "GET")
	request.Header.Set("X-Original-URI", "/api/v1/rlinf.io/v1alpha1/jobs")
	request.Header.Set("Authorization", "Bearer "+loginBody.Token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || response.Header().Get("X-IAM-Upstream") != "gateway-secret" || response.Header().Get("X-IAM-User") != "admin" {
		t.Fatalf("authorization response=%d headers=%v", response.Code, response.Header())
	}
	for _, path := range []string{"/api/v1/unknown", "/api/v1/a/../b", "/api/v1/a//b"} {
		request := httptest.NewRequest(http.MethodGet, "/internal/authorize", nil)
		request.Header.Set("X-Original-Method", "GET")
		request.Header.Set("X-Original-URI", path)
		request.Header.Set("Authorization", "Bearer "+operatorBody.Token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 403 {
			t.Errorf("path %q status=%d, want 403", path, response.Code)
		}
	}
	missing := NewServer(store, time.Hour, false, nil).Handler()
	missingLogin := httptest.NewRequest(http.MethodPost, "/api/iam/login", strings.NewReader(`{"username":"admin","password":"astribot"}`))
	missingLoginResponse := httptest.NewRecorder()
	missing.ServeHTTP(missingLoginResponse, missingLogin)
	if missingLoginResponse.Code != 200 {
		t.Fatalf("login with missing upstream tokens status=%d", missingLoginResponse.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/internal/authorize", nil)
	request.Header.Set("X-Original-Method", "GET")
	request.Header.Set("X-Original-URI", "/api/v1/rlinf.io/v1alpha1/jobs")
	request.Header.Set("Authorization", "Bearer "+loginBody.Token)
	response = httptest.NewRecorder()
	missing.ServeHTTP(response, request)
	if response.Code != 503 {
		t.Fatalf("missing upstream credentials status=%d, want 503", response.Code)
	}
}
