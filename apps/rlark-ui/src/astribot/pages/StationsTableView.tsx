import { useLayoutEffect, useRef, useState, type MouseEvent } from "react";
import { Camera } from "lucide-react";
import { NodeName } from "../components/NodeName";
import type { CRDNode } from "../../types";
import { computeVirtualWindow, type FleetStation, type StationFleetMeta } from "../lib/stationFleet";
import { stationPhase, stationPhaseLabel, stationStatusClass } from "../lib/stationDisplay";
import { isRobotNode, robotPowerActionBlocked, robotPowerState, type RobotPowerAction } from "../lib/siteNodeDisplay";
import "./StationsTableView.css";

const ROW_HEIGHT = 50;

export const fleetKey = (item: FleetStation) => `${item.metadata.namespace}/${item.metadata.name}`;

export function fleetAlertLabel(meta: StationFleetMeta, zh: boolean) {
  if (!zh) return meta.alertReason;
  const labels: Record<string, string> = {
    "Emergency stop": "急停", "Power or self-check fault": "电源或自检故障",
    Conflict: "冲突", Offline: "离线", "Camera fault": "相机异常", Maintenance: "维护中",
  };
  return labels[meta.alertReason] ?? meta.alertReason;
}

const powerActionLabels: [RobotPowerAction, string, string][] = [
  ["power_on", "上电", "Power on"], ["power_off", "下电", "Power off"],
  ["driver_on", "启驱动", "Start driver"], ["driver_off", "关驱动", "Stop driver"],
];

function uptime(boot: string | undefined, now: number) {
  const started = Date.parse(boot ?? "");
  if (!Number.isFinite(started)) return "—";
  const minutes = Math.max(0, Math.floor((now - started) / 60000));
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  return hours < 24 ? `${hours}h ${minutes % 60}m` : `${Math.floor(hours / 24)}d ${hours % 24}h`;
}

