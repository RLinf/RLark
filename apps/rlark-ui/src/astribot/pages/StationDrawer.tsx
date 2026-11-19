import { useEffect, useState } from "react";
import { AlertTriangle, Fingerprint, Network, Power, ShieldCheck, X } from "lucide-react";
import { stationsApi, type StationItem, type StationObservationItem, type StationRebindOperation } from "../api";
import type { ClusterSummary } from "../../types";
import { formatChinaDateTime } from "../../utils/time.js";
import { CopyButton } from "../components/CopyButton";
import { NodeName } from "../components/NodeName";
import { useAction } from "../components/useAction";
import { useFollow } from "../components/useFollow";
import { nodeLabel } from "../lib/displayName";
import {
  isRobotNode,
  robotMaintenanceLabel,
  robotMaintenanceState,
  robotPowerDesired,
  robotPowerState,
  robotPowerStateClass,
  robotPowerStateLabel,
  robotSelfCheckLabel,
  robotSelfCheckState,
  type RobotPowerAction,
} from "../lib/siteNodeDisplay";
import { formatRelativeTime, shortUID, stationPhase, stationPhaseLabel, stationStatusClass, stationUptime } from "../lib/stationDisplay";

type NodeRef = { metadata: { name: string }; spec?: { displayName?: string } };
// A rebind submitted from the drawer of station (namespace/name), followed while the drawer shows that station.
export type TrackedRebind = { clusterID: string; name: string; station: string };
type RebindRequest = Parameters<typeof stationsApi.createRebind>[0];

function CopyValue({ value, zh }: { value?: string; zh: boolean }) {
  if (!value) return <strong>—</strong>;
  return (
    <span className="station-copy-value">
      <code className="inline-code" title={value}>{value}</code>
      <CopyButton value={value} zh={zh} iconOnly />
    </span>
  );
}

