import type { StationItem } from "../api.js";
import type { ClusterSummary } from "../../types.js";
import type { NodeAdmission } from "./siteNodeDisplay.js";
import { admissionStateLabel } from "./siteNodeDisplay.js";
import { clusterForStation, formatRelativeTime, stationUptime } from "./stationDisplay.js";
import { stationDeletable } from "./nodeRemoval.js";

const conditionNames: Record<string, [string, string]> = {
  Observed: ["已观测", "Observed"],
  IdentityMatched: ["身份匹配", "Identity matched"],
  NetworkVerified: ["网络已验证", "Network verified"],
  RLarkLinked: ["RLark 已连接", "RLark linked"],
  Rebooted: ["已重启", "Rebooted"],
};

export function conditionLabel(type: string, zh: boolean): string {
  return conditionNames[type]?.[zh ? 0 : 1] ?? type;
}

export function conditionStatusClass(status: string): string {
  if (status === "True") return "status-online";
  if (status === "False") return "status-failed";
  return "status-pending";
}

export function conditionView(item: StationItem, now: number, zh: boolean) {
  return [...(item.status?.node?.conditions ?? [])]
    .sort((a, b) => Number(a.status === "True") - Number(b.status === "True") || a.type.localeCompare(b.type))
    .map((condition) => ({
      ...condition,
      label: conditionLabel(condition.type, zh),
      statusClass: conditionStatusClass(condition.status),
      relativeTime: formatRelativeTime(condition.lastTransitionTime, now, zh),
    }));
}

export function stationFacts(item: StationItem, cluster: ClusterSummary | undefined, now: number, zh: boolean) {
  const node = item.status?.node;
  return [
    { key: "uptime", label: zh ? "运行时间" : "Uptime", value: stationUptime(node?.bootTime, now, zh) },
    { key: "reboots", label: zh ? "重启次数" : "Reboots", value: String(node?.reboots ?? 0) },
    { key: "lastSeen", label: zh ? "最近上报" : "Last seen", value: formatRelativeTime(node?.lastSeenTime, now, zh) },
    { key: "cluster", label: zh ? "集群" : "Cluster", value: cluster?.name ?? (zh ? "集群未确定" : "Cluster unresolved") },
  ];
}

export function stationDeleteReason(item: StationItem, hasNode: boolean, zh: boolean): string {
  if (stationDeletable(item, hasNode)) return "";
  if (hasNode) return zh ? "集群中仍有同名 Node，需先完成节点移除后才能删除记录。" : "A Node with this name is still present in the cluster. Remove it before deleting this record.";
  return zh ? "当前记录状态不允许删除。" : "The current record state does not allow deletion.";
}

export function admissionLabels(admission: NodeAdmission | undefined, zh: boolean) {
  if (!admission) return { state: zh ? "未使用入网码" : "No admission code", issuer: "—", expires: "—" };
  return {
    state: admissionStateLabel(admission.state, zh),
    issuer: admission.issued_by || "—",
    expires: admission.expires_at || "—",
  };
}

export function stationDrawerCluster(clusters: ClusterSummary[], item: StationItem) {
  return clusterForStation(clusters, item.metadata.namespace);
}
