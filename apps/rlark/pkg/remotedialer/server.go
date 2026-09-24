package remotedialer

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	// ErrFailedAuth indicates that a connection request was not authorized.
	ErrFailedAuth = errors.New("failed authentication")
)

// Authorizer authenticates an incoming WebSocket request and returns the key
// used to identify the connected client.
type Authorizer func(req *http.Request) (clientKey string, authed bool, err error)

// ErrorWriter writes an HTTP error response before a WebSocket upgrade.
type ErrorWriter func(rw http.ResponseWriter, req *http.Request, code int, err error)

// DefaultErrorWriter writes the status code followed by the error message.
func DefaultErrorWriter(rw http.ResponseWriter, req *http.Request, code int, err error) {
	rw.WriteHeader(code)
	_, _ = rw.Write([]byte(err.Error()))
}

// Server accepts remote-dialer clients and routes dial requests to their
// active sessions. A Server also implements http.Handler.
type Server struct {
	// PeerID and PeerToken are credentials presented when connecting to peers.
	PeerID    string
	PeerToken string
	// ClientConnectAuthorizer controls which destinations connected clients may dial.
	ClientConnectAuthorizer ConnectAuthorizer
	authorizer              Authorizer
	errorWriter             ErrorWriter
	sessions                *sessionManager
	peers                   map[string]peer
	peerLock                sync.Mutex
}

// New constructs a Server using auth for incoming client authentication and
// errorWriter for HTTP and WebSocket upgrade failures.
func New(auth Authorizer, errorWriter ErrorWriter) *Server {
	return &Server{
		peers:       map[string]peer{},
		authorizer:  auth,
		errorWriter: errorWriter,
		sessions:    newSessionManager(),
	}
}

// ServeHTTP authenticates and upgrades an incoming request, then serves its
// remote-dialer session until the request is canceled or the connection closes.
func (s *Server) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	clientKey, authed, peer, err := s.auth(req)
	if err != nil {
		s.errorWriter(rw, req, 400, err)
		return
	}
	if !authed {
		s.errorWriter(rw, req, 401, ErrFailedAuth)
		return
	}

	logrus.Infof("Handling backend connection request [%s]", clientKey)

	upgrader := websocket.Upgrader{
		HandshakeTimeout: 5 * time.Second,
		CheckOrigin:      func(r *http.Request) bool { return true },
		Error:            s.errorWriter,
	}

	wsConn, err := upgrader.Upgrade(rw, req, nil)
	if err != nil {
		s.errorWriter(rw, req, 400, errors.Wrapf(err, "Error during upgrade for host [%v]", clientKey))
		return
	}

	session := s.sessions.add(clientKey, wsConn, peer)
	session.auth = s.ClientConnectAuthorizer
	defer s.sessions.remove(session)

	code, err := session.Serve(req.Context())
	if err != nil {
		// Hijacked so we can't write to the client
		stage, duration := session.Diagnostics()
		logrus.WithFields(logrus.Fields{
			"clientKey": clientKey,
			"remote":    req.RemoteAddr,
			"duration":  duration,
			"stage":     stage,
			"code":      code,
		}).WithError(err).Info("remotedialer session ended")
	}
}

// AddSession registers a WebSocket connection for clientKey. If peer is true,
// the session is treated as a peer server rather than a directly connected client.
func (s *Server) AddSession(clientKey string, wsConn *websocket.Conn, peer bool) *Session {
	session := s.sessions.add(clientKey, wsConn, peer)
	session.auth = s.ClientConnectAuthorizer
	return session
}

// RemoveSession unregisters and closes session.
func (s *Server) RemoveSession(session *Session) {
	s.sessions.remove(session)
}

// ListClients returns the keys of directly connected clients.
func (s *Server) ListClients() []string {
	return s.sessions.listClients()
}

// ListPeers returns the keys of connected peer servers.
func (s *Server) ListPeers() []string {
	return s.sessions.listPeers()
}

func (s *Server) auth(req *http.Request) (clientKey string, authed, peer bool, err error) {
	id := req.Header.Get(ID)
	token := req.Header.Get(Token)
	if id != "" && token != "" {
		// peer authentication
		s.peerLock.Lock()
		p, ok := s.peers[id]
		s.peerLock.Unlock()

		if ok && p.token == token {
			return id, true, true, nil
		}
	}

	id, authed, err = s.authorizer(req)
	return id, authed, false, err
}
