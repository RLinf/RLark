package iam

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const cookieName = "astribot_session"

type Server struct {
	store        Store
	ttl          time.Duration
	cookieSecure bool
	upstream     map[Role]string
	mu           sync.Mutex
	failures     map[string]failureState
}

type failureState struct {
	count int
	until time.Time
}

func NewServer(store Store, ttl time.Duration, cookieSecure bool, upstream map[Role]string) *Server {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	return &Server{store: store, ttl: ttl, cookieSecure: cookieSecure, upstream: upstream, failures: map[string]failureState{}}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) })
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/iam/login", s.handleLogin)
	mux.HandleFunc("POST /api/iam/logout", s.handleLogout)
	mux.HandleFunc("GET /api/iam/me", s.handleMe)
	mux.HandleFunc("GET /api/iam/session", s.handleSession)
	mux.HandleFunc("POST /api/iam/password", s.handlePassword)
	mux.HandleFunc("GET /api/iam/sessions", s.handleSessions)
	mux.HandleFunc("POST /api/iam/sessions/revoke-others", s.handleRevokeOthers)
	mux.HandleFunc("GET /api/iam/users", s.handleUsers)
	mux.HandleFunc("POST /api/iam/users", s.handleUsers)
	mux.HandleFunc("PUT /api/iam/users/{id}", s.handleUser)
	mux.HandleFunc("DELETE /api/iam/users/{id}", s.handleUser)
	mux.HandleFunc("POST /api/iam/users/{id}/reset-password", s.handleResetPassword)
	mux.HandleFunc("GET /api/iam/audit", s.handleAudit)
	mux.HandleFunc("GET /internal/authorize", s.handleAuthorize)
	mux.HandleFunc("GET /internal/admit", s.handleAdmit)
	return securityHeaders(mux)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if decodeJSON(w, r, &req) != nil {
		writeError(w, 400, "invalid request")
		return
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	ip := clientIP(r)
	if s.limited(username, ip) {
		s.audit(r, "anonymous", "", "login", username, "failure", nil)
		writeError(w, 429, "too many attempts")
		return
	}
	u, err := s.store.Authenticate(r.Context(), username)
	if err != nil && !errors.Is(err, ErrNotFound) {
		writeError(w, 503, "service unavailable")
		return
	}
	if err == nil && u.Disabled {
		s.audit(r, u.Username, u.ID, "login", u.Username, "failure", map[string]any{"reason": "disabled"})
		writeError(w, 403, "account disabled")
		return
	}
	if errors.Is(err, ErrNotFound) || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		s.failed(username, ip)
		actor := username
		if actor == "" {
			actor = "anonymous"
		}
		s.audit(r, actor, "", "login", username, "failure", nil)
		writeError(w, 401, "invalid credentials")
		return
	}
	s.clearFailure(username, ip)
	token, err := randomToken()
	if err != nil {
		writeError(w, 500, "internal error")
		return
	}
	now := time.Now().UTC()
	expires := now.Add(s.ttl)
	hash := tokenHash(token)
	if err = s.store.CreateSession(r.Context(), Session{ID: hash, UserID: u.ID, CreatedAt: now, ExpiresAt: expires, LastSeenAt: now, UserAgent: r.UserAgent(), IP: ip}); err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	if err = s.store.TouchLogin(r.Context(), u.ID, now); err != nil {
		_ = s.store.DeleteSession(r.Context(), hash)
		writeError(w, 503, "service unavailable")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", Expires: expires, MaxAge: int(s.ttl.Seconds()), HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode})
	s.audit(r, u.Username, u.ID, "login", u.Username, "success", nil)
	writeJSON(w, 200, map[string]any{"ok": true, "token": token, "role": u.Role, "expiresAt": expires, "user": publicUser(u)})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	a, code := s.authenticate(r, true)
	if code != 0 {
		writeError(w, code, "unauthorized")
		return
	}
	if err := s.store.DeleteSession(r.Context(), a.session.ID); err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	s.audit(r, a.user.Username, a.user.ID, "logout", a.user.Username, "success", nil)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	a, code := s.authenticate(r, false)
	if code != 0 {
		writeError(w, code, "unauthorized")
		return
	}
	writeJSON(w, 200, map[string]any{"user": publicUser(a.user)})
}

