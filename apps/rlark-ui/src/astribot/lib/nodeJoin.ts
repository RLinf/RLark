import type { INodeJoin } from "../../generated/rlark/rlinf.io/v1alpha1/NodeJoin.js";
import { stagesView, type Lang } from "./clusterProvision.js";

export type NodeJoin = INodeJoin;

// A join page opened for one known machine: the cluster, an SSH join to show when one already exists, and the
// values the SSH form starts with when none does. Nothing here submits a join; the operator still does that.
export interface NodeJoinTarget {
  clusterID?: string;
  // An existing NodeJoin's metadata.name (cluster.node), to open instead of a blank form.
  name?: string;
  nodeName?: string;
  address?: string;
  user?: string;
  port?: string;
}

// user@host:port, as the Gateway records an SSH join's login (net.JoinHostPort brackets IPv6).
export function parseSSHLogin(login?: string): { address: string; user?: string; port?: string } | undefined {
  const value = (login ?? "").trim();
  if (!value) return undefined;
  const at = value.indexOf("@");
  const user = at >= 0 ? value.slice(0, at).trim() : "";
  const host = (at >= 0 ? value.slice(at + 1) : value).trim();
  if (!host) return undefined;
  // A port follows the last colon; an IPv6 host keeps its colons inside brackets.
  const colon = host.lastIndexOf(":");
  const bracketed = host.startsWith("[");
  let address = host;
  let port = "";
  if (colon > 0 && (!bracketed || host.indexOf("]") < colon)) {
    const tail = host.slice(colon + 1);
    if (/^\d+$/.test(tail)) {
      address = host.slice(0, colon);
      port = tail;
    }
  }
  address = address.replace(/^\[|\]$/g, "");
  if (!address) return undefined;
  return { address, ...(user ? { user } : {}), ...(port ? { port } : {}) };
}

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

export function nodeJoinView(j: NodeJoin, lang: Lang) {
  const stages = stagesView(joinStages, j.status?.stages, lang);
  const phase = j.status?.phase ?? "Running";
  const state = (name: string) => stages.find((s) => s.name === name)?.state ?? "Pending";
  return {
    phase,
    stages,
    canRetry: phase === "Failed",
    retryNeedsPassword: phase === "Failed" && joinStageNeedsPassword(j.status?.stage ?? ""),
    // A join that does not run can be cleared.
    canClear: phase !== "Running",
    // It started the machine's own tunnel and its code's line has not run: clearing logs in to take the tunnel away.
    clearReachesMachine: !["Pending", "Skipped"].includes(state("tunnel")) && state("install") === "Pending",
  };
}