function newRequestID() {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = [...bytes].map((value) => value.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

// One site node's identity, binding, robot power and conditions, and the rebind of one in Conflict or Offline.
export function StationDrawer({ station, cluster, displayName, labels, nodeAt, now, zh, power, tracked, onRebind, onClose }: {
  station: StationItem;
  cluster?: ClusterSummary;
  displayName: string;
  // The node's labels as the Gateway's site-node view has them.
  labels?: Record<string, string>;
  nodeAt: (name: string) => NodeRef;
  now: number;
  zh: boolean;
  power: { pending: boolean; message: string; press: (action: RobotPowerAction, label: string) => void };
  tracked?: TrackedRebind;
  onRebind: (tracked: TrackedRebind) => void;
  onClose: () => void;
}) {
  const key = `${station.metadata.namespace}/${station.metadata.name}`;
  const { binding } = station.spec;
  const phase = stationPhase(station);
  const boundNode = nodeAt(binding.nodeName);
  const powerNow = robotPowerState(labels);
  const selfCheck = robotSelfCheckState(labels);
  const maintenance = robotMaintenanceState(labels);
  const desired = robotPowerDesired(labels);
  const rebindable = phase === "Conflict" || phase === "Offline";

  const [rebindOpen, setRebindOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const { pending, error, setError, run } = useAction();
  // The verified observations of other Nodes, the one picked and why.
  const [candidates, setCandidates] = useState<StationObservationItem[]>([]);
  const [targetName, setTargetName] = useState("");
  const [reason, setReason] = useState("");
  const [requestID, setRequestID] = useState("");
  // A request that failed is retried as it was: the same request ID, target and reason.
  const [submitted, setSubmitted] = useState<RebindRequest>();
  const [operation, setOperation] = useState<StationRebindOperation>();
  const [operationError, setOperationError] = useState("");
  const target = candidates.find((item) => item.metadata.name === targetName);

  const followed = tracked?.station === key ? tracked : undefined;
  useFollow(
    followed && `${followed.clusterID}/${followed.name}`,
    (signal) => stationsApi.getRebind(followed!.name, followed!.clusterID, signal),
    (value) => value.status?.phase === "Applied" || value.status?.phase === "Rejected",
    (value) => {
      setOperation(value);
      setOperationError("");
    },
    setOperationError,
    40,
  );

  const close = () => {
    if (!pending) onClose();
  };
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  const openRebind = async () => {
    setRebindOpen(true);
    setError("");
    setCandidates([]);
    setTargetName("");
    setReason("");
    setSubmitted(undefined);
    setRequestID(newRequestID());
    setLoading(true);
    try {
      const { items } = await stationsApi.observations(cluster!.id);
      const others = items.filter((item) => item.status?.nodeUID && item.status.nodeUID !== binding.nodeUID);
      setCandidates(others);
      setTargetName(others[0]?.metadata.name ?? "");
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const submitRebind = () => target && !pending && run(async () => {
    const request = submitted ?? {
      cluster_id: cluster!.id,
      request_id: requestID,
      station_name: station.metadata.name,
      station_uid: station.metadata.uid!,
      expected_binding_version: binding.version,
      target_node_name: target.metadata.name,
      target_node_uid: target.status.nodeUID,
      target_system_uuid: target.status.systemUUID,
      reason: reason.trim(),
    };
    setSubmitted(request);
    const { operation } = await stationsApi.createRebind(request);
    onRebind({ clusterID: request.cluster_id, name: operation.metadata.name, station: key });
    setRebindOpen(false);
    setSubmitted(undefined);
  });

  const closeRebind = () => {
    setRebindOpen(false);
    setSubmitted(undefined);
    setError("");
  };

  return <>
    <div className="addon-drawer-overlay" onClick={close} />
    <aside className="addon-drawer station-drawer" role="dialog" aria-modal="true" aria-labelledby="station-drawer-title">
      <div className="addon-drawer-header station-drawer-header">
        <div>
          <span className="eyebrow"><Fingerprint size={13} />{zh ? "身份详情" : "Identity details"}</span>
          <h3 id="station-drawer-title">{displayName}</h3>
          <div className="station-drawer-meta">
            <span className={`status ${stationStatusClass(phase)}`}><i />{stationPhaseLabel(phase, zh)}</span>
            <code className="inline-code">{station.metadata.name}</code>
          </div>
        </div>
        <button type="button" className="icon-button" onClick={close} disabled={pending} aria-label={zh ? "关闭" : "Close"} title={zh ? "关闭" : "Close"}><X size={16} /></button>
      </div>

      <div className="addon-drawer-body station-drawer-body">
        {phase === "Conflict" && <section className="station-conflict-banner" role="note">
          <AlertTriangle size={16} aria-hidden="true" />
          <div>
            <strong>{zh ? "绑定冲突" : "Binding conflict"}</strong>
            <p>{zh ? "节点 " : "Node "}<NodeName node={boundNode} />{zh ? " 上报的身份与当前绑定不一致，Station 已暂停接受新的绑定。请核对证据后确认重绑。" : " reported an identity that does not match the current binding. Review the evidence before confirming a rebind."}</p>
          </div>
        </section>}

        <section className="node-detail-section station-drawer-section">
          <span className="node-detail-label"><Fingerprint size={14} />{zh ? "身份" : "Identity"}</span>
          <div className="node-detail-grid station-detail-grid">
            <div><span className="muted">{zh ? "场站节点 ID" : "Site node ID"}</span><CopyValue value={station.metadata.name} zh={zh} /></div>
            <div><span className="muted">{zh ? "对象 UID" : "Object UID"}</span><CopyValue value={station.metadata.uid} zh={zh} /></div>
            <div><span className="muted">{zh ? "集群" : "Cluster"}</span><strong>{cluster?.name ?? (zh ? "集群未确定" : "Cluster unresolved")}</strong></div>
            <div><span className="muted">Namespace</span><strong>{station.metadata.namespace}</strong></div>
            <div><span className="muted">{zh ? "序号" : "Sequence"}</span><strong>{station.spec.sequence}</strong></div>
            <div><span className="muted">{zh ? "启动时间" : "Boot time"}</span><strong>{formatChinaDateTime(station.status?.bootTime)}</strong></div>
            <div><span className="muted">{zh ? "运行时间" : "Uptime"}</span><strong>{stationUptime(station.status?.bootTime, now, zh)}</strong></div>
            <div><span className="muted">{zh ? "最近上报" : "Last seen"}</span><strong title={formatChinaDateTime(station.status?.lastSeenTime)}>{formatRelativeTime(station.status?.lastSeenTime, now, zh)}</strong></div>
          </div>
        </section>

        <section className="node-detail-section station-drawer-section">
          <span className="node-detail-label"><Network size={14} />{zh ? "节点绑定" : "Node binding"}</span>
          <div className="node-detail-grid station-detail-grid">
            <div><span className="muted">{zh ? "节点" : "Node"}</span><strong><NodeName node={boundNode} /></strong></div>
            <div><span className="muted">{zh ? "Binding 版本" : "Binding version"}</span><strong>v{binding.version} · {binding.boundBy}</strong></div>
            <div className="station-detail-wide"><span className="muted">Node UID</span><CopyValue value={binding.nodeUID} zh={zh} /></div>
            <div className="station-detail-wide"><span className="muted">System UUID</span><CopyValue value={binding.systemUUID} zh={zh} /></div>
          </div>
        </section>

        {isRobotNode(labels) && <section className="node-detail-section station-drawer-section stations-power">
          <span className="node-detail-label"><Power size={14} />{zh ? "机器人电源" : "Robot power"}</span>
          <div className="node-detail-grid station-detail-grid">
            <div><span className="muted">{zh ? "期望" : "Desired"}</span><span className={`status ${robotPowerStateClass(desired)}`}><i />{robotPowerStateLabel(desired, zh)}</span></div>
            <div><span className="muted">{zh ? "现在" : "Now"}</span>{powerNow
              ? <span className={`status ${robotPowerStateClass(powerNow)}`}><i />{robotPowerStateLabel(powerNow, zh)}</span>
              : <span className="muted">{zh ? "机器人插件未上报状态" : "The robot's Addon reports no state"}</span>}</div>
            {maintenance && <div><span className="muted">{zh ? "维护" : "Maintenance"}</span><span className={`status ${maintenance === "failed" ? "status-error" : "status-pending"}`}><i />{robotMaintenanceLabel(maintenance, zh)}</span></div>}
            {selfCheck && <div><span className="muted">{zh ? "自检" : "Self-check"}</span><span className={`status ${selfCheck === "ok" ? "status-online" : selfCheck === "failed" ? "status-error" : "status-pending"}`}><i />{robotSelfCheckLabel(selfCheck, zh)}</span></div>}
          </div>
          <small className="muted">{zh ? "期望状态保存在 RLark 的节点上，机器人不在任何任务里时也一样：该上电没电、该跑驱动而驱动掉了，都会自动恢复；急停或硬件报错时不重拉。新入网的机器人默认上电、驱动在跑。要让机器人保持下电，请在这里下电：期望没改而在厂商页面上下电会被恢复。机器人在用（有 Run 或会话占着）时下电会等它空出来。没人用时，规格要的重启、换 Orin 载荷由机器人自己做一圈维护：临时降下来、做完、按这里的期望恢复，期间不接新的会话；做什么、在等什么见节点注解 rlark.io/maintenance。" : "The desired state is kept on RLark's node, in a job or not: motors that should be on and a driver that should run are brought back by themselves, not under an emergency stop or a hardware error. A robot that joins is wanted powered with its driver running. To keep a robot off, power it off here: a power-off at the vendor's page with the desired state unchanged is recovered from. Powering off waits while the robot is in use (a Run or a session holds it). While nobody uses it, a restart its node profile needs and a switch of an Orin payload are done by the robot itself as a maintenance: lowered for it, done, brought back to the desired state set here, no new session admitted meanwhile; what it is and what it waits for is the Node's annotation rlark.io/maintenance."}</small>
          <div className="stations-power-actions">
            {([["power_on", zh ? "上电" : "Power on"], ["driver_on", zh ? "启动驱动" : "Start driver"], ["driver_off", zh ? "关闭驱动" : "Stop driver"], ["power_off", zh ? "下电" : "Power off"]] as [RobotPowerAction, string][]).map(([action, label]) => (
              <button key={action} type="button" className="secondary-button" disabled={power.pending} onClick={() => power.press(action, label)}>{label}</button>
            ))}
          </div>
          {power.message && <small role="status">{power.message}</small>}
        </section>}

        <section className="node-detail-section station-drawer-section">
          <span className="node-detail-label"><ShieldCheck size={14} />{zh ? "健康条件" : "Conditions"}</span>
          {station.status?.conditions?.length ? <ul className="station-conditions">
            {station.status.conditions.map((condition) => <li key={condition.type}>
              <span className={`station-condition-dot is-${condition.status.toLowerCase()}`} aria-hidden="true" />
              <div>
                <strong>{condition.type}</strong>
                <small className="muted">{[condition.reason, condition.message].filter(Boolean).join(" · ") || "—"}</small>
              </div>
              <div className="station-condition-side">
                <span>{condition.status}</span>
                {condition.lastTransitionTime && <small className="muted" title={formatChinaDateTime(condition.lastTransitionTime)}>{formatRelativeTime(condition.lastTransitionTime, now, zh)}</small>}
              </div>
            </li>)}
          </ul> : <small className="muted">{zh ? "暂无条件上报" : "No conditions reported"}</small>}
          <small className="muted">{zh ? "网络状态取自 NetworkVerified 条件；IP 与 WireGuard 详情暂未通过 Station API 提供。" : "Network status comes from the NetworkVerified condition; IP and WireGuard details are not exposed by the Station API yet."}</small>
        </section>

        {rebindable && <section className="node-detail-section station-drawer-section stations-rebind">
          <span className="node-detail-label"><AlertTriangle size={14} />{zh ? "重绑确认" : "Rebind review"}</span>
          {!rebindOpen ? <div className="station-rebind-start">
            <small className="muted">{zh ? "加载该集群当前已验证的 Node 观测，选择目标节点后提交重绑。" : "Load the cluster's current verified Node observations, then pick a target and submit a rebind."}</small>
            <button type="button" className="secondary-button" onClick={() => void openRebind()} disabled={!cluster || !station.metadata.uid}>
              {zh ? "确认重绑…" : "Review rebind…"}
            </button>
          </div> : <div className="stations-rebind-form">
            <table className="station-evidence">
              <thead><tr><th>{zh ? "证据" : "Evidence"}</th><th>{zh ? "原绑定" : "Current binding"}</th><th>{zh ? "目标观测" : "Target observation"}</th></tr></thead>
              <tbody>
                <tr><td>{zh ? "节点" : "Node"}</td><td><NodeName node={boundNode} /></td><td>{target ? <NodeName node={nodeAt(target.metadata.name)} /> : "—"}</td></tr>
                <tr><td>Node UID</td><td><code className="inline-code" title={binding.nodeUID}>{shortUID(binding.nodeUID)}</code></td><td><code className="inline-code" title={target?.status.nodeUID}>{shortUID(target?.status.nodeUID)}</code></td></tr>
                <tr><td>System UUID</td><td><code className="inline-code" title={binding.systemUUID}>{shortUID(binding.systemUUID)}</code></td><td><code className="inline-code" title={target?.status.systemUUID}>{shortUID(target?.status.systemUUID)}</code></td></tr>
                <tr><td>{zh ? "最近观测" : "Observed"}</td><td>—</td><td>{target ? formatRelativeTime(target.status.reportedAt, now, zh) : "—"}</td></tr>
              </tbody>
            </table>
            {target && binding.systemUUID && target.status.systemUUID === binding.systemUUID && <small className="muted">
              {zh ? "systemUUID 相同而 Node UID 不同，可能是同一台机器重装了系统或重新加入集群。" : "Same systemUUID with a different Node UID usually means the same machine was reinstalled or re-joined."}
            </small>}
            <label className="station-field">
              <span>{zh ? "目标节点（已验证观测）" : "Target node (verified observation)"}</span>
              <select value={targetName} onChange={(event) => setTargetName(event.target.value)} disabled={loading || pending || !!submitted}>
                <option value="">{loading ? (zh ? "正在加载…" : "Loading…") : (zh ? "选择节点" : "Select a node")}</option>
                {candidates.map((item) => {
                  const label = nodeLabel(nodeAt(item.metadata.name));
                  return <option key={`${item.metadata.name}/${item.status.nodeUID}`} value={item.metadata.name}>{label === item.metadata.name ? label : `${label} · ${item.metadata.name}`} · {item.status.nodeUID.slice(0, 8)}</option>;
                })}
              </select>
            </label>
            <label className="station-field">
              <span>{zh ? "重绑原因" : "Reason"}</span>
              <textarea value={submitted?.reason ?? reason} onChange={(event) => setReason(event.target.value)} maxLength={1024} rows={3} disabled={pending || !!submitted} placeholder={zh ? "例如：节点重装系统后重新加入集群" : "e.g. Node was reinstalled and re-joined"} />
            </label>
            {error && <div className="cert-error" role="alert">{error}</div>}
            <small className="muted">{zh ? "提交后由 controller 再次核验目标 Node UID；受理不代表重绑完成。" : "The controller revalidates the target Node UID. Acceptance does not mean the rebind is complete."}</small>
          </div>}
        </section>}

        {followed && <section className="node-detail-section station-drawer-section stations-rebind-status" aria-live="polite">
          <span className="node-detail-label">{zh ? "重绑操作" : "Rebind operation"}</span>
          <div className="station-operation-head">
            <span className={`status ${operation?.status?.phase === "Applied" ? "status-online" : operation?.status?.phase === "Rejected" ? "status-failed" : "status-pending"}`}><i />{operation?.status?.phase ?? "Accepted"}</span>
            <code className="inline-code">{followed.name}</code>
          </div>
          {operation?.status?.conditions?.map((condition) => <small className="muted" key={condition.type}>{condition.reason ?? condition.message ?? condition.type}</small>)}
          {operationError && <div className="cert-error" role="alert">{operationError}</div>}
        </section>}
      </div>

      {rebindable && rebindOpen && <div className="addon-drawer-footer">
        <button type="button" className="secondary-button" onClick={closeRebind} disabled={pending}>{zh ? "取消" : "Cancel"}</button>
        <button type="button" className="primary-button" onClick={() => void submitRebind()} disabled={loading || pending || !targetName || !reason.trim()}>{pending ? (zh ? "提交中…" : "Submitting…") : submitted ? (zh ? "使用相同请求重试" : "Retry same request") : (zh ? "提交重绑" : "Submit rebind")}</button>
      </div>}
    </aside>
  </>;
}
