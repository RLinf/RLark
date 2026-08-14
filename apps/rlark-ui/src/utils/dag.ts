import type { JobType } from "../data";
import type { DAGNode, DAGEdge, RoleResource } from "../types";
import { computePvcStorageMap } from "./job";

export function makeDefaultRoleResources(
  type: JobType,
  roles: string[],
  clusterDisplayNames: string[] = [],
  jobName?: string,
): Record<string, RoleResource> {
  const rr: Record<string, RoleResource> = {};
  roles.forEach((role, index) => {
    const mounts = [
      {
        type: "host" as const,
        objectStorage: "",
        mountPath: "/mnt/dataset",
        hostPath: "/host/dataset",
      },
    ];
    rr[role] = {
      role,
      cluster: clusterDisplayNames[0] ?? "",
      nodeSelector: "",
      replicas: 0,
      cpu: "",
      memory: "",
      gpu: index === 0 ? "4" : "0",
      devices: [],
      image: "",
      prepareScript: "",
      envs: [{ key: "RLARK_TASK_ROLE", value: role }],
      mounts,
      pvcStorageMap: computePvcStorageMap(role, mounts, jobName),
    };
  });
  return rr;
}

export function hasCycle(edges: DAGEdge[], nodes: DAGNode[]): boolean {
  const adj = new Map<string, string[]>();
  nodes.forEach((n) => adj.set(n.id, []));
  edges.forEach((e) => adj.get(e.from)?.push(e.to));
  const visited = new Set<string>();
  const stack = new Set<string>();
  const dfs = (id: string): boolean => {
    visited.add(id);
    stack.add(id);
    for (const next of adj.get(id) ?? []) {
      if (!visited.has(next) && dfs(next)) return true;
      if (stack.has(next)) return true;
    }
    stack.delete(id);
    return false;
  };
  for (const n of nodes) {
    if (!visited.has(n.id) && dfs(n.id)) return true;
  }
  return false;
}
