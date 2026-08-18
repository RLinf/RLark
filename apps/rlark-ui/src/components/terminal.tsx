import { useEffect, useRef, useState, type ChangeEvent } from "react";
import { Download, TerminalSquare, Upload } from "lucide-react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import {
  getSafariTerminalInput,
  getTerminalCloseExitCode,
  getTerminalProcessExitCode,
  isTerminalProcessExitMessage,
  isSafariUserAgent,
  stripLegacyProxyCloseMessage,
} from "../utils/terminalKeyboard";
import "@xterm/xterm/css/xterm.css";

const workerStatusLabels: Record<string, string> = {
  Running: "运行中",
  Pending: "等待中",
  Succeeded: "已完成",
  Failed: "失败",
  Stopped: "已停止",
  Online: "在线",
  Offline: "离线",
};

export function TerminalPage({
  workerCRName,
  workerName,
  jobName,
  workerStatus,
}: {
  workerCRName: string;
  workerName: string;
  jobName: string;
  workerStatus: string;
}) {
  const termRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const termRefInner = useRef<Terminal | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const [transferStatus, setTransferStatus] = useState<string>("");
  const [connectionState, setConnectionState] = useState<
    "connecting" | "connected" | "disconnected"
  >("connecting");

  useEffect(() => {
    const terminalContainer = termRef.current;
    if (!terminalContainer) return;
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: "Menlo, Monaco, 'Courier New', monospace",
      theme: { background: "#1a1a2e" },
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(new WebLinksAddon());
    term.open(terminalContainer);
    fitAddon.fit();
    term.focus();
    termRefInner.current = term;

    term.writeln(`Connecting to ${workerName} ...`);

    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${proto}//${location.host}/api/v1/rlinf.io/v1alpha1/pods/${encodeURIComponent(workerCRName)}/terminal`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;
    ws.binaryType = "arraybuffer";

    const sendResize = () => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: "resize",
            rows: term.rows,
            cols: term.cols,
          }),
        );
      }
    };

    let downloading = false;
    let downloadChunks: Uint8Array[] = [];
    let downloadName = "";
    let legacyProxyCloseReceived = false;
    let terminalProcessExitCode: number | null = null;

    ws.onopen = () => {
      setConnectionState("connected");
      term.writeln("\r\nConnected. Starting shell ...\r\n");
      sendResize();
    };
    ws.onmessage = (e) => {
      if (typeof e.data === "string") {
        const sanitized = stripLegacyProxyCloseMessage(e.data);
        if (sanitized.legacyClose) {
          legacyProxyCloseReceived = true;
          if (!sanitized.output) return;
        }
        const messageData = sanitized.output;
        if (messageData.startsWith("{")) {
          let msg: {
            type?: string;
            name?: string;
            size?: number;
            success?: boolean;
            error?: string;
            message?: string;
          };
          try {
            msg = JSON.parse(messageData);
          } catch {
            term.write(messageData);
            return;
          }
          if (msg.type === "file-download-start") {
            downloading = true;
            downloadChunks = [];
            downloadName = msg.name || "download";
            setTransferStatus(
              `Downloading ${downloadName} (${msg.size || 0} bytes)...`,
            );
            return;
          }
          if (msg.type === "file-download-end") {
            if (downloading) {
              const blob = new Blob(downloadChunks as BlobPart[], {
                type: "application/octet-stream",
              });
              const url = URL.createObjectURL(blob);
              const a = document.createElement("a");
              a.href = url;
              a.download = downloadName;
              a.click();
              URL.revokeObjectURL(url);
              downloading = false;
              downloadChunks = [];
              setTransferStatus("");
              term.writeln(`\r\n\x1b[32mDownloaded ${downloadName}.\x1b[0m`);
            }
            return;
          }
          if (msg.type === "file-transfer-done") {
            setTransferStatus("");
            if (msg.success) {
              term.writeln("\r\n\x1b[32mFile transfer complete.\x1b[0m");
            } else {
              term.writeln(
                `\r\n\x1b[31mFile transfer failed: ${msg.error || "unknown"}\x1b[0m`,
              );
            }
            return;
          }
          if (msg.type === "error") {
            const errorMessage = msg.message || msg.error || "unknown error";
            if (isTerminalProcessExitMessage(errorMessage)) {
              terminalProcessExitCode =
                getTerminalProcessExitCode(errorMessage);
              return;
            }
            term.writeln(`\r\n\x1b[31m${errorMessage}\x1b[0m`);
            return;
          }
        }
        term.write(messageData);
      } else if (e.data instanceof ArrayBuffer) {
        if (downloading) {
          downloadChunks.push(new Uint8Array(e.data));
        } else {
          term.write(new Uint8Array(e.data));
        }
      }
    };
    ws.onerror = () => {
      setConnectionState("disconnected");
    };
    ws.onclose = (event) => {
      setConnectionState("disconnected");
      const exitCode =
        terminalProcessExitCode ?? getTerminalCloseExitCode(event.reason);
      if (exitCode !== null) {
        term.writeln(
          `\r\n\x1b[33mSession ended (exit code ${exitCode}).\x1b[0m`,
        );
      } else if (event.code === 1000 || legacyProxyCloseReceived) {
        term.writeln("\r\n\x1b[33mSession ended.\x1b[0m");
      } else {
        const reason = event.reason ? `: ${event.reason}` : "";
        term.writeln(
          `\r\n\x1b[31mConnection closed unexpectedly (${event.code})${reason}.\x1b[0m`,
        );
      }
    };
    const encoder = new TextEncoder();
    const sendTerminalInput = (data: string) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(encoder.encode(data));
      }
    };
    term.onData(sendTerminalInput);
    if (isSafariUserAgent(navigator.userAgent)) {
      term.attachCustomKeyEventHandler((event) => {
        const data = getSafariTerminalInput(event);
        if (data === null) return true;
        event.preventDefault();
        event.stopPropagation();
        sendTerminalInput(data);
        return false;
      });
    }
    term.onResize(() => sendResize());

    const focusTerminal = () => term.focus();
    terminalContainer.addEventListener("mousedown", focusTerminal);

    const onResize = () => fitAddon.fit();
    window.addEventListener("resize", onResize);

    return () => {
      window.removeEventListener("resize", onResize);
      terminalContainer.removeEventListener("mousedown", focusTerminal);
      ws.close();
      term.dispose();
      termRefInner.current = null;
    };
  }, [workerCRName, workerName]);

  const handleUploadClick = () => {
    fileInputRef.current?.click();
  };

  const handleFileSelected = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN)
      return;
    const ws = wsRef.current;
    const destPath = `./${file.name}`;
    setTransferStatus(
      `Uploading ${file.name} (${file.size} bytes) to ${destPath}...`,
    );

    ws.send(
      JSON.stringify({
        type: "file-upload",
        path: destPath,
        size: file.size,
        mode: 0o644,
      }),
    );

    const chunkSize = 32 * 1024;
    let offset = 0;
    const reader = new FileReader();

    const readChunk = () => {
      const slice = file.slice(offset, offset + chunkSize);
      reader.onload = () => {
        if (reader.result instanceof ArrayBuffer) {
          ws.send(reader.result);
          offset += reader.result.byteLength;
          if (offset < file.size) {
            readChunk();
          } else {
            ws.send(JSON.stringify({ type: "file-upload-end" }));
          }
        }
      };
      reader.readAsArrayBuffer(slice);
    };
    readChunk();
    e.target.value = "";
  };

  const [showDownloadInput, setShowDownloadInput] = useState(false);
  const [dlPath, setDlPath] = useState("");

  const handleDownloadSubmit = () => {
    if (
      !dlPath ||
      !wsRef.current ||
      wsRef.current.readyState !== WebSocket.OPEN
    )
      return;
    wsRef.current.send(JSON.stringify({ type: "file-download", path: dlPath }));
    setTransferStatus(`Requesting ${dlPath}...`);
    setShowDownloadInput(false);
    setDlPath("");
  };

  return (
    <main className="terminal-page">
      <div className="terminal-page-panel">
        <header className="terminal-toolbar">
          <div className="terminal-identity">
            <span className="terminal-app-icon">
              <TerminalSquare size={17} />
            </span>
            <div className="terminal-title">
              {jobName && <small>{jobName}</small>}
              <strong>{workerName}</strong>
              <span>
                {workerStatus && (
                  <em
                    className={`terminal-worker-status ${workerStatus.toLowerCase()}`}
                  >
                    {workerStatusLabels[workerStatus] ?? workerStatus}
                  </em>
                )}
                <i className={`terminal-status-dot ${connectionState}`} />
                {connectionState === "connected"
                  ? "已连接"
                  : connectionState === "connecting"
                    ? "连接中"
                    : "连接已断开"}
              </span>
            </div>
          </div>
          <div className="terminal-actions">
            <button
              className="terminal-action-button"
              title="上传文件"
              onClick={handleUploadClick}
            >
              <Upload size={16} />
              <span>上传</span>
            </button>
            <button
              className={`terminal-action-button ${showDownloadInput ? "active" : ""}`}
              title="下载文件"
              onClick={() => setShowDownloadInput((v) => !v)}
            >
              <Download size={16} />
              <span>下载</span>
            </button>
          </div>
        </header>
        {showDownloadInput && (
          <div className="terminal-download-bar">
            <input
              type="text"
              value={dlPath}
              onChange={(e) => setDlPath(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleDownloadSubmit()}
              placeholder="输入 Pod 内的文件路径，例如 /workspace/output.log"
              autoFocus
            />
            <button
              className="terminal-download-submit"
              onClick={handleDownloadSubmit}
              disabled={!dlPath.trim()}
            >
              下载
            </button>
          </div>
        )}
        {transferStatus && (
          <div className="terminal-transfer-status">{transferStatus}</div>
        )}
        <div className="terminal-body">
          <div ref={termRef} className="terminal-container" />
          <input
            ref={fileInputRef}
            type="file"
            style={{ display: "none" }}
            onChange={handleFileSelected}
          />
        </div>
      </div>
    </main>
  );
}
