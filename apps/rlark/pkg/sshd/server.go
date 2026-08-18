// Package sshd provides a minimal in-pod SSH server for interactive shell
// access via public key authentication. It is designed to be injected into
// pods by the rlark agent so that users can SSH directly into containers
// without depending on the image providing sshd.
package sshd

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
	gossh "golang.org/x/crypto/ssh"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// ptyRequestMsg maps to the SSH "pty-req" channel request payload (RFC 4254 §6.2).
type ptyRequestMsg struct {
	Term     string
	Columns  uint32
	Rows     uint32
	Width    uint32
	Height   uint32
	Modelist string
}

// windowChangeMsg maps to the SSH "window-change" channel request payload (RFC 4254 §6.7).
type windowChangeMsg struct {
	Columns uint32
	Rows    uint32
	Width   uint32
	Height  uint32
}

// Server is a minimal SSH server that grants interactive shell access to
// callers presenting an authorized public key.
type Server struct {
	// Port to listen on. Defaults to "22" if empty.
	Port string
	// Shell executed for interactive sessions. Defaults to /bin/bash.
	Shell string
	// AuthorizedKeys is a list of authorized_keys format entries. If empty,
	// keys are read from RLARK_SSH_PUBLIC_KEY env var.
	AuthorizedKeys []string
}

// ListenAndServe starts the SSH server and blocks until a fatal error occurs.
func (s *Server) ListenAndServe() error {
	hostSigner, err := generateHostKey()
	if err != nil {
		return fmt.Errorf("sshd: generate host key: %w", err)
	}

	authorizedKeys, err := s.loadAuthorizedKeys()
	if err != nil {
		return err
	}

	cfg := &gossh.ServerConfig{
		PublicKeyCallback: func(_ gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			wire := key.Marshal()
			for _, ak := range authorizedKeys {
				if bytesEqual(wire, ak.Marshal()) {
					return nil, nil
				}
			}
			return nil, fmt.Errorf("sshd: public key not authorized")
		},
	}
	cfg.AddHostKey(hostSigner)

	port := s.Port
	if port == "" {
		port = "22"
	}

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("sshd: listen :%s: %w", port, err)
	}
	log.GetLogger().Info("sshd listening", "port", port)

	for {
		conn, err := ln.Accept()
		if err != nil {
			return fmt.Errorf("sshd: accept: %w", err)
		}
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetKeepAlive(true)
			_ = tc.SetKeepAlivePeriod(30 * time.Second)
		}
		go s.serveConn(conn, cfg)
	}
}

// serveConn handles a single TCP connection: SSH handshake, channel
// multiplexing, and session dispatch.
func (s *Server) serveConn(conn net.Conn, cfg *gossh.ServerConfig) {
	defer func() { _ = conn.Close() }()
	sc, chans, globalReqs, err := gossh.NewServerConn(conn, cfg)
	if err != nil {
		log.GetLogger().Error(err, "sshd handshake failed")
		return
	}
	defer func() { _ = sc.Close() }()
	go s.keepAlive(sc, globalReqs)

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(gossh.Prohibited, "only session channels are supported")
			continue
		}
		ch, chReqs, err := newCh.Accept()
		if err != nil {
			continue
		}
		go s.serveSession(ch, chReqs)
	}
}

// keepAlive sends periodic global keepalive requests to detect dead
// connections. If a keepalive fails the SSH connection is closed.
func (s *Server) keepAlive(sc *gossh.ServerConn, globalReqs <-chan *gossh.Request) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case req, ok := <-globalReqs:
			if !ok {
				return
			}
			_ = req.Reply(false, nil)
		case <-ticker.C:
			if _, _, err := sc.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				_ = sc.Close()
				return
			}
		}
	}
}

// serveSession processes SSH session requests for a single channel. It waits
// for a "shell" or "exec" request (optionally preceded by a "pty-req") and
// then runs the command to completion.
func (s *Server) serveSession(ch gossh.Channel, reqs <-chan *gossh.Request) {
	defer func() { _ = ch.Close() }()

	shell := s.Shell
	if shell == "" {
		shell = "/bin/bash"
	}

	var ptyReq ptyRequestMsg
	var hasPTY bool

	for req := range reqs {
		switch req.Type {
		case "pty-req":
			if err := gossh.Unmarshal(req.Payload, &ptyReq); err == nil {
				hasPTY = true
			}
			_ = req.Reply(true, nil)

		case "shell":
			_ = req.Reply(true, nil)
			s.runCommand(ch, exec.Command(shell, "-l"), &ptyReq, hasPTY, reqs)
			return

		case "exec":
			_ = req.Reply(true, nil)
			var msg struct{ Command string }
			if err := gossh.Unmarshal(req.Payload, &msg); err == nil {
				s.runCommand(ch, exec.Command(shell, "-c", msg.Command), &ptyReq, hasPTY, reqs)
			}
			return

		case "window-change":
			_ = req.Reply(true, nil)

		default:
			_ = req.Reply(false, nil)
		}
	}
}

