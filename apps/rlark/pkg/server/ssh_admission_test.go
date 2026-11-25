package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/ssh"
	"github.com/go-logr/zapr"
	"github.com/gorilla/websocket"
	"github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	"github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlark "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/remotedialer"
	"github.com/rlinf/rlark/apps/rlark/pkg/server/reverseproxy"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	gossh "golang.org/x/crypto/ssh"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type admissionContext struct {
	ssh.Context
	inner       context.Context
	mu          sync.Mutex
	values      map[interface{}]interface{}
	permissions *ssh.Permissions
}

type admissionRoundTripper func(*http.Request) (*http.Response, error)

func (f admissionRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSSHRouteLookupCancellationAndSharedDeadline(t *testing.T) {
	started := make(chan time.Duration, 1)
	client, err := versioned.NewForConfig(&rest.Config{Host: "https://management.invalid", Transport: admissionRoundTripper(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Error("route lookup has no deadline")
		}
		started <- time.Until(deadline)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})})
	require.NoError(t, err)
	s := &Server{config: DefaultConfig(), rlarkClient: client, dialerFactory: reverseproxy.NewDialerFactory()}
	ctx := newAdmissionContext()
	inner, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx.inner = inner
	ch := &pendingChannel{}
	done := make(chan struct{})
	go func() { defer close(done); s.handleSSHChannel(nil, nil, ch, ctx) }()
	select {
	case remaining := <-started:
		require.Greater(t, remaining, 25*time.Second)
		require.LessOrEqual(t, remaining, 30*time.Second)
	case <-time.After(time.Second):
		t.Fatal("route lookup did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSH disconnect did not cancel route lookup")
	}
	require.True(t, ch.rejected)
	require.False(t, ch.accepted)
	require.Equal(t, gossh.ConnectionFailed, ch.code)
}

func (c *admissionContext) Lock()   { c.mu.Lock() }
func (c *admissionContext) Unlock() { c.mu.Unlock() }

func TestSSHRealChannelTrafficAndUpstreamFailure(t *testing.T) {
	factory := reverseproxy.NewDialerFactory()
	endpoint := httptest.NewServer(factory)
	defer endpoint.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	headers := make(http.Header)
	headers.Set(reverseproxy.ClientKeyHeader, "cluster")
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(endpoint.URL, "http"), headers)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()
	clientSession := remotedialer.NewClientSessionWithDialer(func(string, string) bool { return true }, ws, func(context.Context, string, string) (net.Conn, error) {
		c, peer := net.Pipe()
		go func() { defer func() { _ = peer.Close() }(); _, _ = io.Copy(peer, peer) }()
		return c, nil
	})
	defer clientSession.Close()
	tunnelDone := make(chan struct{})
	go func() { defer close(tunnelDone); _, _ = clientSession.Serve(ctx) }()
	t.Cleanup(func() { cancel(); clientSession.Close(); <-tunnelDone })
	s := &Server{config: DefaultConfig(), rlarkClient: fake.NewSimpleClientset(&rlark.DomainPeer{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: apis.RLarkAgentNamespacePrefix + "cluster"},
		Spec:       rlark.DomainPeerSpec{Pods: []rlark.DomainPodInfo{{LocalIP: "10.0.0.2"}}},
	}), dialerFactory: factory}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(key)
	require.NoError(t, err)
	sshServer := &ssh.Server{PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
		ctx.Permissions().Permissions = newAdmissionContext().permissions.Permissions
		return true
	}}
	sshServer.AddHostKey(signer)
	require.NoError(t, s.channelOption(sshServer))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverDone := make(chan struct{})
	go func() { defer close(serverDone); _ = sshServer.Serve(listener) }()
	t.Cleanup(func() { _ = sshServer.Close(); <-serverDone })
	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{User: "test", Auth: []gossh.AuthMethod{gossh.PublicKeys(signer)}, HostKeyCallback: gossh.FixedHostKey(signer.PublicKey()), Timeout: time.Second})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()
	// DialerFactory registration may follow the WebSocket client handshake.
	deadline := time.Now().Add(time.Second)
	for {
		_, err = factory.GetDialer(ctx, "cluster")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	open := func() net.Conn {
		conn, err := client.Dial("tcp", "10.0.0.2.cluster.agent:80")
		require.NoError(t, err)
		return conn
	}
	exchange := func(conn net.Conn) {
		done := make(chan error, 1)
		go func() {
			_, err := conn.Write([]byte("payload"))
			if err == nil {
				data := make([]byte, 7)
				_, err = io.ReadFull(conn, data)
				if err == nil && string(data) != "payload" {
					err = io.ErrUnexpectedEOF
				}
			}
			done <- err
		}()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("SSH traffic stalled")
		}
	}
	active := open()
	defer func() { _ = active.Close() }()
	exchange(active)
	for range 12 {
		conn := open()
		exchange(conn)
		_ = conn.Close()
	}
	exchange(active)
	// A different unavailable route is rejected; it must not retire the shared
	// SSH connection or interrupt the existing channel.
	conn, err := client.Dial("tcp", "10.0.0.2.missing.agent:80")
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err)
	var rejected *gossh.OpenChannelError
	require.ErrorAs(t, err, &rejected)
	require.Equal(t, gossh.Prohibited, rejected.Reason)
	exchange(active)
	fresh := open()
	exchange(fresh)
	_ = fresh.Close()
}

