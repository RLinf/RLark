import { memo, useEffect, useRef, useState, type KeyboardEvent, type ReactNode, type RefObject } from "react";
import { Camera, ChevronDown } from "lucide-react";
import { request } from "../../api";
import type { StationItem } from "../api";
import type { ClusterSummary } from "../../types";
import { NodeName, type NodeRef } from "../components/NodeName";
import { RobotPoseButton } from "../components/RobotControls";
import type { RobotPowerRequest } from "../components/useRobotPower";
import { cameraMediaScheduler } from "../lib/cameraMediaScheduler";
import { useCameraClock } from "../components/useCameraClock";
import {
  CAMERA_FPS_OPTIONS, DEFAULT_CAMERA_FPS, cameraCells, cameraFramePath, cameraStreamPath, formatCameraMilliseconds,
  isCameraFps, publishedCameras, readCameraAge,
  type CameraCell, type CameraFps, type CameraView,
} from "../lib/cameras";
import { readCameraFrames, sameCameraFrame } from "../lib/cameraStream";
import { stationFacts } from "../lib/stationDrawer";
import { shortUID, stationNetworkState, stationPhase, stationPhaseLabel, stationRecoveryHint, stationStatusClass } from "../lib/stationDisplay";
import {
  outstandingRestartText, restartProgressText,
  type NodeRestartObservation, type NodeRestartOutstanding, type NodeRestartProgress,
} from "../lib/nodeRestart";
import {
  admissionStateClass, admissionStateLabel, machineModel, robotMaintenanceLabel, robotMaintenanceState,
  robotPowerActionBlocked, robotPowerActionTiming, robotPowerActionWait, robotPowerDesired, robotPowerState,
  robotPowerStateNote, robotSelfCheckLabel,
  robotSelfCheckState, type NodeAdmission, type RobotPowerAction, type SiteNode,
} from "../lib/siteNodeDisplay";
import { NetworkBadge } from "./StationFields";

// A robot's cameras as the tile's media surface: the main view streams at the card's preview rate, and the strip
// of smaller views refreshes a still frame every five seconds. A frame click enlarges that camera and never
// expands the card. The card stops its stream while it is off screen, the tab is hidden, the robot is offline or
// the detail/zoom dialog is open, which then owns the only stream.
type Snapshot = { url: string; age?: number; fetchMs: number; receivedAt: number };
// The newest streamed frame: its capture time, the camera's reported age and the time the response arrived.
type LiveFrame = { capturedAtMs?: number; ageSeconds?: number; fetchMs?: number; receivedAt: number };

