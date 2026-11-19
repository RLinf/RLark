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
	target := net.JoinHostPort(address, strconv.Itoa(port))
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, "", &Error{Unreachable, fmt.Errorf("cannot reach %s: %v", target, err)}
	}
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
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
			return nil
		},
	}
	connection, channels, requests, err := ssh.NewClientConn(conn, target, config)
	if err != nil {
		conn.Close()
		if errors.Is(err, errHostKeyChanged) {
			return nil, "", &Error{HostKeyChanged, fmt.Errorf("%s shows the host key %s, not %s", target, shown, hostKey)}
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, "", &Error{LoginRejected, fmt.Errorf("%s did not accept the user %s with this password", target, user)}
		}
		return nil, "", &Error{Unreachable, fmt.Errorf("no SSH login at %s: %v", target, err)}
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(connection, channels, requests), shown, nil
}

var errHostKeyChanged = errors.New("the host key changed")

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
