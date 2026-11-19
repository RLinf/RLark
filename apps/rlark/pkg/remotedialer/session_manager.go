package remotedialer

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type sessionListener interface {
	sessionAdded(clientKey string, sessionKey int64)
	sessionRemoved(clientKey string, sessionKey int64)
}

type sessionManager struct {
	sync.Mutex
	clients   map[string][]*Session
	peers     map[string][]*Session
	listeners map[sessionListener]bool
}

func newSessionManager() *sessionManager {
	return &sessionManager{
		clients:   map[string][]*Session{},
		peers:     map[string][]*Session{},
		listeners: map[sessionListener]bool{},
	}
}

func toDialer(s *Session, prefix string) Dialer {
	return func(ctx context.Context, proto, address string) (net.Conn, error) {
		if prefix == "" {
			return s.serverConnectContext(ctx, proto, address)
		}
		return s.serverConnectContext(ctx, prefix+"::"+proto, address)
	}
}

func (sm *sessionManager) removeListener(listener sessionListener) {
	sm.Lock()
	defer sm.Unlock()

	delete(sm.listeners, listener)
}

func (sm *sessionManager) addListener(listener sessionListener) {
	sm.Lock()
	defer sm.Unlock()

	sm.listeners[listener] = true

	for k, sessions := range sm.clients {
		for _, session := range sessions {
			listener.sessionAdded(k, session.sessionKey)
		}
	}

	for k, sessions := range sm.peers {
		for _, session := range sessions {
			listener.sessionAdded(k, session.sessionKey)
		}
	}
}

func (sm *sessionManager) listClients() []string {
	sm.Lock()
	defer sm.Unlock()
	clients := make([]string, 0, len(sm.clients))
	for c := range sm.clients {
		clients = append(clients, c)
	}
	return clients
}

func (sm *sessionManager) listPeers() []string {
	sm.Lock()
	defer sm.Unlock()
	peers := make([]string, 0, len(sm.peers))
	for p := range sm.peers {
		peers = append(peers, p)
	}
	return peers
}

func (sm *sessionManager) getDialer(clientKey string) (Dialer, error) {
	sm.Lock()
	defer sm.Unlock()
	session, routeKey, err := sm.findSession(clientKey)
	if err != nil {
		return nil, err
	}
	return toDialer(session, routeKey), nil
}

func (sm *sessionManager) findSession(clientKey string) (*Session, string, error) {
	if prefix, ok := strings.CutSuffix(clientKey, "*"); ok {
		// Any matching direct node-agent is an equivalent fallback. Map iteration
		// intentionally leaves selection unspecified when multiple nodes match.
		for k, sessions := range sm.clients {
			if strings.HasPrefix(k, prefix) && len(sessions) > 0 {
				return sessions[0], "", nil
			}
		}
		// The full matched key is required so the peer can route to that direct client.
		for _, sessions := range sm.peers {
			for _, session := range sessions {
				if matchedClientKey, ok := session.clientKeyWithPrefix(prefix); ok {
					return session, matchedClientKey, nil
				}
			}
		}
		return nil, "", fmt.Errorf("failed to find session for client with prefix %s", prefix)
	}

	sessions := sm.clients[clientKey]
	if len(sessions) > 0 {
		return sessions[0], "", nil
	}

	for _, sessions := range sm.peers {
		for _, session := range sessions {
			keys := session.getSessionKeys(clientKey)
			if len(keys) > 0 {
				return session, clientKey, nil
			}
		}
	}

	return nil, "", fmt.Errorf("failed to find session for client %s", clientKey)
}

func (sm *sessionManager) add(clientKey string, conn *websocket.Conn, peer bool) *Session {
	sessionKey := rand.Int63()
	session := NewServerSession(sessionKey, clientKey, NewWSConn(conn))

	sm.Lock()
	defer sm.Unlock()

	if peer {
		sm.peers[clientKey] = append(sm.peers[clientKey], session)
	} else {
		sm.clients[clientKey] = append(sm.clients[clientKey], session)
	}

	for l := range sm.listeners {
		l.sessionAdded(clientKey, session.sessionKey)
	}

	return session
}

func (sm *sessionManager) remove(s *Session) {
	sm.Lock()
	defer sm.Unlock()

	for _, store := range []map[string][]*Session{sm.clients, sm.peers} {
		var newSessions []*Session

		for _, v := range store[s.clientKey] {
			if v.sessionKey == s.sessionKey {
				continue
			}
			newSessions = append(newSessions, v)
		}

		if len(newSessions) == 0 {
			delete(store, s.clientKey)
		} else {
			store[s.clientKey] = newSessions
		}
	}

	for l := range sm.listeners {
		l.sessionRemoved(s.clientKey, s.sessionKey)
	}

	s.Close()
}
