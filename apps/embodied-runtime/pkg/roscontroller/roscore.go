package roscontroller

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// ROSCore manages the lifecycle of a ROS Core (roscore) process.
//
// roscore is launched as a child process with ROS_MASTER_URI set to
// the specified port on the pod IP. Each robot gets its own roscore on a
// unique port to avoid node name / topic conflicts.
type ROSCore struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	port    int
	podIP   string
	running bool
}

// NewROSCore creates a new ROSCore manager for the given port and pod IP.
func NewROSCore(port int, podIP string) *ROSCore {
	return &ROSCore{port: port, podIP: podIP}
}

// URI returns the ROS_MASTER_URI for this roscore instance, using the
// pod IP so roslaunch can reach it.
func (s *ROSCore) URI() string {
	ip := s.podIP
	if ip == "" {
		ip = "0.0.0.0"
	}
	return fmt.Sprintf("http://%s:%d", ip, s.port)
}

// Port returns the port this roscore is listening on.
func (s *ROSCore) Port() int {
	return s.port
}

// Start launches the ROS Core process on the configured port.
//
// roscore runs in the container's network namespace, binding to
// port s.port on all interfaces. ROS nodes connect via ROS_MASTER_URI.
func (s *ROSCore) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("roscore already running on port %d", s.port)
	}

	cmd := exec.Command("roscore", "-p", fmt.Sprintf("%d", s.port))
	cmd.Env = append(os.Environ(),
		// roscore itself must bind to 0.0.0.0 so it listens on all interfaces.
		fmt.Sprintf("ROS_MASTER_URI=http://0.0.0.0:%d", s.port),
	)
	if s.podIP != "" {
		cmd.Env = append(cmd.Env, "ROS_IP="+s.podIP)
	}
	cmd.Stdout = &processLogger{prefix: fmt.Sprintf("[roscore:%d]", s.port)}
	cmd.Stderr = cmd.Stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start roscore on port %d: %w", s.port, err)
	}

	s.cmd = cmd
	s.running = true
	log.Printf("[ros-controller] ROS Core: started on port %d (pid=%d)", s.port, cmd.Process.Pid)
	return nil
}

// WaitReady blocks until the roscore HTTP server is responding, or the
// timeout expires. It polls the XML-RPC endpoint with HTTP GET requests
// to verify the server is fully initialized and accepting connections.
func (s *ROSCore) WaitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	client := &http.Client{Timeout: time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("http://%s/", addr))
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("roscore on port %d not ready within %v", s.port, timeout)
}

// Healthy checks whether the roscore HTTP server is currently responding.
// It makes a lightweight HTTP GET request to the XML-RPC endpoint.
// Returns false if the process is not running or the server is unresponsive.
func (s *ROSCore) Healthy() bool {
	s.mu.Lock()
	if !s.running || s.cmd == nil || s.cmd.Process == nil {
		s.mu.Unlock()
		return false
	}
	s.mu.Unlock()

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/", addr))
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

// Stop gracefully shuts down the ROS Core process.
func (s *ROSCore) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.cmd == nil || s.cmd.Process == nil {
		s.running = false
		return nil
	}

	pid := s.cmd.Process.Pid

	// Send SIGTERM to the process group so roscore and any child
	// processes (rosout, etc.) shut down cleanly.
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
	}

	state, err := s.cmd.Process.Wait()
	if err != nil {
		return fmt.Errorf("wait for roscore: %w", err)
	}

	s.running = false
	log.Printf("[ros-controller] ROS Core: stopped (pid=%d, state=%v)", pid, state)
	return nil
}

// IsRunning returns whether the ROS Core process is still alive (signal 0).
func (s *ROSCore) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.cmd == nil || s.cmd.Process == nil {
		return false
	}

	// Signal(0) checks if the process is alive without sending a signal.
	if err := s.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		s.running = false
		return false
	}
	return true
}

// Wait blocks until the ROS Core process exits.
func (s *ROSCore) Wait() error {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()

	if cmd == nil {
		return nil
	}

	state, err := cmd.Process.Wait()
	log.Printf("[ros-controller] ROS Core: exited (pid=%d, state=%v)", cmd.Process.Pid, state)
	return err
}
