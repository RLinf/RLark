// Package nodessh is how RLark logs in to a machine that is not a node yet
// with an account an administrator typed: a password login that accepts and
// answers the host key the machine shows, commands with the sudo password on
// their standard input, and the lines of a long command as they come. The
// password is the caller's: nothing here keeps it.
package nodessh

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// The reasons a login or sudo is refused, each one an administrator acts on differently.
const (
	Unreachable          = "Unreachable"
	HostKeyChanged       = "HostKeyChanged"
	LoginRejected        = "LoginRejected"
	NoSudo               = "NoSudo"
	SudoPasswordRejected = "SudoPasswordRejected"
	RelayRejected        = "RelayRejected"
	SSHFailure           = "SSHFailure"
)

// Error is a refusal with its reason.
type Error struct {
	Reason string
	Err    error
}

func (e *Error) Error() string { return e.Err.Error() }

// Dial logs in with the password (also as the answer to every
// keyboard-interactive question) and answers the SHA256 fingerprint of the
// host key the machine showed. With hostKey empty the machine is not known
// before and its key is accepted; with it, a machine that shows another key
// is refused before anything is sent to authenticate.
func Dial(ctx context.Context, address string, port int, user, hostKey string, password []byte) (*ssh.Client, string, *Error) {
	return DialPinned(ctx, address, port, user, hostKey, "", password)
}

// DialPinned also verifies the negotiated host-key algorithm when one was
// recorded during discovery.
func DialPinned(ctx context.Context, address string, port int, user, hostKey, hostKeyAlgorithm string, password []byte) (*ssh.Client, string, *Error) {
	target := net.JoinHostPort(address, strconv.Itoa(port))
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, "", &Error{Unreachable, fmt.Errorf("cannot reach %s: %v", target, err)}
	}
	return dialSSHConn(ctx, conn, target, user, hostKey, hostKeyAlgorithm, password)
}

// bufferedConn preserves bytes a relay target may send immediately after its
// acknowledgement while the bounded line reader is still active.
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(buffer []byte) (int, error) { return c.reader.Read(buffer) }

// DialRelay asks an authenticated Station Hub relay to connect to target, then
// performs the ordinary pinned SSH/password handshake over the returned byte
// stream. Only the relay token and target address are sent to the Hub.
func DialRelay(ctx context.Context, relayEndpoint, address string, port int, user, hostKey, hostKeyAlgorithm, token string, password []byte) (*ssh.Client, string, *Error) {
	if strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n \t") {
		return nil, "", &Error{RelayRejected, errors.New("relay token is missing or malformed")}
	}
	target := net.JoinHostPort(address, strconv.Itoa(port))
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", relayEndpoint)
	if err != nil {
		return nil, "", &Error{Unreachable, fmt.Errorf("cannot reach site relay %s: %v", relayEndpoint, err)}
	}
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()
	setHandshakeDeadline(ctx, conn)
	if _, err := fmt.Fprintf(conn, "gateway %s %s\n", token, target); err != nil {
		_ = conn.Close()
		return nil, "", &Error{Unreachable, fmt.Errorf("write site relay request: %v", err)}
	}
	reader := bufio.NewReaderSize(conn, 1)
	line, err := readLineLimited(reader, 256)
	if err != nil {
		_ = conn.Close()
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, "", &Error{RelayRejected, errors.New("site relay closed without confirming the target connection")}
		}
		if isNetworkFailure(err) {
			return nil, "", &Error{Unreachable, fmt.Errorf("read site relay response: %v", err)}
		}
		return nil, "", &Error{RelayRejected, errors.New("site relay did not confirm the target connection")}
	}
	if line != "ok\n" {
		_ = conn.Close()
		return nil, "", &Error{RelayRejected, errors.New("site relay refused the target connection")}
	}
	return dialSSHConn(ctx, &bufferedConn{Conn: conn, reader: reader}, target, user, hostKey, hostKeyAlgorithm, password)
}

