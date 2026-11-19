package remotedialer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"github.com/xtaci/smux"
)

// Session multiplexes logical connections over a single websocket transport
// using smux. Every logical connection is a smux stream; the first stream of a
// session is reserved as a control channel used to synchronize peer routing
// information (AddClient/RemoveClient).
type Session struct {
	sync.RWMutex

	clientKey  string
	sessionKey int64
	conn       WSConn
	mux        *smux.Session
	initErr    error
	stage      string
	startedAt  time.Time

	// control is the reserved control stream used to exchange peering commands.
	control net.Conn

	remoteClientKeys map[string]map[int]bool

	auth   ConnectAuthorizer
	dialer Dialer
	client bool
}

// ContextKey is the context key type used for remote-dialer diagnostic values.
type ContextKey struct{}

// ContextKeyCaller identifies a context value describing the caller that
// established a session.
var ContextKeyCaller = ContextKey{}

// ValueFromContext returns the caller description stored under ContextKeyCaller,
// or an empty string if the value is absent or not a string.
func ValueFromContext(ctx context.Context) string {
	v := ctx.Value(ContextKeyCaller)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// PrintTunnelData enables debug logging for tunnel routing changes. It defaults
// to true when CATTLE_TUNNEL_DATA_DEBUG is set to "true".
var PrintTunnelData bool

func init() {
	if os.Getenv("CATTLE_TUNNEL_DATA_DEBUG") == "true" {
		PrintTunnelData = true
	}
}

// smuxConfig returns the smux configuration shared by client and server.
// smux natively handles keepalive, flow control (back pressure) and stream
// lifecycle, replacing the previous hand-rolled ping/pause/resume/sync logic.
func smuxConfig() *smux.Config {
	config := smux.DefaultConfig()
	// Version 2 is required: with v1, Stream.WriteTo (used by io.Copy) does not
	// emit window updates to the peer, which deadlocks a full-duplex tunnel once
	// the peer's send window is exhausted. v2 sends updates from WriteTo and
	// also provides fair per-stream scheduling.
	config.Version = 2
	config.KeepAliveInterval = PingWriteInterval
	config.KeepAliveTimeout = PingWaitDuration
	config.MaxReceiveBuffer = MaxBuffer
	config.MaxStreamBuffer = MaxStreamBuffer
	return config
}

// NewClientSession creates a client-side session using the default network dialer.
func NewClientSession(auth ConnectAuthorizer, conn *websocket.Conn) *Session {
	return NewClientSessionWithDialer(auth, conn, nil)
}

// NewClientSessionWithDialer creates a client-side session. auth authorizes
// server-requested connections, and dialer opens them; nil uses net.Dialer.
func NewClientSessionWithDialer(auth ConnectAuthorizer, conn *websocket.Conn, dialer Dialer) *Session {
	s := &Session{
		clientKey:        "client",
		conn:             NewWSConn(conn),
		auth:             auth,
		client:           true,
		dialer:           dialer,
		remoteClientKeys: map[string]map[int]bool{},
		stage:            "websocket-upgraded",
		startedAt:        time.Now(),
	}
	if err := s.initMux(); err != nil {
		s.initErr = err
		s.stage = "initialization-failed"
		_ = s.conn.Close()
		logrus.WithError(err).Warn("failed to initialize smux client session")
	}
	return s
}

// NewServerSession creates a server-side session for clientKey. conn may be nil
// for callers that only use session bookkeeping.
func NewServerSession(sessionKey int64, clientKey string, conn WSConn) *Session {
	s := &Session{
		clientKey:        clientKey,
		sessionKey:       sessionKey,
		conn:             conn,
		remoteClientKeys: map[string]map[int]bool{},
		stage:            "websocket-upgraded",
		startedAt:        time.Now(),
	}
	// conn may be nil in unit tests that only exercise session bookkeeping.
	if conn != nil {
		if err := s.initMux(); err != nil {
			s.initErr = err
			s.stage = "initialization-failed"
			_ = s.conn.Close()
			logrus.WithError(err).Warn("failed to initialize smux server session")
		}
	}
	return s
}

// initMux constructs the smux session and the reserved control stream so the
// Session is ready for OpenStream()/writeControl() before Serve is called.
// Serve will only run the accept loop.
func (s *Session) initMux() error {
	var (
		mux *smux.Session
		err error
	)
	if s.client {
		mux, err = smux.Client(s.conn, smuxConfig())
	} else {
		mux, err = smux.Server(s.conn, smuxConfig())
	}
	if err != nil {
		return err
	}
	s.Lock()
	s.stage = "mux-created"
	s.Unlock()

	// Establish the reserved control stream synchronously. The client opens
	// stream 0, the server accepts it, guaranteeing symmetric wiring before
	// either side starts issuing peering commands.
	type result struct {
		control net.Conn
		err     error
	}
	resultCh := make(chan result, 1)
	go func() {
		var control net.Conn
		var controlErr error
		if s.client {
			control, controlErr = mux.OpenStream()
		} else {
			control, controlErr = mux.AcceptStream()
		}
		resultCh <- result{control: control, err: controlErr}
	}()

	var control net.Conn
	select {
	case result := <-resultCh:
		control, err = result.control, result.err
	case <-time.After(ControlStreamTimeout):
		_ = mux.Close()
		return fmt.Errorf("control stream handshake timed out after %s", ControlStreamTimeout)
	}
	if err != nil {
		_ = mux.Close()
		return fmt.Errorf("control stream handshake: %w", err)
	}

	s.Lock()
	s.mux = mux
	s.control = control
	if s.client {
		s.stage = "control-opened"
	} else {
		s.stage = "control-accepted"
	}
	s.Unlock()
	return nil
}

// addSessionKey registers a new session key for a given client key
func (s *Session) addSessionKey(clientKey string, sessionKey int) {
	s.Lock()
	defer s.Unlock()

	keys := s.remoteClientKeys[clientKey]
	if keys == nil {
		keys = map[int]bool{}
		s.remoteClientKeys[clientKey] = keys
	}
	keys[sessionKey] = true
}

// removeSessionKey removes a specific session key for a client key
func (s *Session) removeSessionKey(clientKey string, sessionKey int) {
	s.Lock()
	defer s.Unlock()

	keys := s.remoteClientKeys[clientKey]
	delete(keys, sessionKey)
	if len(keys) == 0 {
		delete(s.remoteClientKeys, clientKey)
	}
}

// getSessionKeys retrieves all session keys for a given client key
func (s *Session) getSessionKeys(clientKey string) map[int]bool {
	s.RLock()
	defer s.RUnlock()
	return s.remoteClientKeys[clientKey]
}

// clientKeyWithPrefix returns a remote client key that starts with prefix.
func (s *Session) clientKeyWithPrefix(prefix string) (string, bool) {
	s.RLock()
	defer s.RUnlock()
	for k, keys := range s.remoteClientKeys {
		if strings.HasPrefix(k, prefix) && len(keys) > 0 {
			return k, true
		}
	}
	return "", false
}

// Serve accepts and handles logical connections for the lifetime of the session.
// It returns an HTTP-like status code describing why serving stopped.
func (s *Session) Serve(ctx context.Context) (int, error) {
	s.RLock()
	mux := s.mux
	control := s.control
	initErr := s.initErr
	s.RUnlock()

	if initErr != nil {
		return 500, initErr
	}
	if mux == nil {
		return 400, errors.New("session not initialized")
	}
	defer func() { _ = mux.Close() }()
	s.Lock()
	s.stage = "serving"
	s.Unlock()

	if control != nil {
		go s.serveControl(control)
	}

	for {
		select {
		case <-ctx.Done():
			return 200, ctx.Err()
		default:
		}

		stream, err := mux.AcceptStream()
		if err != nil {
			if mux.IsClosed() {
				return 200, nil
			}
			return 500, err
		}
		go s.handleStream(ctx, stream)
	}
}

// handleStream reads the connect header from a new stream, dials the requested
// target and pipes data in both directions. Back pressure and stream teardown
// are handled by smux.
func (s *Session) handleStream(ctx context.Context, stream *smux.Stream) {
	proto, address, err := readConnectHeader(stream)
	if err != nil {
		logrus.WithError(err).Debug("failed to read connect header")
		_ = stream.Close()
		return
	}

	if s.auth == nil || !s.auth(proto, address) {
		logrus.Debugf("connect not allowed for %s/%s", proto, address)
		_ = stream.Close()
		return
	}

	clientDial(ctx, s.dialer, stream, proto, address)
}

// Dial opens a logical connection to network proto and address through the
// remote end of the session.
func (s *Session) Dial(ctx context.Context, proto, address string) (net.Conn, error) {
	return s.serverConnectContext(ctx, proto, address)
}

func (s *Session) serverConnectContext(ctx context.Context, proto, address string) (net.Conn, error) {
	s.RLock()
	mux := s.mux
	s.RUnlock()
	if mux == nil {
		return nil, errors.New("session not serving")
	}

	stream, err := mux.OpenStream()
	if err != nil {
		return nil, err
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = stream.SetWriteDeadline(deadline)
	}

	if err := writeConnectHeader(stream, proto, address); err != nil {
		_ = stream.Close()
		return nil, err
	}
	_ = stream.SetWriteDeadline(time.Time{})

	return stream, nil
}

// Close closes the control stream, multiplexed session, and WebSocket transport.
func (s *Session) Close() {
	s.Lock()
	mux := s.mux
	control := s.control
	s.stage = "closed"
	s.Unlock()

	if control != nil {
		_ = control.Close()
	}
	if mux != nil {
		_ = mux.Close()
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
}

// Diagnostics reports the current lifecycle stage and session age.
func (s *Session) Diagnostics() (string, time.Duration) {
	s.RLock()
	defer s.RUnlock()
	return s.stage, time.Since(s.startedAt)
}

// sessionAdded notifies the remote end that a client became reachable through this session.
func (s *Session) sessionAdded(clientKey string, sessionKey int64) {
	client := fmt.Sprintf("%s/%d", clientKey, sessionKey)
	if err := s.writeControl(AddClient, client); err != nil {
		s.Close()
	}
}

// sessionRemoved notifies the remote end that a client is no longer reachable through this session.
func (s *Session) sessionRemoved(clientKey string, sessionKey int64) {
	client := fmt.Sprintf("%s/%d", clientKey, sessionKey)
	if err := s.writeControl(RemoveClient, client); err != nil {
		s.Close()
	}
}

func parseAddress(address string) (string, int, error) {
	parts := strings.SplitN(address, "/", 2)
	if len(parts) != 2 {
		return "", 0, errors.New("not / separated")
	}
	v, err := strconv.Atoi(parts[1])
	return parts[0], v, err
}

// connect header helpers -----------------------------------------------------

// The connect header is a single length-prefixed line: "<proto>/<address>\n".
func writeConnectHeader(w io.Writer, proto, address string) error {
	_, err := io.WriteString(w, fmt.Sprintf("%s/%s\n", proto, address))
	return err
}

func readConnectHeader(r io.Reader) (proto, address string, err error) {
	line, err := readLine(r)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(line, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("failed to parse connect header %q", line)
	}
	return parts[0], parts[1], nil
}

// readLine reads a single '\n' terminated line byte by byte, so no extra bytes
// belonging to the stream payload are consumed.
func readLine(r io.Reader) (string, error) {
	var (
		buf [1]byte
		sb  strings.Builder
	)
	for sb.Len() < 512 {
		n, err := r.Read(buf[:])
		if n > 0 {
			if buf[0] == '\n' {
				return sb.String(), nil
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			if err == io.EOF && sb.Len() > 0 {
				return sb.String(), nil
			}
			return "", err
		}
	}
	return "", errors.New("connect header too long")
}
