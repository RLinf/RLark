import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { Camera, ChevronDown } from "lucide-react";
import { request } from "../../api";
import type { StationItem } from "../api";
import type { ClusterSummary } from "../../types";
import { isControlPlaneLabels } from "../../utils/nodeVisibility";
import { NodeName, type NodeRef } from "../components/NodeName";
import { RobotPoseButton } from "../components/RobotControls";
import { cameraCells, cameraFramePath, publishedCameras, type CameraCell, type CameraView } from "../lib/cameras";
import { stationFacts } from "../lib/stationDrawer";
import { shortUID, stationNetworkState, stationPhase, stationPhaseLabel, stationStatusClass } from "../lib/stationDisplay";
import {
  admissionStateClass, admissionStateLabel, machineModel, robotMaintenanceLabel, robotMaintenanceState,
  robotPowerDesired, robotPowerState, robotSelfCheckLabel,
  robotSelfCheckState, type NodeAdmission, type RobotPowerAction, type SiteNode,
} from "../lib/siteNodeDisplay";
import { NetworkBadge } from "./StationFields";

// A robot's cameras on its card: the published state, plus one slow frame per view so the card shows the machine
// without the live wall that only an open detail needs. A frame click enlarges that camera; it never expands the
// card. Requests stop while the wall is off screen, the tab is hidden or the detail below is open, which owns the
// live frames.
function CameraWall({ cells, node, namespace, zh, expanded, onZoom }: {
  cells: CameraCell[];
  node: string;
  namespace: string;
  zh: boolean;
  expanded: boolean;
  onZoom: (view: CameraView, trigger: HTMLElement) => void;
}) {
  const wall = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(false);
  const [frames, setFrames] = useState<Partial<Record<CameraView, string>>>({});
  const views = cells.filter((cell) => cell.frames).map((cell) => cell.view);
  const viewsKey = views.join(",");

  useEffect(() => {
    const element = wall.current;
    if (!element) return;
    if (typeof IntersectionObserver === "undefined") {
      setVisible(true);
      return;
    }
    const observer = new IntersectionObserver((entries) => setVisible(entries.some((entry) => entry.isIntersecting)), { rootMargin: "160px" });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!visible || expanded || !viewsKey) return;
    let stopped = false;
    let timer = 0;
    const shots: Partial<Record<CameraView, string>> = {};
    setFrames({});
    const ask = async () => {
      for (const view of views) {
        if (stopped) return;
        try {
          const response = await request(cameraFramePath(node, namespace, view), { signal: AbortSignal.timeout(4000) });
          const blob = await response.blob();
          if (stopped) return;
          if (shots[view]) URL.revokeObjectURL(shots[view]!);
          shots[view] = URL.createObjectURL(blob);
          setFrames({ ...shots });
        } catch {
          // A view without a frame keeps its state badge and the placeholder.
        }
      }
    };
    void ask();
    timer = window.setInterval(() => { if (!document.hidden) void ask(); }, 5000);
    return () => {
      stopped = true;
      window.clearInterval(timer);
      Object.values(shots).forEach((url) => { if (url) URL.revokeObjectURL(url); });
    };
    // The frame set follows the published report; node and namespace identify the machine.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, expanded, node, namespace, viewsKey]);

  return <div ref={wall} className="station-camera-wall" role="group" aria-label={zh ? "相机" : "Cameras"}>
    {cells.map((cell) => <button key={cell.view} type="button" className={`station-camera${cell.present === false ? " is-missing" : ""}`}
      onClick={(event) => onZoom(cell.view, event.currentTarget)} aria-haspopup="dialog"
      aria-label={`${cell.name} · ${cell.badge}`} title={[cell.badge, cell.note, cell.advice].filter(Boolean).join(" · ")}>
      <span className={`station-camera-frame is-${cell.tone}`}>{frames[cell.view] ? <img src={frames[cell.view]} alt="" loading="lazy" /> : cell.present === false ? <span aria-hidden="true">—</span> : <Camera size={18} aria-hidden="true" />}</span>
      <span className="station-camera-caption"><strong>{cell.name}</strong><span className={`station-camera-state is-${cell.tone}`}><i />{cell.badge}</span></span>
    </button>)}
  </div>;
}

// The card says a robot's state in two or three words; the open detail has the full wording.
const cardPowerDesired = (state: string, zh: boolean) => state === "ready" ? (zh ? "运行" : "Run") : state === "off" ? (zh ? "下电" : "Off") : state;
const cardPowerActual = (state: string | null, zh: boolean) =>
  state === "ready" ? (zh ? "运行中" : "Running")
    : state === "powered" ? (zh ? "已上电" : "Powered")
      : state === "off" ? (zh ? "未上电" : "Off")
        : state === "estop" ? (zh ? "急停" : "E-stop")
          : state === "fault" ? (zh ? "硬件报错" : "Hardware error")
            : state === "unknown" ? (zh ? "读不到" : "Unreadable")
              : "";

export type StationCardVariant = "robot" | "node" | "control";

export function StationCard({
  station, cluster, node, siteNode, admission, labels, siteDataState, now, zh, expanded, chosen, power, variant,
  onToggle, onChoose, onZoom,
}: {
  station: StationItem;
  cluster?: ClusterSummary;
  node: NodeRef;
  siteNode?: SiteNode;
  admission?: NodeAdmission;
  labels?: Record<string, string>;
  siteDataState: "loading" | "ready" | "unavailable";
  now: number;
  zh: boolean;
  expanded: boolean;
  chosen: boolean;
  power: { pending: boolean; press: (action: RobotPowerAction, label: string) => void };
  variant: StationCardVariant;
  onToggle: () => void;
  onChoose: (checked: boolean) => void;
  onZoom: (view: CameraView, trigger: HTMLElement) => void;
}) {
  const phase = stationPhase(station);
  const retired = phase === "Retired";
  const robot = variant === "robot";
  const control = variant === "control";
  const controlPlane = isControlPlaneLabels(labels);
  const facts = stationFacts(station, cluster, now, zh);
  const powerNow = robotPowerState(labels);
  const powerDesired = robotPowerDesired(labels);
  const selfCheck = robotSelfCheckState(labels);
  const maintenance = robotMaintenanceState(labels);
  const quickPower: RobotPowerAction = powerNow === "off" || (!powerNow && powerDesired === "off") ? "power_on" : "power_off";
  const quickPowerLabel = quickPower === "power_on" ? (zh ? "上电" : "Power on") : (zh ? "下电" : "Power off");
  // A robot that is on but whose driver is stopped can be started from its card, and one whose driver runs can stop
  // it there: the detail still has all four power and driver actions.
  const driverAction: RobotPowerAction | "" = powerNow === "ready" ? "driver_off" : powerNow === "powered" ? "driver_on" : "";
  const driverLabel = driverAction === "driver_off" ? (zh ? "关闭驱动" : "Stop driver") : driverAction === "driver_on" ? (zh ? "启动驱动" : "Start driver") : "";
  const detailsID = `station-details-${station.metadata.namespace}-${station.metadata.name}`;
  const nameID = `${detailsID}-card-name`;
  const expanderID = `station-expand-${station.metadata.namespace}-${station.metadata.name}`;
  const cameraList = robot ? cameraCells(publishedCameras(siteNode?.cameras), zh, siteDataState !== "unavailable") : [];

  const onKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== "Escape" || !expanded) return;
    event.preventDefault();
    onToggle();
  };

  const expander = <button id={expanderID} type="button" className="secondary-button station-expand" aria-expanded={expanded} aria-controls={detailsID} onClick={onToggle}>
    {expanded ? (zh ? "收起详情" : "Collapse details") : (zh ? "展开详情" : "Expand details")}
    <ChevronDown size={14} className={expanded ? "is-open" : ""} aria-hidden="true" />
  </button>;

  if (control) {
    return <article className={`station-control-row${expanded ? " is-expanded" : ""}`} aria-labelledby={nameID} onKeyDown={onKeyDown}>
      <div className="station-control-name">
        <h3 id={nameID}><NodeName node={node} /></h3>
        <small>{zh ? "控制面" : "Control plane"} · UID {shortUID(station.spec.nodeUID)}</small>
      </div>
      <span className={`status ${stationStatusClass(phase)}`}><i />{stationPhaseLabel(phase, zh)}</span>
      {expander}
    </article>;
  }

  return <article className={`station-card ${robot ? "is-robot" : "is-node"}${expanded ? " is-expanded" : ""}${chosen && robot ? " is-selected" : ""}${retired ? " is-retired" : ""}`} aria-labelledby={nameID} onKeyDown={onKeyDown}>
    <header className="station-card-head">
      {robot && <input type="checkbox" className="station-card-check" aria-label={zh ? `选中 ${station.metadata.name}` : `Select ${station.metadata.name}`} checked={chosen} onChange={(event) => onChoose(event.target.checked)} />}
      <div className="station-card-name">
        <h3 id={nameID}><NodeName node={node} /></h3>
        <small>{controlPlane && <span className="control-plane-badge">{zh ? "控制面" : "Control plane"}</span>}{machineModel(labels) || "—"} · <code className="inline-code">UID {shortUID(station.spec.nodeUID)}</code></small>
      </div>
      <span className={`status ${stationStatusClass(phase)}`}><i />{stationPhaseLabel(phase, zh)}</span>
    </header>

    <div className="station-card-meta">
      {(siteNode?.declared_type || siteNode?.observed_type) && <span className="station-card-kind" title={siteNode?.identity_reason}>{siteNode?.declared_type || siteNode?.observed_type}{(siteNode?.declared_model || siteNode?.observed_model) ? ` · ${siteNode.declared_model || siteNode.observed_model}` : ""}{siteNode?.identity_verified === "False" ? (zh ? " · 身份不匹配" : " · identity mismatch") : ""}</span>}
      <NetworkBadge state={stationNetworkState(station)} zh={zh} />
      {admission ? <span className={`status ${admissionStateClass(admission.state)}`} title={admission.reason}><i />{admissionStateLabel(admission.state, zh)}</span> : <span className="muted">{zh ? "未用入网码" : "No code"}</span>}
      {robot && <>
        <span className="station-card-power"><small>{zh ? "期望" : "Desired"}</small><strong>{cardPowerDesired(powerDesired, zh)}</strong></span>
        <span className="station-card-power"><small>{zh ? "实际" : "Actual"}</small>{powerNow ? <strong className={powerNow === "fault" || powerNow === "estop" ? "is-alert" : powerNow === "off" ? "muted" : ""}>{cardPowerActual(powerNow, zh)}</strong> : <span className="muted">{zh ? "未上报" : "No report"}</span>}</span>
        <span className="station-card-power"><small>{zh ? "自检" : "Self-check"}</small>{selfCheck ? <strong className={selfCheck === "failed" ? "is-alert" : selfCheck === "pending" ? "is-warning" : ""}>{robotSelfCheckLabel(selfCheck, zh)}</strong> : <span className="muted">{zh ? "未上报" : "No report"}</span>}</span>
        {maintenance && <span className="station-card-power" title={robotMaintenanceLabel(maintenance, zh)}><small>{zh ? "维护" : "Maintenance"}</small><strong className="is-warning">{zh ? "维护中" : "Maintenance"}</strong></span>}
      </>}
    </div>

    {robot && <>
      <CameraWall cells={cameraList} node={station.metadata.name} namespace={station.metadata.namespace} zh={zh} expanded={expanded} onZoom={onZoom} />
      <div className="station-camera-line"><span>{zh ? "相机" : "Cameras"}</span><span>{expanded ? (zh ? "实时画面在下方" : "Live frames below") : (zh ? "慢速预览 · 5 秒" : "Preview · 5 s")}</span></div>
    </>}

    <dl className="station-card-facts">{facts.slice(0, 3).map((fact) => <div key={fact.key}><dt>{fact.label}</dt><dd>{fact.value}</dd></div>)}</dl>

    <footer className="station-card-foot">
      <div className="station-card-quick">{robot && <>
        {driverAction && <button type="button" className="secondary-button" disabled={power.pending} onClick={() => power.press(driverAction, driverLabel)}>{driverLabel}</button>}
        <button type="button" className="secondary-button" disabled={power.pending} onClick={() => power.press(quickPower, quickPowerLabel)}>{quickPowerLabel}</button>
        <RobotPoseButton robots={[{ key: `${station.metadata.namespace}/${station.metadata.name}`, node: station.metadata.name, namespace: station.metadata.namespace, labels }]} zh={zh} />
      </>}</div>
      {expander}
    </footer>
  </article>;
}
