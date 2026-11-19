package ros

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
)

// ---------------------------------------------------------------------------
// LocalManager — manages the ros-controller as a local subprocess.
// ---------------------------------------------------------------------------

// LocalManager manages the lifecycle of the ros-controller subprocess.
// It structurally satisfies deviceplugin.ControllerManager.
type LocalManager struct {
	mu sync.Mutex

	binPath       string
	configPath    string
	rosctrBinPath string

	cmd                 *exec.Cmd
	running             bool
	lastConfig          []byte // latest applied via ApplyConfig
	lastRestartedConfig []byte // config used when Start was last called
}

// NewLocalManager creates a new LocalManager for the ros-controller.
func NewLocalManager(binPath, configPath, rosctrBinPath string) *LocalManager {
	return &LocalManager{
		binPath:       binPath,
		configPath:    configPath,
		rosctrBinPath: rosctrBinPath,
	}
}

// ApplyConfig writes the ros-controller YAML config to the configured path
// and records it as the latest expected config for subsequent Maintain calls.
func (m *LocalManager) ApplyConfig(ctx context.Context, config []byte) error {
	m.mu.Lock()
	m.lastConfig = config
	m.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(m.configPath), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(m.configPath, config, 0644); err != nil {
		return fmt.Errorf("write ros-controller config: %w", err)
	}
	log.Printf("[device-plugin] ros-controller config written: %s", m.configPath)
	return nil
}

// Maintain checks whether the latest applied config differs from the
// currently running configuration. If it does, the ros-controller is
// restarted (or started if not running). Returns true when a restart or
// start was triggered.
func (m *LocalManager) Maintain(ctx context.Context) (bool, error) {
	m.mu.Lock()
	config := m.lastConfig
	runningCfg := m.lastRestartedConfig
	m.mu.Unlock()

	if bytes.Equal(config, runningCfg) {
		return false, nil
	}

	// Config was updated — write and restart.
	if err := m.ApplyConfig(ctx, config); err != nil {
		return false, err
	}

	m.mu.Lock()
	running := m.running
	m.mu.Unlock()

	if running {
		m.Stop(ctx)
	}

	if err := m.Start(ctx); err != nil {
		return false, fmt.Errorf("(re)start ros-controller: %w", err)
	}

	return true, nil
}

// Start launches the ros-controller subprocess.
func (m *LocalManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		log.Println("[device-plugin] ros-controller already running")
		return nil
	}

	// Check that the binary exists.
	if _, err := os.Stat(m.binPath); err != nil {
		return fmt.Errorf("ros-controller binary not found at %s: %w", m.binPath, err)
	}

	cmd := exec.Command(m.binPath,
		"--config="+m.configPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ros-controller: %w", err)
	}

	m.cmd = cmd
	m.running = true
	m.lastRestartedConfig = append([]byte(nil), m.lastConfig...)
	log.Printf("[device-plugin] ros-controller started (pid=%d)", cmd.Process.Pid)

	// Monitor the process in a goroutine.
	go func() {
		state, err := cmd.Process.Wait()
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		if err != nil {
			log.Printf("[device-plugin] ros-controller exited with error: %v (state=%v)", err, state)
		} else {
			log.Printf("[device-plugin] ros-controller exited: %v", state)
		}
	}()

	return nil
}

// Stop gracefully shuts down the ros-controller subprocess.
func (m *LocalManager) Stop(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		return
	}

	pid := m.cmd.Process.Pid
	log.Printf("[device-plugin] stopping ros-controller (pid=%d)...", pid)

	// Send SIGTERM to the process group.
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	m.running = false
}

// IsRunning returns whether the ros-controller subprocess is running.
func (m *LocalManager) IsRunning(ctx context.Context) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}
