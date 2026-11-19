import type { StationItem } from "../api.js";
import type { CRDPod } from "../../types.js";

// The Kubernetes Node UID RLark verified for a node: its unretired Station's,
// unless that Station is in Conflict (bound to a Node of the name that is
// gone). Empty, the node is removed by its name: the Node its cluster has, or,
// when it has none, what RLark keeps of the name.
export function verifiedNodeUID(stations: StationItem[], namespace: string, nodeName: string): string {
  return stations.find((station) => station.metadata.namespace === namespace && station.spec.binding.nodeName === nodeName && !station.spec.retired && station.status?.phase !== "Conflict")?.spec.binding.nodeUID ?? "";
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
// A retired Station is the record of a node that was removed, or of the earlier node of a name a machine joined
// again under. It is no node of the site: the list leaves it out unless it is asked for, shows nothing of its
// network (what it holds is from before), and lets an administrator delete it.
export function stationRetired(item: StationItem): boolean {
  return item.spec.retired === true || item.status?.phase === "Retired";
}

// The Stations a phase filter shows: every one that is not retired, or those of the phase asked for.
export function stationsOfPhase(items: StationItem[], phase: string): StationItem[] {
  if (phase === "Retired") return items.filter(stationRetired);
  return items.filter((item) => !stationRetired(item) && (!phase || item.status?.phase === phase));
}

// Asked to be deleted, and station-controller has not answered yet.
export function stationDeleteAsked(item: StationItem): boolean {
  return !!item.metadata.annotations?.["station.rlark.io/delete-requested-by"];
}

// Why station-controller did not delete it, when it was asked to.
export function stationDeleteRefusal(item: StationItem): string {
  return item.metadata.annotations?.["station.rlark.io/delete-refused"] ?? "";
}
