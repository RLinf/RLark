import type { StationClusterSummary, StationItem, StationSummary, StationSummaryPhase } from "../api";
import type { ClusterSummary } from "../../types";
import { clusterForStation, stationPhase } from "./stationDisplay.js";
import { isRobotNode, machineModel, robotMaintenanceState, robotPowerState, robotSelfCheckState } from "./siteNodeDisplay.js";
import { cameraCells, publishedCameras } from "./cameras.js";

export type FleetViewMode = "table" | "cards" | "wall";
export type FleetSortKey = "alert" | "name" | "heartbeat" | "restarts";
export type FleetPowerFilter = "all" | "running" | "powered" | "off" | "alert";
export type FleetOccupancyFilter = "all" | "occupied" | "idle";
export interface FleetFilters {
  q?: string; phase?: string; cluster?: string; model?: string; zone?: string;
  power?: FleetPowerFilter; occupancy?: FleetOccupancyFilter; alertOnly?: boolean; sort?: FleetSortKey;
}
export type StationSummaryMetrics = StationSummary;

export interface StationFleetMeta {
  model: string; zone: string; powerBucket: "running" | "powered" | "off" | "alert" | "unknown";
  driverReady: boolean; cameraFault: boolean; occupied: boolean; alertSeverity: number;
  alertReason: string; heartbeatMs: number; restarts: number;
}
export interface FleetStation extends StationItem {
  clusterName?: string; clusterID?: string; labels?: Record<string, string>; annotations?: Record<string, string>;
  site?: { name?: string; zone?: string; labels?: Record<string, string>; declared_model?: string; declared_type?: string; observed_model?: string; observed_type?: string; cameras?: string };
}

export function defaultViewModeForCount(robotCount: number, saved: FleetViewMode | null): FleetViewMode {
  return saved ?? (robotCount > 12 ? "table" : "cards");
}

export function extractStationMeta(item: FleetStation): StationFleetMeta {
  const labels = item.labels ?? {};
  const state = robotPowerState(labels);
  const maintenance = robotMaintenanceState(labels);
  const annotations = item.annotations ?? {};
  const cameraReport = publishedCameras(annotations["rlark.io/cameras"] ?? item.site?.cameras);
  const cameraFault = Boolean(labels["rlark.io/camera-fault"] || annotations["rlark.io/camera-fault"] || cameraReport && cameraCells(cameraReport, false).some((camera) => camera.tone === "failed"));
  const conditions = item.status?.node?.conditions ?? [];
  const selfCheckFailed = robotSelfCheckState(labels) === "failed" || conditions.some((condition) => condition.type === "SelfCheck" && condition.status === "False");
  const phase = stationPhase(item);
  const severity = state === "fault" || state === "estop" || selfCheckFailed ? 3
    : phase === "Conflict" || phase === "Offline" ? 2
      : cameraFault || Boolean(maintenance) ? 1 : 0;
  const lastSeen = Date.parse(item.status?.node?.lastSeenTime ?? "");
  const zone = item.site?.zone || item.site?.name || item.site?.labels?.["site.rlark.io/zone"] || labels["site.rlark.io/zone"] || labels["rlark.io/zone"] || "";
  return {
    model: machineModel(labels) || item.site?.declared_model || item.site?.observed_model || item.site?.declared_type || item.site?.observed_type || item.site?.labels?.["rlark.io/robot-model"] || "",
    zone,
    powerBucket: severity >= 3 ? "alert" : state === "ready" ? "running" : state === "powered" ? "powered" : state === "off" ? "off" : "unknown",
    driverReady: state === "ready",
    cameraFault,
    occupied: labels["rlark.io/occupied"] === "true" || labels["rlark.io/occupancy"] === "occupied" || annotations["rlark.io/occupancy"] === "occupied",
    alertSeverity: severity,
    alertReason: severity ? (state === "estop" ? "Emergency stop" : state === "fault" || selfCheckFailed ? "Power or self-check fault" : phase === "Conflict" ? "Conflict" : phase === "Offline" ? "Offline" : cameraFault ? "Camera fault" : "Maintenance") : "",
    heartbeatMs: Number.isFinite(lastSeen) ? lastSeen : 0,
    restarts: item.status?.node?.reboots ?? 0,
  };
}

