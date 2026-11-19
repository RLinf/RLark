package remotedialer

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/sirupsen/logrus"
)

// controlCommand identifies a message sent over the reserved control stream.
type controlCommand string

const (
	controlAddClient    controlCommand = "ADD"
	controlRemoveClient controlCommand = "REMOVE"
)

const (
	// AddClient identifies a control message advertising a reachable client.
	AddClient = "AddClient"
	// RemoveClient identifies a control message withdrawing a reachable client.
	RemoveClient = "RemoveClient"
)

// writeControl sends a single peering command over the control stream.
// The wire format is a newline delimited "<command> <address>" line.
func (s *Session) writeControl(kind string, address string) error {
	s.RLock()
	control := s.control
	s.RUnlock()
	if control == nil {
		// No peer is listening on this session (e.g. a plain client session).
		return nil
	}

	var cmd controlCommand
	switch kind {
	case AddClient:
		cmd = controlAddClient
	case RemoveClient:
		cmd = controlRemoveClient
	default:
		return fmt.Errorf("unknown control command %q", kind)
	}

	_, err := io.WriteString(control, fmt.Sprintf("%s %s\n", cmd, address))
	return err
}

// serveControl consumes peering commands from the reserved control stream until
// it is closed.
func (s *Session) serveControl(control net.Conn) {
	scanner := bufio.NewScanner(control)
	defer func() {
		_ = scanner.Err() // ignore errors on close
	}()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			logrus.Warnf("malformed control command %q", line)
			continue
		}
		cmd, address := controlCommand(parts[0]), parts[1]
		switch cmd {
		case controlAddClient:
			if err := s.addRemoteClient(address); err != nil {
				logrus.WithError(err).Warn("failed to add remote client")
			}
		case controlRemoveClient:
			if err := s.removeRemoteClient(address); err != nil {
				logrus.WithError(err).Warn("failed to remove remote client")
			}
		default:
			logrus.Warnf("unknown control command %q", cmd)
		}
	}
}

// addRemoteClient registers a new remote client, making it accessible for requests
func (s *Session) addRemoteClient(address string) error {
	if s.remoteClientKeys == nil {
		return nil
	}

	clientKey, sessionKey, err := parseAddress(address)
	if err != nil {
		return fmt.Errorf("invalid remote Session %s: %v", address, err)
	}
	s.addSessionKey(clientKey, sessionKey)

	if PrintTunnelData {
		logrus.Debugf("ADD REMOTE CLIENT %s, SESSION %d", address, s.sessionKey)
	}

	return nil
}

// removeRemoteClient removes a given client from a session
func (s *Session) removeRemoteClient(address string) error {
	clientKey, sessionKey, err := parseAddress(address)
	if err != nil {
		return fmt.Errorf("invalid remote Session %s: %v", address, err)
	}
	s.removeSessionKey(clientKey, sessionKey)

	if PrintTunnelData {
		logrus.Debugf("REMOVE REMOTE CLIENT %s, SESSION %d", address, s.sessionKey)
	}

	return nil
}
