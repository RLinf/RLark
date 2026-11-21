import { useEffect, useRef, useState } from "react";
import { Camera } from "lucide-react";
import { request, requestJson } from "../../api.js";
import { cameraCells, cameraFramePath, camerasPath, publishedCameras, type CamerasReport, type CameraView } from "../lib/cameras";
import "./NodeCameras.css";

// A robot's cameras, 2 x 2 (head, torso / wrist_left, wrist_right), for a node with a robot model only: each view's
// newest JPEG through the Gateway and its state as the robot's camera service tells it. The console holds one
// connection for the report and one for the frames, never a stream: over HTTP/1.1 the browser has six to the host for
// every tab of the console, and an <img> stream is not reliably let go when it leaves the page.
export function NodeCameras({ node, namespace, labels, published, zh }: {
  node: string;
  namespace: string;
  labels?: Record<string, string>;
  published?: string;
  zh: boolean;
}) {
  const robot = Boolean(labels?.["rlark.io/robot-model"]);
  const [report, setReport] = useState<CamerasReport>();
  const [failure, setFailure] = useState("");
  const [frames, setFrames] = useState<Partial<Record<CameraView, string>>>({});
  // The views that have sent a frame, as the last report says; the frame loop asks for these in turn.
  const views = useRef<CameraView[]>([]);

  useEffect(() => {
    if (!robot) return;
    const stop = new AbortController();
    const within = (ms: number) => AbortSignal.any([stop.signal, AbortSignal.timeout(ms)]);
    const pause = (ms: number) => new Promise((resolve) => window.setTimeout(resolve, ms));
    const urls: Partial<Record<CameraView, string>> = {};

    // The report: asked for, answered or timed out, 2 s, again.
    void (async () => {
      while (!stop.signal.aborted) {
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

    // The frames: one request at a time, each view in turn, about 5 a second each.
    void (async () => {
      while (!stop.signal.aborted) {
        const started = performance.now();
        for (const view of views.current) {
          const frame = await request(cameraFramePath(node, namespace, view), { signal: within(3000) }).then((response) => response.blob(), () => undefined);
          if (stop.signal.aborted) return;
          if (!frame) continue; // a view that sends no frame keeps its last one; the report says how old it is
          if (urls[view]) URL.revokeObjectURL(urls[view]);
          urls[view] = URL.createObjectURL(frame);
          setFrames({ ...urls });
        }
        await pause(Math.max(0, 200 - (performance.now() - started)));
      }
    })();

    return () => {
      stop.abort();
      Object.values(urls).forEach((url) => URL.revokeObjectURL(url));
      views.current = [];
      setReport(undefined);
      setFailure("");
      setFrames({});
    };
  }, [robot, node, namespace]);

  if (!robot) return null;
  const cells = failure ? cameraCells(publishedCameras(published), zh, false) : cameraCells(report, zh);
  return <section className="node-detail-section node-cameras">
    <span className="node-detail-label"><Camera size={14} />{zh ? "相机" : "Cameras"}</span>
    <div className="node-cameras-grid">
      {cells.map((cell) => <figure key={cell.view} className={`node-camera${cell.present === false ? " is-missing" : ""}`}>
        {cell.frames && frames[cell.view]
          ? <img src={frames[cell.view]} alt={cell.name} />
          : <div className="node-camera-blank">{cell.badge}</div>}
        <figcaption>
          <strong>{cell.name}</strong>
          {cell.present && <span className={`status status-${cell.tone}`}><i />{cell.badge}</span>}
          {cell.present && report && <small className="muted">{cell.age}</small>}
        </figcaption>
      </figure>)}
    </div>
    {failure && <small role="status" className="muted">{zh ? "读不到相机状态：" : "The cameras' state is not read: "}{failure}</small>}
  </section>;
}
