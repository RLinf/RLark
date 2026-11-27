import { useEffect, useState } from "react";
import { Box } from "lucide-react";
import { imageReferenceTag } from "../../utils/imageReference";
import { stationsApi, type StationRunningImage } from "../api";
import { CopyValue } from "./StationFields";

type Snapshot = { state: "loading" | "unavailable" } | { state: "ready"; items: StationRunningImage[] };
type NodeIdentity = { name: string; clusterID: string; uid: string };

export function StationRunningImages({ name, clusterID, uid, zh }: NodeIdentity & { zh: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <details className="station-running-images" onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary><Box size={14} aria-hidden="true" />{zh ? "运行中的容器镜像" : "Running container images"}</summary>
      {open && (clusterID
        ? <RunningImageSnapshot key={JSON.stringify([clusterID, name, uid])} name={name} clusterID={clusterID} uid={uid} zh={zh} />
        : <p className="station-detail-note">{zh ? "无法确认节点所属集群。" : "The node’s cluster could not be resolved."}</p>)}
    </details>
  );
}

function RunningImageSnapshot({ name, clusterID, uid, zh }: NodeIdentity & { zh: boolean }) {
  const [snapshot, setSnapshot] = useState<Snapshot>({ state: "loading" });
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    void stationsApi.runningImages(name, clusterID, uid, controller.signal).then(
      ({ items }) => { if (!controller.signal.aborted) setSnapshot({ state: "ready", items }); },
      () => { if (!controller.signal.aborted) setSnapshot({ state: "unavailable" }); },
    );
    return () => controller.abort();
  }, [name, clusterID, uid, revision]);

  const refresh = () => {
    setSnapshot({ state: "loading" });
    setRevision((value) => value + 1);
  };
  return (
    <>
      <div className="station-images-toolbar">
        <p className="station-detail-note">{zh ? "来自 Kubernetes 容器状态。" : "Reported by Kubernetes container status."}</p>
        <button type="button" className="secondary-button" onClick={refresh} disabled={snapshot.state === "loading"}>
          {snapshot.state === "unavailable" ? (zh ? "重试" : "Retry") : (zh ? "刷新" : "Refresh")}
        </button>
      </div>
      {snapshot.state === "loading" && <p className="station-detail-note" role="status">{zh ? "正在读取运行中的容器…" : "Loading running containers…"}</p>}
      {snapshot.state === "unavailable" && <p className="station-detail-note" role="alert">{zh ? "暂时无法读取镜像，请确认节点在线后重试。" : "Images are unavailable. Verify the node is online and try again."}</p>}
      {snapshot.state === "ready" && (snapshot.items.length === 0
        ? <p className="station-detail-note">{zh ? "当前没有上报运行中的容器。" : "No running containers were reported."}</p>
        : <div className="station-images-table-wrap"><table className="station-images-table">
          <thead><tr>
            <th scope="col">Pod / {zh ? "容器" : "Container"}</th>
            <th scope="col">{zh ? "镜像" : "Image"}</th>
            <th scope="col">{zh ? "标签" : "Tag"}</th>
            <th scope="col">{zh ? "运行时镜像 ID" : "Runtime image ID"}</th>
          </tr></thead>
          <tbody>{snapshot.items.map((item) => <tr key={`${item.namespace}/${item.pod}/${item.container}`}>
            <td><strong>{item.container}</strong><small>{item.namespace}/{item.pod}</small></td>
            <td><CopyValue value={item.image} zh={zh} /></td>
            <td>{imageReferenceTag(item.image) ?? "—"}</td>
            <td><CopyValue value={item.imageID} zh={zh} /></td>
          </tr>)}</tbody>
        </table></div>)}
    </>
  );
}
