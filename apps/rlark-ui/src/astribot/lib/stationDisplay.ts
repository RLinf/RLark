import type { StationItem } from "../api";
import type { ClusterSummary } from "../../types";

export function clusterForStation(clusters: ClusterSummary[], namespace: string) {
  const matches = clusters.filter((cluster) => cluster.agentNamespace === namespace);
  return matches.length === 1 && !matches[0].agentNamespaceConflict ? matches[0] : undefined;
}

export function stationDisplayName(item: StationItem, clusters: ClusterSummary[]) {
  const cluster = clusterForStation(clusters, item.metadata.namespace);
  return cluster ? `${cluster.name}-${String(item.spec.sequence).padStart(4, "0")}` : item.spec.displayName;
}

export type StationPhase = NonNullable<NonNullable<StationItem["status"]>["phase"]> | "Unknown";

export function stationPhase(item: StationItem): StationPhase {
  return item.status?.phase ?? "Unknown";
}

// Maps a Station phase onto the shared `.status-*` pill variants.
export function stationStatusClass(phase: StationPhase) {
  switch (phase) {
    case "Bound": return "status-online";
    case "Offline": return "status-pending";
    case "Conflict": return "status-failed";
    default: return "status-stopped";
  }
}

export function stationPhaseLabel(phase: StationPhase, zh: boolean) {
  return zh ? { Bound: "已绑定", Offline: "离线", Conflict: "冲突", Retired: "已退役", Unknown: "未知" }[phase] : phase;
}

export type StationNetworkState = "verified" | "failed" | "unknown";

// Network health is reported only through the NetworkVerified condition; the
// Station API carries no IP or WireGuard data.
export function stationNetworkState(item: StationItem): StationNetworkState {
  const condition = item.status?.conditions?.find((entry) => entry.type === "NetworkVerified");
  if (condition?.status === "True") return "verified";
  if (condition?.status === "False") return "failed";
  return "unknown";
}

// How long since the machine booted: "3 天 3 小时", "1h 30m", "45 分钟".
export function stationUptime(bootTime: string | undefined, now: number, zh: boolean) {
  const ms = now - Date.parse(bootTime ?? "");
  if (!(ms >= 0)) return "—";
  const minutes = Math.floor(ms / 60000);
  const days = Math.floor(minutes / 1440);
  const hours = Math.floor((minutes % 1440) / 60);
  const mins = minutes % 60;
  if (days > 0) return zh ? `${days} 天${hours ? ` ${hours} 小时` : ""}` : `${days}d${hours ? ` ${hours}h` : ""}`;
  if (hours > 0) return zh ? `${hours} 小时${mins ? ` ${mins} 分钟` : ""}` : `${hours}h${mins ? ` ${mins}m` : ""}`;
  return zh ? `${mins} 分钟` : `${mins}m`;
}

// "12 秒前", "3h ago": how long ago a time was, for every page of ours; "—" for none.
export function formatRelativeTime(value: string | undefined, now: number, zh: boolean) {
  const time = Date.parse(value ?? "");
  if (Number.isNaN(time)) return "—";
  const seconds = Math.max(0, Math.floor((now - time) / 1000));
  if (seconds < 60) return zh ? `${seconds} 秒前` : `${seconds}s ago`;
  if (seconds < 3600) return zh ? `${Math.floor(seconds / 60)} 分钟前` : `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return zh ? `${Math.floor(seconds / 3600)} 小时前` : `${Math.floor(seconds / 3600)}h ago`;
  return zh ? `${Math.floor(seconds / 86400)} 天前` : `${Math.floor(seconds / 86400)}d ago`;
}

export function shortUID(value: string | undefined) {
  if (!value) return "—";
  return value.length > 13 ? `${value.slice(0, 8)}…${value.slice(-4)}` : value;
}
