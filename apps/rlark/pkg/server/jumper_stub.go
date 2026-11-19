package server

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/ssh"
	"github.com/gorilla/websocket"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/jumper"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"k8s.io/apimachinery/pkg/labels"
)

// JumperStub implements jumper.TargetResolver and jumper.TerminalDialer for the
// SSH bastion host. It is created per SSH session and bridges the SSH pty's I/O
// with the jumper.Terminal I/O bridge.
//
// JumperStub is passed to jumper.Serve which handles the target selection menu
// and terminal session loop. The SSH pty's stdin/stdout and resize channel are
// translated and passed to the jumper package.
type JumperStub struct {
	s    *Server
	sess ssh.Session

	// User is the authenticated SSH username from the session.
	User string
	// Meta holds certificate metadata extracted from the SSH certificate.
	Meta map[string]string
}

// NewJumperStub creates a new JumperStub for the given SSH session.
func NewJumperStub(s *Server, sess ssh.Session, user string, meta map[string]string) *JumperStub {
	return &JumperStub{
		s:    s,
		sess: sess,
		User: user,
		Meta: meta,
	}
}

// JumperAvailable reports whether the bastion host is available for the caller.
// It always returns true; future versions may check SSH certificate metadata.
func (j *JumperStub) JumperAvailable() bool {
	return true
}

// Resolve implements jumper.TargetResolver. It returns all running pods sorted
// by name that the caller may connect to via the bastion host.
//
// TODO: filter pods based on the caller's permissions.
func (j *JumperStub) Resolve(ctx context.Context) ([]jumper.Target, error) {
	logger := log.GetLogger()
	pods, err := j.s.podLister.List(labels.Everything())
	if err != nil {
		logger.Error(err, "failed to list pods")
		return nil, fmt.Errorf("list pods: %w", err)
	}
	targets := make([]jumper.Target, 0, len(pods))
	for _, pod := range pods {
		if pod.Status.Phase != "Running" || pod.Status.IP == "" {
			continue
		}
		agentId := strings.TrimPrefix(pod.Namespace, apis.RLarkAgentNamespacePrefix)
		targets = append(targets, jumper.Target{
			ID:   pod.Namespace + "/" + pod.Name,
			Name: pod.Spec.TaskNamespace + "/" + pod.Spec.TaskName,
			Info: fmt.Sprintf("Pod(%s/%s) in %s", pod.Spec.PodNamespace, pod.Spec.PodName, agentId),
		})
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})
	return targets, nil
}

// Dial implements jumper.TerminalDialer. It establishes a WebSocket terminal
// session to the given target through the agent tunnel and returns a
// jumper.Terminal backed by the WebSocket connection.
//
// TODO: enforce permission control for the caller against the target.
func (j *JumperStub) Dial(ctx context.Context, target jumper.Target) (jumper.Terminal, error) {
	ns, name, _ := strings.Cut(target.ID, "/")
	if ns == "" || name == "" {
		return nil, fmt.Errorf("invalid target ID: %s", target.ID)
	}
	pod, err := j.s.podLister.Pods(ns).Get(name)
	if err != nil {
		return nil, fmt.Errorf("get pod: %w", err)
	}
	if pod.Status.Phase != "Running" || pod.Status.IP == "" {
		return nil, fmt.Errorf("pod is not running")
	}
	agentId := strings.TrimPrefix(pod.Namespace, apis.RLarkAgentNamespacePrefix)
	dialer, host, err := j.s.GetDial(ctx, "default", agentId+":80", j.Meta)
	if err != nil {
		return nil, fmt.Errorf("get dial: %w", err)
	}
	wsDialer := websocket.Dialer{
		NetDialContext: dialer,
	}
	targetURL := "ws://" + host + "/api/terminal/" + pod.Spec.PodNamespace + "/" + pod.Spec.PodName
	wsConn, _, err := wsDialer.DialContext(ctx, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("dial websocket: %w", err)
	}
	return jumper.NewWSTerminalFromConn(wsConn), nil
}

// Serve handles the SSH bastion host session. It bridges the SSH pty resize
// events into jumper.Window events, delegates to jumper.Serve for the target
// selection menu and terminal I/O loop, and blocks until the session ends.
//
// When a real pty is allocated, I/O goes through pty.Slave (an *os.File) so
// the jumper can use select() for interruptible reads and set raw mode on the
// fd. When the pty is emulated (no slave device), I/O goes through the SSH
// session channel directly.
func (j *JumperStub) Serve(ctx context.Context, pty ssh.Pty, sizeCh <-chan ssh.Window) {
	logger := log.GetLogger()
	resizeCh := make(chan jumper.Window, 1)
	go func() {
		defer close(resizeCh)
		for w := range sizeCh {
			resizeCh <- jumper.Window{Width: w.Width, Height: w.Height}
		}
	}()

	var stdin io.Reader = pty.Slave
	var stdout io.Writer = pty.Slave
	if j.sess.EmulatedPty() {
		stdin = j.sess
		stdout = j.sess
	}
	if err := jumper.Serve(ctx, j, j, stdin, stdout, resizeCh); err != nil {
		logger.Error(err, "bastion session ended with error")
	}
}
