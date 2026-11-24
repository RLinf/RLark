import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { AlertTriangle, Fingerprint, KeyRound, Network, Power, X } from "lucide-react";
import type { StationItem } from "../api";
import { NodeLinks } from "../components/NodeLinks";
import { NodeCameras } from "../components/NodeCameras";
import { CopyButton } from "../components/CopyButton";
import { NodeName, type NodeRef } from "../components/NodeName";
import { RobotPoseButton } from "../components/RobotControls";
import type { ClusterSummary } from "../../types";
import { formatChinaDateTime } from "../../utils/time.js";
import { admissionStateClass, isRobotNode, robotMaintenanceLabel, robotMaintenanceState, robotPowerDesired, robotPowerState, robotPowerStateClass, robotPowerStateLabel, robotSelfCheckLabel, robotSelfCheckState, type NodeAdmission, type RobotPowerAction, type SiteNode } from "../lib/siteNodeDisplay";
import { formatRelativeTime, stationNetworkState, stationPhase, stationPhaseLabel, stationStatusClass } from "../lib/stationDisplay";
import { admissionLabels, conditionView, stationDeleteReason, stationFacts } from "../lib/stationDrawer";

const tabs = ["overview", "network", "conditions"] as const;
type Tab = typeof tabs[number];

function CopyValue({ value, zh }: { value?: string; zh: boolean }) {
  if (!value) return <strong>—</strong>;
  return <span className="station-copy-value"><code className="inline-code" title={value}>{value}</code><CopyButton value={value} zh={zh} iconOnly /></span>;
}

