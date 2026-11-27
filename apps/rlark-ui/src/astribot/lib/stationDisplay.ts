import type { StationItem } from "../api";
import type { ClusterSummary } from "../../types";

export function clusterForStation(clusters: ClusterSummary[], namespace: string) {
  const matches = clusters.filter((cluster) => cluster.agentNamespace === namespace);
  return matches.length === 1 && !matches[0].agentNamespaceConflict ? matches[0] : undefined;
}

export type StationPhase = "Bound" | "Offline" | "Conflict" | "Retired" | "Unknown";

// Unknown until the node's first report; a removed node is Retired whatever it last reported.
export function stationPhase(item: StationItem): StationPhase {
  return item.spec.retired ? "Retired" : item.status?.node?.state ?? "Unknown";
}

// Maps a site node's phase onto the shared `.status-*` pill variants.
export function stationStatusClass(phase: StationPhase) {
  switch (phase) {
    case "Bound": return "status-online";
    case "Offline": return "status-pending";
    case "Conflict": return "status-failed";
    default: return "status-stopped";
  }
}

export function stationPhaseLabel(phase: StationPhase, zh: boolean) {
  return zh ? { Bound: "在线", Offline: "离线", Conflict: "冲突", Retired: "已移除", Unknown: "未上报" }[phase] : phase;
}

export type StationNetworkState = "verified" | "failed" | "unknown";

// Whether the node reaches its peers through the tunnel: the NetworkVerified condition.
export function stationNetworkState(item: StationItem): StationNetworkState {
  const condition = item.status?.node?.conditions?.find((entry) => entry.type === "NetworkVerified");
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

// The recovery line of a station the plane does not see as Bound: what it last reported, whether the tunnel verified,
// and the boundary of what a browser-side re-check can do. Re-check re-reads the node and the site network through the
// page's normal load; if both the machine and its site link are unreachable, only someone at the machine or an
// out-of-band path can recover it.
export function stationRecoveryHint(item: StationItem, now: number, zh: boolean): string {
  const seen = formatRelativeTime(item.status?.node?.lastSeenTime, now, zh);
  const network = stationNetworkState(item);
  const link = zh
    ? ({ verified: "已验证", failed: "未通过", unknown: "未验证" } as const)[network]
    : ({ verified: "verified", failed: "not verified", unknown: "unverified" } as const)[network];
  return zh
    ? `最近上报 ${seen}；隧道${link}。重新检测会重新读取节点与场站网络；若主机和站点链路都不可达，需到现场或走带外接入。`
    : `Last report ${seen}; tunnel ${link}. Re-check reads the node and the site network again; if both the host and its site link are unreachable, someone must go to the machine or use an out-of-band path.`;
}
