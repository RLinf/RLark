package gateway

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/nodessh"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func sshTestSigner(t *testing.T) ssh.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func serveSSHForJoin(listener net.Listener, signer ssh.Signer) {
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		config := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
		config.AddHostKey(signer)
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			_ = conn.Close()
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			channel, channelRequests, err := incoming.Accept()
			if err != nil {
				continue
			}
			go func() {
				defer channel.Close()
				for request := range channelRequests {
					if request.Type == "exec" {
						_ = request.Reply(true, nil)
						_, _ = channel.Write([]byte("joined over relay\n"))
						_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
						_ = channel.CloseWrite()
						return
					}
					_ = request.Reply(true, nil)
				}
			}()
		}
	}()
}

func TestNodeJoinDialUsesDirectBeforeRelayAndFallsBackOnlyWhenUnreachable(t *testing.T) {
	signer := sshTestSigner(t)
	key := ssh.FingerprintSHA256(signer.PublicKey())

	direct, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveSSHForJoin(direct, signer)
	relay, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	called := make(chan struct{}, 1)
	go func() {
		conn, err := relay.Accept()
		if err != nil {
			return
		}
		called <- struct{}{}
		_ = conn.Close()
	}()
	machine := rlarkv1alpha1.ClusterProvisionMachine{Address: direct.Addr().(*net.TCPAddr).IP.String(), Port: int32(direct.Addr().(*net.TCPAddr).Port), User: "root"}
	connected, _, err := dialProvisionMachinePinned(context.Background(), machine, []byte("login-password"), []byte("sudo-password"), key, signer.PublicKey().Type(), relay.Addr().String(), "relay-secret")
	if err != nil {
		t.Fatal(err)
	}
	_ = connected.Close()
	select {
	case <-called:
		t.Fatal("relay was called after a successful direct SSH login")
	case <-time.After(100 * time.Millisecond):
	}
	_ = direct.Close()

	server, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	request := make(chan string, 1)
	go func() {
		conn, err := server.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReaderSize(conn, 1)
		line, _ := reader.ReadString('\n')
		request <- line
		_, _ = io.WriteString(conn, "ok\n")
		serveSSHOnConn(conn, signer)
	}()
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedTarget := closed.Addr().(*net.TCPAddr)
	_ = closed.Close()
	machine = rlarkv1alpha1.ClusterProvisionMachine{Address: closedTarget.IP.String(), Port: int32(closedTarget.Port), User: "root"}
	connected, _, err = dialProvisionMachinePinned(context.Background(), machine, []byte("login-password"), []byte("sudo-password"), key, signer.PublicKey().Type(), server.Addr().String(), "relay-secret")
	if err != nil {
		t.Fatal(err)
	}
	_ = connected.Close()
	got := <-request
	if got != "gateway relay-secret "+machine.Address+":"+stringPort(machine.Port)+"\n" || strings.Contains(got, "login-password") || strings.Contains(got, "sudo-password") {
		t.Fatalf("unexpected relay request %q", got)
	}
}

func serveSSHOnConn(conn net.Conn, signer ssh.Signer) {
	config := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	config.AddHostKey(signer)
	server, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer server.Close()
	go ssh.DiscardRequests(requests)
	for incoming := range channels {
		channel, channelRequests, err := incoming.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer channel.Close()
			for request := range channelRequests {
				if request.Type == "exec" {
					_ = request.Reply(true, nil)
					_, _ = channel.Write([]byte("joined over relay\n"))
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
					_ = channel.CloseWrite()
					return
				}
				_ = request.Reply(true, nil)
			}
		}()
	}
}

func stringPort(port int32) string { return strconv.Itoa(int(port)) }

