import { useEffect, useRef, useState } from "react";
import { Camera } from "lucide-react";
import { request, requestJson } from "../../api.js";
import { cameraAge, cameraCells, cameraFramePath, camerasPath, publishedCameras, type CamerasReport, type CameraView } from "../lib/cameras";
import "./NodeCameras.css";

// A robot's cameras, 2 x 2 (head, torso / wrist_left, wrist_right), for a node with a robot model only: each view's
// newest JPEG through the Gateway and its state as the robot's camera service tells it. The console holds one
// connection for the report and one for the frames, never a stream: over HTTP/1.1 the browser has six to the host for
// every tab of the console, and an <img> stream is not reliably let go when it leaves the page. A hidden tab holds none.
export function NodeCameras({ node, namespace, labels, published, zh, heading, onOpen }: {
  node: string;
  namespace: string;
  labels?: Record<string, string>;
  published?: string;
  zh: boolean;
  heading?: string;
  onOpen?: (view: CameraView, trigger: HTMLElement) => void;
}) {
  const robot = Boolean(labels?.["rlark.io/robot-model"]);
  const [report, setReport] = useState<CamerasReport>();
  const [failure, setFailure] = useState("");
  // Each view's last frame: its object URL, its age when it came (X-Age-Seconds) and when that was.
  const [frames, setFrames] = useState<Partial<Record<CameraView, { url: string; age: number; at: number }>>>({});
  // Why a view's last frame request failed, until one succeeds.
  const [missed, setMissed] = useState<Partial<Record<CameraView, string>>>({});
  // The views that have sent a frame, as the last report says; the frame loop asks for these in turn.
  const views = useRef<CameraView[]>([]);

  useEffect(() => {
    if (!robot) return;
    const stop = new AbortController();
    const within = (ms: number) => AbortSignal.any([stop.signal, AbortSignal.timeout(ms)]);
    const pause = (ms: number) => new Promise((resolve) => window.setTimeout(resolve, ms));
    const shown = async () => {
      while (document.hidden && !stop.signal.aborted) {
        await new Promise<void>((resolve) => {
          document.addEventListener("visibilitychange", () => resolve(), { once: true, signal: stop.signal });
          stop.signal.addEventListener("abort", () => resolve(), { once: true });
        });
      }
    };
    const shot: Partial<Record<CameraView, { url: string; age: number; at: number }>> = {};

    // The report: asked for, answered or timed out, 2 s, again.
    void (async () => {
      while (!stop.signal.aborted) {
        await shown();
        if (stop.signal.aborted) return;
        await requestJson<CamerasReport>(camerasPath(node, namespace), { signal: within(5000) }).then(
          (next) => {
            if (stop.signal.aborted) return;
            views.current = cameraCells(next, true).filter((cell) => cell.frames).map((cell) => cell.view);
            setReport(next);
            setFailure("");
          },
          (error: Error) => {
            if (stop.signal.aborted) return;
            views.current = [];
            setReport(undefined);
            setFailure(error.message);
          },
        );
        await pause(2000);
      }
    })();

    // The frames: one request at a time, each view in turn, at most 5 a second each; with four views the Gateway's
    // round trip (authorization, Node, pod list, proxy) sets the rate.
    void (async () => {
      while (!stop.signal.aborted) {
        const started = performance.now();
        for (const view of views.current) {
          await shown();
          if (stop.signal.aborted) return;
          const frame = await request(cameraFramePath(node, namespace, view), { signal: within(3000) })
            .then(async (response) => ({ blob: await response.blob(), age: Number(response.headers.get("X-Age-Seconds")) }))
            .catch((error: Error) => error);
          if (stop.signal.aborted) return;
          if (frame instanceof Error) {
            setMissed((missed) => ({ ...missed, [view]: frame.message }));
            continue; // a view whose frame is not got keeps its last one, which ages
          }
          if (shot[view]) URL.revokeObjectURL(shot[view].url);
          shot[view] = { url: URL.createObjectURL(frame.blob), age: frame.age, at: performance.now() };
          setFrames({ ...shot });
          setMissed((missed) => ({ ...missed, [view]: undefined }));
        }
        await pause(Math.max(0, 200 - (performance.now() - started)));
      }
    })();

    return () => {
      stop.abort();
      Object.values(shot).forEach((frame) => URL.revokeObjectURL(frame.url));
      views.current = [];
      setReport(undefined);
      setFailure("");
      setFrames({});
      setMissed({});
    };
  }, [robot, node, namespace]);

  if (!robot) return null;
  const cells = failure ? cameraCells(publishedCameras(published), zh, false) : cameraCells(report, zh);
  const now = performance.now();
  return <section className="node-detail-section node-cameras">
    <span className="node-detail-label"><Camera size={14} />{heading ?? (zh ? "相机" : "Cameras")}</span>
    <div className="node-cameras-grid">
      {cells.map((cell) => {
        const frame = cell.frames ? frames[cell.view] : undefined;
        // A camera the robot calls fine whose frame the console has not got says so, not the camera's state.
        const blank = !cell.frames ? cell.badge
          : missed[cell.view] ? (zh ? "读不到画面：" : "No frame: ") + missed[cell.view]
          : zh ? "等待画面" : "Waiting for a frame";
        const media = frame ? <img src={frame.url} alt={cell.name} /> : <div className="node-camera-blank">{blank}</div>;
        return <figure key={cell.view} className={`node-camera${cell.present === false ? " is-missing" : ""}`}>
          {onOpen
            ? <button type="button" className="node-camera-open" onClick={(event) => onOpen(cell.view, event.currentTarget)} aria-label={`${cell.name} · ${cell.badge}`} aria-haspopup="dialog">{media}</button>
            : media}
          <figcaption>
            <strong>{cell.name}</strong>
            {cell.present && <span className={`status status-${cell.tone}`}><i />{cell.badge}</span>}
            {frame
              ? <small className="muted">{cameraAge(frame.age + (now - frame.at) / 1000, zh)}</small>
              : cell.present && report && <small className="muted">{cell.age}</small>}
          </figcaption>
          {cell.note && <small className="node-camera-note">{cell.note}</small>}
          {cell.advice && <small className="node-camera-note muted">{cell.advice}</small>}
        </figure>;
      })}
    </div>
    {failure && <small role="status" className="muted">{zh ? "读不到相机状态：" : "The cameras' state is not read: "}{failure}</small>}
  </section>;
}