export function filterAndSortFleetItems<T extends FleetStation>(items: T[], filters: FleetFilters = {}, clusters: ClusterSummary[] = []): T[] {
  const query = filters.q?.trim().toLocaleLowerCase() ?? "";
  const selected = items.filter((item) => {
    const meta = extractStationMeta(item);
    const cluster = clusterForStation(clusters, item.metadata.namespace);
    const values = [item.metadata.name, item.metadata.namespace, item.spec.nodeUID, cluster?.name ?? item.clusterName ?? "", meta.model, meta.zone];
    if (query && !values.some((value) => value.toLocaleLowerCase().includes(query))) return false;
    if (filters.phase && stationPhase(item) !== filters.phase) return false;
    if (filters.cluster && (cluster?.id ?? item.clusterID ?? item.metadata.namespace) !== filters.cluster) return false;
    if (filters.model && meta.model !== filters.model) return false;
    if (filters.zone && meta.zone !== filters.zone) return false;
    if (filters.power && filters.power !== "all" && !(filters.power === "powered" ? meta.powerBucket === "powered" || meta.powerBucket === "running" : meta.powerBucket === filters.power)) return false;
    if (filters.occupancy === "occupied" && !meta.occupied || filters.occupancy === "idle" && meta.occupied) return false;
    if (filters.alertOnly && !meta.alertSeverity) return false;
    return true;
  });
  const sort = filters.sort ?? "alert";
  return selected.sort((a, b) => {
    const ma = extractStationMeta(a), mb = extractStationMeta(b);
    if (sort === "alert" && ma.alertSeverity !== mb.alertSeverity) return mb.alertSeverity - ma.alertSeverity;
    if (sort === "heartbeat" && ma.heartbeatMs !== mb.heartbeatMs) return mb.heartbeatMs - ma.heartbeatMs;
    if (sort === "restarts" && ma.restarts !== mb.restarts) return mb.restarts - ma.restarts;
    if (sort === "name") return a.metadata.name.localeCompare(b.metadata.name);
    const ca = clusterForStation(clusters, a.metadata.namespace)?.name ?? a.clusterName ?? a.metadata.namespace;
    const cb = clusterForStation(clusters, b.metadata.namespace)?.name ?? b.clusterName ?? b.metadata.namespace;
    return ca.localeCompare(cb) || a.metadata.name.localeCompare(b.metadata.name);
  });
}

function summaryPhase(item: FleetStation): StationSummaryPhase {
  const phase = stationPhase(item).toLowerCase();
  switch (phase) {
    case "bound": return "Bound";
    case "offline": return "Offline";
    case "conflict": return "Conflict";
    case "retired": return "Retired";
    default: return "Unknown";
  }
}

const isCount = (value: unknown): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;

function isClusterSummary(value: unknown): value is StationClusterSummary {
  if (!value || typeof value !== "object") return false;
  const cluster = value as Record<string, unknown>;
  return typeof cluster.clusterId === "string" && typeof cluster.namespace === "string" &&
    [cluster.total, cluster.robots, cluster.bound, cluster.offline, cluster.conflict,
      cluster.alerts, cluster.driverReady, cluster.cameraFault].every(isCount);
}

