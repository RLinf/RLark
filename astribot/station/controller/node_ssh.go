package controller

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// NodeSSHConfig is supplied from a short-lived operation credential, not status.
type NodeSSHConfig struct {
	Address, User, HostKeyFingerprint string
	Password                          []byte
	Timeout                           time.Duration
	// HostKeyAlgorithms are the host key types the host is asked for; a
	// fingerprint a scan read is that of one type (ED25519).
	HostKeyAlgorithms []string
	// Via is the relay of a site's hub ("address:port"), which connects to
	// Address in the site's own network when it is told the address in one line.
	Via string
	// Jump is dialed first and Address through it. Each host shows its key
	// before anything is sent to it to authenticate with.
	Jump *NodeSSHConfig
	// LearnedHostKey, with an empty HostKeyFingerprint and a Jump, receives
	// the key of a host seen for the first time: one that can only be
	// reached through a device whose key was verified.
	LearnedHostKey *string
}

// What a dial can say of a machine; everything else is a fault of the dial.
var (
	ErrSSHCredentialRejected = errors.New("the host refused the account")
	ErrSSHJumpUnreachable    = errors.New("the jump host does not answer")
	ErrSSHTargetUnreachable  = errors.New("the host behind the jump host does not answer")
	// The relay of the site's hub answers that it cannot connect to the host:
	// the host does not answer on the site's network, or the hub is no longer on it.
	ErrSSHSiteHostUnreachable = errors.New("the site's hub cannot connect to the host")
	// The relay of the site's hub does not answer: nothing of the site's network is reached.
	ErrSSHRelayUnreachable = errors.New("the site's hub does not answer")
)

// SSHHostKeyMismatchError is a host that showed another key than the one it
// is known by. Nothing was sent to it.
type SSHHostKeyMismatchError struct{ Received string }

func (e *SSHHostKeyMismatchError) Error() string {
	return "SSH host key fingerprint mismatch: received " + e.Received
}

// RunNodeSudoScript runs a script of trusted code, never of an API request,
// as root: the sudo password and the script go over stdin. Passwordless sudo
// (a nil password) must be selected explicitly so sudo cannot consume the
// first line of the script.
func RunNodeSudoScript(ctx context.Context, client *ssh.Client, script string, password []byte) ([]byte, error) {
	if !strings.HasSuffix(script, "\n") {
		script += "\n"
	}
	return RunNodeSudoStream(ctx, client, "exec /bin/sh -s", password, strings.NewReader(script))
}

const maxNodeCommandOutput = 64 << 10

// RunNodeSudoStream runs a command of trusted code as root with what follows
// the sudo password on stdin as its input: a file that is sent to the host.
func RunNodeSudoStream(ctx context.Context, client *ssh.Client, command string, password []byte, input io.Reader) ([]byte, error) {
	if bytes.ContainsAny(password, "\r\n\x00") {
		return nil, errors.New("sudo password contains unsupported characters")
	}
	if password == nil {
		return RunNodeSSHStream(ctx, client, "sudo -n /bin/sh -c "+shellQuote(command), input)
	}
	first := append(append([]byte(nil), password...), '\n')
	defer clear(first)
	return RunNodeSSHStream(ctx, client, "sudo -k -S -p '' /bin/sh -c "+shellQuote(command), io.MultiReader(bytes.NewReader(first), input))
}

func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// RunNodeSSHStream runs a command of trusted code without exposing remote
// stderr or stdin in errors. Cancellation closes the session.
func RunNodeSSHStream(ctx context.Context, client *ssh.Client, command string, stdin io.Reader) ([]byte, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()
	var output boundedNodeOutput
	session.Stdout = &output
	session.Stderr = io.Discard
	session.Stdin = stdin
	stop := context.AfterFunc(ctx, func() { _ = session.Close() })
	defer stop()
	if err := session.Run(command); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("remote command failed")
	}
	if output.truncated {
		return nil, errors.New("remote command output exceeded limit")
	}
	return output.data.Bytes(), nil
}

type boundedNodeOutput struct {
	data      bytes.Buffer
	truncated bool
}

func (w *boundedNodeOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := maxNodeCommandOutput - w.data.Len()
	if remaining < len(p) {
		w.truncated = true
		p = p[:remaining]
	}
	_, _ = w.data.Write(p)
	return n, nil
}

