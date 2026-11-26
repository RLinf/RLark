import { type KeyboardEvent } from "react";
import { AlertTriangle, Fingerprint, KeyRound, ListFilter, Power } from "lucide-react";
import type { ClusterSummary } from "../../types";
import type { StationItem } from "../api";
import { formatChinaDateTime } from "../../utils/time.js";
import { isControlPlaneLabels } from "../../utils/nodeVisibility";
import { NodeCameras } from "../components/NodeCameras";
import { NodeLinks } from "../components/NodeLinks";
import { RobotPoseButton } from "../components/RobotControls";
import { cameraCells, publishedCameras, type CameraView } from "../lib/cameras";
import { admissionLabels, conditionView, stationDeleteReason } from "../lib/stationDrawer";
import { stationNetworkState, stationPhase, stationPhaseLabel, stationStatusClass, shortUID } from "../lib/stationDisplay";
import {
  admissionStateClass, isRobotNode, robotMaintenanceLabel, robotMaintenanceState,
  robotPowerDesired, robotPowerState, robotPowerStateClass, robotPowerStateLabel, robotSelfCheckLabel,
  robotSelfCheckState, type NodeAdmission, type RobotPowerAction, type SiteNode,
} from "../lib/siteNodeDisplay";
import { CopyValue, DetailField, NetworkBadge } from "./StationFields";
import { StationRunningImages } from "./StationRunningImages";

export interface StationPower {
  pending: boolean;
  press: (action: RobotPowerAction, label: string) => void;
}

