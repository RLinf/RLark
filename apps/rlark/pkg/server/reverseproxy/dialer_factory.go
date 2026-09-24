package reverseproxy

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/remotedialer"
)

// Variables used by the package.
var (
	ClientKeyHeader = "X-API-Tunnel-Client-Key"
	PeerIDHeader    = remotedialer.ID
	PeerTokenHeader = remotedialer.Token
)

const maxRejections = 5

type clientState struct {
	connections int
	rejections  int
}

// DialerFactory wraps a remotedialer.Server and adds a soft load-balancing
// heuristic: when a clientKey already has an active session the factory
// rejects the first maxRejections reconnection attempts so that the agent's
// retries may land on a different Server instance. After the threshold it
// falls back to allowing multiple sessions for the same key.
type DialerFactory struct {
	dialerServer *remotedialer.Server

	clients map[string]*clientState
	mu      sync.Mutex
}

// NewDialerFactory creates a new DialerFactory.
func NewDialerFactory() *DialerFactory {
	f := &DialerFactory{
		clients: make(map[string]*clientState),
	}
	f.dialerServer = remotedialer.New(f.auth, remotedialer.DefaultErrorWriter)
	f.dialerServer.PeerID = uuid.NewString()
	f.dialerServer.PeerToken = uuid.NewString()
	return f
}

func (f *DialerFactory) auth(req *http.Request) (string, bool, error) {
	clientKey := req.Header.Get(ClientKeyHeader)
	if clientKey == "" {
		return "", false, fmt.Errorf("invalid client key")
	}
	return clientKey, true, nil
}

// tryAccept implements a soft load-balancing heuristic: when a clientKey
// already has an active session the factory rejects the first maxRejections
// reconnection attempts, so that agent retries may land on a different
// Server instance and connections are distributed across replicas.
// After the threshold it falls back to allowing multiple sessions for the
// same key.
func (f *DialerFactory) tryAccept(clientKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	state := f.clients[clientKey]
	if state == nil {
		state = &clientState{}
		f.clients[clientKey] = state
	}
	if state.connections > 0 && state.rejections < maxRejections {
		state.rejections++
		return fmt.Errorf("client %s already connected", clientKey)
	}
	state.connections++
	state.rejections = 0
	return nil
}

func (f *DialerFactory) disconnect(clientKey string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	state := f.clients[clientKey]
	if state == nil {
		return
	}
	state.connections--
	state.rejections = 0
	if state.connections == 0 {
		delete(f.clients, clientKey)
	}
}

// ServeHTTP serves HTTP requests.
func (f *DialerFactory) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	logger := log.GetLogger()
	if req.Header.Get(PeerIDHeader) != "" && req.Header.Get(PeerTokenHeader) != "" {
		f.dialerServer.ServeHTTP(rw, req)
		return
	}

	clientKey, _, err := f.auth(req)
	if err != nil {
		remotedialer.DefaultErrorWriter(rw, req, http.StatusBadRequest, err)
		return
	}

	if err := f.tryAccept(clientKey); err != nil {
		remotedialer.DefaultErrorWriter(rw, req, http.StatusInternalServerError, err)
		return
	}
	defer f.disconnect(clientKey)

	logger.Info("Client connected", "clientKey", clientKey)

	f.dialerServer.ServeHTTP(rw, req)
}

// GetDialer returns the dialer for the given clientKey, or an error if no
// active session exists.
func (f *DialerFactory) GetDialer(ctx context.Context, clientKey string) (remotedialer.Dialer, error) {
	return f.dialerServer.GetDialer(clientKey)
}

// GetPeerID returns the peerID.
func (f *DialerFactory) GetPeerID() string {
	return f.dialerServer.PeerID
}

// GetPeerToken returns the peerToken.
func (f *DialerFactory) GetPeerToken() string {
	return f.dialerServer.PeerToken
}

// AddPeer adds the peer.
func (f *DialerFactory) AddPeer(server, peerID, peerToken string) {
	f.dialerServer.AddPeer(server, peerID, peerToken)
}

// RemovePeer removes the peer.
func (f *DialerFactory) RemovePeer(peerID string) {
	f.dialerServer.RemovePeer(peerID)
}

// SetPeerHeaders sets the peerHeaders.
func SetPeerHeaders(req *http.Request, peerID, peerToken string) {
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Del(ClientKeyHeader)
	req.Header.Del(PeerIDHeader)
	req.Header.Del(PeerTokenHeader)

	req.Header.Set(PeerIDHeader, peerID)
	req.Header.Set(PeerTokenHeader, peerToken)
}

// SetClientHeader sets the clientHeader.
func SetClientHeader(req *http.Request, clientKey string) {
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Del(ClientKeyHeader)
	req.Header.Del(PeerIDHeader)
	req.Header.Del(PeerTokenHeader)

	req.Header.Set(ClientKeyHeader, clientKey)
}
