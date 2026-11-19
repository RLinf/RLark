import type { INodeJoin } from "../../generated/rlark/rlinf.io/v1alpha1/NodeJoin.js";
import { stagesView, type Lang, type StageView } from "./clusterProvision.js";

export type NodeJoin = INodeJoin;

// The stages the Gateway joins a machine over SSH in, in order (astribot_node_join.go).
export const joinStages: { name: string; zh: string; en: string }[] = [
  { name: "precheck", zh: "登录和预检", en: "Login and pre-check" },
  { name: "bundle", zh: "安装包和隧道密钥", en: "Installer and tunnel key" },
  { name: "declare", zh: "声明 Hub", en: "Declare the hub" },
  { name: "tunnel", zh: "隧道", en: "Tunnel" },
  { name: "install", zh: "入网码兑换和安装", en: "Code redeemed and install" },
  { name: "node-ready", zh: "节点 Ready", en: "Node Ready" },
  { name: "station", zh: "入网登记", en: "Network enrollment" },
  { name: "gpu", zh: "GPU 与标签", en: "GPUs and label" },
];

// A retry from this stage logs in to the machine again: every stage up to install (the declaration's tunnel follows
// it at once).
export function joinStageNeedsPassword(stage: string): boolean {
  const index = joinStages.findIndex((s) => s.name === stage);
  return index >= 0 && index <= joinStages.findIndex((s) => s.name === "install");
}

export function joinStageLabel(stage: string, lang: Lang): string {
  return joinStages.find((s) => s.name === stage)?.[lang] ?? stage;
}

export interface NodeJoinView {
  phase: string;
  stages: StageView[];
  canRetry: boolean;
  retryNeedsPassword: boolean;
  // A join that does not run can be cleared.
  canClear: boolean;
  // It started the machine's own tunnel and its code's line has not run: clearing logs in to take the tunnel away.
  clearReachesMachine: boolean;
}

export function nodeJoinView(j: NodeJoin, lang: Lang): NodeJoinView {
  const stages = stagesView(joinStages, j.status?.stages, lang);
  const phase = j.status?.phase ?? "Running";
  const state = (name: string) => stages.find((s) => s.name === name)?.state ?? "Pending";
  return {
    phase,
    stages,
    canRetry: phase === "Failed",
    retryNeedsPassword: phase === "Failed" && joinStageNeedsPassword(j.status?.stage ?? ""),
    canClear: phase !== "Running",
    clearReachesMachine: !["Pending", "Skipped"].includes(state("tunnel")) && state("install") === "Pending",
  };
}
