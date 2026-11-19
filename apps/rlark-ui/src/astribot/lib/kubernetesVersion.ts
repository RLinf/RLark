import type { CRDNode } from "../../types";

// station-controller reads these from the member cluster onto each RLark Node (astribot/station/controller/kubernetes_versions.go).
const kubernetesVersionAnnotation = "station.rlark.io/kubernetes-version";
const kubeletVersionAnnotation = "station.rlark.io/kubelet-version";
const errorAnnotation = "station.rlark.io/kubernetes-version-error";

export interface KubernetesVersions {
  // The version the cluster's API server runs.
  version?: string;
  // The kubelet versions its nodes report other than that one.
  apart: Array<{ version: string; nodes: number }>;
  // The last read of the member failed; the versions are the ones read before.
  unreadable: boolean;
}

export function kubernetesVersionsOf(nodes: CRDNode[]): KubernetesVersions {
  const version = nodes.map((node) => node.metadata.annotations?.[kubernetesVersionAnnotation]).find(Boolean);
  const counts = new Map<string, number>();
  for (const node of nodes) {
    const kubelet = node.metadata.annotations?.[kubeletVersionAnnotation];
    if (kubelet && kubelet !== version) counts.set(kubelet, (counts.get(kubelet) ?? 0) + 1);
  }
  return {
    version,
    apart: [...counts]
      .map(([kubelet, count]) => ({ version: kubelet, nodes: count }))
      .sort((a, b) => a.version.localeCompare(b.version)),
    unreadable: nodes.some((node) => Boolean(node.metadata.annotations?.[errorAnnotation])),
  };
}

// "v1.36.6", "v1.36.6 · 1 节点 v1.33.13" while some nodes run another kubelet, "v1.36.6 · 未能读取" while the
// member cannot be read; "—" when never read.
export function kubernetesVersionText(versions: KubernetesVersions | undefined, zh: boolean): string {
  if (!versions?.version && !versions?.unreadable) return "—";
  return [
    ...(versions.version ? [versions.version] : []),
    ...versions.apart.map(({ version, nodes }) =>
      zh ? `${nodes} 节点 ${version}` : `${nodes} ${nodes === 1 ? "node" : "nodes"} ${version}`,
    ),
    ...(versions.unreadable ? [zh ? "未能读取" : "unreadable"] : []),
  ].join(" · ");
}