func (c *admissionContext) Deadline() (time.Time, bool) { return c.inner.Deadline() }
func (c *admissionContext) Done() <-chan struct{}       { return c.inner.Done() }
func (c *admissionContext) Err() error                  { return c.inner.Err() }

func (c *admissionContext) Value(k interface{}) interface{} {
	if value, ok := c.values[k]; ok {
		return value
	}
	return c.inner.Value(k)
}
func (c *admissionContext) SetValue(k, v interface{})     { c.values[k] = v }
func (c *admissionContext) User() string                  { return "test" }
func (c *admissionContext) SessionID() string             { return "test-session" }
func (c *admissionContext) Permissions() *ssh.Permissions { return c.permissions }

func newAdmissionContext() *admissionContext {
	certificate := &gossh.Certificate{}
	cert.SetSSHCertMeta(certificate, map[string]string{apis.MetaCertRole: "domain", apis.MetaDomainID: "test"})
	return &admissionContext{inner: context.Background(), values: make(map[interface{}]interface{}), permissions: &ssh.Permissions{Permissions: &certificate.Permissions}}
}

type pendingChannel struct {
	accepted bool
	rejected bool
	message  string
	code     gossh.RejectionReason
}

func (*pendingChannel) ChannelType() string { return "direct-tcpip" }
func (*pendingChannel) ExtraData() []byte {
	return gossh.Marshal(&apis.SSHDirectPayload{Host: "10.0.0.2.cluster.agent", Port: 80})
}
func (c *pendingChannel) Accept() (gossh.Channel, <-chan *gossh.Request, error) {
	c.accepted = true
	panic("unavailable upstream must not be accepted")
}
func (c *pendingChannel) Reject(reason gossh.RejectionReason, message string) error {
	c.rejected = true
	c.message = message
	c.code = reason
	return nil
}

func TestSSHUnavailableUpstreamRejectedBeforeAccept(t *testing.T) {
	s := &Server{config: DefaultConfig(), rlarkClient: fake.NewSimpleClientset(&rlark.DomainPeer{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: apis.RLarkAgentNamespacePrefix + "cluster"},
		Spec:       rlark.DomainPeerSpec{Pods: []rlark.DomainPodInfo{{LocalIP: "10.0.0.2"}}},
	}), dialerFactory: reverseproxy.NewDialerFactory()}
	ctx := newAdmissionContext()
	for range 3 {
		ch := &pendingChannel{}
		s.handleSSHChannel(nil, nil, ch, ctx)
		require.False(t, ch.accepted)
		require.True(t, ch.rejected)
		require.Equal(t, "Target connection unavailable", ch.message)
		require.Equal(t, gossh.ConnectionFailed, ch.code)
	}
}

