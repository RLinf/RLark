import type { ClusterSummary } from "../../types";
import { clusterForStation } from "./stationDisplay.js";
import { isValidJobDisplayName } from "../../utils/job.js";
import { ApiError } from "../../api.js";

// A Cluster's or Node's spec.displayName is only what the console shows: its
// metadata.name (the cluster ID, the Kubernetes Node name) is what routes,
// Jobs, Stations and SiteNetwork key on, and never changes.

export function nodeLabel(node: { metadata: { name: string }; spec?: { displayName?: string } }) {
  return node.spec?.displayName || node.metadata.name;
}

// The name a namespace's cluster is shown as; a namespace that maps to no
// single cluster (clusterForStation's rule) shows as itself.
export function clusterLabel(clusters: ClusterSummary[], namespace: string) {
  return clusterForStation(clusters, namespace)?.name ?? namespace;
}

// The names a node is found by in a search: its Kubernetes name, its display
// name, its cluster's namespace and the name that cluster is shown as.
export function nodeSearchNames(
  node: { metadata: { name: string; namespace?: string }; spec?: { displayName?: string } },
  clusterName: string,
) {
  return `${node.metadata.name} ${node.spec?.displayName ?? ""} ${node.metadata.namespace ?? ""} ${clusterName}`;
}

export interface NamedObject {
  // What a 409 names the object by: the cluster ID, a Node's namespace/name.
  id: string;
  name: string;
  displayName?: string;
}

// The Gateway's rule for a new display name, so a form can say it before the
// save: a trimmed empty name clears it; otherwise 1-64 letters, digits,
// Chinese characters, '.', '_' or '-' that no other object shows, as its
// display name or as its name.
export function displayNameError(name: string, self: string, objects: NamedObject[]) {
  const trimmed = name.trim();
  if (!trimmed) return undefined;
  if (!isValidJobDisplayName(trimmed))
    return {
      status: 400 as const,
      message: "displayName must be at most 64 letters, digits, Chinese characters, '.', '_' or '-'",
    };
  const other = objects.find((object) => object.id !== self && (object.name === trimmed || object.displayName === trimmed));
  return other
    ? { status: 409 as const, message: `display name already used by ${other.id}`, usedBy: other.id }
    : undefined;
}

// A rename the Gateway refused, in displayNameError's shape: its 400 for the
// format, its 409 whose message ends "used by <id>". Other errors are not one.
export function displayNameRefusal(error: unknown) {
  if (!(error instanceof ApiError) || (error.status !== 400 && error.status !== 409)) return undefined;
  return { status: error.status, usedBy: error.message.split(" used by ")[1] };
}

// What the console says about a refused display name, for a cluster (used
// by its display name or ID) or a node (by its display name or Kubernetes name).
export function displayNameMessage(
  refusal: { status: number; usedBy?: string },
  kind: "cluster" | "node",
  zh: boolean,
) {
  if (refusal.status === 400)
    return zh
      ? "名称格式不正确：最多 64 个字符，仅支持中英文、数字以及-_."
      : "Invalid name: at most 64 Chinese/English letters, digits, -, _ and .";
  if (kind === "cluster")
    return zh
      ? `该名称已被集群 ${refusal.usedBy} 使用（作为显示名或 ID）`
      : `Already used by cluster ${refusal.usedBy} (as its display name or ID)`;
  return zh
    ? `该名称已被节点 ${refusal.usedBy} 使用（作为显示名或 Kubernetes 名）`
    : `Already used by node ${refusal.usedBy} (as its display name or Kubernetes name)`;
}
