package camera

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
// LocalManager — manages the camera-controller as a local subprocess.
// ---------------------------------------------------------------------------

// LocalManager manages the lifecycle of the camera-controller subprocess.
// It structurally satisfies deviceplugin.ControllerManager.
type LocalManager struct {
	mu sync.Mutex

	binPath       string
	configPath    string
	camctrBinPath string

	cmd                 *exec.Cmd
	running             bool
	lastConfig          []byte // latest applied via ApplyConfig
	lastRestartedConfig []byte // config used when Start was last called
}

// NewLocalManager creates a new LocalManager for the camera-controller.
func NewLocalManager(binPath, configPath, camctrBinPath string) *LocalManager {
	return &LocalManager{
		binPath:       binPath,
		configPath:    configPath,
		camctrBinPath: camctrBinPath,
	}
}

// ApplyConfig writes the camera-controller YAML config to the configured path
// and records it as the latest expected config for subsequent Maintain calls.
func (m *LocalManager) ApplyConfig(ctx context.Context, config []byte) error {
	m.mu.Lock()
	m.lastConfig = config
	m.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(m.configPath), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(m.configPath, config, 0644); err != nil {
		return fmt.Errorf("write camera-controller config: %w", err)
	}
	log.Printf("[device-plugin] camera-controller config written: %s", m.configPath)
	return nil
}

// Maintain checks whether the latest applied config differs from the
// currently running configuration. If it does, the camera-controller is
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
		return false, fmt.Errorf("(re)start camera-controller: %w", err)
	}

	return true, nil
}

// Start launches the camera-controller subprocess.
func (m *LocalManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		log.Println("[device-plugin] camera-controller already running")
		return nil
	}

	// Check that the binary exists.
	if _, err := os.Stat(m.binPath); err != nil {
		return fmt.Errorf("camera-controller binary not found at %s: %w", m.binPath, err)
	}

	cmd := exec.Command(m.binPath,
		"--config="+m.configPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start camera-controller: %w", err)
	}

	m.cmd = cmd
	m.running = true
	m.lastRestartedConfig = append([]byte(nil), m.lastConfig...)
	log.Printf("[device-plugin] camera-controller started (pid=%d)", cmd.Process.Pid)

	// Monitor the process in a goroutine.
	go func() {
		state, err := cmd.Process.Wait()
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		if err != nil {
			log.Printf("[device-plugin] camera-controller exited with error: %v (state=%v)", err, state)
		} else {
			log.Printf("[device-plugin] camera-controller exited: %v", state)
		}
	}()

	return nil
}

// Stop gracefully shuts down the camera-controller subprocess.
func (m *LocalManager) Stop(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		return
	}

	pid := m.cmd.Process.Pid
	log.Printf("[device-plugin] stopping camera-controller (pid=%d)...", pid)

	// Send SIGTERM to the process group.
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	m.running = false
}

// IsRunning returns whether the camera-controller subprocess is running.
func (m *LocalManager) IsRunning(ctx context.Context) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}