// handleSession gives a console that holds the session cookie but not the token
// in its sessionStorage (a new tab, a bookmark) what it needs to call the API as
// this session again, so the person is not sent back through the sign-in page.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	a, code := s.authenticate(r, false)
	if code != 0 {
		writeError(w, code, "unauthorized")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "token": a.token, "role": a.user.Role, "expiresAt": a.session.ExpiresAt, "user": publicUser(a.user)})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	a, code := s.authenticate(r, false)
	if code != 0 {
		writeError(w, code, "unauthorized")
		return
	}
	sessions, err := s.store.ListSessions(r.Context(), a.user.ID, a.session.ID)
	if err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"sessions": sessions})
}
func (s *Server) handleRevokeOthers(w http.ResponseWriter, r *http.Request) {
	a, code := s.authenticate(r, true)
	if code != 0 {
		writeError(w, code, "unauthorized")
		return
	}
	if err := s.store.DeleteOtherSessions(r.Context(), a.user.ID, a.session.ID); err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	s.audit(r, a.user.Username, a.user.ID, "revoke-sessions", a.user.Username, "success", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	a, code := s.authenticate(r, true)
	if code != 0 {
		writeError(w, code, "unauthorized")
		return
	}
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if decodeJSON(w, r, &req) != nil {
		writeError(w, 400, "invalid request")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(a.user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		s.audit(r, a.user.Username, a.user.ID, "password-change", a.user.Username, "failure", nil)
		writeError(w, 403, "current password is incorrect")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, 400, "password must be at least 8 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		writeError(w, 500, "internal error")
		return
	}
	if err = s.store.SetPassword(r.Context(), a.user.ID, string(hash)); err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	s.audit(r, a.user.Username, a.user.ID, "password-change", a.user.Username, "success", nil)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	a, code := s.requireAdmin(w, r)
	if code != 0 {
		return
	}
	if r.Method == http.MethodGet {
		users, err := s.store.ListUsers(r.Context())
		if err != nil {
			writeError(w, 503, "service unavailable")
			return
		}
		out := make([]User, 0, len(users))
		for _, u := range users {
			out = append(out, publicUser(u))
		}
		writeJSON(w, 200, map[string]any{"users": out})
		return
	}
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Role        Role   `json:"role"`
		Password    string `json:"password"`
		Disabled    bool   `json:"disabled"`
	}
	if decodeJSON(w, r, &req) != nil {
		writeError(w, 400, "invalid request")
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if !validUsername(req.Username) || req.DisplayName == "" || !validRole(req.Role) || !validPassword(req.Password) {
		writeError(w, 400, "invalid user data")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		writeError(w, 500, "internal error")
		return
	}
	id, err := newID("u_")
	if err != nil {
		writeError(w, 500, "internal error")
		return
	}
	u := User{ID: id, Username: req.Username, DisplayName: req.DisplayName, Role: req.Role, Disabled: req.Disabled, PasswordHash: string(hash)}
	if err = s.store.CreateUser(r.Context(), u); errors.Is(err, ErrConflict) {
		s.audit(r, a.user.Username, a.user.ID, "user-create", req.Username, "failure", nil)
		writeError(w, 409, "username already exists")
		return
	} else if err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	s.audit(r, a.user.Username, a.user.ID, "user-create", u.Username, "success", nil)
	u, err = s.store.GetUser(r.Context(), u.ID)
	if err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	writeJSON(w, 201, map[string]any{"user": publicUser(u)})
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	a, code := s.requireAdmin(w, r)
	if code != 0 {
		return
	}
	id := r.PathValue("id")
	target, err := s.store.GetUser(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, 404, "user not found")
		return
	}
	if err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	if r.Method == http.MethodDelete {
		if target.ID == a.user.ID {
			writeError(w, 400, "cannot delete the current user")
			return
		}
		if err = s.store.DeleteUser(r.Context(), id); err != nil {
			writeError(w, 503, "service unavailable")
			return
		}
		s.audit(r, a.user.Username, a.user.ID, "user-delete", target.Username, "success", nil)
		w.WriteHeader(204)
		return
	}
	var req struct {
		DisplayName string `json:"displayName"`
		Role        Role   `json:"role"`
		Disabled    *bool  `json:"disabled"`
	}
	if decodeJSON(w, r, &req) != nil {
		writeError(w, 400, "invalid request")
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" || !validRole(req.Role) {
		writeError(w, 400, "invalid user data")
		return
	}
	disabled := target.Disabled
	if req.Disabled != nil {
		disabled = *req.Disabled
	}
	if target.ID == a.user.ID && (disabled || req.Role != RoleAdmin) {
		writeError(w, 400, "cannot disable or demote the current admin")
		return
	}
	wasDisabled := target.Disabled
	target.DisplayName = req.DisplayName
	target.Role = req.Role
	target.Disabled = disabled
	if err = s.store.UpdateUser(r.Context(), target); err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	action := "user-update"
	if !wasDisabled && disabled {
		action = "user-disable"
	} else if wasDisabled && !disabled {
		action = "user-enable"
	}
	s.audit(r, a.user.Username, a.user.ID, action, target.Username, "success", nil)
	writeJSON(w, 200, map[string]any{"user": publicUser(target)})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	a, code := s.requireAdmin(w, r)
	if code != 0 {
		return
	}
	id := r.PathValue("id")
	target, err := s.store.GetUser(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, 404, "user not found")
		return
	}
	if err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	var req struct {
		NewPassword string `json:"newPassword"`
	}
	if decodeJSON(w, r, &req) != nil || !validPassword(req.NewPassword) {
		writeError(w, 400, "password must be at least 8 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		writeError(w, 500, "internal error")
		return
	}
	if err = s.store.SetPassword(r.Context(), id, string(hash)); err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	s.audit(r, a.user.Username, a.user.ID, "password-reset", target.Username, "success", nil)
	w.WriteHeader(204)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if _, code := s.requireAdmin(w, r); code != 0 {
		return
	}
	limit := 100
	if n := r.URL.Query().Get("limit"); n != "" {
		parsed, err := strconv.Atoi(n)
		if err != nil || parsed < 1 || parsed > 500 {
			writeError(w, 400, "limit must be between 1 and 500")
			return
		}
		limit = parsed
	}
	events, err := s.store.ListAudit(r.Context(), r.URL.Query().Get("actor"), r.URL.Query().Get("action"), r.URL.Query().Get("result"), limit)
	if err != nil {
		writeError(w, 503, "service unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"events": events})
}

// originalRequest is what nginx asks about: the method and the address as the
// browser wrote them (a subrequest's own URI is the internal location's).
func originalRequest(r *http.Request) (method, uri string, ok bool) {
	method, uri = r.Header.Get("X-Original-Method"), r.Header.Get("X-Original-URI")
	return method, uri, method != "" && uri != ""
}

// handleAuthorize answers for a request nginx routes to the Gateway (its /api/
// location): the session, the role's permission for the method and the path,
// then the role's upstream token. nginx picks this endpoint by the location the
// normalized address matched, so an address written another way (//api/…,
// /%61pi/…) is judged as the API call it is, never as a page.
func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	method, uri, ok := originalRequest(r)
	if !ok {
		s.audit(r, "anonymous", "", "authorization", "", "failure", map[string]any{"reason": "missing original request metadata"})
		w.WriteHeader(403)
		return
	}
	a, code := s.authenticate(r, false)
	if code != 0 {
		s.audit(r, "anonymous", "", "authorization", uri, "failure", map[string]any{"method": method})
		w.WriteHeader(code)
		return
	}
	if method != "GET" && method != "HEAD" && r.Header.Get("Authorization") == "" {
		s.audit(r, a.user.Username, a.user.ID, "authorization", uri, "denied", map[string]any{"method": method, "reason": "bearer required"})
		w.WriteHeader(401)
		return
	}
	if !allowed(a.user.Role, method, uri) {
		s.audit(r, a.user.Username, a.user.ID, "authorization", uri, "denied", map[string]any{"method": method})
		w.WriteHeader(403)
		return
	}
	if s.upstream == nil || s.upstream[a.user.Role] == "" {
		w.WriteHeader(503)
		return
	}
	w.Header().Set("X-IAM-Upstream", s.upstream[a.user.Role])
	w.Header().Set("X-IAM-User", a.user.Username)
	w.WriteHeader(200)
}

// handleAdmit answers for one of the console's pages (nginx's other locations).
// The pages are static and each business call authorizes on its own, so a
// session is the whole of it; nothing is written through a page. A page view
// is not audited.
func (s *Server) handleAdmit(w http.ResponseWriter, r *http.Request) {
	method, uri, ok := originalRequest(r)
	if !ok {
		w.WriteHeader(403)
		return
	}
	a, code := s.authenticate(r, false)
	if code != 0 {
		w.WriteHeader(code)
		return
	}
	if method != "GET" && method != "HEAD" {
		s.audit(r, a.user.Username, a.user.ID, "authorization", uri, "denied", map[string]any{"method": method, "reason": "a page is only read"})
		w.WriteHeader(403)
		return
	}
	w.Header().Set("X-IAM-User", a.user.Username)
	w.WriteHeader(200)
}

type authContext struct {
	user    User
	session Session
	token   string
}

func (s *Server) authenticate(r *http.Request, requireBearer bool) (authContext, int) {
	authorization := r.Header.Get("Authorization")
	token, hasBearer := bearer(authorization)
	if requireBearer && !hasBearer {
		return authContext{}, 401
	}
	if authorization == "" && token == "" {
		if c, err := r.Cookie(cookieName); err == nil {
			token = c.Value
		}
	}
	if token == "" {
		return authContext{}, 401
	}
	session, user, err := s.store.GetSession(r.Context(), tokenHash(token), time.Now().UTC())
	if err != nil && !errors.Is(err, ErrNotFound) {
		return authContext{}, 503
	}
	if err != nil || user.Disabled {
		return authContext{}, 401
	}
	return authContext{user: user, session: session, token: token}, 0
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (authContext, int) {
	a, code := s.authenticate(r, true)
	if code != 0 {
		message := "unauthorized"
		if code == 503 {
			message = "service unavailable"
		} else {
			s.audit(r, "anonymous", "", "authorization", r.URL.Path, "denied", nil)
		}
		writeError(w, code, message)
		return authContext{}, code
	}
	if a.user.Role != RoleAdmin {
		writeError(w, 403, "forbidden")
		s.audit(r, a.user.Username, a.user.ID, "authorization", r.URL.Path, "denied", nil)
		return authContext{}, 403
	}
	return a, 0
}

func (s *Server) audit(r *http.Request, actor, actorID, action, target, result string, detail map[string]any) {
	if err := s.store.InsertAudit(r.Context(), AuditEvent{Time: time.Now().UTC(), Actor: actor, ActorID: actorID, Action: action, Target: target, Result: result, IP: clientIP(r), Detail: detail}); err != nil {
		slog.Error("persist IAM audit event", "action", action, "error", err)
	}
}
func (s *Server) CleanupExpiredSessions(ctx context.Context) error {
	return s.store.DeleteExpiredSessions(ctx, time.Now().UTC())
}
func (s *Server) limited(username, ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := username + "\x00" + ip
	state := s.failures[key]
	if _, exists := s.failures[key]; !exists && len(s.failures) >= 10000 {
		return true
	}
	if time.Now().Before(state.until) {
		return true
	}
	if !state.until.IsZero() {
		delete(s.failures, key)
	}
	return false
}
func (s *Server) failed(username, ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := username + "\x00" + ip
	if state, exists := s.failures[key]; exists {
		state.count++
		if state.count >= 5 {
			state.count = 0
			state.until = time.Now().Add(30 * time.Second)
		}
		s.failures[key] = state
		return
	}
	if len(s.failures) >= 10000 {
		now := time.Now()
		for key, state := range s.failures {
			if !state.until.IsZero() && !now.Before(state.until) {
				delete(s.failures, key)
			}
		}
		if len(s.failures) >= 10000 {
			return
		}
	}
	state := s.failures[key]
	state.count++
	if state.count >= 5 {
		state.count = 0
		state.until = time.Now().Add(30 * time.Second)
	}
	s.failures[key] = state
}
func (s *Server) clearFailure(username, ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failures, username+"\x00"+ip)
}
func publicUser(u User) User { u.PasswordHash = ""; return u }
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func bearer(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
func validRole(role Role) bool {
	return role == RoleAdmin || role == RoleOperator || role == RoleViewer
}
func validUsername(v string) bool {
	if len(v) < 3 || len(v) > 32 {
		return false
	}
	for i, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || i > 0 && (c == '_' || c == '-')) {
			return false
		}
	}
	return true
}

func validPassword(value string) bool { return utf8.RuneCountInString(value) >= 8 && len(value) <= 72 }

// clientIP is the address audit records and the login backoff key on. The
// service sits behind the console's nginx in every supported deployment, so
// the proxy's X-Real-IP / X-Forwarded-For name the real client; the socket
// address is only the fallback for direct calls.
func clientIP(r *http.Request) string {
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" && net.ParseIP(real) != nil {
		return real
	}
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		first, _, _ := strings.Cut(forwarded, ",")
		first = strings.TrimSpace(first)
		if net.ParseIP(first) != nil {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
