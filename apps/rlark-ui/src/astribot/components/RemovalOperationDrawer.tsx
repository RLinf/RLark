import { CheckCircle2, Circle, X } from "lucide-react";
import type { RemovalEntry } from "./removalProgressContext";
import { removalFailureMessage, removalLooksStuck, removalNoProgressMs, removalOutcome, removalStageLabel, removalStages, removalStagesDone, removalIsActive } from "../lib/nodeRemoval";

function durationLabel(milliseconds: number, zh: boolean): string {
  const totalMinutes = Math.floor(milliseconds / 60_000);
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (zh) return hours ? `${hours} 小时 ${minutes} 分钟` : `${totalMinutes} 分钟`;
  return hours ? `${hours}h ${minutes}m` : `${totalMinutes}m`;
}

// Shows the server-owned details of one node removal operation.
export function RemovalOperationDrawer({ entry, onClose, onRetry, onDismiss, zh }: { entry: RemovalEntry; onClose: () => void; onRetry: () => void; onDismiss: () => void; zh: boolean }) {
  const operation = entry.operation;
  const phase = operation?.status?.phase ?? "Pending";
  const active = !operation || removalIsActive(operation);
  const failed = operation ? removalOutcome(operation) === "failed" : false;
  const done = removalStagesDone(operation?.status?.stage);
  const stuck = !!operation && removalLooksStuck(operation);
  const noProgressMs = operation ? removalNoProgressMs(operation) : undefined;
  const condition = operation?.status?.conditions ?? [];
  return <>
    <div className="removal-drawer-scrim" onClick={onClose} />
    <aside className="removal-operation-drawer" role="dialog" aria-modal="true" aria-labelledby="removal-operation-title">
      <header><div><span className="eyebrow">{zh ? "删除节点" : "Remove node"}</span><h2 id="removal-operation-title">{zh ? `删除 ${entry.nodeName}` : `Remove ${entry.nodeName}`}</h2><div className="removal-drawer-meta"><span className={`removal-pill ${failed ? "failed" : phase === "Succeeded" ? "succeeded" : "active"}`}>{phase}</span>{operation?.metadata.name && <code>{operation.metadata.name}</code>}<span>{entry.clusterID}</span></div></div><button className="icon-button" type="button" aria-label={zh ? "关闭" : "Close"} onClick={onClose}><X size={17} /></button></header>
      <div className="removal-drawer-body"><div className="removal-drawer-summary"><small>{zh ? "当前阶段" : "Current stage"}</small><strong>{removalStageLabel(operation?.status?.stage ?? "Preflight", zh)}</strong></div>
        {stuck && operation && noProgressMs !== undefined && <div className="removal-stuck-callout" role="status"><strong>{zh ? `长时间无进展 · 已 ${durationLabel(noProgressMs, true)}` : `No progress for ${durationLabel(noProgressMs, false)}`}</strong><span>{zh ? `最后完成阶段：${removalStageLabel(operation.status?.stage ?? "Preflight", true)}。成员集群或机器可能不可达；操作仍在服务端等待。` : `Last completed stage: ${removalStageLabel(operation.status?.stage ?? "Preflight", false)}. The member cluster or machine may be unreachable. The operation is still waiting on the server.`}</span></div>}
        <ol className="removal-drawer-stepper">{removalStages.map((stage, index) => <li className={index < done ? "is-done" : index === done && active ? "is-current" : ""} key={stage}>{index < done ? <CheckCircle2 size={16} /> : <Circle size={16} />}<span>{removalStageLabel(stage, zh)}</span></li>)}</ol>
        {entry.error && <div className="removal-condition error" role="alert">{entry.error}</div>}
        {failed && operation && <div className="removal-condition error" role="alert"><strong>{operation.status?.conditions?.find((item) => item.type === "Progressing")?.reason ?? phase}</strong><span>{removalFailureMessage(operation)}</span></div>}
        {condition.map((item) => <div className="removal-condition" key={`${item.type}-${item.lastTransitionTime ?? ""}`}><strong>{item.type} · {item.status}{item.reason ? ` · ${item.reason}` : ""}</strong>{item.message && <span>{item.message}</span>}</div>)}
        {operation?.metadata.annotations?.["station.rlark.io/power-off"] && <div className="removal-power-note"><small>{zh ? "下电记录" : "Power-off result"}</small><span>{operation.metadata.annotations["station.rlark.io/power-off"]}</span></div>}
      </div>
      <footer><button className="secondary-button" type="button" onClick={onClose}>{zh ? "关闭" : "Close"}</button>{failed && <button className="secondary-button" type="button" onClick={onRetry}>{zh ? "重试" : "Retry"}</button>}{!active && <button className="secondary-button" type="button" onClick={onDismiss}>{zh ? "移除记录" : "Dismiss"}</button>}</footer>
    </aside>
  </>;
}
