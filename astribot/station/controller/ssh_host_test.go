package controller

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
)

// testSSHHost is an SSH server in the test's process: it counts the
// authentication requests it is sent, answers commands from a function, and
// forwards connections as a jump host does.
type testSSHHost struct {
	listener net.Listener
	password string
	// auths counts every password the host was sent, right or wrong.
	auths atomic.Int32
	// run answers a command with its output and exit status.
	run func(command string, stdin []byte) (string, uint32)
	// behind maps the addresses the host forwards to, as its side of the network has them.
	behind map[string]string

	mu     sync.Mutex
	signer ssh.Signer
}

func newTestSSHKey(t *testing.T) ssh.Signer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func newTestSSHHost(t *testing.T, password string) *testSSHHost {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := &testSSHHost{listener: listener, password: password, signer: newTestSSHKey(t), behind: map[string]string{},
		run: func(string, []byte) (string, uint32) { return "", 0 }}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go host.serve(conn)
		}
	}()
	return host
}

func (h *testSSHHost) address() string { return h.listener.Addr().String() }

func (h *testSSHHost) port() int32 {
	_, port, _ := net.SplitHostPort(h.address())
	number, _ := strconv.Atoi(port)
	return int32(number)
}

func (h *testSSHHost) fingerprint() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return ssh.FingerprintSHA256(h.signer.PublicKey())
}

func (h *testSSHHost) setKey(signer ssh.Signer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.signer = signer
}

func (h *testSSHHost) serve(conn net.Conn) {
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		h.auths.Add(1)
		if string(password) != h.password {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, nil
	}}
	h.mu.Lock()
	config.AddHostKey(h.signer)
	h.mu.Unlock()
	server, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer server.Close()
	go ssh.DiscardRequests(requests)
	for incoming := range channels {
		switch incoming.ChannelType() {
		case "session":
			channel, requests, err := incoming.Accept()
			if err != nil {
				continue
			}
			go h.session(channel, requests)
		case "direct-tcpip":
			var target struct {
				Address    string
				Port       uint32
				Origin     string
				OriginPort uint32
			}
			_ = ssh.Unmarshal(incoming.ExtraData(), &target)
			real, known := h.behind[net.JoinHostPort(target.Address, strconv.Itoa(int(target.Port)))]
			if !known {
				_ = incoming.Reject(ssh.ConnectionFailed, "no route to host")
				continue
			}
			far, err := net.Dial("tcp", real)
			if err != nil {
				_ = incoming.Reject(ssh.ConnectionFailed, err.Error())
				continue
			}
			channel, requests, err := incoming.Accept()
			if err != nil {
				_ = far.Close()
				continue
			}
			go ssh.DiscardRequests(requests)
			go func() { _, _ = io.Copy(far, channel); _ = far.Close() }()
			go func() { _, _ = io.Copy(channel, far); _ = channel.Close() }()
		default:
			_ = incoming.Reject(ssh.UnknownChannelType, "unsupported")
		}
	}
}

func (h *testSSHHost) session(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	for request := range requests {
		if request.Type != "exec" {
			_ = request.Reply(false, nil)
			continue
		}
		var command struct{ Text string }
		_ = ssh.Unmarshal(request.Payload, &command)
		_ = request.Reply(true, nil)
		stdin, _ := io.ReadAll(channel)
		output, status := h.run(command.Text, stdin)
		_, _ = io.WriteString(channel, output)
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
		return
	}
}