export function CameraWall({ cells, node, namespace, zh, online, expanded = false, fps, measuredFps, onZoom, overlayTop, liveEnabled = true, monitor = false }: {
  cells: CameraCell[];
  node: string;
  namespace: string;
  zh: boolean;
  online: boolean;
  expanded?: boolean;
  liveEnabled?: boolean;
  monitor?: boolean;
  fps: CameraFps;
  measuredFps: RefObject<HTMLSpanElement | null>;
  onZoom: (view: CameraView, trigger: HTMLElement, fps: CameraFps) => void;
  overlayTop?: (metrics: RefObject<HTMLSpanElement | null>) => ReactNode;
}) {
  const wall = useRef<HTMLDivElement>(null);
  const bigImage = useRef<HTMLImageElement>(null);
  const metrics = useRef<HTMLSpanElement>(null);
  const live = useRef<LiveFrame | undefined>(undefined);
  const [visible, setVisible] = useState(false);
  const [hasLiveFrame, setHasLiveFrame] = useState(false);
  const [frames, setFrames] = useState<Partial<Record<CameraView, Snapshot>>>({});
  const [big, ...small] = cells;
  // Only a camera that has sent a frame can be streamed; the detail and zoom dialogs take over the picture.
  const streamView = big?.frames && big.present !== false ? big.view : undefined;
  const [foreground, setForeground] = useState(!document.hidden);
  const [liveGranted, setLiveGranted] = useState(false);
  const leaseID = `${monitor ? "wall" : "card"}:${namespace}/${node}`;
  const previewActive = visible && foreground && !expanded && online;
  const wantsLive = previewActive && liveEnabled && streamView !== undefined;
  const streaming = wantsLive && liveGranted;
  const cameraClock = useCameraClock(node, namespace, streaming);
  const stripKey = monitor ? "" : small.filter((cell) => cell.frames).map((cell) => cell.view).join(",");
  const snapshotKey = [!streaming && streamView, stripKey].filter(Boolean).join(",");

  useEffect(() => {
    const visibility = () => setForeground(!document.hidden);
    document.addEventListener("visibilitychange", visibility);
    return () => document.removeEventListener("visibilitychange", visibility);
  }, []);

  useEffect(() => {
    if (!wantsLive) return;
    const release = cameraMediaScheduler.acquire(leaseID, "card", setLiveGranted);
    return release;
  }, [leaseID, wantsLive]);

  const pin = () => {
    for (const activeID of cameraMediaScheduler.getStats().activeIds) {
      if (activeID !== leaseID && activeID.startsWith("card:")) cameraMediaScheduler.promote(activeID, "card");
    }
    cameraMediaScheduler.promote(leaseID, "pinned");
  };

  useEffect(() => {
    const element = wall.current;
    if (!element) return;
    if (typeof IntersectionObserver === "undefined") {
      setVisible(true);
      return;
    }
    const observer = new IntersectionObserver((entries) => setVisible(entries.some((entry) => entry.isIntersecting)), { rootMargin: "0px" });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  // The strip stays a slow still preview: one frame per view every five seconds, paused while the tab is hidden
  // and torn down (timers, in-flight requests, object URLs) when the card stops.
  useEffect(() => {
    if (!previewActive || !snapshotKey) return;
    const snapshotViews = snapshotKey.split(",") as CameraView[];
    let inFlight: AbortController | undefined;
    const shots: Partial<Record<CameraView, Snapshot>> = {};
    setFrames({});
    let timer = 0;
    let stopped = false;
    const schedule = () => {
      timer = window.setTimeout(() => void ask(), cameraMediaScheduler.nextSnapshotDelayMs());
    };
    const ask = async () => {
      if (inFlight || document.hidden) return;
      const batch = new AbortController();
      inFlight = batch;
      try {
        for (const view of snapshotViews) {
          if (batch.signal.aborted || document.hidden) return;
          const started = performance.now();
          try {
            const signal = AbortSignal.any([batch.signal, AbortSignal.timeout(4000)]);
            const response = await request(cameraFramePath(node, namespace, view), { signal });
            const blob = await response.blob();
            if (batch.signal.aborted) return;
            const receivedAt = performance.now();
            const previous = shots[view];
            if (previous) URL.revokeObjectURL(previous.url);
            shots[view] = {
              url: URL.createObjectURL(blob),
              age: readCameraAge(response.headers.get("X-Age-Seconds")),
              fetchMs: receivedAt - started,
              receivedAt,
            };
            setFrames({ ...shots });
          } catch {
            // Keep the last picture until the next successful fetch.
          }
        }
      } finally {
        inFlight = undefined;
        if (!stopped && !document.hidden) schedule();
      }
    };
    timer = window.setTimeout(() => void ask(), cameraMediaScheduler.nextSnapshotDelayMs(1800, 300));
    return () => {
      stopped = true;
      inFlight?.abort();
      window.clearTimeout(timer);
      Object.values(shots).forEach((shot) => { if (shot) URL.revokeObjectURL(shot.url); });
    };
  }, [previewActive, node, namespace, snapshotKey]);

  // The main camera streams at the card's rate. Frames are painted straight onto the <img> and the frame-age /
  // fetch-time / measured-rate lines straight onto their DOM nodes, throttled by requestAnimationFrame for the
  // first two and by the one-second tick for the rate, so a 30 FPS preview never re-renders React. Changing the
  // rate aborts the old stream before opening the new one, so there is never more than one subscription; hiding
  // the tab closes it and coming back reopens exactly one.
  useEffect(() => {
    const paint = () => {
      const target = metrics.current;
      if (!target) return;
      const shot = live.current;
      if (!shot) { target.textContent = ""; return; }
      const at = performance.now();
      const age = shot.ageSeconds !== undefined
        ? shot.ageSeconds * 1000
        : shot.capturedAtMs !== undefined && cameraClock.current
          ? Math.max(0, at + cameraClock.current.offsetMs - shot.capturedAtMs)
          : undefined;
      // A frame kept for more than ten seconds is stale even if the service reported a smaller age for it.
      const stale = at - shot.receivedAt >= 10000 || (age !== undefined && age >= 5000);
      target.textContent = [
        big ? `${big.name} · ${zh ? "帧龄" : "Frame age"} ${formatCameraMilliseconds(age)}` : "",
        `${zh ? "取图" : "Fetch"} ${formatCameraMilliseconds(shot.fetchMs)}`,
        stale && (zh ? "画面已过期" : "Stale frame"),
      ].filter(Boolean).join(" · ");
    };
    // The measured rate counts distinct JPEGs over about one second, so it follows what the browser really
    // painted rather than the requested rate. A dash means nothing was measured: no stream yet, a broken stream,
    // a hidden tab, or a still frame standing in for the stream.
    let distinct = 0;
    let measuredAt = 0;
    const showMeasuredFps = (rate: number | null) => {
      if (measuredFps.current) measuredFps.current.textContent = rate === null ? "—" : `${zh ? "实测" : "Measured"} ${rate.toFixed(1)} FPS`;
    };
    const clear = () => {
      live.current = undefined;
      distinct = 0;
      measuredAt = 0;
      showMeasuredFps(null);
      if (metrics.current) metrics.current.textContent = "";
    };
    setHasLiveFrame(false);
    if (!streaming || !streamView) {
      clear();
      return;
    }
    // The <img> is stable for the life of this effect; capture it so cleanup does not read a changed ref.
    const image = bigImage.current;
    let connection: AbortController | undefined;
    let still: AbortController | undefined;
    let retry = 0;
    let url = "";
    let previousUrl = "";
    let hasFrame = false;
    let painting = 0;
    const schedulePaint = () => {
      if (painting) return;
      painting = window.requestAnimationFrame(() => { painting = 0; paint(); });
    };

    const connect = async () => {
      if (document.hidden || connection) return;
      const controller = new AbortController();
      connection = controller;
      let watchdog = 0;
      let lastFrame: Uint8Array | undefined;
      let firstFrameFetch: number | undefined;
      const started = performance.now();
      const resetWatchdog = () => {
        window.clearTimeout(watchdog);
        watchdog = window.setTimeout(() => controller.abort(), 8000);
      };
      resetWatchdog();
      try {
        const response = await request(cameraStreamPath(node, namespace, streamView, fps), { signal: controller.signal });
        if (connection !== controller) return;
        distinct = 0;
        measuredAt = performance.now();
        for await (const { jpeg, capturedAtMs, ageSeconds } of readCameraFrames(response)) {
          if (connection !== controller || controller.signal.aborted) break;
          resetWatchdog();
          if (sameCameraFrame(lastFrame, jpeg)) continue;
          lastFrame = jpeg;
          distinct++;
          const receivedAt = performance.now();
          const next = URL.createObjectURL(new Blob([jpeg], { type: "image/jpeg" }));
          if (image) image.src = next;
          if (previousUrl) URL.revokeObjectURL(previousUrl);
          previousUrl = url;
          url = next;
          // Fetch time runs from the request to the first complete JPEG, not merely to the response headers.
          firstFrameFetch ??= receivedAt - started;
          live.current = { capturedAtMs, ageSeconds, fetchMs: firstFrameFetch, receivedAt };
          if (!hasFrame) { hasFrame = true; setHasLiveFrame(true); }
          schedulePaint();
        }
      } catch {
        // A disconnected stream is retried below while the card is on screen.
      } finally {
        window.clearTimeout(watchdog);
        controller.abort();
        if (connection === controller) {
          connection = undefined;
          distinct = 0;
          measuredAt = 0;
          showMeasuredFps(null);
          // Keep the last frame's record so the metrics line can turn stale if the picture stays for ten seconds.
          schedulePaint();
          // A gateway that cannot stream still has its still-frame endpoint: show one instead of a blank card,
          // and retry the stream less often while the still is what the card shows.
          if (!document.hidden) {
            if (!hasFrame) void fetchStill();
            retry = window.setTimeout(() => void connect(), hasFrame ? 1500 : 5000);
          }
        }
      }
    };
    const fetchStill = async () => {
      if (document.hidden || still || connection) return;
      const controller = new AbortController();
      still = controller;
      const started = performance.now();
      try {
        const response = await request(cameraFramePath(node, namespace, streamView), { signal: AbortSignal.any([controller.signal, AbortSignal.timeout(4000)]) });
        const blob = await response.blob();
        if (controller.signal.aborted) return;
        const receivedAt = performance.now();
        const next = URL.createObjectURL(blob);
        if (image) image.src = next;
        if (previousUrl) URL.revokeObjectURL(previousUrl);
        previousUrl = url;
        url = next;
        live.current = { ageSeconds: readCameraAge(response.headers.get("X-Age-Seconds")), fetchMs: receivedAt - started, receivedAt };
        setHasLiveFrame(true);
        paint();
      } catch {
        // The stream keeps retrying; a still frame is only the fallback when it cannot.
      } finally {
        if (still === controller) still = undefined;
      }
    };
    const disconnect = () => {
      window.clearTimeout(retry);
      const active = connection;
      connection = undefined;
      active?.abort();
      const pendingStill = still;
      still = undefined;
      pendingStill?.abort();
      distinct = 0;
      measuredAt = 0;
      showMeasuredFps(null);
    };
    const visibility = () => {
      if (document.hidden) {
        disconnect();
        live.current = undefined;
        paint();
      } else {
        void connect();
      }
    };
    document.addEventListener("visibilitychange", visibility);
    // One timer paints the frame-age line and closes the measured-rate window, so the rate is at most one
    // DOM write per second and never one per frame.
    const tick = window.setInterval(() => {
      if (document.hidden) return;
      paint();
      if (!measuredAt) return;
      const now = performance.now();
      showMeasuredFps(distinct * 1000 / (now - measuredAt));
      distinct = 0;
      measuredAt = now;
    }, 1000);
    showMeasuredFps(null);
    // Give the revoked stream's abort time to reach the HTTP peer before handoff.
    retry = window.setTimeout(() => void connect(), 100);
    return () => {
      document.removeEventListener("visibilitychange", visibility);
      disconnect();
      window.clearInterval(tick);
      window.cancelAnimationFrame(painting);
      if (url) URL.revokeObjectURL(url);
      if (previousUrl) URL.revokeObjectURL(previousUrl);
      if (image) image.removeAttribute("src");
      clear();
    };
    // The stream follows the card, the machine and the chosen rate; changing any of them aborts the old stream.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [streaming, node, namespace, streamView, fps, zh]);

  const bigTitle = big ? [big.badge, big.note, big.advice].filter(Boolean).join(" · ") : undefined;
  const metricsFor = (cell: CameraCell) => {
    if (cell.present === false) return "";
    const shot = frames[cell.view];
    const at = performance.now();
    const stale = shot && ((shot.age ?? 0) >= 5 || at - shot.receivedAt >= 10000);
    const age = formatCameraMilliseconds(shot?.age === undefined ? undefined : shot.age * 1000);
    return [
      `${zh ? "帧龄" : "Frame age"} ${age}`,
      `${zh ? "取图" : "Fetch"} ${formatCameraMilliseconds(shot?.fetchMs)}`,
      stale && (zh ? "画面已过期" : "Stale frame"),
    ].filter(Boolean).join(" · ");
  };
  return <div ref={wall} className="station-tile-media" role="group" aria-label={zh ? "相机" : "Cameras"}>
    <div className="station-tile-main">
      {big ? <button type="button" className={`station-camera is-big${big.present === false ? " is-missing" : ""}`}
        onClick={(event) => onZoom(big.view, event.currentTarget, fps)} aria-haspopup="dialog"
        aria-label={`${big.name} · ${big.badge}`} title={bigTitle}>
        <span className={`station-camera-frame is-${big.tone}`}>
          {big.present === false
            ? <span aria-hidden="true">—</span>
            : <>
              {streaming && hasLiveFrame ? null : frames[big.view] ? <img src={frames[big.view]?.url} alt="" /> : <Camera size={22} aria-hidden="true" />}
              <img ref={bigImage} alt="" hidden={!streaming || !hasLiveFrame} />
            </>}
        </span>
      </button> : <div className="station-camera is-big is-empty" aria-hidden="true">
        <span className="station-camera-frame"><Camera size={22} aria-hidden="true" /></span>
      </div>}
      {overlayTop?.(metrics)}
      {wantsLive && !liveGranted && <button type="button" className="station-camera-pin" onClick={pin}>
        {zh ? "抽帧预览 · 点击开流" : "Snapshots · Click for live"}
      </button>}
      {!liveEnabled && previewActive && <span className="station-camera-pin">{zh ? "抽帧预览" : "Snapshots"}</span>}
    </div>
    {!monitor && small.length > 0 && <div className="station-tile-strip" role="group" aria-label={zh ? "其余相机" : "Other cameras"}>
      {small.map((cell) => <button key={cell.view} type="button" className={`station-camera${cell.present === false ? " is-missing" : ""}`}
        onClick={(event) => onZoom(cell.view, event.currentTarget, fps)} aria-haspopup="dialog"
        aria-label={`${cell.name} · ${cell.badge}`} title={[cell.badge, metricsFor(cell), cell.note, cell.advice].filter(Boolean).join(" · ")}>
        <span className={`station-camera-frame is-${cell.tone}`}>{frames[cell.view] ? <img src={frames[cell.view]?.url} alt="" loading="lazy" /> : cell.present === false ? <span aria-hidden="true">—</span> : <Camera size={16} aria-hidden="true" />}</span>
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

// What a just-sent power request reads on its card: the command on its way, then the Gateway's acceptance, then
// what the robot itself reports within the action's bounded window.
const powerRequestText = (request: RobotPowerRequest, zh: boolean): { text: string; title: string } => {
  const timing = request.action ? robotPowerActionTiming(request.action, zh) : "";
  const wait = request.action ? robotPowerActionWait(request.action, zh) : "";
  if (request.phase === "sending") {
    return { text: zh ? "请求中…" : "Sending…", title: zh ? "命令正在发送" : "The command is on its way" };
  }
  if (request.phase === "accepted") {
    return {
      text: [zh ? "已受理" : "Accepted", request.waiting || timing].filter(Boolean).join(" · "),
      title: [zh ? "Gateway 已受理，机器人还没上报到目标状态" : "Accepted by the Gateway; the robot has not reported the target state yet", wait, request.waiting && `${zh ? "电源服务" : "Power service"}: ${request.waiting}`].filter(Boolean).join(" · "),
    };
  }
  if (request.phase === "observed") {
    return { text: zh ? "已生效" : "Applied", title: zh ? "机器人已上报目标状态" : "The robot reports the target state" };
  }
  if (request.phase === "unconfirmed") {
    return {
      text: zh ? "已受理，未观测到" : "Accepted, not observed",
      title: [zh ? "限时内没有观测到目标状态" : "The target state was not observed in time", request.waiting ? `${zh ? "电源服务" : "Power service"}: ${request.waiting}` : (zh ? "原因见节点注解 rlark.io/power 的 waiting" : "See the Node annotation rlark.io/power (waiting)")].join(" · "),
    };
  }
  return { text: zh ? "失败" : "Failed", title: request.error || (zh ? "命令失败" : "The command failed") };
};

export type StationCardVariant = "robot" | "node" | "control";

// The recovery entry of a station the plane does not see as Bound: the page's normal load re-reads the node and the
// site network, and says where a browser-side check stops.
export interface StationRecheck {
  pending: boolean;
  run: () => void;
}

// The administrator's whole-machine restart: a one-shot Node label the robot's tuner acts on once. The card shows the
// records it read from the Node and the progress the page is following, and leaves the tuner's own guards in charge.
export interface StationRestart {
  available: boolean;
  pending: boolean;
  blocked?: string;
  request?: NodeRestartProgress;
  outstanding?: NodeRestartOutstanding;
  observation?: NodeRestartObservation;
  start: () => void;
}

interface StationCardProps {
  station: StationItem;
  cluster?: ClusterSummary;
  node: NodeRef;
  siteNode?: SiteNode;
  cameraReport?: string;
  admission?: NodeAdmission;
  labels?: Record<string, string>;
  siteDataState: "loading" | "ready" | "unavailable";
  now: number;
  zh: boolean;
  expanded: boolean;
  chosen: boolean;
  power: { pending: boolean; press: (action: RobotPowerAction, label: string) => void; request?: RobotPowerRequest };
  restart?: StationRestart;
  recheck: StationRecheck;
  // Opens the join page at this node; the operator submits there, and no record is changed from the card.
  rejoin: () => void;
  variant: StationCardVariant;
  liveEnabled: boolean;
  onToggle: () => void;
  onChoose: (checked: boolean) => void;
  onZoom: (view: CameraView, trigger: HTMLElement, fps: CameraFps) => void;
}

export const StationCard = memo(function StationCard({
  station, cluster, node, siteNode, admission, labels, siteDataState, now, zh, expanded, chosen, power, restart, recheck, rejoin, variant, liveEnabled,
  onToggle, onChoose, onZoom, cameraReport,
}: StationCardProps) {
  const phase = stationPhase(station);
  const retired = phase === "Retired";
  const robot = variant === "robot";
  const control = variant === "control";
  const facts = stationFacts(station, cluster, now, zh);
  const powerNow = robotPowerState(labels);
  const powerDesired = robotPowerDesired(labels);
  const selfCheck = robotSelfCheckState(labels);
  const maintenance = robotMaintenanceState(labels);
  // The tile always carries both power actions, both driver actions and the home hold, grouped by function. What the
  // current state cannot act on stays visible but disabled, with the reason in its tooltip and on the note line; the
  // robot's power service still has the final say over any command that is sent.
  const actionGroups: { key: "power" | "driver"; label: string; actions: { action: RobotPowerAction; label: string }[] }[] = [
    { key: "power", label: zh ? "电源" : "Power", actions: [
      { action: "power_on", label: zh ? "上电" : "Power on" },
      { action: "power_off", label: zh ? "下电" : "Power off" },
    ] },
    { key: "driver", label: zh ? "驱动" : "Driver", actions: [
      { action: "driver_on", label: zh ? "启动驱动" : "Start driver" },
      { action: "driver_off", label: zh ? "关闭驱动" : "Stop driver" },
    ] },
  ];
  const stateNote = robotPowerStateNote(powerNow, zh);
  const requestView = power.request ? powerRequestText(power.request, zh) : undefined;
  const detailsID = `station-details-${station.metadata.namespace}-${station.metadata.name}`;
  const nameID = `${detailsID}-card-name`;
  const expanderID = `station-expand-${station.metadata.namespace}-${station.metadata.name}`;
  const cameraList = robot ? cameraCells(publishedCameras(siteNode?.cameras ?? cameraReport), zh, siteDataState !== "unavailable") : [];
  // The card's own preview rate; the lowest by default so a wall of robots does not stream at high rates.
  const [previewFps, setPreviewFps] = useState<CameraFps>(DEFAULT_CAMERA_FPS);
  // The measured rate is painted straight into this node by the stream effect, like the frame-age line.
  const measuredFps = useRef<HTMLSpanElement>(null);
  const online = phase === "Bound";
  // Only a station the plane does not see as Bound gets the recovery line; the facts above it already carry the last
  // report, and the tags carry the tunnel's state.
  const recoveryHint = !online && !retired ? stationRecoveryHint(station, now, zh) : "";
  // A request this page follows wins over an older outstanding one; otherwise the Node's own record is shown so an
  // untracked request is visible to everyone who opens the tile.
  const restartView = restart?.request
    ? restartProgressText(restart.request, zh, restart.observation, now)
    : restart?.outstanding
      ? outstandingRestartText(restart.outstanding, zh)
      : undefined;
  const restartActive = Boolean(restart?.request && restart.request.phase !== "done" && restart.request.phase !== "failed");
  const restartBlocked = restart?.blocked
    || (restartActive ? (zh ? "这次重启还没有结束，等它结束或失败后再提交新的请求" : "This restart has not finished; wait for it to end or fail before asking again") : "");
  const restartPhase = restart?.request?.phase ?? restart?.outstanding?.phase ?? "submitted";
  const restartAlert = restart?.request?.phase === "failed";

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
    const overlayTop = (cameraMetrics: RefObject<HTMLSpanElement | null>) => <>
      <div className="station-tile-overlay is-top">
        <div className="station-tile-top">
          <input type="checkbox" className="station-card-check" aria-label={zh ? `选中 ${station.metadata.name}` : `Select ${station.metadata.name}`} checked={chosen} onChange={(event) => onChoose(event.target.checked)} />
          <div className="station-tile-name">
            <div className="station-tile-title">
              <h3 id={nameID}><NodeName node={node} /></h3>
              <small ref={cameraMetrics} className="station-tile-camera-metrics" title={zh ? "主相机名称、帧龄与取图耗时。帧龄取自相机服务上报或采集时间戳对时，取图耗时由浏览器测量。" : "Main camera name, frame age and fetch time. Frame age comes from the camera service or its capture stamp; fetch time is measured in the browser."} />
            </div>
            <small>{machineModel(labels) || siteNode?.declared_type || siteNode?.observed_type || "—"} · <code className="inline-code">UID {shortUID(station.spec.nodeUID)}</code></small>
          </div>
          <label className="station-tile-fps" title={zh ? "该卡主相机预览帧率；右侧实测值由浏览器按约一秒内收到的不同画面统计，断流或未测得时显示「—」" : "Preview rate for this card's main camera; the measured rate counts different frames the browser received in about a second, and reads — until measured or while the stream is down"}>
            <select value={previewFps} aria-label={zh ? "主相机预览帧率" : "Main camera preview frame rate"}
              onChange={(event) => { const rate = Number(event.target.value); if (isCameraFps(rate)) setPreviewFps(rate); }}>
              {CAMERA_FPS_OPTIONS.map((rate) => <option key={rate} value={rate}>{rate} FPS</option>)}
            </select>
            <small ref={measuredFps} className="station-tile-measured-fps" />
          </label>
          <span className={`station-tile-phase status ${stationStatusClass(phase)}`}><i />{stationPhaseLabel(phase, zh)}</span>
          <button id={expanderID} type="button" className="station-tile-expand" aria-expanded={expanded} aria-haspopup="dialog" aria-controls={detailsID} aria-label={expanded ? (zh ? "收起详情" : "Collapse details") : (zh ? "展开详情" : "Expand details")} title={expanded ? (zh ? "收起详情" : "Collapse details") : (zh ? "展开详情" : "Expand details")} onClick={onToggle}>
            <span className="station-tile-expand-label">{expanded ? (zh ? "收起" : "Hide") : (zh ? "详情" : "Details")}</span>
            <ChevronDown size={13} className={expanded ? "is-open" : ""} aria-hidden="true" />
          </button>
        </div>
        <div className="station-tile-tags">
          {admission ? <span className={`station-tile-tag is-${admissionStateClass(admission.state)}`} title={admission.reason}>{admissionStateLabel(admission.state, zh)}</span> : null}
          {selfCheck && <span className={`station-tile-tag is-${selfCheck === "failed" ? "alert" : selfCheck === "pending" ? "warning" : "good"}`}>{robotSelfCheckLabel(selfCheck, zh)}</span>}
          {maintenance && <span className="station-tile-tag is-warning" title={robotMaintenanceLabel(maintenance, zh)}>{zh ? "维护中" : "Maintenance"}</span>}
          {siteNode?.identity_verified === "False" && <span className="station-tile-tag is-alert" title={siteNode?.identity_reason}>{zh ? "身份不匹配" : "identity mismatch"}</span>}
          <NetworkBadge state={stationNetworkState(station)} zh={zh} />
        </div>
      </div>
    </>;
    // The state and hardware controls sit in a dock under the picture rather than on it, so nothing covers the
    // camera: a quiet status line (desired, then the robot's own actual state, then the request or restart chip)
    // above the functional control groups.
    const powerTone = powerNow === "fault" || powerNow === "estop" ? "alert"
      : powerNow === "ready" ? "ready"
        : powerNow === "powered" ? "powered"
          : powerNow === "off" ? "off"
            : powerNow === "unknown" ? "unknown" : "none";
    const dock = <footer className="station-tile-dock">
      <div className="station-tile-dock-status" role="group" aria-label={zh ? "运行状态" : "Run state"}>
        <span className="station-tile-pill is-desired">
          <small>{zh ? "期望" : "Desired"}</small>
          <strong>{cardPowerDesired(powerDesired, zh)}</strong>
        </span>
        <span className={`station-tile-pill is-actual is-${powerTone}`}>
          <small>{zh ? "实际" : "Actual"}</small>
          {powerNow
            ? <strong className={powerNow === "off" || powerNow === "unknown" ? "is-muted" : ""}>{cardPowerActual(powerNow, zh)}</strong>
            : <strong className="is-muted">{zh ? "未上报" : "No report"}</strong>}
        </span>
        {restartView && <small className={`station-tile-restart is-${restartPhase}`} role={restartAlert ? "alert" : "status"} title={restartView.title}>{restartView.text}</small>}
        {requestView && <small className={`station-tile-request is-${power.request?.phase}`} role={power.request?.phase === "failed" ? "alert" : "status"} title={requestView.title}>{requestView.text}</small>}
      </div>
      <div className="station-tile-actions">
        <div className="station-tile-action-groups">
          {actionGroups.map((group) => <div key={group.key} className={`station-tile-action-group is-${group.key}`} role="group" aria-label={group.label}>
            <small className="station-tile-action-group-label" aria-hidden="true">{group.label}</small>
            {group.actions.map(({ action, label }) => {
              const blocked = robotPowerActionBlocked(action, powerNow, zh);
              return <button key={action} type="button" className="station-tile-action" disabled={power.pending || restart?.pending || restartActive || Boolean(blocked)}
                title={blocked || robotPowerActionWait(action, zh)} onClick={() => power.press(action, label)}>{label}</button>;
            })}
          </div>)}
          <div className="station-tile-action-group is-pose" role="group" aria-label={zh ? "回位" : "Home"}>
            <small className="station-tile-action-group-label" aria-hidden="true">{zh ? "回位" : "Home"}</small>
            <RobotPoseButton robots={[{ key: `${station.metadata.namespace}/${station.metadata.name}`, node: station.metadata.name, namespace: station.metadata.namespace, labels }]} zh={zh} />
          </div>
          {restart?.available && <div className={`station-tile-action-group is-restart${restartBlocked ? " is-blocked" : ""}`} role="group" aria-label={zh ? "整机" : "Machine"}>
            <small className="station-tile-action-group-label" aria-hidden="true">{zh ? "整机" : "Machine"}</small>
            <button type="button" className="station-tile-action is-restart" disabled={power.pending || restart.pending || Boolean(restartBlocked)}
              title={restartBlocked || (zh ? "写入一次性 Node 标签 rlark.io/restart-request：robot tuner 按节点配置、占用与急停条件执行一次，结果写进 rlark.io/restart-request-state" : "Writes the one-shot Node label rlark.io/restart-request: the robot tuner runs it once under its node-profile, occupancy and emergency-stop guards and records the outcome in rlark.io/restart-request-state")}
              onClick={restart.start}>{restart.pending ? (zh ? "提交中…" : "Requesting…") : (zh ? "整机重启" : "Restart")}</button>
          </div>}
        </div>
        {stateNote && <small className="station-tile-action-note">{stateNote}</small>}
      </div>
    </footer>;
    return <article className={`station-tile${expanded ? " is-expanded" : ""}${chosen ? " is-selected" : ""}${retired ? " is-retired" : ""}${tileAlert ? " is-alert" : ""}`} aria-labelledby={nameID} onKeyDown={onKeyDown}>
      <CameraWall cells={cameraList} node={station.metadata.name} namespace={station.metadata.namespace} zh={zh} online={online} expanded={expanded} liveEnabled={liveEnabled} fps={previewFps} measuredFps={measuredFps} onZoom={onZoom} overlayTop={overlayTop} />
      {dock}
      <dl className="station-tile-facts">{facts.slice(0, 3).map((fact) => <div key={fact.key}><dt>{fact.label}</dt><dd>{fact.value}</dd></div>)}</dl>
      {recoveryHint && <div className="station-tile-recovery" role="note">
        <small className="station-tile-recovery-note">{recoveryHint}</small>
        <div className="station-tile-recovery-actions">
          <button type="button" className="station-tile-recheck" disabled={recheck.pending} onClick={recheck.run}>{recheck.pending ? (zh ? "检测中…" : "Checking…") : (zh ? "重新检测" : "Re-check")}</button>
          <button type="button" className="station-tile-recheck station-tile-rejoin" title={zh ? "打开加入节点页并带上这个节点的集群与名称；有 SSH 入网记录时打开它，否则预填表单。不会自动入网或改动记录。" : "Open the add-node page with this node's cluster and name; opens its SSH join record when one exists, otherwise a prefilled form. Nothing joins or changes records here."} onClick={rejoin}>{zh ? "重新接入" : "Rejoin"}</button>
        </div>
      </div>}
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
      {admission ? <span className={`status ${admissionStateClass(admission.state)}`} title={admission.reason}><i />{admissionStateLabel(admission.state, zh)}</span> : null}
    </div>

    <dl className="station-card-facts">{facts.slice(0, 3).map((fact) => <div key={fact.key}><dt>{fact.label}</dt><dd>{fact.value}</dd></div>)}</dl>

    <footer className="station-card-foot">
      <div className="station-card-quick"></div>
      {expander}
    </footer>
  </article>;
}, (previous, next) => {
  const { power: beforePower, restart: beforeRestart, recheck: beforeRecheck, ...beforeCard } = previous;
  const { power: afterPower, restart: afterRestart, recheck: afterRecheck, ...afterCard } = next;
  return Object.keys(beforeCard).every((key) => {
    const field = key as keyof typeof beforeCard;
    return Object.is(beforeCard[field], afterCard[field]);
  }) && beforePower.pending === afterPower.pending && beforePower.request === afterPower.request
    && beforePower.press === afterPower.press && beforeRecheck.pending === afterRecheck.pending && beforeRecheck.run === afterRecheck.run
    && beforeRestart?.start === afterRestart?.start && beforeRestart?.available === afterRestart?.available
    && beforeRestart?.pending === afterRestart?.pending && beforeRestart?.blocked === afterRestart?.blocked
    && beforeRestart?.request === afterRestart?.request
    && JSON.stringify(beforeRestart?.outstanding) === JSON.stringify(afterRestart?.outstanding)
    && JSON.stringify(beforeRestart?.observation) === JSON.stringify(afterRestart?.observation);
});
