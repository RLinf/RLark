import type { NodeOperation, StationItem } from "../api.js";
import type { CRDPod } from "../../types.js";

// The Kubernetes Node UID RLark verified for a node: its record's, unless the
// node was removed or another Node reports under the name (Conflict). Empty,
// the node is removed by its name: the Node its cluster has, or, when it has
// none, what RLark keeps of the name.
export function verifiedNodeUID(stations: StationItem[], namespace: string, nodeName: string): string {
  const station = stations.find((item) => item.metadata.namespace === namespace && item.metadata.name === nodeName);
  return station && !station.spec.retired && station.status?.node?.state !== "Conflict" ? station.spec.nodeUID : "";
}

// The RLark pods on the node that the removal stops; their tasks start
// them again elsewhere.
export function podsToStop(pods: CRDPod[], namespace: string, nodeName: string): string[] {
  return pods
    .filter((pod) => pod.status?.node === nodeName && (pod.metadata?.namespace ?? "") === namespace && !["Succeeded", "Failed"].includes(pod.status?.phase ?? ""))
    .map((pod) => pod.spec?.taskName || pod.spec?.podName || pod.metadata?.name || "")
    .sort();
}

// The stages of a removal, as the NodeOperation names them.
export const removalStages = ["Preflight", "PowerOff", "Drain", "RemovePeer", "Reset", "DeleteNode", "VerifyRemoval"];

const stageLabels: Record<string, { zh: string; en: string }> = {
  Preflight: { zh: "检查", en: "Checking" },
  PowerOff: { zh: "机器人下电", en: "Powering the robot off" },
  Drain: { zh: "清空节点", en: "Draining" },
  RemovePeer: { zh: "移出隧道", en: "Leaving the tunnel" },
  Reset: { zh: "重置机器", en: "Resetting the machine" },
  DeleteNode: { zh: "删除节点对象", en: "Deleting the Node" },
  VerifyRemoval: { zh: "确认已删除", en: "Verifying" },
};

export function removalStageLabel(stage: string, zh: boolean): string {
  return stageLabels[stage]?.[zh ? "zh" : "en"] ?? stage;
}

// How many stages are done: a NodeOperation names the last one it completed.
export function removalStagesDone(completed: string | undefined): number {
  return completed ? removalStages.indexOf(completed) + 1 : 0;
}

const terminalPhases = new Set(["Succeeded", "Failed", "Blocked"]);
const maxRemovalAgeMs = 24 * 60 * 60 * 1000;

export function removalOutcome(operation: NodeOperation): "active" | "succeeded" | "failed" {
  if (operation.status?.phase === "Succeeded") return "succeeded";
  if (operation.status?.phase === "Failed" || operation.status?.phase === "Blocked") return "failed";
  return "active";
}

export function removalIsActive(operation: NodeOperation): boolean {
  return !terminalPhases.has(operation.status?.phase ?? "");
}

export function removalStageProgress(stage: string | undefined): number {
  return Math.round((removalStagesDone(stage) / removalStages.length) * 100);
}

export function removalNoProgressMs(operation: NodeOperation, now = Date.now()): number | undefined {
  if (!removalIsActive(operation)) return undefined;
  const progressing = operation.status?.conditions?.find((condition) => condition.type === "Progressing");
  const lastTransition = Date.parse(progressing?.lastTransitionTime ?? "");
  return Number.isFinite(lastTransition) ? Math.max(0, now - lastTransition) : undefined;
}

export function removalLooksStuck(operation: NodeOperation, now = Date.now()): boolean {
  return (removalNoProgressMs(operation, now) ?? 0) > 10 * 60 * 1000;
}

export function removalOperationKey(clusterID: string, nodeName: string): string {
  return `${clusterID}/${nodeName}`;
}

export function latestRemovalOperation(operations: NodeOperation[], nodeName: string): NodeOperation | undefined {
  return operations
    .filter((operation) => operation.spec.action === "Remove" && operation.spec.nodeName === nodeName)
    .sort((left, right) => (Date.parse(right.metadata.creationTimestamp ?? "") || 0) - (Date.parse(left.metadata.creationTimestamp ?? "") || 0))[0];
}

// nodeUID is the member Kubernetes Node UID (a site-node record's
// spec.nodeUID), never the management Node CR's metadata.uid.
export function removalAppliesToNode(operation: NodeOperation, nodeUID?: string, nodeCreationTimestamp?: string, now = Date.now()): boolean {
  const createdAt = Date.parse(operation.metadata.creationTimestamp ?? "");
  if (!removalIsActive(operation) && (!Number.isFinite(createdAt) || now - createdAt > maxRemovalAgeMs)) return false;
  if (operation.spec.nodeUID && nodeUID && operation.spec.nodeUID !== nodeUID) return false;
  if (operation.status?.phase === "Succeeded" && nodeCreationTimestamp && Number.isFinite(createdAt) && Date.parse(nodeCreationTimestamp) > createdAt) return false;
  return true;
}

export function removalRequestID(clusterID: string, nodeName: string, nodeUID: string, attempt: number): string {
  const input = `${clusterID}\u0000${nodeName}\u0000${nodeUID}\u0000${attempt}`;
  let hash = 0xcbf29ce484222325n;
  for (let index = 0; index < input.length; index++) {
    hash ^= BigInt(input.charCodeAt(index));
    hash = BigInt.asUintN(64, hash * 0x100000001b3n);
  }
  return `remove-${hash.toString(16).padStart(16, "0")}`;
}

export function removalNextPollDelayMs(failureCount: number, random = Math.random): number {
  const delays = [3000, 6000, 12000, 30000];
  const base = delays[Math.min(Math.max(failureCount - 1, 0), delays.length - 1)];
  return Math.round(base * (0.8 + random() * 0.4));
}

export function removalFailureMessage(operation: NodeOperation): string {
  const progressing = operation.status?.conditions?.find((condition) => condition.type === "Progressing");
  return [progressing?.reason, progressing?.message].filter(Boolean).join(" · ") || operation.status?.phase || "Removal failed";
}

export interface StoredRemoval {
  clusterID: string;
  nodeName: string;
  operationName: string;
  attempt: number;
  startedAt: number;
}

export function parseStoredRemovals(value: string | null, now = Date.now()): StoredRemoval[] {
  if (!value) return [];
  try {
    const parsed: unknown = JSON.parse(value);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((entry): entry is StoredRemoval => {
      if (!entry || typeof entry !== "object") return false;
      const item = entry as Partial<StoredRemoval>;
      return typeof item.clusterID === "string" && typeof item.nodeName === "string" &&
        typeof item.operationName === "string" && Number.isInteger(item.attempt) &&
        typeof item.startedAt === "number" && now - item.startedAt <= maxRemovalAgeMs && now >= item.startedAt;
    }).slice(0, 10);
  } catch {
    return [];
  }
}
// The site nodes a phase filter shows: every one that was not removed, or those of the phase asked for. A removed
// node's record is no node of the site: the list leaves it out unless it is asked for.
export function stationsOfPhase(items: StationItem[], phase: string): StationItem[] {
  if (phase === "Retired") return items.filter((item) => item.spec.retired);
  return items.filter((item) => !item.spec.retired && (!phase || item.status?.node?.state === phase));
}

// A record an administrator may delete: its node was removed, another Node reports under its name, or its node
// does not report and RLark has no Node of the name (hasNode).
export function stationDeletable(item: StationItem, hasNode: boolean): boolean {
  const state = item.status?.node?.state;
  return item.spec.retired === true || state === "Conflict" || (!hasNode && state !== "Bound");
}
