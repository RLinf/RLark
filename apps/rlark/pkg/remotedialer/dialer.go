package remotedialer

import (
	"context"
	"net"
)

// Dialer opens a network connection. It has the same calling convention as
// net.Dialer.DialContext.
type Dialer func(ctx context.Context, network, address string) (net.Conn, error)

// HasSession reports whether clientKey is reachable through a direct client
// session or a connected peer.
func (s *Server) HasSession(clientKey string) bool {
	_, err := s.sessions.getDialer(clientKey)
	return err == nil
}

// Dialer returns a Dialer that resolves an active route to clientKey for each
// call and opens the connection through that route.
func (s *Server) Dialer(clientKey string) Dialer {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		d, err := s.sessions.getDialer(clientKey)
		if err != nil {
			return nil, err
		}

		return d(ctx, network, address)
	}
}

// GetDialer returns a Dialer for only available sessions to the given clientKey.
func (s *Server) GetDialer(clientKey string) (Dialer, error) {
	d, err := s.sessions.getDialer(clientKey)
	if err != nil {
		return nil, err
	}
	return d, nil
}