// DialNodeSSH pins the host key before authentication and honors cancellation
// during both TCP connect and SSH handshake. It never logs credentials.
// Closing the client of a host behind a jump host closes the jump host's too.
func DialNodeSSH(ctx context.Context, cfg NodeSSHConfig) (*ssh.Client, error) {
	if cfg.Jump == nil && cfg.Via != "" {
		conn, err := (&net.Dialer{Timeout: nodeSSHTimeout(cfg)}).DialContext(ctx, "tcp", cfg.Via)
		if err != nil {
			return nil, fmt.Errorf("%w: connect to its relay %s: %v", ErrSSHRelayUnreachable, cfg.Via, err)
		}
		_ = conn.SetDeadline(time.Now().Add(nodeSSHTimeout(cfg)))
		answer := make([]byte, 0, 16)
		if _, err = io.WriteString(conn, cfg.Address+"\n"); err == nil {
			// One byte at a time: what follows the line is the host's.
			for one := make([]byte, 1); err == nil && !bytes.HasSuffix(answer, []byte("\n")) && len(answer) < cap(answer); {
				if _, err = conn.Read(one); err == nil {
					answer = append(answer, one[0])
				}
			}
		}
		if err != nil || string(answer) != "ok\n" {
			_ = conn.Close()
			switch string(answer) {
			case "unreachable\n":
				return nil, fmt.Errorf("%w: %s, through relay %s", ErrSSHSiteHostUnreachable, cfg.Address, cfg.Via)
			case "refused\n":
				return nil, fmt.Errorf("the relay %s refuses %s: the address or port is outside its site's discovery", cfg.Via, cfg.Address)
			}
			return nil, fmt.Errorf("%w: its relay %s answered %q: %v", ErrSSHRelayUnreachable, cfg.Via, strings.TrimSpace(string(answer)), err)
		}
		return nodeSSHHandshake(ctx, conn, cfg)
	}
	if cfg.Jump == nil {
		conn, err := (&net.Dialer{Timeout: nodeSSHTimeout(cfg)}).DialContext(ctx, "tcp", cfg.Address)
		if err != nil {
			return nil, fmt.Errorf("connect SSH: %w", err)
		}
		return nodeSSHHandshake(ctx, conn, cfg)
	}
	jump, err := DialNodeSSH(ctx, *cfg.Jump)
	if err != nil {
		var mismatch *SSHHostKeyMismatchError
		if errors.Is(err, ErrSSHCredentialRejected) || errors.As(err, &mismatch) || errors.Is(err, ErrSSHRelayUnreachable) || ctx.Err() != nil {
			return nil, fmt.Errorf("jump host: %w", err)
		}
		return nil, fmt.Errorf("%w: %v", ErrSSHJumpUnreachable, err)
	}
	dialing, cancel := context.WithTimeout(ctx, nodeSSHTimeout(cfg))
	defer cancel()
	conn, err := jump.DialContext(dialing, "tcp", cfg.Address)
	if err != nil {
		_ = jump.Close()
		return nil, fmt.Errorf("%w: %v", ErrSSHTargetUnreachable, err)
	}
	client, err := nodeSSHHandshake(ctx, conn, cfg)
	if err != nil {
		_ = jump.Close()
		return nil, err
	}
	go func() {
		_ = client.Wait()
		_ = jump.Close()
	}()
	return client, nil
}

func nodeSSHTimeout(cfg NodeSSHConfig) time.Duration {
	if cfg.Timeout <= 0 {
		return 15 * time.Second
	}
	return cfg.Timeout
}

func nodeSSHHandshake(ctx context.Context, conn net.Conn, cfg NodeSSHConfig) (*ssh.Client, error) {
	learns := cfg.HostKeyFingerprint == "" && cfg.Jump != nil && cfg.LearnedHostKey != nil
	if cfg.Address == "" || cfg.User == "" || cfg.HostKeyFingerprint == "" && !learns {
		_ = conn.Close()
		return nil, errors.New("SSH address, user and verified host fingerprint are required")
	}
	ctx, cancel := context.WithTimeout(ctx, nodeSSHTimeout(cfg))
	defer cancel()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	var mismatch *SSHHostKeyMismatchError
	shown := false
	config := &ssh.ClientConfig{User: cfg.User, Auth: []ssh.AuthMethod{ssh.Password(string(cfg.Password))}, HostKeyAlgorithms: cfg.HostKeyAlgorithms, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		actual := ssh.FingerprintSHA256(key)
		shown = true
		if learns {
			*cfg.LearnedHostKey = actual
			return nil
		}
		if subtle.ConstantTimeCompare([]byte(actual), []byte(cfg.HostKeyFingerprint)) != 1 {
			mismatch = &SSHHostKeyMismatchError{Received: actual}
			return mismatch
		}
		return nil
	}}
	transport, channels, requests, err := ssh.NewClientConn(conn, cfg.Address, config)
	if err != nil {
		stop()
		_ = conn.Close()
		switch {
		case mismatch != nil:
			return nil, mismatch
		case shown && ctx.Err() == nil && strings.Contains(err.Error(), "unable to authenticate"):
			return nil, ErrSSHCredentialRejected
		}
		return nil, fmt.Errorf("SSH handshake: %w", err)
	}
	if !stop() {
		_ = transport.Close()
		return nil, ctx.Err()
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(transport, channels, requests), nil
}