func dialSSHConn(ctx context.Context, conn net.Conn, target, user, hostKey, hostKeyAlgorithm string, password []byte) (*ssh.Client, string, *Error) {
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()
	setHandshakeDeadline(ctx, conn)
	shown := ""
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(string(password)),
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = string(password)
				}
				return answers, nil
			}),
		},
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			shown = ssh.FingerprintSHA256(key)
			if hostKey != "" && shown != hostKey {
				return errHostKeyChanged
			}
			if hostKeyAlgorithm != "" && key.Type() != hostKeyAlgorithm {
				return errHostKeyAlgorithmChanged
			}
			return nil
		},
	}
	connection, channels, requests, err := ssh.NewClientConn(conn, target, config)
	if err != nil {
		conn.Close()
		if errors.Is(err, errHostKeyChanged) {
			return nil, "", &Error{HostKeyChanged, fmt.Errorf("%s shows the host key %s, not %s", target, shown, hostKey)}
		}
		if errors.Is(err, errHostKeyAlgorithmChanged) {
			return nil, "", &Error{HostKeyChanged, fmt.Errorf("%s shows a different host key algorithm than the scanned candidate", target)}
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, "", &Error{LoginRejected, fmt.Errorf("%s did not accept the user %s with this password", target, user)}
		}
		if isNetworkFailure(err) {
			return nil, "", &Error{Unreachable, fmt.Errorf("no SSH login at %s: %v", target, err)}
		}
		return nil, "", &Error{SSHFailure, fmt.Errorf("SSH handshake with %s failed: %v", target, err)}
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(connection, channels, requests), shown, nil
}

var errHostKeyChanged = errors.New("the host key changed")
var errHostKeyAlgorithmChanged = errors.New("the host key algorithm changed")

func setHandshakeDeadline(ctx context.Context, conn net.Conn) {
	deadline := time.Now().Add(20 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = conn.SetDeadline(deadline)
}

func isNetworkFailure(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError)
}

func readLineLimited(reader *bufio.Reader, limit int) (string, error) {
	var line strings.Builder
	for line.Len() <= limit {
		part, err := reader.ReadString('\n')
		line.WriteString(part)
		if err != nil {
			return line.String(), err
		}
		if line.Len() > limit {
			return "", errors.New("relay response exceeded its limit")
		}
		return line.String(), nil
	}
	return "", errors.New("relay response exceeded its limit")
}

// Run runs a command and answers what it printed, both streams.
func Run(client *ssh.Client, command string, stdin io.Reader) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	session.Stdin = stdin
	output, err := session.CombinedOutput(command)
	return string(output), err
}

// Sudo is the prefix that runs a command as root for user: sudo reads the
// password from its standard input (SudoInput). root needs none.
func Sudo(user string) string {
	if user == "root" {
		return ""
	}
	return "sudo -S -p '' "
}

// SudoInput is what sudo reads the password from; the caller clears it.
func SudoInput(password []byte) []byte {
	return append(append([]byte{}, password...), '\n')
}

// SudoStdin is the standard input of a command run with Sudo: input
// (SudoInput, which the caller clears) first, for sudo, then what the command
// itself reads (nil for nothing).
func SudoStdin(user string, input []byte, stdin io.Reader) io.Reader {
	if user == "root" {
		return stdin
	}
	if stdin == nil {
		return bytes.NewReader(input)
	}
	return io.MultiReader(bytes.NewReader(input), stdin)
}

// CheckSudo says whether user may run commands as root with the password.
func CheckSudo(client *ssh.Client, user string, password []byte) *Error {
	if user == "root" {
		return nil
	}
	input := SudoInput(password)
	defer clear(input)
	output, err := Run(client, Sudo(user)+"true", bytes.NewReader(input))
	if err == nil {
		return nil
	}
	if strings.Contains(output, "incorrect password") || strings.Contains(output, "Sorry, try again") {
		return &Error{SudoPasswordRejected, fmt.Errorf("sudo on the machine does not take the login password of %s", user)}
	}
	return &Error{NoSudo, fmt.Errorf("%s may not use sudo on the machine: %s", user, strings.TrimSpace(output))}
}

// Follow runs a command and hands each line it prints to line, until it
// ends; stdin is what it reads (SudoStdin for one run with Sudo).
func Follow(client *ssh.Client, command string, stdin io.Reader, line func(string)) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = stdin
	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	if err := session.Start(command); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line(scanner.Text())
	}
	return session.Wait()
}

// Quote is text as one word of the shell.
func Quote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}
