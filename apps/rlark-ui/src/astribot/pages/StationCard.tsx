import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { Camera, ChevronDown } from "lucide-react";
import { request } from "../../api";
import type { StationItem } from "../api";
import type { ClusterSummary } from "../../types";
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

// A robot's cameras as the tile's media surface: one large view carrying the identity and control overlays, and a
// strip of the smaller views. Frames are slow previews (one frame per view, five seconds); a frame click enlarges
// that camera and never expands the card. Requests stop while the tile is off screen, the tab is hidden or the
// detail below is open, which owns the live frames.
function CameraWall({ cells, node, namespace, zh, expanded, onZoom, overlayTop, overlayBottom }: {
  cells: CameraCell[];
  node: string;
  namespace: string;
  zh: boolean;
  expanded: boolean;
  onZoom: (view: CameraView, trigger: HTMLElement) => void;
  overlayTop?: ReactNode;
  overlayBottom?: ReactNode;
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

  const [big, ...small] = cells;
  const bigTitle = big ? [big.badge, big.note, big.advice].filter(Boolean).join(" · ") : undefined;
  return <div ref={wall} className="station-tile-media" role="group" aria-label={zh ? "相机" : "Cameras"}>
    <div className="station-tile-main">
      {big ? <button type="button" className={`station-camera is-big${big.present === false ? " is-missing" : ""}`}
        onClick={(event) => onZoom(big.view, event.currentTarget)} aria-haspopup="dialog"
        aria-label={`${big.name} · ${big.badge}`} title={bigTitle}>
        <span className={`station-camera-frame is-${big.tone}`}>{frames[big.view] ? <img src={frames[big.view]} alt="" loading="lazy" /> : big.present === false ? <span aria-hidden="true">—</span> : <Camera size={22} aria-hidden="true" />}</span>
      </button> : <div className="station-camera is-big is-empty" aria-hidden="true">
        <span className="station-camera-frame"><Camera size={22} aria-hidden="true" /></span>
      </div>}
      {overlayTop}
      {overlayBottom}
    </div>
    {small.length > 0 && <div className="station-tile-strip" role="group" aria-label={zh ? "其余相机" : "Other cameras"}>
      {small.map((cell) => <button key={cell.view} type="button" className={`station-camera${cell.present === false ? " is-missing" : ""}`}
        onClick={(event) => onZoom(cell.view, event.currentTarget)} aria-haspopup="dialog"
        aria-label={`${cell.name} · ${cell.badge}`} title={[cell.badge, cell.note, cell.advice].filter(Boolean).join(" · ")}>
        <span className={`station-camera-frame is-${cell.tone}`}>{frames[cell.view] ? <img src={frames[cell.view]} alt="" loading="lazy" /> : cell.present === false ? <span aria-hidden="true">—</span> : <Camera size={16} aria-hidden="true" />}</span>
        <span className="station-camera-caption"><strong>{cell.name}</strong><span className={`station-camera-state is-${cell.tone}`}><i />{cell.badge}</span></span>
      </button>)}
    </div>}
  </div>;
}

// The card says a robot's state in two or three words; the open detail has the full wording.
const cardPowerDesired = (state: string, zh: boolean) =>
  state === "ready" ? (zh ? "运行" : "Run")
    : state === "powered" ? (zh ? "已上电" : "Powered")
      : state === "off" ? (zh ? "下电" : "Off")
        : state;
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

  if (robot) {
    // The camera-wall tile: identity and controls live on the picture itself. An alert state (a power fault or
    // e-stop, a failed self-check, or maintenance) rings the tile so a problem reads from across the room.
    const tileAlert = powerNow === "fault" || powerNow === "estop" || selfCheck === "failed" || Boolean(maintenance);
    const overlayTop = <>
      <div className="station-tile-overlay is-top">
        <div className="station-tile-top">
          <input type="checkbox" className="station-card-check" aria-label={zh ? `选中 ${station.metadata.name}` : `Select ${station.metadata.name}`} checked={chosen} onChange={(event) => onChoose(event.target.checked)} />
          <div className="station-tile-name">
            <h3 id={nameID}><NodeName node={node} /></h3>
            <small>{machineModel(labels) || siteNode?.declared_type || siteNode?.observed_type || "—"} · <code className="inline-code">UID {shortUID(station.spec.nodeUID)}</code></small>
          </div>
          <span className={`station-tile-phase status ${stationStatusClass(phase)}`}><i />{stationPhaseLabel(phase, zh)}</span>
          <button id={expanderID} type="button" className="station-tile-expand" aria-expanded={expanded} aria-haspopup="dialog" aria-controls={detailsID} aria-label={expanded ? (zh ? "收起详情" : "Collapse details") : (zh ? "展开详情" : "Expand details")} title={expanded ? (zh ? "收起详情" : "Collapse details") : (zh ? "展开详情" : "Expand details")} onClick={onToggle}>
            <ChevronDown size={16} className={expanded ? "is-open" : ""} aria-hidden="true" />
          </button>
        </div>
        <div className="station-tile-tags">
          {admission ? <span className={`station-tile-tag is-${admissionStateClass(admission.state)}`} title={admission.reason}>{admissionStateLabel(admission.state, zh)}</span> : <span className="station-tile-tag">{zh ? "未用入网码" : "No code"}</span>}
          {selfCheck && <span className={`station-tile-tag is-${selfCheck === "failed" ? "alert" : selfCheck === "pending" ? "warning" : "good"}`}>{robotSelfCheckLabel(selfCheck, zh)}</span>}
          {maintenance && <span className="station-tile-tag is-warning" title={robotMaintenanceLabel(maintenance, zh)}>{zh ? "维护中" : "Maintenance"}</span>}
          {siteNode?.identity_verified === "False" && <span className="station-tile-tag is-alert" title={siteNode?.identity_reason}>{zh ? "身份不匹配" : "identity mismatch"}</span>}
          <NetworkBadge state={stationNetworkState(station)} zh={zh} />
        </div>
      </div>
    </>;
    const overlayBottom = <>
      <div className="station-tile-overlay is-bottom">
        <div className="station-tile-power">
          <span><small>{zh ? "期望" : "Desired"}</small><strong>{cardPowerDesired(powerDesired, zh)}</strong></span>
          <span><small>{zh ? "实际" : "Actual"}</small>{powerNow ? <strong className={powerNow === "fault" || powerNow === "estop" ? "is-alert" : powerNow === "off" ? "muted" : ""}>{cardPowerActual(powerNow, zh)}</strong> : <span className="muted">{zh ? "未上报" : "No report"}</span>}</span>
        </div>
        <div className="station-tile-actions">
          {driverAction && <button type="button" className="station-tile-action" disabled={power.pending} onClick={() => power.press(driverAction, driverLabel)}>{driverLabel}</button>}
          <button type="button" className="station-tile-action" disabled={power.pending} onClick={() => power.press(quickPower, quickPowerLabel)}>{quickPowerLabel}</button>
          <RobotPoseButton robots={[{ key: `${station.metadata.namespace}/${station.metadata.name}`, node: station.metadata.name, namespace: station.metadata.namespace, labels }]} zh={zh} />
        </div>
      </div>
    </>;
    return <article className={`station-tile${expanded ? " is-expanded" : ""}${chosen ? " is-selected" : ""}${retired ? " is-retired" : ""}${tileAlert ? " is-alert" : ""}`} aria-labelledby={nameID} onKeyDown={onKeyDown}>
      <CameraWall cells={cameraList} node={station.metadata.name} namespace={station.metadata.namespace} zh={zh} expanded={expanded} onZoom={onZoom} overlayTop={overlayTop} overlayBottom={overlayBottom} />
      <dl className="station-tile-facts">{facts.slice(0, 3).map((fact) => <div key={fact.key}><dt>{fact.label}</dt><dd>{fact.value}</dd></div>)}</dl>
    </article>;
  }

  // Plain inventory nodes keep the compact card; the Stations page renders them as rows, so this branch is a
  // fallback for embedders that still pass variant "node".
  return <article className={`station-card is-node${expanded ? " is-expanded" : ""}${retired ? " is-retired" : ""}`} aria-labelledby={nameID} onKeyDown={onKeyDown}>
    <header className="station-card-head">
      <div className="station-card-name">
        <h3 id={nameID}><NodeName node={node} /></h3>
        <small>{machineModel(labels) || "—"} · <code className="inline-code">UID {shortUID(station.spec.nodeUID)}</code></small>
      </div>
      <span className={`status ${stationStatusClass(phase)}`}><i />{stationPhaseLabel(phase, zh)}</span>
    </header>

    <div className="station-card-meta">
      {(siteNode?.declared_type || siteNode?.observed_type) && <span className="station-card-kind" title={siteNode?.identity_reason}>{siteNode?.declared_type || siteNode?.observed_type}{(siteNode?.declared_model || siteNode?.observed_model) ? ` · ${siteNode.declared_model || siteNode.observed_model}` : ""}{siteNode?.identity_verified === "False" ? (zh ? " · 身份不匹配" : " · identity mismatch") : ""}</span>}
      <NetworkBadge state={stationNetworkState(station)} zh={zh} />
      {admission ? <span className={`status ${admissionStateClass(admission.state)}`} title={admission.reason}><i />{admissionStateLabel(admission.state, zh)}</span> : <span className="muted">{zh ? "未用入网码" : "No code"}</span>}
    </div>

    <dl className="station-card-facts">{facts.slice(0, 3).map((fact) => <div key={fact.key}><dt>{fact.label}</dt><dd>{fact.value}</dd></div>)}</dl>

    <footer className="station-card-foot">
      <div className="station-card-quick"></div>
      {expander}
    </footer>
  </article>;
}
