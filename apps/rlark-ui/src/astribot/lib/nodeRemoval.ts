import type { StationItem } from "../api.js";
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
