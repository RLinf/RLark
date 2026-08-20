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
	"path/filepath"
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

	cfg := &gossh.ServerConfig{
		PublicKeyCallback: func(_ gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			authorizedKeys, err := s.loadAuthorizedKeys()
			if err != nil {
				return nil, fmt.Errorf("sshd: load authorized keys: %w", err)
			}
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
	defer log.GetLogger().Info("sshd connection closed", "remote", conn.RemoteAddr())
	log.GetLogger().Info("sshd connection established", "remote", conn.RemoteAddr(), "user", sc.User())
	go s.keepAlive(sc, globalReqs)

	for newCh := range chans {
		switch newCh.ChannelType() {
		case "session":
			ch, chReqs, err := newCh.Accept()
			if err != nil {
				continue
			}
			go s.serveSession(ch, chReqs)

		case "direct-tcpip":
			extraData := newCh.ExtraData()
			ch, chReqs, err := newCh.Accept()
			if err != nil {
				continue
			}
			go s.serveDirectTCP(ch, chReqs, extraData)

		default:
			_ = newCh.Reject(gossh.UnknownChannelType, fmt.Sprintf("unsupported channel type: %s", newCh.ChannelType()))
		}
	}
}

// directTCPMsg maps to the SSH "direct-tcpip" channel request payload (RFC 4254 §7.2).
type directTCPMsg struct {
	Addr       string
	Port       uint32
	OriginAddr string
	OriginPort uint32
}

// serveDirectTCP handles a direct-tcpip channel: dials the requested address
// and bridges I/O bidirectionally. This enables SSH port forwarding (-L/-D),
// which VSCode Remote SSH requires.
func (s *Server) serveDirectTCP(ch gossh.Channel, reqs <-chan *gossh.Request, extraData []byte) {
	defer func() { _ = ch.Close() }()
	go gossh.DiscardRequests(reqs)

	var msg directTCPMsg
	if err := gossh.Unmarshal(extraData, &msg); err != nil {
		return
	}

	dst := net.JoinHostPort(msg.Addr, fmt.Sprintf("%d", msg.Port))
	log.GetLogger().Info("sshd direct-tcpip request", "dst", dst)
	conn, err := net.Dial("tcp", dst)
	if err != nil {
		log.GetLogger().Error(err, "sshd direct-tcpip dial failed", "dst", dst)
		_, _ = fmt.Fprintf(ch.Stderr(), "sshd: connect %s: %v\r\n", dst, err)
		return
	}
	defer func() { _ = conn.Close() }()

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(conn, ch)
		close(done)
	}()
	_, _ = io.Copy(ch, conn)
	<-done
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
			// Only use a login shell (-l) for interactive PTY sessions. For
			// non-PTY shells (e.g. VSCode's `ssh -T -D <port> host` keepalive
			// connection), -l causes bash to source /etc/profile etc., which
			// can exit early in minimal images and tear down the forwarding
			// channel.
			var cmd *exec.Cmd
			if hasPTY {
				cmd = exec.Command(shell, "-l")
			} else {
				cmd = exec.Command(shell)
			}
			log.GetLogger().Info("sshd shell request", "shell", shell, "hasPTY", hasPTY)
			s.runCommand(ch, cmd, &ptyReq, hasPTY, reqs)
			log.GetLogger().Info("sshd shell session ended")
			return

		case "exec":
			_ = req.Reply(true, nil)
			var msg struct{ Command string }
			if err := gossh.Unmarshal(req.Payload, &msg); err == nil {
				log.GetLogger().Info("sshd exec request", "command", msg.Command, "hasPTY", hasPTY)
				s.runCommand(ch, exec.Command(shell, "-c", msg.Command), &ptyReq, hasPTY, reqs)
			} else {
				log.GetLogger().Error(err, "sshd failed to unmarshal exec payload")
			}
			return

		case "subsystem":
			_ = req.Reply(true, nil)
			var msg struct{ Subsystem string }
			if err := gossh.Unmarshal(req.Payload, &msg); err == nil && msg.Subsystem == "sftp" {
				s.runSFTP(ch)
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
		log.GetLogger().Info("sshd command exited with error", "err", err, "exitCode", exitCode)
	} else {
		log.GetLogger().Info("sshd command exited cleanly")
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
// stderr is sent via SSH extended-data (stderr) so it doesn't pollute stdout.
func (s *Server) runPipe(ch gossh.Channel, cmd *exec.Cmd) {
	cmd.Stdin = ch
	cmd.Stdout = ch

	stderrWriter := ch.Stderr()
	cmd.Stderr = stderrWriter

	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(stderrWriter, "sshd: failed to start: %v\r\n", err)
		return
	}
}

// runSFTP starts an SFTP server subsystem on the given channel.
func (s *Server) runSFTP(ch gossh.Channel) {
	cmd := exec.Command("sftp-server")
	cmd.Stdin = ch
	cmd.Stdout = ch
	cmd.Stderr = ch.Stderr()
	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(ch.Stderr(), "sshd: sftp-server not found: %v\r\n", err)
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
// the RLARK_SSH_PUBLIC_KEY env var, and ~/.ssh/authorized_keys.
func (s *Server) loadAuthorizedKeys() ([]gossh.PublicKey, error) {
	sources := s.AuthorizedKeys
	if v := os.Getenv("RLARK_SSH_PUBLIC_KEY"); v != "" {
		sources = append(sources, strings.Split(v, "\n")...)
	}

	if home, err := os.UserHomeDir(); err == nil {
		if data, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys")); err == nil {
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
