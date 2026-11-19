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
import { terminalApi } from "../backend";
import type { Lang } from "../i18n";

const workerStatusLabels: Record<string, { zh: string; en: string }> = {
  Running: { zh: "运行中", en: "Running" },
  Pending: { zh: "等待中", en: "Pending" },
  Succeeded: { zh: "已完成", en: "Succeeded" },
  Failed: { zh: "失败", en: "Failed" },
  Stopped: { zh: "已停止", en: "Stopped" },
  Online: { zh: "在线", en: "Online" },
  Offline: { zh: "离线", en: "Offline" },
};

function workerStatusLabel(status: string, zh: boolean): string {
  const label = workerStatusLabels[status];
  if (!label) return status;
  return zh ? label.zh : label.en;
}

export function TerminalPage({
  workerCRName,
  workerName,
  jobName,
  workerStatus,
  lang,
}: {
  workerCRName: string;
  workerName: string;
  jobName: string;
  workerStatus: string;
  lang: Lang;
}) {
  const zh = lang === "zh";
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

    term.writeln(
      zh ? `正在连接 ${workerName} ...` : `Connecting to ${workerName} ...`,
    );

    const ws = terminalApi.createSocket(workerCRName);
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
      term.writeln(
        zh
          ? "\r\n已连接，正在启动 Shell ...\r\n"
          : "\r\nConnected. Starting shell ...\r\n",
      );
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
              zh
                ? `正在下载 ${downloadName}（${msg.size || 0} 字节）...`
                : `Downloading ${downloadName} (${msg.size || 0} bytes)...`,
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
              term.writeln(
                zh
                  ? `\r\n\x1b[32m已下载 ${downloadName}。\x1b[0m`
                  : `\r\n\x1b[32mDownloaded ${downloadName}.\x1b[0m`,
              );
            }
            return;
          }
          if (msg.type === "file-transfer-done") {
            setTransferStatus("");
            if (msg.success) {
              term.writeln(
                zh
                  ? "\r\n\x1b[32m文件传输完成。\x1b[0m"
                  : "\r\n\x1b[32mFile transfer complete.\x1b[0m",
              );
            } else {
              const detail = msg.error || (zh ? "未知错误" : "unknown");
              term.writeln(
                zh
                  ? `\r\n\x1b[31m文件传输失败：${detail}\x1b[0m`
                  : `\r\n\x1b[31mFile transfer failed: ${detail}\x1b[0m`,
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
          zh
            ? `\r\n\x1b[33m会话已结束（退出码 ${exitCode}）。\x1b[0m`
            : `\r\n\x1b[33mSession ended (exit code ${exitCode}).\x1b[0m`,
        );
      } else if (event.code === 1000 || legacyProxyCloseReceived) {
        term.writeln(
          zh
            ? "\r\n\x1b[33m会话已结束。\x1b[0m"
            : "\r\n\x1b[33mSession ended.\x1b[0m",
        );
      } else {
        const reason = event.reason ? `: ${event.reason}` : "";
        term.writeln(
          zh
            ? `\r\n\x1b[31m连接意外中断（${event.code}）${reason}。\x1b[0m`
            : `\r\n\x1b[31mConnection closed unexpectedly (${event.code})${reason}.\x1b[0m`,
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
  }, [workerCRName, workerName, zh]);

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
      zh
        ? `正在上传 ${file.name}（${file.size} 字节）到 ${destPath}...`
        : `Uploading ${file.name} (${file.size} bytes) to ${destPath}...`,
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
    setTransferStatus(zh ? `正在请求 ${dlPath}...` : `Requesting ${dlPath}...`);
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
                    {workerStatusLabel(workerStatus, zh)}
                  </em>
                )}
                <i className={`terminal-status-dot ${connectionState}`} />
                {connectionState === "connected"
                  ? zh
                    ? "已连接"
                    : "Connected"
                  : connectionState === "connecting"
                    ? zh
                      ? "连接中"
                      : "Connecting"
                    : zh
                      ? "连接已断开"
                      : "Disconnected"}
              </span>
            </div>
          </div>
          <div className="terminal-actions">
            <button
              className="terminal-action-button"
              title={zh ? "上传文件" : "Upload file"}
              onClick={handleUploadClick}
            >
              <Upload size={16} />
              <span>{zh ? "上传" : "Upload"}</span>
            </button>
            <button
              className={`terminal-action-button ${showDownloadInput ? "active" : ""}`}
              title={zh ? "下载文件" : "Download file"}
              onClick={() => setShowDownloadInput((v) => !v)}
            >
              <Download size={16} />
              <span>{zh ? "下载" : "Download"}</span>
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
              placeholder={
                zh
                  ? "输入 Pod 内的文件路径，例如 /workspace/output.log"
                  : "Enter a file path inside the Pod, e.g. /workspace/output.log"
              }
              autoFocus
            />
            <button
              className="terminal-download-submit"
              onClick={handleDownloadSubmit}
              disabled={!dlPath.trim()}
            >
              {zh ? "下载" : "Download"}
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
