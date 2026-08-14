package roscontroller

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// LaunchConfig contains everything needed to start a roslaunch process.
type LaunchConfig struct {
	Package    string
	LaunchFile string
	Args       []string
	Env        []string
}

// RobotProcess manages the lifecycle of a roslaunch process.
type RobotProcess struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
	pid     int

	// logBuf is a ring buffer of recent log lines.
	logBuf *logBuffer
	logger *processLogger

	// onExit is called when the process exits (in a goroutine).
	onExit func(err error)
}

// NewRobotProcess creates a new RobotProcess with an optional exit callback.
func NewRobotProcess(onExit func(err error)) *RobotProcess {
	buf := newLogBuffer(500)
	return &RobotProcess{
		logBuf: buf,
		logger: &processLogger{buf: buf},
		onExit: onExit,
	}
}

// Start launches a roslaunch process according to the given LaunchConfig.
func (p *RobotProcess) Start(cfg LaunchConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("process already running")
	}

	argv := []string{"roslaunch", cfg.Package, cfg.LaunchFile}
	argv = append(argv, cfg.Args...)

	p.cmd = exec.Command(argv[0], argv[1:]...)
	p.cmd.Env = os.Environ()
	p.cmd.Env = append(p.cmd.Env, cfg.Env...)

	p.logger.prefix = fmt.Sprintf("[roslaunch %s/%s]", cfg.Package, cfg.LaunchFile)
	p.cmd.Stdout = p.logger
	p.cmd.Stderr = p.logger
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("start roslaunch: %w", err)
	}

	p.pid = p.cmd.Process.Pid
	p.running = true
	log.Printf("[ros-controller] started roslaunch %s/%s (pid=%d)", cfg.Package, cfg.LaunchFile, p.pid)

	// Watcher goroutine: reap the child process and notify on exit.
	go p.watch()

	return nil
}

// watch blocks on cmd.Wait() then updates state and calls onExit.
func (p *RobotProcess) watch() {
	state, err := p.cmd.Process.Wait()

	p.mu.Lock()
	p.running = false
	p.mu.Unlock()

	if err != nil {
		log.Printf("[ros-controller] roslaunch wait error: %v (pid=%d)", err, p.pid)
	} else {
		log.Printf("[ros-controller] roslaunch exited: %v (pid=%d)", state, p.pid)
	}

	if p.onExit != nil {
		p.onExit(err)
	}
}

// PID returns the process ID.
func (p *RobotProcess) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pid
}

// Stop terminates the roslaunch process by sending SIGTERM to the process group.
func (p *RobotProcess) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running || p.cmd == nil || p.cmd.Process == nil {
		p.running = false
		return nil
	}

	if p.pid > 0 {
		if err := syscall.Kill(-p.pid, syscall.SIGTERM); err != nil {
			_ = p.cmd.Process.Signal(syscall.SIGTERM)
		}
	}

	state, err := p.cmd.Process.Wait()
	if err != nil {
		return fmt.Errorf("wait for roslaunch: %w", err)
	}

	p.running = false
	log.Printf("[ros-controller] roslaunch exited: %v (pid=%d)", state, p.pid)
	return nil
}

// IsRunning returns whether the roslaunch process is currently running.
func (p *RobotProcess) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running || p.cmd == nil || p.cmd.Process == nil {
		return false
	}

	return true
}

// Logs returns recent log lines from the process.
// tail=0 returns all buffered lines; tail>0 returns the last N lines.
func (p *RobotProcess) Logs(tail int) []string {
	return p.logBuf.Lines(tail)
}

// ---------------------------------------------------------------------------
// Log buffer — fixed-capacity ring buffer
// ---------------------------------------------------------------------------

type logBuffer struct {
	mu   sync.Mutex
	buf  []string
	cap  int
	pos  int // next write position
	full bool
}

func newLogBuffer(capacity int) *logBuffer {
	return &logBuffer{
		buf: make([]string, capacity),
		cap: capacity,
	}
}

func (b *logBuffer) Write(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buf[b.pos] = line
	b.pos++
	if b.pos >= b.cap {
		b.pos = 0
		b.full = true
	}
}

func (b *logBuffer) Lines(tail int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.full {
		// Buffer not yet full: return [0, pos).
		n := b.pos
		if tail > 0 && tail < n {
			n = tail
		}
		out := make([]string, n)
		copy(out, b.buf[:n])
		return out
	}

	// Buffer is full: return the last N (or all) entries.
	n := b.cap
	if tail > 0 && tail < n {
		n = tail
	}
	out := make([]string, n)
	// Ring buffer: b.pos is the oldest entry.
	start := b.pos
	for i := 0; i < n; i++ {
		out[i] = b.buf[(start+i)%b.cap]
	}
	return out
}

// ---------------------------------------------------------------------------
// processLogger
// ---------------------------------------------------------------------------

type processLogger struct {
	prefix string
	buf    *logBuffer
}

func (l *processLogger) Write(p []byte) (n int, err error) {
	line := fmt.Sprintf("%s %s", l.prefix, p)
	log.Print(line)
	if l.buf != nil {
		l.buf.Write(line)
	}
	return len(p), nil
}