// Gateways that predate the canonical casing are readable when the rest of the summary is complete.
export function parseStationSummary(value: unknown): StationSummary | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { total, robots, otherNodes, byPhase, alerts, driverReady, cameraFault, occupied, clusters, generatedAt } =
    value as Record<string, unknown>;
  if (!isCount(total) || !isCount(robots) || !isCount(otherNodes) || robots + otherNodes !== total ||
      !isCount(alerts) || !isCount(driverReady) || !isCount(cameraFault) || !isCount(occupied) ||
      !Array.isArray(clusters) || !clusters.every(isClusterSummary) || !byPhase || typeof byPhase !== "object") return undefined;
  if (clusters.reduce((count, cluster) => count + cluster.total, 0) !== total) return undefined;
  const phases = byPhase as Record<string, unknown>;
  const bound = phases.Bound ?? phases.bound;
  const offline = phases.Offline ?? phases.offline;
  const conflict = phases.Conflict ?? phases.conflict;
  const retired = phases.Retired ?? phases.retired;
  const unknown = phases.Unknown ?? phases.unknown ?? 0;
  if (!isCount(bound) || !isCount(offline) || !isCount(conflict) || !isCount(retired) || !isCount(unknown) ||
      bound + offline + conflict + retired + unknown !== total) return undefined;
  return { total, robots, otherNodes, alerts, driverReady, cameraFault, occupied, clusters,
    byPhase: { Bound: bound, Offline: offline, Conflict: conflict, Retired: retired, Unknown: unknown },
    ...(typeof generatedAt === "string" ? { generatedAt } : {}) };
}

export function selectFleetSummary(server: StationSummary | undefined, local: StationSummary): StationSummary {
  // Older Gateways may see enrollment records without the robot labels the UI reads from the Node and site views.
  return server && server.total >= local.total && server.robots >= local.robots ? server : local;
}

export function computeFleetSummary(items: FleetStation[], clusters: ClusterSummary[] = []): StationSummaryMetrics {
  const byPhase = { Bound: 0, Offline: 0, Conflict: 0, Retired: 0, Unknown: 0 };
  let robots = 0, alerts = 0, driverReady = 0, cameraFault = 0, occupied = 0;
  const byCluster = new Map<string, StationClusterSummary>();
  for (const item of items) {
    const phase = summaryPhase(item);
    byPhase[phase]++;
    const namespace = item.metadata.namespace;
    const clusterId = clusterForStation(clusters, namespace)?.id ?? item.clusterID ?? namespace;
    let cluster = byCluster.get(clusterId);
    if (!cluster) {
      cluster = { clusterId, namespace, total: 0, robots: 0, bound: 0, offline: 0, conflict: 0, alerts: 0, driverReady: 0, cameraFault: 0 };
      byCluster.set(clusterId, cluster);
    }
    cluster.total++;
    if (phase === "Bound") cluster.bound++;
    if (phase === "Offline") cluster.offline++;
    if (phase === "Conflict") cluster.conflict++;
    const labels = item.labels;
    const robot = isRobotNode(labels) || Boolean(labels?.["rlark.io/robot-id"]) || labels?.["rlark.io/node-category"] === "robot";
    if (robot) { robots++; cluster.robots++; }
    const meta = extractStationMeta(item);
    if (meta.alertSeverity) { alerts++; cluster.alerts++; }
    if (meta.driverReady) { driverReady++; cluster.driverReady++; }
    if (meta.cameraFault) { cameraFault++; cluster.cameraFault++; }
    if (meta.occupied) occupied++;
  }
  return { total: items.length, robots, otherNodes: items.length - robots, byPhase, alerts, driverReady, cameraFault, occupied,
    clusters: [...byCluster.values()].sort((a, b) => a.clusterId.localeCompare(b.clusterId)) };
}

export function computeVirtualWindow({ totalCount, scrollTop, viewportHeight, rowHeight, overscan = 5 }: {
  totalCount: number; scrollTop: number; viewportHeight: number; rowHeight: number; overscan?: number;
}) {
  const count = Math.max(0, Math.floor(totalCount));
  const height = Math.max(1, rowHeight);
  const startIndex = Math.max(0, Math.min(count, Math.floor(Math.max(0, scrollTop) / height) - Math.max(0, overscan)));
  const endIndex = Math.min(count, Math.ceil((Math.max(0, scrollTop) + Math.max(0, viewportHeight)) / height) + Math.max(0, overscan));
  return { startIndex, endIndex, offsetTop: startIndex * height, totalHeight: count * height };
}
