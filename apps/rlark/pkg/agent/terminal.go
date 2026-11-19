package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
)

const fileChunkSize = 32 * 1024

type fileUploadStart struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	Mode uint32 `json:"mode"`
}

type fileDownloadStart struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type fileDownloadResp struct {
	Type  string `json:"type"`
	Size  int64  `json:"size"`
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

type fileUploadEnd struct {
	Type string `json:"type"`
}

type fileDownloadEnd struct {
	Type string `json:"type"`
}

type fileTransferDone struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// wsResizeMsg is the resize control message sent by the client.
type wsResizeMsg struct {
	Type string `json:"type"`
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

// wsErrorMsg is a structured error message sent to the client so it can be
// displayed on the terminal instead of being silently dropped.
type wsErrorMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// wsSizeQueue implements remotecommand.TerminalSizeQueue. It receives resize
// events from the WebSocket readLoop and blocks Next() until a new size is
// available or the queue is closed.
type wsSizeQueue struct {
	mu   sync.Mutex
	cond *sync.Cond
	size remotecommand.TerminalSize
	has  bool
	done bool
}

func newWSSizeQueue() *wsSizeQueue {
	q := &wsSizeQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *wsSizeQueue) resize(rows, cols uint16) {
	q.mu.Lock()
	q.size = remotecommand.TerminalSize{Width: cols, Height: rows}
	q.has = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (q *wsSizeQueue) close() {
	q.mu.Lock()
	q.done = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

// Next blocks until a new terminal size is available or the queue is closed.
// It returns nil when monitoring has been stopped.
func (q *wsSizeQueue) Next() *remotecommand.TerminalSize {
	q.mu.Lock()
	for !q.has && !q.done {
		q.cond.Wait()
	}
	if q.done {
		q.mu.Unlock()
		return nil
	}
	size := q.size
	q.has = false
	q.mu.Unlock()
	return &size
}

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (a *Agent) handleTerminal(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())

	if a.localKubeClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "kubernetes client not available"})
		return
	}

	namespace := c.Param("namespace")
	podName := c.Param("pod")
	container := c.DefaultQuery("container", "main")
	cmd := c.DefaultQuery("command", "/bin/bash")

	if namespace == "" || podName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "namespace and pod are required"})
		return
	}

	ws, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error(err, "failed to upgrade WebSocket connection")
		return
	}
	defer func() { _ = ws.Close() }()

	execReq := a.localKubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("exec").
		Param("container", container).
		Param("command", cmd).
		Param("stdin", "true").
		Param("stdout", "true").
		Param("stderr", "true").
		Param("tty", "true")

	pipe := newWSPipe(ws, a.localKubeConfig, a.localKubeClient, namespace, podName, container, logger)
	defer func() {
		pipe.sizeQueue.close()
		_ = pipe.Close()
	}()

	executor, err := remotecommand.NewSPDYExecutor(a.localKubeConfig, "POST", execReq.URL())
	if err != nil {
		logger.Error(err, "failed to create SPDY executor")
		pipe.sendError(fmt.Sprintf("failed to create executor: %v", err))
		return
	}

	err = executor.StreamWithContext(c.Request.Context(), remotecommand.StreamOptions{
		Stdin:             pipe,
		Stdout:            pipe,
		Stderr:            pipe,
		Tty:               true,
		TerminalSizeQueue: pipe.sizeQueue,
	})
	if err != nil {
		var exitErr utilexec.ExitError
		if errors.As(err, &exitErr) && exitErr.Exited() {
			logger.Info("terminal session ended", "exitCode", exitErr.ExitStatus())
			pipe.closeNormal(fmt.Sprintf("terminal exited with code %d", exitErr.ExitStatus()))
			return
		}
		logger.Error(err, "exec stream error")
		pipe.sendError(fmt.Sprintf("exec error: %v", err))
		return
	}
	pipe.closeNormal("terminal session ended")
}

type wsPipe struct {
	ws        *websocket.Conn
	wmu       sync.Mutex
	buf       []byte
	rcond     *sync.Cond
	closed    bool
	sizeQueue *wsSizeQueue

	kubeConfig *rest.Config
	kubeClient kubernetes.Interface
	namespace  string
	podName    string
	container  string
	logger     logr.Logger
}