export function StationDrawer({ station, cluster, node, siteNode, admission, hasNode, siteDataState, refreshError, labels, cameras, now, zh, power, deleting, admissionPending, onClose, onDelete, onReissue, onRevoke, onRefresh }: {
  station: StationItem;
  cluster?: ClusterSummary;
  node: NodeRef;
  siteNode?: SiteNode;
  admission?: NodeAdmission;
  hasNode: boolean;
  siteDataState: "loading" | "ready" | "unavailable";
  refreshError: string;
  labels?: Record<string, string>;
  cameras?: string;
  now: number;
  zh: boolean;
  power: { pending: boolean; message: string; press: (action: RobotPowerAction, label: string) => void };
  deleting: boolean;
  admissionPending: "reissue" | "revoke" | "";
  onClose: () => void;
  onDelete: () => void;
  onReissue: () => void;
  onRevoke: () => void;
  onRefresh: () => void;
}) {
  const phase = stationPhase(station);
  const status = station.status?.node;
  const networkState = stationNetworkState(station);
  const powerNow = robotPowerState(labels);
  const selfCheck = robotSelfCheckState(labels);
  const facts = stationFacts(station, cluster, now, zh);
  const labelsForAdmission = admissionLabels(admission, zh);
  const [tab, setTab] = useState<Tab>("overview");
  const tabRefs = useRef<Record<Tab, HTMLButtonElement | null>>({ overview: null, network: null, conditions: null });
  const drawerRef = useRef<HTMLElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const deleteReason = stationDeleteReason(station, hasNode, zh);

  useEffect(() => {
    setTab("overview");
    closeRef.current?.focus();
  }, [station.metadata.name, station.metadata.namespace]);

  const selectTab = (next: Tab, focus = false) => {
    setTab(next);
    if (focus) tabRefs.current[next]?.focus();
  };
  const onTabKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const index = tabs.indexOf(tab);
    const next = event.key === "ArrowRight" ? tabs[(index + 1) % tabs.length]
      : event.key === "ArrowLeft" ? tabs[(index + tabs.length - 1) % tabs.length]
        : event.key === "Home" ? tabs[0] : event.key === "End" ? tabs[tabs.length - 1] : undefined;
    if (next) { event.preventDefault(); selectTab(next, true); }
  };
  const onDrawerKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== "Tab") return;
    const candidates = drawerRef.current?.querySelectorAll<HTMLElement>("button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], [tabindex]:not([tabindex='-1'])");
    const focusable = candidates ? [...candidates].filter((element) => element.getClientRects().length > 0) : [];
    if (!focusable.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  };

  return <>
    <div className="addon-drawer-overlay" onClick={onClose} />
    <aside ref={drawerRef} className="addon-drawer station-drawer station-drawer-redesign" role="dialog" aria-modal="true" aria-labelledby="station-drawer-title" aria-busy={deleting || !!admissionPending} onKeyDown={onDrawerKeyDown}>
      <div className="addon-drawer-header station-drawer-header">
        <div className="station-drawer-heading">
          <h3 id="station-drawer-title"><NodeName node={node} /></h3>
          <div className="station-drawer-meta">
            <span className={`status ${stationStatusClass(phase)}`}><i />{stationPhaseLabel(phase, zh)}</span>
            <span className="station-drawer-cluster">{cluster?.name ?? (zh ? "集群未确定" : "Cluster unresolved")}</span>
            <span className={`station-network station-network-${networkState}`}><Network size={14} aria-hidden="true" />{networkState === "verified" ? (zh ? "网络已验证" : "Network verified") : networkState === "failed" ? (zh ? "网络探测失败" : "Network probe failed") : (zh ? "网络未知" : "Network unknown")}</span>
            <span className="station-drawer-seen">{zh ? "最近上报" : "Last seen"} {formatRelativeTime(status?.lastSeenTime, now, zh)}</span>
          </div>
        </div>
        <button ref={closeRef} type="button" className="icon-button" onClick={onClose} aria-label={zh ? "关闭" : "Close"}><X size={16} /></button>
      </div>
      <div className="station-drawer-tabs" role="tablist" aria-label={zh ? "场站详情" : "Station details"} onKeyDown={onTabKeyDown}>
        {tabs.map((value) => <button key={value} ref={(element) => { tabRefs.current[value] = element; }} type="button" role="tab" id={`station-tab-${value}`} aria-controls={`station-panel-${value}`} aria-selected={tab === value} tabIndex={tab === value ? 0 : -1} onClick={() => selectTab(value)}>{value === "overview" ? zh ? "概览" : "Overview" : value === "network" ? zh ? "网络" : "Network" : zh ? "条件" : "Conditions"}</button>)}
      </div>
      <div className="addon-drawer-body station-drawer-body station-drawer-scroll" aria-live="polite">
        {refreshError && <div className="cert-error station-drawer-alert" role="alert"><span>{refreshError}</span><button type="button" className="secondary-button" onClick={onRefresh}>{zh ? "重试" : "Retry"}</button></div>}
        {phase === "Conflict" && <section className="station-conflict-banner" role="note"><AlertTriangle size={16} aria-hidden="true" /><div><strong>{zh ? "节点 UID 变了" : "Node UID changed"}</strong><p>{zh ? `这个名字现在上报的节点 UID 是 ${station.status?.observation?.nodeUID ?? "—"}，记录属于 UID ${station.spec.nodeUID}。此记录不进入隧道网。删除后，仍在上报的节点会按新机器重新登记。` : `This name now reports Node UID ${station.status?.observation?.nodeUID ?? "—"}, while this record belongs to ${station.spec.nodeUID}. It stays out of the tunnel network. If deleted, a node still reporting under this name registers as a new machine.`}</p></div></section>}

        <section id="station-panel-overview" role="tabpanel" aria-labelledby="station-tab-overview" hidden={tab !== "overview"}>
          <div className="station-facts-strip">{facts.map((fact) => <div key={fact.key}><small>{fact.label}</small><strong>{fact.value}</strong></div>)}</div>
          <section className="node-detail-section station-drawer-section"><span className="node-detail-label"><Fingerprint size={14} />{zh ? "身份" : "Identity"}</span><div className="node-detail-grid station-detail-grid">
            <div><span className="muted">{zh ? "记录名" : "Record name"}</span><CopyValue value={station.metadata.name} zh={zh} /></div>
            <div><span className="muted">{zh ? "集群" : "Cluster"}</span><strong>{cluster?.name ?? (zh ? "集群未确定" : "Cluster unresolved")}</strong></div>
            <div className="station-detail-wide"><span className="muted">Node UID</span><CopyValue value={station.spec.nodeUID} zh={zh} /></div>
            <div className="station-detail-wide"><span className="muted">System UUID</span><CopyValue value={station.status?.observation?.systemUUID} zh={zh} /></div>
          </div></section>
          {isRobotNode(labels) && <section className="node-detail-section station-drawer-section stations-power"><span className="node-detail-label"><Power size={14} />{zh ? "机器人电源" : "Robot power"}</span>
            <div className="station-robot-grid">
              <div><small>{zh ? "期望" : "Desired"}</small><span className={`status ${robotPowerStateClass(robotPowerDesired(labels))}`}><i />{robotPowerStateLabel(robotPowerDesired(labels), zh)}</span></div>
              <div><small>{zh ? "现在" : "Now"}</small>{powerNow ? <span className={`status ${robotPowerStateClass(powerNow)}`}><i />{robotPowerStateLabel(powerNow, zh)}</span> : <span className="muted">{zh ? "插件未上报" : "No Addon report"}</span>}</div>
              {robotMaintenanceState(labels) && <div><small>{zh ? "维护" : "Maintenance"}</small><span className="status status-pending"><i />{robotMaintenanceLabel(robotMaintenanceState(labels), zh)}</span></div>}
              {selfCheck && <div><small>{zh ? "自检" : "Self-check"}</small><span className={`status ${selfCheck === "ok" ? "status-online" : selfCheck === "failed" ? "status-failed" : "status-pending"}`}><i />{robotSelfCheckLabel(selfCheck, zh)}</span></div>}
            </div>
            <details className="station-power-rules"><summary>{zh ? "电源与维护规则" : "Power and maintenance rules"}</summary><p>{zh ? "期望状态保存在节点上；该上电没电或该运行的驱动停止时会自动恢复。急停或硬件故障时不会重拉。机器人正在使用时，下电会等它空闲。" : "The desired state stays on the Node and is reconciled when power or the driver is unexpectedly lost. It is not restored during an emergency stop or hardware fault. Power-off waits until the robot is idle."}</p></details>
            <div className="stations-power-actions">{([ ["power_on", zh ? "上电" : "Power on"], ["driver_on", zh ? "启动驱动" : "Start driver"], ["driver_off", zh ? "关闭驱动" : "Stop driver"], ["power_off", zh ? "下电" : "Power off"] ] as [RobotPowerAction, string][]).map(([action, label]) => <button key={action} type="button" className="secondary-button" disabled={power.pending} onClick={() => power.press(action, label)}>{label}</button>)}<RobotPoseButton robots={[{ key: `${station.metadata.namespace}/${station.metadata.name}`, node: station.metadata.name, namespace: station.metadata.namespace, labels }]} zh={zh} /></div>
            {power.message && <small role="status">{power.message}</small>}
          </section>}
          {isRobotNode(labels) && <NodeCameras node={station.metadata.name} namespace={station.metadata.namespace} labels={labels} published={cameras} zh={zh} />}
        </section>

        <section id="station-panel-network" role="tabpanel" aria-labelledby="station-tab-network" hidden={tab !== "network"}>
          {siteDataState === "loading" ? <p className="muted" role="status">{zh ? "正在读取网络与入网信息…" : "Loading network and admission details…"}</p> : siteDataState === "unavailable" ? <p className="muted">{zh ? "网络与入网信息暂不可用。" : "Network and admission details are unavailable."}</p> : <>
            <section className="node-detail-section station-drawer-section"><span className="node-detail-label"><KeyRound size={14} />{zh ? "隧道与入网" : "Tunnel and admission"}</span><div className="node-detail-grid station-detail-grid">
              <div className="station-detail-wide"><span className="muted">{zh ? "NetworkVerified" : "NetworkVerified"}</span><span className={`status ${networkState === "verified" ? "status-online" : networkState === "failed" ? "status-failed" : "status-pending"}`}><i />{networkState === "verified" ? (zh ? "已验证" : "Verified") : networkState === "failed" ? (zh ? "失败" : "Failed") : (zh ? "未知" : "Unknown")}</span></div>
              <div className="station-detail-wide"><span className="muted">WireGuard {zh ? "公钥" : "public key"}</span><CopyValue value={siteNode?.network?.public_key ?? admission?.public_key} zh={zh} /></div>
              <div><span className="muted">{zh ? "隧道地址" : "Tunnel address"}</span><CopyValue value={admission?.tunnel_address} zh={zh} /></div>
              <div><span className="muted">{zh ? "入网状态" : "Admission"}</span><span className={`status ${admission ? admissionStateClass(admission.state) : "status-stopped"}`}><i />{labelsForAdmission.state}</span></div>
              <div><span className="muted">{zh ? "过期时间" : "Expires"}</span><strong>{formatChinaDateTime(admission?.expires_at)}</strong></div>
              <div><span className="muted">{zh ? "发码人" : "Issued by"}</span><strong>{labelsForAdmission.issuer}</strong></div>
              <div className="station-detail-wide"><span className="muted">SSH {zh ? "登录" : "login"}</span><strong><code>{admission?.ssh_login || "—"}</code></strong></div>
              <div className="station-detail-wide"><span className="muted">{zh ? "SSH 主机指纹" : "SSH host fingerprint"}</span><CopyValue value={admission?.ssh_host_key} zh={zh} /></div>
              {admission?.reason && <div className="station-detail-wide"><span className="muted">{zh ? "入网说明" : "Admission detail"}</span><strong>{admission.reason}</strong></div>}
            </div></section>
            {siteNode ? <NodeLinks network={{ site: { online: siteNode.online, links: siteNode.network?.links ?? [], summary: siteNode.network?.link_summary ?? "" } }} zh={zh} /> : <p className="muted">{zh ? "该节点当前不在场站网络中。" : "This node is not currently in the site network."}</p>}
          </>}
        </section>
        <section id="station-panel-conditions" role="tabpanel" aria-labelledby="station-tab-conditions" hidden={tab !== "conditions"}>
          {conditionView(station, now, zh).length ? <ul className="station-conditions">{conditionView(station, now, zh).map((condition) => <li key={condition.type}><span className={`station-condition-dot is-${condition.status.toLowerCase()}`} aria-hidden="true" /><div><strong>{condition.label}</strong><small className="muted">{[condition.reason, condition.message].filter(Boolean).join(" · ") || "—"}</small></div><div className="station-condition-side"><span className={`status ${condition.statusClass}`}><i />{condition.status}</span>{condition.lastTransitionTime && <small className="muted" title={formatChinaDateTime(condition.lastTransitionTime)}>{condition.relativeTime}</small>}</div></li>)}</ul> : <p className="muted">{zh ? "暂无条件上报" : "No conditions reported"}</p>}
        </section>
      </div>
      <div className="addon-drawer-footer station-drawer-footer">
        <button type="button" className="secondary-button" onClick={onClose}>{zh ? "关闭" : "Close"}</button>
        <div className="station-drawer-footer-actions">
          <button type="button" className="secondary-button" disabled={!cluster || !!admissionPending || deleting} onClick={onReissue}>{admissionPending === "reissue" ? (zh ? "重发中…" : "Reissuing…") : (zh ? "重发入网码" : "Reissue code")}</button>
          <button type="button" className="secondary-button danger" disabled={!cluster || !admission || !!admissionPending || deleting} onClick={onRevoke}>{admissionPending === "revoke" ? (zh ? "作废中…" : "Revoking…") : (zh ? "作废入网码" : "Revoke code")}</button>
          <button type="button" className="secondary-button danger" disabled={!!deleteReason || deleting || !!admissionPending} aria-describedby={deleteReason ? "station-delete-reason" : undefined} onClick={onDelete}>{deleting ? (zh ? "删除中…" : "Deleting…") : (zh ? "删除记录" : "Delete record")}</button>
        </div>
        {deleteReason && <small id="station-delete-reason" className="station-delete-reason">{deleteReason}</small>}
      </div>
    </aside>
  </>;
}