func TestNodeJoinHostKeyMismatchDoesNotFallBack(t *testing.T) {
	signer := sshTestSigner(t)
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	serveSSHForJoin(target, signer)
	relay, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	called := make(chan struct{}, 1)
	go func() {
		conn, err := relay.Accept()
		if err == nil {
			called <- struct{}{}
			_ = conn.Close()
		}
	}()
	addr := target.Addr().(*net.TCPAddr)
	_, _, err = dialProvisionMachinePinned(context.Background(), rlarkv1alpha1.ClusterProvisionMachine{Address: addr.IP.String(), Port: int32(addr.Port), User: "root"}, []byte("login"), []byte("sudo"), "SHA256:wrong", "", relay.Addr().String(), "relay-secret")
	var failure *nodessh.Error
	if !errors.As(err, &failure) || failure.Reason != nodessh.HostKeyChanged {
		t.Fatalf("expected pinned host-key refusal, got %v", err)
	}
	select {
	case <-called:
		t.Fatal("relay was called after a host-key mismatch")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestScannedNodeJoinReadsItsSiteRelayAndRunsCommandsOverIt(t *testing.T) {
	signer := sshTestSigner(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	const relayToken = "site-secret-token"
	requestLine := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReaderSize(conn, 1)
		line, _ := reader.ReadString('\n')
		requestLine <- line
		_, _ = io.WriteString(conn, "ok\n")
		serveSSHOnConn(conn, signer)
	}()
	closedTarget, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	target := closedTarget.Addr().(*net.TCPAddr)
	address, port := target.IP.String(), int32(target.Port)
	_ = closedTarget.Close()
	entryKey := ssh.FingerprintSHA256(signer.PublicKey())
	machineObject := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "Machine",
		"metadata": map[string]interface{}{"name": "candidate-1", "namespace": machineNamespace},
		"spec":     map[string]interface{}{"site": "office"},
		"status":   map[string]interface{}{"entry": map[string]interface{}{"via": listener.Addr().String()}},
	}}
	station := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{machineGVR: "MachineList"}, machineObject)
	raw := kubefake.NewSimpleClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "station-relay-token-office", Namespace: "default"}, Data: map[string][]byte{"token": []byte(relayToken)}})
	annotations := map[string]string{nodeJoinMachineAnnotation: "candidate-1", nodeJoinHostKeyAnnotation: entryKey, nodeJoinHostKeyAlgorithmAnnotation: signer.PublicKey().Type(), nodeJoinTypeAnnotation: "Server"}
	gateway := &Gateway{kubeClient: clientset.NewSimpleClientset(), rawClient: raw, astribotState: astribotState{stationClient: station}}
	join := &rlarkv1alpha1.NodeJoin{ObjectMeta: metav1.ObjectMeta{Annotations: annotations}, Spec: rlarkv1alpha1.NodeJoinSpec{Machine: rlarkv1alpha1.NodeJoinMachine{Address: address, Port: port, User: "root"}}}
	connected, _, err := gateway.dialNodeJoinMachine(context.Background(), join, []byte("login-password"), []byte("sudo-password"))
	if err != nil {
		t.Fatal(err)
	}
	defer connected.Close()
	out, err := connected.Run(context.Background(), "echo test", nil, nil)
	if err != nil || !strings.Contains(out, "joined over relay") {
		t.Fatalf("relay command output %q, err %v", out, err)
	}
	if got := <-requestLine; got != "gateway "+relayToken+" "+net.JoinHostPort(address, strconv.Itoa(int(port)))+"\n" {
		t.Fatalf("unexpected relay handshake %q", got)
	}
}

func TestDirectAuthenticationAndSudoFailuresDoNotFallBack(t *testing.T) {
	for _, test := range []struct {
		name           string
		authenticate   bool
		expectedReason string
	}{
		{name: "login rejected", authenticate: false, expectedReason: nodessh.LoginRejected},
		{name: "sudo rejected", authenticate: true, expectedReason: nodessh.SudoPasswordRejected},
	} {
		t.Run(test.name, func(t *testing.T) {
			signer := sshTestSigner(t)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
					if test.authenticate && string(password) == "login" {
						return nil, nil
					}
					return nil, errors.New("authentication refused")
				}}
				config.AddHostKey(signer)
				server, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					_ = conn.Close()
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					channel, channelRequests, err := incoming.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer channel.Close()
						for request := range channelRequests {
							if request.Type == "exec" {
								_ = request.Reply(true, nil)
								_, _ = channel.Write([]byte("Sorry, try again\n"))
								_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 1}))
								_ = channel.CloseWrite()
								return
							}
							_ = request.Reply(true, nil)
						}
					}()
				}
			}()
			relay, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer relay.Close()
			called := make(chan struct{}, 1)
			go func() {
				conn, err := relay.Accept()
				if err == nil {
					called <- struct{}{}
					_ = conn.Close()
				}
			}()
			addr := listener.Addr().(*net.TCPAddr)
			_, _, err = dialProvisionMachinePinned(context.Background(), rlarkv1alpha1.ClusterProvisionMachine{Address: addr.IP.String(), Port: int32(addr.Port), User: "ops"}, []byte("login"), []byte("wrong-sudo"), ssh.FingerprintSHA256(signer.PublicKey()), signer.PublicKey().Type(), relay.Addr().String(), "relay-secret")
			var failure *nodessh.Error
			if !errors.As(err, &failure) || failure.Reason != test.expectedReason {
				t.Fatalf("expected %s, got %v", test.expectedReason, err)
			}
			if strings.Contains(err.Error(), "relay-secret") {
				t.Fatal("relay token appeared in the returned error")
			}
			select {
			case <-called:
				t.Fatal("relay was called after authentication or sudo refusal")
			case <-time.After(100 * time.Millisecond):
			}
		})
	}
}

func TestNodeJoinAnnotationsPersistIntentButNeverCredentials(t *testing.T) {
	annotations, err := nodeJoinAnnotations(createNodeJoinRequest{MachineName: "candidate-1", MachineType: "Server", MachineModel: "x86-v2", HostKeyAlgorithm: "ssh-ed25519", Labels: map[string]string{"rlark.io/node-category": "server"}}, "SHA256:pinned")
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		nodeJoinMachineAnnotation: "candidate-1", nodeJoinTypeAnnotation: "Server", nodeJoinModelAnnotation: "x86-v2",
		nodeJoinHostKeyAnnotation: "SHA256:pinned", nodeJoinHostKeyAlgorithmAnnotation: "ssh-ed25519",
		nodeJoinInstallLabelsAnnotation: `{"rlark.io/node-category":"server"}`, nodeJoinNetworkRoleAnnotation: "Hub", nodeJoinReasonAnnotation: "SSH",
	} {
		if annotations[key] != value {
			t.Errorf("annotation %s = %q, want %q", key, annotations[key], value)
		}
	}
	for _, value := range annotations {
		if strings.Contains(value, "password") || strings.Contains(value, "token") {
			t.Fatalf("secret-like value stored in annotation %q", value)
		}
	}
}
