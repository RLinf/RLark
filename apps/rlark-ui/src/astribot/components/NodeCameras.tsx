import { useEffect, useRef, useState } from "react";
import { Camera } from "lucide-react";
import { requestJson } from "../../api.js";
import { cameraCells, camerasPath, cameraStreamPath, publishedCameras, returnedViews, type CamerasReport } from "../lib/cameras";
import "./NodeCameras.css";

// A robot's cameras, 2 x 2 (head, torso / wrist_left, wrist_right): each view's live stream through the Gateway with
// its state as the robot's camera service tells it (polled every 2 s), for a node with a robot model only. While the
// robot does not answer, the state its Node last had (the annotation rlark.io/cameras, ``published``).
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
  // Each view's stream is asked for again when it failed, and when its camera sends again (returnedViews).
  const [rounds, setRounds] = useState<Record<string, number>>({});
  const last = useRef<CamerasReport | undefined>(undefined);

  useEffect(() => {
    if (!robot) return;
    let live = true;
    const read = () => requestJson<CamerasReport>(camerasPath(node, namespace)).then(
      (next) => {
        if (!live) return;
        const back = returnedViews(last.current, next);
        last.current = next;
        if (back.length) setRounds((before) => ({ ...before, ...Object.fromEntries(back.map((view) => [view, (before[view] ?? 0) + 1])) }));
        setReport(next);
        setFailure("");
      },
      (error: Error) => {
        if (!live) return;
        last.current = undefined;
        setReport(undefined);
        setFailure(error.message);
      },
    );
    void read();
    const timer = window.setInterval(read, 2000);
    return () => { live = false; window.clearInterval(timer); };
  }, [robot, node, namespace]);

  if (!robot) return null;
  const again = (view: string) => window.setTimeout(() => setRounds((before) => ({ ...before, [view]: (before[view] ?? 0) + 1 })), 3000);
  return <section className="node-detail-section node-cameras">
    <span className="node-detail-label"><Camera size={14} />{zh ? "相机" : "Cameras"}</span>
    <div className="node-cameras-grid">
      {cameraCells(report ?? publishedCameras(published), zh).map((cell) => <figure key={cell.view} className={`node-camera${cell.present === false ? " is-missing" : ""}`}>
        {cell.streams && report
          ? <img key={rounds[cell.view] ?? 0} src={cameraStreamPath(node, namespace, cell.view, 10)} alt={cell.name} onError={() => again(cell.view)} />
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
