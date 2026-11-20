import { useEffect } from "react";
import { AlertTriangle, Fingerprint, Power, ShieldCheck, X } from "lucide-react";
import type { StationItem } from "../api";
import type { ClusterSummary } from "../../types";
import { formatChinaDateTime } from "../../utils/time.js";
import { CopyButton } from "../components/CopyButton";
import { NodeName, type NodeRef } from "../components/NodeName";
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
import { formatRelativeTime, stationPhase, stationPhaseLabel, stationStatusClass, stationUptime } from "../lib/stationDisplay";

function CopyValue({ value, zh }: { value?: string; zh: boolean }) {
  if (!value) return <strong>—</strong>;
  return (
    <span className="station-copy-value">
      <code className="inline-code" title={value}>{value}</code>
      <CopyButton value={value} zh={zh} iconOnly />
    </span>
  );
}

// One site node's identity, heartbeat, robot power and conditions.
export function StationDrawer({ station, cluster, node, labels, now, zh, power, onClose }: {
  station: StationItem;
  cluster?: ClusterSummary;
  // The RLark Node of the name, or the name alone when it is gone.
  node: NodeRef;
  // The node's labels as the Gateway's site-node view has them.
  labels?: Record<string, string>;
  now: number;
  zh: boolean;
  power: { pending: boolean; message: string; press: (action: RobotPowerAction, label: string) => void };
  onClose: () => void;
}) {
  const phase = stationPhase(station);
  const status = station.status?.node;
  const reporting = station.status?.observation?.nodeUID;
  const powerNow = robotPowerState(labels);
  const selfCheck = robotSelfCheckState(labels);
  const maintenance = robotMaintenanceState(labels);
  const desired = robotPowerDesired(labels);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  return <>
    <div className="addon-drawer-overlay" onClick={onClose} />
    <aside className="addon-drawer station-drawer" role="dialog" aria-modal="true" aria-labelledby="station-drawer-title">
      <div className="addon-drawer-header station-drawer-header">
        <div>
          <span className="eyebrow"><Fingerprint size={13} />{zh ? "身份详情" : "Identity details"}</span>
          <h3 id="station-drawer-title"><NodeName node={node} /></h3>
          <div className="station-drawer-meta">
            <span className={`status ${stationStatusClass(phase)}`}><i />{stationPhaseLabel(phase, zh)}</span>
            <code className="inline-code">{station.metadata.name}</code>
          </div>
        </div>
        <button type="button" className="icon-button" onClick={onClose} aria-label={zh ? "关闭" : "Close"} title={zh ? "关闭" : "Close"}><X size={16} /></button>
      </div>

      <div className="addon-drawer-body station-drawer-body">
        {phase === "Conflict" && <section className="station-conflict-banner" role="note">
          <AlertTriangle size={16} aria-hidden="true" />
          <div>
            <strong>{zh ? "节点 UID 变了" : "Node UID changed"}</strong>
            <p>{zh
              ? `这个名字现在上报的节点 UID 是 ${reporting}，记录属于 UID ${station.spec.nodeUID}，之后没有新的入网许可（重装或换机后没有重新发码入网）。它不进隧道网。在列表里删除这条记录，节点下一次上报会按新机器重新登记。`
              : `The Node reporting under this name has UID ${reporting}; the record is of UID ${station.spec.nodeUID}, and no admission let a machine in under the name since. It is kept out of the tunnel network. Delete the record in the list and the node registers again as a new machine with its next report.`}</p>
          </div>
        </section>}

        <section className="node-detail-section station-drawer-section">
          <span className="node-detail-label"><Fingerprint size={14} />{zh ? "身份" : "Identity"}</span>
          <div className="node-detail-grid station-detail-grid">
            <div><span className="muted">{zh ? "节点名" : "Node name"}</span><CopyValue value={station.metadata.name} zh={zh} /></div>
            <div><span className="muted">{zh ? "集群" : "Cluster"}</span><strong>{cluster?.name ?? (zh ? "集群未确定" : "Cluster unresolved")}</strong></div>
            <div className="station-detail-wide"><span className="muted">Node UID</span><CopyValue value={station.spec.nodeUID} zh={zh} /></div>
            <div className="station-detail-wide"><span className="muted">System UUID</span><CopyValue value={station.spec.systemUUID} zh={zh} /></div>
            <div><span className="muted">{zh ? "启动时间" : "Boot time"}</span><strong>{formatChinaDateTime(status?.bootTime)}</strong></div>
            <div><span className="muted">{zh ? "运行时间" : "Uptime"}</span><strong>{stationUptime(status?.bootTime, now, zh)}</strong></div>
            <div><span className="muted">{zh ? "重启次数" : "Reboots"}</span><strong>{status?.reboots ?? 0}</strong></div>
            <div><span className="muted">{zh ? "最近上报" : "Last seen"}</span><strong title={formatChinaDateTime(status?.lastSeenTime)}>{formatRelativeTime(status?.lastSeenTime, now, zh)}</strong></div>
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
          {status?.conditions?.length ? <ul className="station-conditions">
            {status.conditions.map((condition) => <li key={condition.type}>
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
        </section>
      </div>
    </aside>
  </>;
}
