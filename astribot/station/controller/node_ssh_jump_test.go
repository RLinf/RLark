package controller

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestDialNodeSSHThroughAJumpHostSendsNoCredentialToAHostWithAnotherKey(t *testing.T) {
	ctx := context.Background()
	entry, node := newTestSSHHost(t, "entry-secret"), newTestSSHHost(t, "node-secret")
	entry.behind["192.168.0.10:22"] = node.address()
	node.run = func(command string, _ []byte) (string, uint32) { return "ran " + command, 0 }
	config := func() NodeSSHConfig {
		jump := NodeSSHConfig{Address: entry.address(), User: "orin", HostKeyFingerprint: entry.fingerprint(), Password: []byte("entry-secret"), Timeout: 2 * time.Second, HostKeyAlgorithms: classHostKeyAlgorithms}
		return NodeSSHConfig{Address: "192.168.0.10:22", User: "robot", HostKeyFingerprint: node.fingerprint(), Password: []byte("node-secret"), Timeout: 2 * time.Second, HostKeyAlgorithms: classHostKeyAlgorithms, Jump: &jump}
	}

	client, err := DialNodeSSH(ctx, config())
	if err != nil {
		t.Fatal(err)
	}
	if output, err := RunNodeSSHStream(ctx, client, "hostname", nil); err != nil || string(output) != "ran hostname" {
		t.Fatalf("a command on the host behind the jump host: %q %v", output, err)
	}
	_ = client.Close()
	entry.auths.Store(0)
	node.auths.Store(0)

	// A device at the entry's address that shows another key is sent nothing,
	// and nothing is dialed through it.
	other := newTestSSHHost(t, "entry-secret")
	cfg := config()
	cfg.Jump.Address = other.address()
	var mismatch *SSHHostKeyMismatchError
	if _, err := DialNodeSSH(ctx, cfg); !errors.As(err, &mismatch) || mismatch.Received != other.fingerprint() {
		t.Fatalf("a jump host with another key: %v", err)
	}
	if other.auths.Load() != 0 || node.auths.Load() != 0 {
		t.Fatalf("authentication requests sent: %d to the unknown device, %d through it", other.auths.Load(), node.auths.Load())
	}
	// Likewise the host behind the entry device.
	cfg = config()
	cfg.HostKeyFingerprint = other.fingerprint()
	if _, err := DialNodeSSH(ctx, cfg); !errors.As(err, &mismatch) || node.auths.Load() != 0 {
		t.Fatalf("a host with another key behind the jump host: %v, %d authentication requests", err, node.auths.Load())
	}

	cfg = config()
	cfg.Jump.Password = []byte("changed")
	if _, err := DialNodeSSH(ctx, cfg); !errors.Is(err, ErrSSHCredentialRejected) || node.auths.Load() != 0 {
		t.Fatalf("a jump host that refuses the account: %v", err)
	}
	cfg = config()
	cfg.Password = []byte("changed")
	if _, err := DialNodeSSH(ctx, cfg); !errors.Is(err, ErrSSHCredentialRejected) {
		t.Fatalf("a host that refuses the account: %v", err)
	}
	cfg = config()
	cfg.Address = "192.168.0.99:22"
	if _, err := DialNodeSSH(ctx, cfg); !errors.Is(err, ErrSSHTargetUnreachable) {
		t.Fatalf("no host behind the jump host: %v", err)
	}
	gone := newTestSSHHost(t, "x")
	_ = gone.listener.Close()
	cfg = config()
	cfg.Jump.Address = gone.address()
	if _, err := DialNodeSSH(ctx, cfg); !errors.Is(err, ErrSSHJumpUnreachable) {
		t.Fatalf("no jump host: %v", err)
	}

	// A host nobody has logged in to is known by the key it shows through its verified entry device.
	cfg = config()
	learned := ""
	cfg.HostKeyFingerprint, cfg.LearnedHostKey = "", &learned
	client, err = DialNodeSSH(ctx, cfg)
	if err != nil || learned != node.fingerprint() {
		t.Fatalf("learn the key of the host behind the jump host: %q %v", learned, err)
	}
	_ = client.Close()

	// The entry device is in its site's network: the site's hub connects to it.
	relay, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	go func() {
		for {
			conn, err := relay.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				line, _ := bufio.NewReaderSize(conn, 64).ReadString('\n')
				if line != "192.168.31.56:22\n" {
					_, _ = io.WriteString(conn, "refused\n")
					return
				}
				far, err := net.Dial("tcp", entry.address())
				if err != nil {
					return
				}
				defer far.Close()
				_, _ = io.WriteString(conn, "ok\n")
				go func() { _, _ = io.Copy(far, conn) }()
				_, _ = io.Copy(conn, far)
			}()
		}
	}()
	cfg = config()
	cfg.Jump.Address, cfg.Jump.Via = "192.168.31.56:22", relay.Addr().String()
	client, err = DialNodeSSH(ctx, cfg)
	if err != nil {
		t.Fatalf("through the site's relay: %v", err)
	}
	_ = client.Close()
	cfg.Jump.Address = "192.168.31.57:22"
	if _, err := DialNodeSSH(ctx, cfg); !errors.Is(err, ErrSSHJumpUnreachable) {
		t.Fatalf("a host the relay refuses: %v", err)
	}
}