func newWSPipe(ws *websocket.Conn, kubeConfig *rest.Config, kubeClient kubernetes.Interface, namespace, podName, container string, logger logr.Logger) *wsPipe {
	p := &wsPipe{
		ws:         ws,
		rcond:      sync.NewCond(&sync.Mutex{}),
		sizeQueue:  newWSSizeQueue(),
		kubeConfig: kubeConfig,
		kubeClient: kubeClient,
		namespace:  namespace,
		podName:    podName,
		container:  container,
		logger:     logger,
	}
	go p.readLoop()
	return p
}

func (p *wsPipe) readLoop() {
	for {
		msgType, data, err := p.ws.ReadMessage()
		if err != nil {
			p.rcond.L.Lock()
			p.closed = true
			p.rcond.Broadcast()
			p.rcond.L.Unlock()
			return
		}

		// Dispatch Text JSON control messages by type. Unknown Text
		// messages are dropped — never injected into the stdin buffer.
		if msgType == websocket.TextMessage {
			if len(data) > 0 && data[0] == '{' {
				var probe struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data, &probe) == nil {
					switch probe.Type {
					case "resize":
						var msg wsResizeMsg
						if json.Unmarshal(data, &msg) == nil {
							p.sizeQueue.resize(msg.Rows, msg.Cols)
						}
						continue
					case "file-upload", "file-download":
						p.handleFileTransfer(data, probe.Type)
						continue
					}
				}
			}
			// Unknown or malformed Text message: drop.
			continue
		}

		// Binary messages carry terminal I/O.
		p.rcond.L.Lock()
		p.buf = append(p.buf, data...)
		p.rcond.Broadcast()
		p.rcond.L.Unlock()
	}
}

func (p *wsPipe) handleFileTransfer(controlData []byte, msgType string) {
	switch msgType {
	case "file-upload":
		p.handleFileUpload(controlData)
	case "file-download":
		p.handleFileDownload(controlData)
	}
}