export function StationsTableView({ items, metaByKey, nodesByKey, chosen, now, zh, busy, restartBusy, recheckKey, onChoose, onPower, onPreview, onDetails, onRecheck, onRejoin }: {
  items: FleetStation[];
  metaByKey: ReadonlyMap<string, StationFleetMeta>;
  nodesByKey: Record<string, CRDNode>;
  chosen: ReadonlySet<string>;
  now: number;
  zh: boolean;
  busy: boolean;
  restartBusy: (item: FleetStation) => boolean;
  recheckKey: string;
  onChoose: (item: FleetStation, checked: boolean) => void;
  onPower: (item: FleetStation, action: RobotPowerAction, label: string) => void;
  onPreview: (item: FleetStation, trigger: HTMLElement) => void;
  onDetails: (item: FleetStation) => void;
  onRecheck: (item: FleetStation) => void;
  onRejoin: (item: FleetStation) => void;
}) {
  const viewport = useRef<HTMLDivElement>(null);
  const [height, setHeight] = useState(650);
  const [scrollTop, setScrollTop] = useState(0);

  useLayoutEffect(() => {
    const element = viewport.current;
    if (!element) return;
    const measure = () => {
      setHeight(element.clientHeight);
      setScrollTop(element.scrollTop);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const boundedTop = Math.min(scrollTop, Math.max(0, items.length * ROW_HEIGHT - height));
  const { startIndex, endIndex, offsetTop, totalHeight } = computeVirtualWindow({
    totalCount: items.length, scrollTop: boundedTop, viewportHeight: height, rowHeight: ROW_HEIGHT, overscan: 8,
  });

  const columns = zh
    ? ["", "节点 / 集群", "型号 / 区域", "生命周期 / 告警", "电源 / 驱动", "相机", "运行 / 上报 / 重启", "快捷操作"]
    : ["", "Node / Cluster", "Model / Zone", "Lifecycle / Alert", "Power / Driver", "Camera", "Uptime / Report / Reboots", "Quick actions"];

  return <div className="stations-fleet-table" role="table" aria-label={zh ? "场站列表" : "Station list"} aria-rowcount={items.length + 1}>
    <div className="stations-table-scroll" ref={viewport} tabIndex={0} onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}>
      <div className="stations-table-head stations-table-columns" role="row" aria-rowindex={1}>
        {columns.map((column, index) => <span key={index} role="columnheader">{column}</span>)}
      </div>
      <div className="stations-table-spacer" style={{ height: totalHeight }} role="rowgroup">
        <div style={{ transform: `translateY(${offsetTop}px)` }} role="presentation">
          {items.slice(startIndex, endIndex).map((item, index) => {
            const key = fleetKey(item);
            const meta = metaByKey.get(key)!;
            const phase = stationPhase(item);
            const robot = isRobotNode(item.labels) && phase !== "Retired";
            const power = robotPowerState(item.labels);
            const powerTone = meta.powerBucket === "running" ? "ready" : meta.powerBucket;
            const powerText = {
              ready: zh ? "驱动就绪" : "Driver ready", powered: zh ? "已上电" : "Powered",
              off: zh ? "未上电" : "Off", estop: zh ? "急停" : "E-stop",
              fault: zh ? "硬件报错" : "Hardware fault", unknown: zh ? "未知" : "Unknown",
            };
            const lastSeen = meta.heartbeatMs ? new Date(meta.heartbeatMs) : undefined;
            const alertLabel = fleetAlertLabel(meta, zh);
            const cameraKnown = Boolean(item.site?.cameras || item.annotations?.["rlark.io/cameras"]);
            const preview = (event: MouseEvent<HTMLButtonElement>) => onPreview(item, event.currentTarget);
            return <div key={key} role="row" aria-rowindex={startIndex + index + 2} className="stations-table-row stations-table-columns" data-severity={meta.alertSeverity}>
              <span role="cell">{robot && <input type="checkbox" checked={chosen.has(key)} aria-label={zh ? `选中 ${item.metadata.name}` : `Select ${item.metadata.name}`} onChange={(event) => onChoose(item, event.target.checked)} />}</span>
              <span role="cell"><strong><NodeName node={nodesByKey[key] ?? { metadata: item.metadata }} /></strong><small>{item.clusterName || item.metadata.namespace}</small></span>
              <span role="cell"><strong>{meta.model || "—"}</strong><small>{meta.zone || "—"}</small></span>
              <span role="cell"><span className={`status ${stationStatusClass(phase)}`}><i />{stationPhaseLabel(phase, zh)}</span>{alertLabel && <small className="stations-table-alert" title={alertLabel}>{alertLabel}</small>}</span>
              <span role="cell">{robot ? <span className={`station-tile-pill is-actual is-${powerTone}`}>{power ? powerText[power] : (zh ? "未上报" : "No report")}</span> : "—"}</span>
              <span role="cell">{robot ? <button type="button" className={`stations-camera-health${meta.cameraFault ? " is-fault" : ""}`} aria-haspopup="dialog" onClick={preview}>
                <Camera size={14} />{meta.cameraFault ? (zh ? "异常" : "Fault") : cameraKnown ? (zh ? "预览" : "Preview") : (zh ? "未上报" : "No report")}
              </button> : "—"}</span>
              <span role="cell" className="stations-table-times"><strong>{uptime(item.status?.node?.bootTime, now)} · {meta.restarts} {zh ? "次" : "reboots"}</strong><small title={lastSeen?.toLocaleString()}>{lastSeen?.toLocaleTimeString(zh ? "zh-CN" : "en-GB") ?? "—"}</small></span>
              <span role="cell" className="stations-table-actions">
                {robot && phase === "Bound" && powerActionLabels.map(([action, chinese, english]) => {
                  const blocked = robotPowerActionBlocked(action, power, zh);
                  const label = zh ? chinese : english;
                  return <button key={action} type="button" disabled={busy || restartBusy(item) || Boolean(blocked)} title={blocked || label} onClick={() => onPower(item, action, label)}>{label}</button>;
                })}
                {phase === "Offline" && <>
                  <button type="button" disabled={recheckKey === key} onClick={() => onRecheck(item)}>{zh ? "重新检测" : "Re-check"}</button>
                  <button type="button" onClick={() => onRejoin(item)}>{zh ? "重新接入" : "Rejoin"}</button>
                </>}
                <button id={`station-expand-${item.metadata.namespace}-${item.metadata.name}`} type="button" className="stations-table-details" aria-haspopup="dialog" onClick={() => onDetails(item)}>{zh ? "详情" : "Details"}</button>
              </span>
            </div>;
          })}
        </div>
      </div>
    </div>
  </div>;
}
