import { useEffect, useRef, useState } from "react";
import { Camera } from "lucide-react";
import { request } from "../../api";
import { cameraFramePath, type CameraCell, type CameraView } from "../lib/cameras";
import "./CameraPreviewDialog.css";

// One camera, enlarged: the card and the details open this instead of expanding the card. The frame is asked for
// while the dialog is open, at a speed that is useful for a look rather than a live wall; Escape or the close button
// returns focus to the camera button that opened it.
export function CameraPreviewDialog({ node, namespace, view, cell, zh, onDismiss }: {
  node: string;
  namespace: string;
  view: CameraView;
  cell?: CameraCell;
  zh: boolean;
  onDismiss: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [frame, setFrame] = useState("");
  const [failure, setFailure] = useState("");
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
    if (cell?.frames === false) return;
    let stopped = false;
    let timer = 0;
    let inFlight: AbortController | null = null;
    let url = "";
    const ask = async () => {
      if (inFlight) return; // one request at a time: a slow frame never stacks a second one
      inFlight = new AbortController();
      const signal = AbortSignal.any([inFlight.signal, AbortSignal.timeout(3000)]);
      try {
        const response = await request(cameraFramePath(node, namespace, view), { signal });
        const blob = await response.blob();
        if (stopped) return;
        if (url) URL.revokeObjectURL(url);
        url = URL.createObjectURL(blob);
        setFrame(url);
        setFailure("");
      } catch (reason) {
        if (!stopped && !signal.aborted) setFailure((reason as Error).message);
      } finally {
        inFlight = null;
      }
    };
    void ask();
    timer = window.setInterval(() => { if (!document.hidden) void ask(); }, 1500);
    return () => {
      stopped = true;
      window.clearInterval(timer);
      inFlight?.abort(); // closing also cancels the request in flight; late responses never build a URL
      if (url) URL.revokeObjectURL(url);
    };
  }, [node, namespace, view, cell?.frames]);

  const title = `${cell?.name ?? view} · ${zh ? "相机" : "Camera"}`;
  return <dialog ref={dialog} className="station-camera-dialog" aria-labelledby="station-camera-dialog-title">
    <header className="station-camera-dialog-head">
      <div><span className="station-camera-dialog-eyebrow">{node}</span><h3 id="station-camera-dialog-title">{title}</h3></div>
      <button type="button" className="secondary-button station-camera-dialog-close" onClick={() => dialog.current?.close()} aria-label={zh ? "关闭" : "Close"} autoFocus>×</button>
    </header>
    <div className="station-camera-dialog-frame">
      {frame ? <img src={frame} alt={cell?.name ?? view} /> : <div className="station-camera-dialog-blank"><Camera size={28} aria-hidden="true" /><span>{cell?.frames === false ? (cell.present === false ? (zh ? "这台机器人没有安装此相机" : "This camera is not fitted") : cell.badge) : failure ? (zh ? "读不到画面：" : "No frame: ") + failure : (zh ? "等待画面" : "Waiting for a frame")}</span></div>}
    </div>
    <footer className="station-camera-dialog-foot">
      {cell && cell.present !== false && <span className={`status status-${cell.tone}`}><i />{cell.badge}</span>}
      {cell?.age && <small>{cell.age}</small>}
      {cell?.note && <small>{cell.note}</small>}
      {cell?.advice && <small>{cell.advice}</small>}
    </footer>
    <div className="station-toast-host" aria-live="polite" />
  </dialog>;
}