// The one open station, under its card row: identity, network and admission, conditions, then a robot's power and
// cameras. It is rendered by the page rather than inside the card so it spans the row and never stretches the cards
// above it. All facts that the card already shows stay on the card; this panel adds the rest.
export function StationDetailPanel({
  station, cluster, siteNode, admission, hasNode, labels, powerNote, siteDataState, refreshError, now, zh,
  power, deleting, admissionPending, onDismiss, onDelete, onReissue, onRevoke, onRefresh, onZoom, cameraActive = true,
}: {
  station: StationItem;
  cluster?: ClusterSummary;
  siteNode?: SiteNode;
  admission?: NodeAdmission;
  hasNode: boolean;
  labels?: Record<string, string>;
  powerNote?: string;
  siteDataState: "loading" | "ready" | "unavailable";
  refreshError: string;
  now: number;
  zh: boolean;
  power: StationPower;
  deleting: boolean;
  admissionPending: "reissue" | "revoke" | "";
  // Present only inside the detail dialog: the panel then renders the dialog's close button in its action row.
  onDismiss?: () => void;
  onDelete: () => void;
  onReissue: () => void;
  onRevoke: () => void;
  onRefresh: () => void;
  onZoom: (view: CameraView, trigger: HTMLElement) => void;
  // False while the camera zoom dialog owns the picture: the wall pauses its loops and resumes on return.
  cameraActive?: boolean;
}) {
  const phase = stationPhase(station);
  const retired = phase === "Retired";
  const robot = isRobotNode(labels) && !retired;
  const controlPlane = isControlPlaneLabels(labels);
  const deleteReason = stationDeleteReason(station, hasNode, zh);
  const admissionFor = admissionLabels(admission, zh);
  const networkState = stationNetworkState(station);
  const powerNow = robotPowerState(labels);
  const powerDesired = robotPowerDesired(labels);
  const selfCheck = robotSelfCheckState(labels);
  const maintenance = robotMaintenanceState(labels);
  const detailsID = `station-details-${station.metadata.namespace}-${station.metadata.name}`;
  const titleID = `${detailsID}-title`;
  const deleteReasonID = `${detailsID}-delete-reason`;

  const conditions = conditionView(station, now, zh);
  const cameraList = cameraCells(publishedCameras(siteNode?.cameras), zh, siteDataState !== "unavailable");
  const cameraIssues = robot ? cameraList.filter((cell) => cell.present !== false && (cell.tone === "failed" || cell.tone === "pending" || Boolean(cell.note) || Boolean(cell.advice))) : [];
  const conflictUID = station.status?.observation?.nodeUID;

  const onKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== "Escape" || !onDismiss) return;
    event.preventDefault();
    onDismiss();
  };

  const reissueDisabled = !cluster || admissionPending === "reissue" || deleting;
  const revokeDisabled = !cluster || !admission || admissionPending === "revoke" || deleting;
  const deleteDisabled = Boolean(deleteReason) || deleting || Boolean(admissionPending);
  const reissueReason = !cluster ? (zh ? "集群未确定。" : "The cluster is unresolved.") : "";
  const revokeReason = !cluster ? (zh ? "集群未确定。" : "The cluster is unresolved.") : !admission ? (zh ? "该节点没有入网记录。" : "This node has no admission record.") : "";

  return <section id={detailsID} className="station-detail-panel" aria-labelledby={titleID} onKeyDown={onKeyDown}>
    <header className="station-detail-head">
      <div className="station-detail-title">
        <span className="station-detail-eyebrow">{zh ? "节点详情" : "Node details"}</span>
        <div className="station-detail-name">
          <h3 id={titleID} data-detail-title tabIndex={-1}>{station.metadata.name}</h3>
          <span className={`status ${stationStatusClass(phase)}`}><i />{stationPhaseLabel(phase, zh)}</span>
        </div>
        <small className="station-detail-sub">
          {controlPlane && <span className="control-plane-badge">{zh ? "控制面" : "Control plane"}</span>}
          <span>UID {shortUID(station.spec.nodeUID)}</span>
        </small>
      </div>
      <div className="station-detail-actions">
        <div className="station-detail-action-row" role="group" aria-label={zh ? "记录操作" : "Record actions"}>
          <button type="button" className="secondary-button" disabled={reissueDisabled} aria-describedby={reissueReason ? `${detailsID}-reissue-reason` : undefined} onClick={onReissue}>{admissionPending === "reissue" ? (zh ? "重发中…" : "Reissuing…") : (zh ? "重发入网码" : "Reissue code")}</button>
          <button type="button" className="secondary-button" disabled={revokeDisabled} aria-describedby={revokeReason ? `${detailsID}-revoke-reason` : undefined} onClick={onRevoke}>{admissionPending === "revoke" ? (zh ? "作废中…" : "Revoking…") : (zh ? "作废入网码" : "Revoke code")}</button>
          <button type="button" className="secondary-button danger" disabled={deleteDisabled} aria-describedby={deleteReason ? deleteReasonID : undefined} onClick={onDelete}>{deleting ? (zh ? "删除中…" : "Deleting…") : (zh ? "删除记录" : "Delete record")}</button>
          {onDismiss && <button type="button" className="secondary-button station-detail-dismiss" onClick={onDismiss} aria-label={zh ? "关闭详情" : "Close details"} title={zh ? "关闭详情" : "Close details"}>×</button>}
        </div>
        {reissueReason && <small id={`${detailsID}-reissue-reason`} className="station-detail-reason">{reissueReason}</small>}
        {revokeReason && <small id={`${detailsID}-revoke-reason`} className="station-detail-reason">{revokeReason}</small>}
        {deleteReason && <small id={deleteReasonID} className="station-detail-reason">{deleteReason}</small>}
      </div>
    </header>

    {refreshError && <div className="cert-error station-detail-alert" role="alert"><span>{refreshError}</span><button type="button" className="secondary-button" onClick={onRefresh}>{zh ? "重试" : "Retry"}</button></div>}

    {phase === "Conflict" && <div className="station-detail-conflict" role="note">
      <AlertTriangle size={16} aria-hidden="true" />
      <div><strong>{zh ? "节点 UID 变了" : "Node UID changed"}</strong>
        <p>{zh ? `这个名字现在上报的节点 UID 是 ${conflictUID ?? "—"}，记录属于 UID ${station.spec.nodeUID}。此记录不进入隧道网。删除后，仍在上报的节点会按新机器重新登记。` : `This name now reports Node UID ${conflictUID ?? "—"} while this record belongs to ${station.spec.nodeUID}. This record stays out of the tunnel network. If deleted, a node still reporting under this name registers as a new machine.`}</p>
        <dl className="station-detail-conflict-ids">
          <div><dt>{zh ? "记录绑定 UID" : "Recorded UID"}</dt><dd><CopyValue value={station.spec.nodeUID} zh={zh} /></dd></div>
          <div><dt>{zh ? "当前上报 UID" : "Reported UID"}</dt><dd><CopyValue value={conflictUID} zh={zh} /></dd></div>
        </dl>
      </div>
    </div>}

    <div className="station-detail-columns">
      <section className="station-detail-column" aria-label={zh ? "身份" : "Identity"}>
        <h4><Fingerprint size={14} aria-hidden="true" />{zh ? "身份" : "Identity"}</h4>
        <dl className="station-detail-fields">
          <DetailField label={zh ? "记录名" : "Record name"}><CopyValue value={station.metadata.name} zh={zh} /></DetailField>
          <DetailField label={zh ? "集群" : "Cluster"}><strong>{cluster?.name ?? (zh ? "集群未确定" : "Cluster unresolved")}</strong></DetailField>
          <DetailField label={zh ? "集群 ID" : "Cluster ID"}><CopyValue value={cluster?.id} zh={zh} /></DetailField>
          <DetailField label={zh ? "声明类型 / 型号" : "Declared type / model"}><strong>{[siteNode?.declared_type, siteNode?.declared_model].filter(Boolean).join(" · ") || "—"}</strong></DetailField>
          <DetailField label={zh ? "观测类型 / 型号" : "Observed type / model"}><strong>{[siteNode?.observed_type, siteNode?.observed_model].filter(Boolean).join(" · ") || "—"}{siteNode?.identity_verified && <small className={siteNode.identity_verified === "False" ? "is-alert" : undefined}>{siteNode.identity_verified}{siteNode.identity_reason ? ` · ${siteNode.identity_reason}` : ""}</small>}</strong></DetailField>
          <DetailField label="Node UID"><CopyValue value={station.spec.nodeUID} zh={zh} /></DetailField>
          <DetailField label="System UUID"><CopyValue value={station.status?.observation?.systemUUID} zh={zh} /></DetailField>
        </dl>
        {cameraIssues.length > 0 && <div className="station-detail-diagnostics">
          <h5>{zh ? "相机诊断" : "Camera diagnostics"}</h5>
          <ul>{cameraIssues.map((cell) => <li key={cell.view}><strong>{cell.name} · {cell.badge}</strong>{(cell.note || cell.advice) && <small>{[cell.note, cell.advice].filter(Boolean).join(" · ")}</small>}</li>)}</ul>
        </div>}
      </section>

      <section className="station-detail-column" aria-label={zh ? "网络与入网" : "Network and admission"}>
        <h4><KeyRound size={14} aria-hidden="true" />{zh ? "网络与入网" : "Network and admission"}</h4>
        {siteDataState === "loading" && <p className="station-detail-note" role="status">{zh ? "正在读取网络与入网信息…" : "Loading network and admission details…"}</p>}
        {siteDataState === "unavailable" && <p className="station-detail-note">{zh ? "网络与入网信息暂不可用。" : "Network and admission details are unavailable."}</p>}
        <dl className="station-detail-fields">
          <DetailField label={zh ? "网络已验证" : "Network verified"}><NetworkBadge state={networkState} zh={zh} /></DetailField>
          <DetailField label={zh ? "WireGuard 公钥" : "WireGuard public key"} wide><CopyValue value={siteNode?.network?.public_key ?? admission?.public_key} zh={zh} /></DetailField>
          <DetailField label={zh ? "隧道地址" : "Tunnel address"} wide><CopyValue value={admission?.tunnel_address} zh={zh} /></DetailField>
          <DetailField label={zh ? "入网状态" : "Admission status"}><span className={`status ${admission ? admissionStateClass(admission.state) : "status-stopped"}`}><i />{admissionFor.state}</span></DetailField>
          <DetailField label={zh ? "发码人" : "Issued by"}><strong>{admissionFor.issuer}</strong></DetailField>
          <DetailField label={zh ? "过期时间" : "Expires"}><strong>{formatChinaDateTime(admission?.expires_at)}</strong></DetailField>
          <DetailField label={zh ? "SSH 登录" : "SSH login"} wide>{admission?.ssh_login ? <code className="inline-code">{admission.ssh_login}</code> : <strong>—</strong>}</DetailField>
          <DetailField label={zh ? "SSH 主机指纹" : "SSH host fingerprint"} wide><CopyValue value={admission?.ssh_host_key} zh={zh} /></DetailField>
          {admission?.reason && <DetailField label={zh ? "入网说明" : "Admission detail"} wide><strong>{admission.reason}</strong></DetailField>}
        </dl>
        {siteDataState === "ready" && !siteNode && <p className="station-detail-note">{zh ? "该节点当前不在场站网络中。" : "This node is not currently in the site network."}</p>}
      </section>

      <section className="station-detail-column" aria-label={zh ? "条件" : "Conditions"}>
        <h4><ListFilter size={14} aria-hidden="true" />{zh ? "条件" : "Conditions"}</h4>
        {conditions.length > 0 ? <ul className="station-condition-list">
          {conditions.map((condition) => <li key={condition.type}>
            <div className="station-condition-main"><strong>{condition.label}</strong><small>{condition.type}</small></div>
            <div className="station-condition-side"><span className={`station-condition-pill is-${condition.status.toLowerCase()}`}><i />{condition.status}</span>{condition.lastTransitionTime && <small title={formatChinaDateTime(condition.lastTransitionTime)}>{condition.relativeTime}</small>}</div>
            {(condition.reason || condition.message) && <small className="station-condition-detail">{[condition.reason, condition.message].filter(Boolean).join(" · ")}</small>}
          </li>)}
        </ul> : <p className="station-detail-note">{zh ? "暂无条件上报" : "No conditions reported"}</p>}
      </section>
    </div>

    {siteNode && <NodeLinks network={{ site: { online: siteNode.online, links: siteNode.network?.links ?? [], summary: siteNode.network?.link_summary ?? "" } }} zh={zh} collapsible />}

    {!retired && <StationRunningImages name={station.metadata.name} clusterID={cluster?.id ?? ""} uid={station.spec.nodeUID} zh={zh} />}

    {robot && <section className="station-detail-robot" aria-label={zh ? "机器人" : "Robot"}>
      <div className="station-detail-robot-head">
        <h4><Power size={14} aria-hidden="true" />{zh ? "机器人电源" : "Robot power"}</h4>
        <div className="station-detail-power-facts">
          <span><small>{zh ? "期望" : "Desired"}</small><strong>{robotPowerStateLabel(powerDesired, zh)}</strong></span>
          <span><small>{zh ? "现在" : "Now"}</small>{powerNow ? <span className={`status ${robotPowerStateClass(powerNow)}`}><i />{robotPowerStateLabel(powerNow, zh)}</span> : <span className="muted">{zh ? "插件未上报" : "No Addon report"}</span>}</span>
          <span><small>{zh ? "自检" : "Self-check"}</small>{selfCheck ? <span className={`status ${selfCheck === "ok" ? "status-online" : selfCheck === "failed" ? "status-failed" : "status-pending"}`}><i />{robotSelfCheckLabel(selfCheck, zh)}</span> : <span className="muted">{zh ? "未上报" : "No report"}</span>}</span>
          {maintenance && <span><small>{zh ? "维护" : "Maintenance"}</small><span className="status status-pending"><i />{robotMaintenanceLabel(maintenance, zh)}</span></span>}
        </div>
      </div>
      <div className="station-detail-action-row">
        {([["power_on", zh ? "上电" : "Power on"], ["driver_on", zh ? "启动驱动" : "Start driver"], ["driver_off", zh ? "关闭驱动" : "Stop driver"], ["power_off", zh ? "下电" : "Power off"]] as [RobotPowerAction, string][]).map(([action, label]) => <button key={action} type="button" className="secondary-button" disabled={power.pending} onClick={() => power.press(action, label)}>{label}</button>)}
        <RobotPoseButton robots={[{ key: `${station.metadata.namespace}/${station.metadata.name}`, node: station.metadata.name, namespace: station.metadata.namespace, labels }]} zh={zh} />
      </div>
      {powerNote && <p className="station-detail-power-note"><strong>{zh ? "电源服务：" : "Power service: "}</strong>{powerNote}</p>}
      <details className="station-detail-rules"><summary>{zh ? "电源与维护规则" : "Power and maintenance rules"}</summary><p>{zh ? "期望状态保存在节点上；该上电没电或该运行的驱动停止时会自动恢复。急停或硬件故障时不会重拉。机器人正在使用时，下电会等它空闲。" : "The desired state stays on the Node and is reconciled when power or the driver is unexpectedly lost. It is not restored during an emergency stop or hardware fault. Power-off waits until the robot is idle."}</p></details>
      <NodeCameras node={station.metadata.name} namespace={station.metadata.namespace} labels={labels} published={siteNode?.cameras} zh={zh} heading={zh ? "实时相机" : "Live cameras"} onOpen={onZoom} active={cameraActive} />
    </section>}
  </section>;
}
