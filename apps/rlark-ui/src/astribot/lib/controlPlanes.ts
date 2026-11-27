import type { CRDNode } from "../../types.js";
import { isControlPlaneLabels } from "../../utils/nodeVisibility.js";

export interface ControlPlaneNode {
  name: string;
  displayName?: string;
  address: string;
}

// The control-plane nodes of a cluster (its agent namespace), with the
// address its Node reports first (InternalIP). None: a managed control plane
// (Kingsoft KCE), whose nodes RLark does not see.
export function controlPlanesOf(nodes: CRDNode[], namespace: string): ControlPlaneNode[] {
  return nodes
    .filter((node) => node.metadata.namespace === namespace && isControlPlaneLabels(node.metadata.labels))
    .map((node) => {
      const addresses = node.status?.addresses ?? [];
      const address = (addresses.find((item) => item.type === "InternalIP") ?? addresses.find((item) => item.type !== "Hostname"))?.address ?? "";
      return { name: node.metadata.name, displayName: node.spec.displayName, address };
    })
    .sort((a, b) => a.name.localeCompare(b.name));
}