func TestSSHPendingGateLimitAndRecovery(t *testing.T) {
	config := DefaultConfig()
	config.SSHMaxPendingTargets = 2
	config.SSHAdmissionWaitTimeout = 30 * time.Millisecond
	s := &Server{config: config}
	ctx := newAdmissionContext()
	releases := make([]func(), 0, config.SSHMaxPendingTargets)
	for range config.SSHMaxPendingTargets {
		release, err := s.acquireSSHTarget(ctx, ctx)
		require.NoError(t, err)
		releases = append(releases, release)
	}
	_, err := s.acquireSSHTarget(ctx, ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Zero(t, len(s.sshTargetGate(ctx).waiters))
	releases[0]()
	releases[0]() // Release is idempotent.
	release, err := s.acquireSSHTarget(ctx, ctx)
	require.NoError(t, err)
	release()
	for _, release := range releases[1:] {
		release()
	}
	// A separate SSH session has its own admission budget.
	other := newAdmissionContext()
	release, err = s.acquireSSHTarget(other, other)
	require.NoError(t, err)
	release()
	inner, cancel := context.WithCancel(context.Background())
	ctx.inner = inner
	cancel()
	_, err = s.acquireSSHTarget(ctx, ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSSHQueuedAdmissionRecoveryAndCancellation(t *testing.T) {
	config := DefaultConfig()
	config.SSHMaxPendingTargets = 1
	s := &Server{config: config}
	ctx := newAdmissionContext()
	release, err := s.acquireSSHTarget(ctx, ctx)
	require.NoError(t, err)
	defer release()
	gate := s.sshTargetGate(ctx)
	result := make(chan func(), 1)
	go func() {
		release, err := s.acquireSSHTarget(ctx, ctx)
		if err != nil {
			result <- nil
			return
		}
		result <- release
	}()
	require.Eventually(t, func() bool { return len(gate.waiters) == 1 }, time.Second, time.Millisecond)
	_, err = s.acquireSSHTarget(ctx, ctx)
	require.ErrorIs(t, err, errSSHAdmissionQueueFull)
	release()
	select {
	case admitted := <-result:
		require.NotNil(t, admitted)
		defer admitted()
	case <-time.After(time.Second):
		t.Fatal("queued open did not resume after a slot was released")
	}
	require.Zero(t, len(gate.waiters))
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		release, err := s.acquireSSHTarget(ctx, waitCtx)
		if release != nil {
			release()
		}
		done <- err
	}()
	require.Eventually(t, func() bool { return len(gate.waiters) == 1 }, time.Second, time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("queued open ignored cancellation")
	}
	require.Zero(t, len(gate.waiters))
	require.Equal(t, 1, len(gate.slots))
}

func TestSSHQueuedAdmissionHasNoRouteEffectsBeforeAdmission(t *testing.T) {
	config := DefaultConfig()
	config.SSHMaxPendingTargets = 1
	config.SSHAdmissionWaitTimeout = 40 * time.Millisecond
	s := &Server{config: config} // Any route access would panic: dependencies are deliberately absent.
	ctx := newAdmissionContext()
	release, err := s.acquireSSHTarget(ctx, ctx)
	require.NoError(t, err)
	defer release()
	ch := &pendingChannel{}
	s.handleSSHChannel(nil, nil, ch, ctx)
	require.True(t, ch.rejected)
	require.False(t, ch.accepted)
	require.Equal(t, gossh.ResourceShortage, ch.code)
	require.Zero(t, len(s.sshTargetGate(ctx).waiters))
}

func TestSSHRejectionCodesAndRateLimitedLogs(t *testing.T) {
	for _, test := range []struct {
		reason string
		code   gossh.RejectionReason
	}{
		{"missing_permissions", gossh.Prohibited},
		{"missing_metadata", gossh.Prohibited},
		{"invalid_payload", gossh.Prohibited},
		{"route_unavailable", gossh.Prohibited},
		{"admission_timeout", gossh.ResourceShortage},
		{"admission_queue_full", gossh.ResourceShortage},
		{"admission_canceled", gossh.ConnectionFailed},
		{"route_timeout", gossh.ConnectionFailed},
		{"upstream_unavailable", gossh.ConnectionFailed},
		{"open_canceled", gossh.ConnectionFailed},
	} {
		t.Run(test.reason, func(t *testing.T) {
			core, logs := observer.New(zap.InfoLevel)
			ctx := newAdmissionContext()
			ctx.inner = log.WithLogger(ctx.inner, zapr.NewLogger(zap.New(core)))
			s := &Server{config: DefaultConfig()}
			for range 6 {
				ch := &pendingChannel{}
				s.rejectSSHChannel(ctx, ch, test.reason, "sensitive-client-payload")
				require.True(t, ch.rejected)
				require.Equal(t, test.code, ch.code)
			}
			require.Len(t, logs.All(), 5)
			for _, entry := range logs.All() {
				require.Equal(t, "SSH channel rejected", entry.Message)
				require.Equal(t, test.reason, entry.ContextMap()["reason"])
				require.EqualValues(t, test.code, entry.ContextMap()["code"])
				require.EqualValues(t, 64, entry.ContextMap()["limit"])
				require.NotContains(t, entry.ContextMap(), "message")
			}
		})
	}
}
