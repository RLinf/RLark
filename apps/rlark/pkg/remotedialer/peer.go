package remotedialer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

var (
	// Token is the HTTP header carrying a peer authentication token.
	Token = "X-API-Tunnel-Token"
	// ID is the HTTP header carrying a peer identifier.
	ID = "X-API-Tunnel-ID"
)

// AddPeer configures and starts a persistent connection to another server.
// An existing peer with the same ID is replaced when its configuration differs.
// The call has no effect unless PeerID and PeerToken are configured.
func (s *Server) AddPeer(url, id, token string) {
	if s.PeerID == "" || s.PeerToken == "" {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	peer := peer{
		url:    url,
		id:     id,
		token:  token,
		cancel: cancel,
	}

	logrus.Infof("Adding peer %s, %s", url, id)

	s.peerLock.Lock()
	defer s.peerLock.Unlock()

	if p, ok := s.peers[id]; ok {
		if p.equals(peer) {
			return
		}
		p.cancel()
	}

	s.peers[id] = peer
	go peer.start(ctx, s)
}

// RemovePeer stops and removes the peer identified by id.
func (s *Server) RemovePeer(id string) {
	s.peerLock.Lock()
	defer s.peerLock.Unlock()

	if p, ok := s.peers[id]; ok {
		logrus.Infof("Removing peer %s", id)
		p.cancel()
	}
	delete(s.peers, id)
}

type peer struct {
	url, id, token string
	cancel         func()
}

func (p peer) equals(other peer) bool {
	return p.url == other.url &&
		p.id == other.id &&
		p.token == other.token
}

func (p *peer) start(ctx context.Context, s *Server) {
	headers := http.Header{
		ID:    {s.PeerID},
		Token: {s.PeerToken},
	}

	dialer := &websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		HandshakeTimeout: HandshakeTimeOut,
	}
	ctx = context.WithValue(ctx, ContextKeyCaller, fmt.Sprintf("Peer url:%s, id:%s", p.url, p.id))

outer:
	for {
		select {
		case <-ctx.Done():
			break outer
		default:
		}

		ws, _, err := dialer.Dial(p.url, headers)
		if err != nil {
			logrus.Errorf("Failed to connect to peer %s [local ID=%s]: %v", p.url, s.PeerID, err)
			time.Sleep(5 * time.Second)
			continue
		}

		session := NewClientSession(func(string, string) bool { return true }, ws)
		session.dialer = func(ctx context.Context, network, address string) (net.Conn, error) {
			parts := strings.SplitN(network, "::", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid clientKey/proto: %s", network)
			}
			d := s.Dialer(parts[0])
			return d(ctx, parts[1], address)
		}

		s.sessions.addListener(session)
		_, err = session.Serve(ctx)
		s.sessions.removeListener(session)
		session.Close()

		if err != nil {
			logrus.Errorf("Failed to serve peer connection %s: %v", p.id, err)
		}

		_ = ws.Close()
		time.Sleep(5 * time.Second)
	}
}