// runCommand starts cmd, wires I/O between the SSH channel and the process,
// and blocks until the process exits. For PTY sessions it uses creack/pty;
// otherwise it pipes directly.
func (s *Server) runCommand(ch gossh.Channel, cmd *exec.Cmd, ptyReq *ptyRequestMsg, hasPTY bool, reqs <-chan *gossh.Request) {
	if hasPTY {
		s.runPTY(ch, cmd, ptyReq, reqs)
	} else {
		s.runPipe(ch, cmd)
	}

	exitCode := 0
	if err := cmd.Wait(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		}
	}
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(exitCode))
	_, _ = ch.SendRequest("exit-status", false, buf[:])
}

// runPTY starts cmd with a pseudo-terminal and bridges I/O. A background
// goroutine processes remaining session requests (e.g. window-change).
func (s *Server) runPTY(ch gossh.Channel, cmd *exec.Cmd, ptyReq *ptyRequestMsg, reqs <-chan *gossh.Request) {
	size := &pty.Winsize{Cols: uint16(ptyReq.Columns), Rows: uint16(ptyReq.Rows)}
	ptyFile, err := pty.StartWithSize(cmd, size)
	if err != nil {
		_, _ = fmt.Fprintf(ch, "sshd: failed to allocate pty: %v\r\n", err)
		return
	}
	defer func() { _ = ptyFile.Close() }()

	// Process remaining requests (window-resize, etc.) in the background.
	go func() {
		for req := range reqs {
			if req.Type == "window-change" {
				var wc windowChangeMsg
				if err := gossh.Unmarshal(req.Payload, &wc); err == nil {
					_ = pty.Setsize(ptyFile, &pty.Winsize{
						Cols: uint16(wc.Columns),
						Rows: uint16(wc.Rows),
					})
				}
			}
			_ = req.Reply(true, nil)
		}
	}()

	// Close the pty when the SSH channel is closed (remote disconnect).
	go func() {
		_, _ = io.Copy(ptyFile, ch)
		_ = ptyFile.Close()
	}()

	_, _ = io.Copy(ch, ptyFile)
}

// runPipe starts cmd with stdio wired directly to the SSH channel (no PTY).
func (s *Server) runPipe(ch gossh.Channel, cmd *exec.Cmd) {
	cmd.Stdin = ch
	cmd.Stdout = ch
	cmd.Stderr = ch
	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(ch, "sshd: failed to start: %v\r\n", err)
		return
	}
}

// generateHostKey creates a fresh ed25519 key pair on each startup. The key is
// ephemeral and not persisted — clients should not pin the host key.
func generateHostKey() (gossh.Signer, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}
	return signer, nil
}

// loadAuthorizedKeys collects authorized public keys from the struct field,
// the RLARK_SSH_PUBLIC_KEY env var, and optionally a file pointed to by
// RLARK_SSH_AUTHORIZED_KEYS_FILE.
func (s *Server) loadAuthorizedKeys() ([]gossh.PublicKey, error) {
	sources := s.AuthorizedKeys
	if v := os.Getenv("RLARK_SSH_PUBLIC_KEY"); v != "" {
		sources = append(sources, strings.Split(v, "\n")...)
	}
	if fn := os.Getenv("RLARK_SSH_AUTHORIZED_KEYS_FILE"); fn != "" {
		if data, err := os.ReadFile(fn); err == nil {
			sources = append(sources, strings.Split(string(data), "\n")...)
		}
	}

	var keys []gossh.PublicKey
	for _, line := range sources {
		if k := parseAuthorizedKey(line); k != nil {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("sshd: no authorized keys configured")
	}
	return keys, nil
}

// parseAuthorizedKey parses a single authorized_keys line. Returns nil for
// blank lines, comments, or unparseable entries.
func parseAuthorizedKey(line string) gossh.PublicKey {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}
	key, _, _, _, err := gossh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		return nil
	}
	return key
}

// bytesEqual is a constant-time comparison for public key wire formats.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
