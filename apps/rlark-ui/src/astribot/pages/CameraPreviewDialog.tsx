import { useEffect, useRef, useState } from "react";
import { Camera } from "lucide-react";
import { request } from "../../api";
import { CAMERA_FPS_OPTIONS, cameraStreamPath, isCameraFps, type CameraCell, type CameraFps, type CameraView } from "../lib/cameras";
import { readCameraFrames, sameCameraFrame } from "../lib/cameraStream";
import { cameraDisplayLatency } from "../lib/cameraLatency";
import { cameraMediaScheduler } from "../lib/cameraMediaScheduler";
import { useCameraClock } from "../components/useCameraClock";
import "./CameraPreviewDialog.css";

const FPS_OPTIONS = CAMERA_FPS_OPTIONS;

// Open a single stream for the selected camera at the requested preview rate. A card that zooms one of its
// cameras passes its current rate so the dialog continues at it instead of starting a second rate.
export function CameraPreviewDialog({ node, namespace, view, cell, zh, initialFps, onDismiss }: {
  node: string;
  namespace: string;
  view: CameraView;
  cell?: CameraCell;
  zh: boolean;
  initialFps?: CameraFps;
  onDismiss: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [frame, setFrame] = useState<{ url: string; capturedAtMs?: number }>();
  const [fps, setFps] = useState<CameraFps>(initialFps ?? 10);
  const [actualFps, setActualFps] = useState<number | null>(null);
  const [latency, setLatency] = useState<ReturnType<typeof cameraDisplayLatency>>();
  const paint = useRef(0);
  const [liveGranted, setLiveGranted] = useState(false);
  const cameraClock = useCameraClock(node, namespace, liveGranted && cell?.present !== false);
  useEffect(() => {
    if (cell?.present === false) return;
    return cameraMediaScheduler.acquire(`dialog:${namespace}/${node}/${view}`, "dialog", setLiveGranted);
  }, [node, namespace, view, cell?.present]);
  const [reconnecting, setReconnecting] = useState(false);
  const dismiss = useRef(onDismiss);
  dismiss.current = onDismiss;

  useEffect(() => {
    const element = dialog.current;
    if (!element) return;
    if (!element.open) element.showModal();
    const closed = () => dismiss.current();
    element.addEventListener("close", closed);
    // The element goes away with the component; closing it here would fire a close event during StrictMode's
    // effect replay and dismiss the dialog the replay just opened.
    return () => element.removeEventListener("close", closed);
  }, []);

  useEffect(() => {
    setFrame(undefined);
    setActualFps(null);
    setLatency(undefined);
    setReconnecting(false);
    if (!liveGranted || cell?.present === false) return;
    let connection: AbortController | undefined;
    let retry = 0;
    let sample = 0;
    let url = "";
    let previousUrl = "";

    const connect = async () => {
      if (document.hidden || connection) return;
      const controller = new AbortController();
      connection = controller;
      let watchdog = 0;
      let lastFrame: Uint8Array | undefined;
      let distinct = 0;
      let measuredAt = performance.now();
      const resetWatchdog = () => {
        window.clearTimeout(watchdog);
        watchdog = window.setTimeout(() => controller.abort(), 8000);
      };
      resetWatchdog();
      try {
        const response = await request(cameraStreamPath(node, namespace, view, fps), { signal: controller.signal });
        if (connection !== controller) return;
        measuredAt = performance.now();
        sample = window.setInterval(() => {
          const now = performance.now();
          setActualFps(distinct * 1000 / (now - measuredAt));
          setLatency((value) => value && now - value.displayAt < 2000 ? value : undefined);
          distinct = 0;
          measuredAt = now;
        }, 1000);
        for await (const { jpeg, capturedAtMs } of readCameraFrames(response)) {
          if (connection !== controller || controller.signal.aborted) break;
          resetWatchdog();
          setReconnecting(false);
          if (sameCameraFrame(lastFrame, jpeg)) continue;
          lastFrame = jpeg;
          distinct++;
          if (previousUrl) URL.revokeObjectURL(previousUrl);
          previousUrl = url;
          url = URL.createObjectURL(new Blob([jpeg], { type: "image/jpeg" }));
          setFrame({ url, capturedAtMs });
        }
      } catch {
        // A disconnected stream is retried below while the preview is visible.
      } finally {
        window.clearTimeout(watchdog);
        controller.abort();
        if (connection === controller) {
          window.clearInterval(sample);
          connection = undefined;
          setActualFps(null);
          setLatency(undefined);
          setReconnecting(true);
          if (!document.hidden) retry = window.setTimeout(() => void connect(), 1500);
        }
      }
    };

    const disconnect = () => {
      window.clearTimeout(retry);
      window.clearInterval(sample);
      window.cancelAnimationFrame(paint.current);
      const active = connection;
      connection = undefined;
      active?.abort();
    };
    const visibility = () => {
      if (document.hidden) {
        disconnect();
        setActualFps(null);
        setLatency(undefined);
      } else {
        void connect();
      }
    };
    document.addEventListener("visibilitychange", visibility);
    // Give the background stream time to close before opening the dialog stream.
    retry = window.setTimeout(() => void connect(), 100);
    return () => {
      document.removeEventListener("visibilitychange", visibility);
      disconnect();
      if (url) URL.revokeObjectURL(url);
      if (previousUrl) URL.revokeObjectURL(previousUrl);
    };
  }, [node, namespace, view, cell?.present, fps, liveGranted]);

  const measurePaint = (image: HTMLImageElement, shown: NonNullable<typeof frame>) => {
    window.cancelAnimationFrame(paint.current);
    paint.current = window.requestAnimationFrame(() => {
      if (document.hidden || !image.isConnected || image.currentSrc !== shown.url) return;
      setLatency(cameraDisplayLatency(shown.capturedAtMs, performance.now(), cameraClock.current));
    });
  };

  const title = `${cell?.name ?? view} · ${zh ? "相机" : "Camera"}`;
  return <dialog ref={dialog} className="station-camera-dialog" aria-labelledby="station-camera-dialog-title">
    <header className="station-camera-dialog-head">
      <div><span className="station-camera-dialog-eyebrow">{node}</span><h3 id="station-camera-dialog-title">{title}</h3></div>
      <button type="button" className="secondary-button station-camera-dialog-close" onClick={() => dialog.current?.close()} aria-label={zh ? "关闭" : "Close"} autoFocus>×</button>
    </header>
    <div className="station-camera-dialog-frame">
      {frame ? <img src={frame.url} alt={cell?.name ?? view} onLoad={(event) => measurePaint(event.currentTarget, frame)} /> : <div className="station-camera-dialog-blank"><Camera size={28} aria-hidden="true" /><span>{cell?.present === false ? (zh ? "这台机器人没有安装此相机" : "This camera is not fitted") : (zh ? "等待画面" : "Waiting for a frame")}</span></div>}
    </div>
    <footer className="station-camera-dialog-foot">
      {cell && cell.present !== false && <span className={`status status-${cell.tone}`}><i />{cell.badge}</span>}
      {cell?.present !== false && <label className="station-preview-fps">
        <span>{zh ? "目标帧率" : "Target FPS"}</span>
        <select value={fps} onChange={(event) => { const rate = Number(event.target.value); if (isCameraFps(rate)) setFps(rate); }}>
          {FPS_OPTIONS.map((rate) => <option key={rate} value={rate}>{rate} FPS</option>)}
        </select>
      </label>}
      {cell?.present !== false && <small className="station-preview-actual-fps">{zh ? "实测" : "Measured"} {actualFps === null ? "—" : `${actualFps.toFixed(1)} FPS`}</small>}
      {cell?.present !== false && <small className="station-preview-latency" title={zh ? "从 ROS 采集时间戳到浏览器绘制机会的估算。± 为浏览器对时误差。" : "Estimate from ROS capture time to the browser paint opportunity. ± is the browser clock uncertainty."}>
        {zh ? "端到端延迟" : "End-to-end latency"} {latency ? `≈ ${Math.round(latency.elapsedMs)} ms (±${Math.ceil(latency.uncertaintyMs)} ms)` : "—"}
      </small>}
      {reconnecting && <small role="status">{zh ? "视频流中断，正在重连" : "Stream interrupted, reconnecting"}</small>}
      {cell?.note && <small>{cell.note}</small>}
      {cell?.advice && <small>{cell.advice}</small>}
    </footer>
    <div className="station-toast-host" aria-live="polite" />
  </dialog>;
}