func (p *wsPipe) handleFileUpload(controlData []byte) {
	var req fileUploadStart
	if err := json.Unmarshal(controlData, &req); err != nil {
		p.sendFileDone(false, fmt.Sprintf("invalid upload request: %v", err))
		return
	}

	tmpFile, err := os.CreateTemp("", "rlark-upload-*")
	if err != nil {
		p.sendFileDone(false, fmt.Sprintf("create temp file: %v", err))
		return
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	defer func() { _ = tmpFile.Close() }()

	for {
		msgType, data, err := p.ws.ReadMessage()
		if err != nil {
			p.sendFileDone(false, fmt.Sprintf("read error during upload: %v", err))
			return
		}

		if msgType == websocket.TextMessage && len(data) > 0 && data[0] == '{' {
			var end fileUploadEnd
			if json.Unmarshal(data, &end) == nil && end.Type == "file-upload-end" {
				break
			}
		}

		if _, werr := tmpFile.Write(data); werr != nil {
			p.sendFileDone(false, fmt.Sprintf("write temp file: %v", werr))
			return
		}
	}

	_ = tmpFile.Sync()
	_, _ = tmpFile.Seek(0, 0)

	if err := p.copyFileToPod(tmpFile, req.Path); err != nil {
		p.sendFileDone(false, fmt.Sprintf("copy to pod: %v", err))
		return
	}

	p.sendFileDone(true, "")
}

func (p *wsPipe) handleFileDownload(controlData []byte) {
	var req fileDownloadStart
	if err := json.Unmarshal(controlData, &req); err != nil {
		p.sendFileDone(false, fmt.Sprintf("invalid download request: %v", err))
		return
	}

	tmpFile, err := os.CreateTemp("", "rlark-download-*")
	if err != nil {
		p.sendFileDone(false, fmt.Sprintf("create temp file: %v", err))
		return
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	defer func() { _ = tmpFile.Close() }()

	size, err := p.copyFileFromPod(tmpFile, req.Path)
	if err != nil {
		p.sendFileDone(false, fmt.Sprintf("copy from pod: %v", err))
		return
	}

	resp := fileDownloadResp{
		Type: "file-download-start",
		Size: size,
		Name: filepath.Base(req.Path),
	}
	header, _ := json.Marshal(resp)
	_ = p.ws.WriteMessage(websocket.TextMessage, header)

	_, _ = tmpFile.Seek(0, 0)
	buf := make([]byte, fileChunkSize)
	for {
		n, rerr := tmpFile.Read(buf)
		if n > 0 {
			_ = p.ws.WriteMessage(websocket.BinaryMessage, buf[:n])
		}
		if rerr == io.EOF || n == 0 {
			break
		}
		if rerr != nil {
			p.sendFileDone(false, fmt.Sprintf("read temp file: %v", rerr))
			return
		}
	}

	end := fileDownloadEnd{Type: "file-download-end"}
	endData, _ := json.Marshal(end)
	_ = p.ws.WriteMessage(websocket.TextMessage, endData)
}

func (p *wsPipe) sendFileDone(success bool, errMsg string) {
	done := fileTransferDone{
		Type:    "file-transfer-done",
		Success: success,
		Error:   errMsg,
	}
	data, _ := json.Marshal(done)
	_ = p.ws.WriteMessage(websocket.TextMessage, data)
}

// sendError sends a structured error message to the client as a Text JSON
// message. The client injects it into the terminal Read buffer so the user
// can see it instead of the error being silently dropped.
func (p *wsPipe) sendError(msg string) {
	data, _ := json.Marshal(wsErrorMsg{Type: "error", Message: msg})
	_ = p.ws.WriteMessage(websocket.TextMessage, data)
}

func (p *wsPipe) copyFileToPod(src *os.File, destPath string) error {
	execReq := p.kubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(p.namespace).
		Name(p.podName).
		SubResource("exec").
		Param("container", p.container).
		Param("command", "sh").
		Param("command", "-c").
		Param("command", fmt.Sprintf("cat > '%s'", destPath)).
		Param("stdin", "true").
		Param("stdout", "true").
		Param("stderr", "true").
		Param("tty", "false")

	executor, err := remotecommand.NewSPDYExecutor(p.kubeConfig, "POST", execReq.URL())
	if err != nil {
		return fmt.Errorf("create executor: %w", err)
	}

	var stderrBuf bytes.Buffer
	err = executor.StreamWithContext(p.ctx(), remotecommand.StreamOptions{
		Stdin:  src,
		Stdout: io.Discard,
		Stderr: &stderrBuf,
		Tty:    false,
	})
	if err != nil {
		return fmt.Errorf("exec stream: %w (stderr: %s)", err, stderrBuf.String())
	}
	return nil
}

func (p *wsPipe) copyFileFromPod(dst *os.File, srcPath string) (int64, error) {
	execReq := p.kubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(p.namespace).
		Name(p.podName).
		SubResource("exec").
		Param("container", p.container).
		Param("command", "cat").
		Param("command", srcPath).
		Param("stdin", "false").
		Param("stdout", "true").
		Param("stderr", "true").
		Param("tty", "false")

	executor, err := remotecommand.NewSPDYExecutor(p.kubeConfig, "POST", execReq.URL())
	if err != nil {
		return 0, fmt.Errorf("create executor: %w", err)
	}

	var stderrBuf bytes.Buffer
	err = executor.StreamWithContext(p.ctx(), remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: dst,
		Stderr: &stderrBuf,
		Tty:    false,
	})
	if err != nil {
		return 0, fmt.Errorf("exec stream: %w (stderr: %s)", err, stderrBuf.String())
	}

	info, _ := dst.Stat()
	if info != nil {
		return info.Size(), nil
	}
	return 0, nil
}

func (p *wsPipe) ctx() context.Context {
	return context.Background()
}

// Read is an exported method.
func (p *wsPipe) Read(dst []byte) (int, error) {
	p.rcond.L.Lock()
	for len(p.buf) == 0 && !p.closed {
		p.rcond.Wait()
	}
	if len(p.buf) == 0 && p.closed {
		return 0, io.EOF
	}
	n := copy(dst, p.buf)
	p.buf = p.buf[n:]
	p.rcond.L.Unlock()
	return n, nil
}

// Write is an exported method.
func (p *wsPipe) Write(data []byte) (int, error) {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	err := p.ws.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

// Close is an exported method.
func (p *wsPipe) Close() error {
	p.rcond.L.Lock()
	p.closed = true
	p.rcond.Broadcast()
	p.rcond.L.Unlock()
	return p.ws.Close()
}

func (p *wsPipe) closeNormal(reason string) {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	_ = p.ws.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason),
		time.Now().Add(time.Second),
	)
}
