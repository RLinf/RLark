import type { CRDNode, NodeCategory } from "../types";

export const NODE_CATEGORY_LABEL = "rlark.io/node-category";
const NODE_CATEGORY_PREFIX = "rlark.io/node-category-";
const CONTROL_PLANE_ROLE_LABELS = [
  "node-role.kubernetes.io/control-plane",
  "node-role.kubernetes.io/master",
];

export function getNodeCategory(node: CRDNode): NodeCategory {
  return getNodeCategories(node)[0] ?? "unknown";
}

export function getNodeCategories(node: CRDNode): NodeCategory[] {
  const labels = node.metadata.labels ?? {};
  const explicit = (["cloud", "edge", "robot"] as const).filter(
    (category) => labels[`${NODE_CATEGORY_PREFIX}${category}`] === "true",
  );
  const values = (labels[NODE_CATEGORY_LABEL] ?? "")
    .split(",")
    .map((value) => value.trim())
    .filter(
      (value): value is Exclude<NodeCategory, "unknown"> =>
        value === "cloud" || value === "edge" || value === "robot",
    );
  if (explicit.length > 0 || values.length > 0) {
    return Array.from(new Set([...explicit, ...values]));
  }
  const resources = {
    ...(node.status?.capacity ?? {}),
    ...(node.status?.allocatable ?? {}),
  };
  if (
    Object.entries(resources).some(
      ([name, quantity]) =>
        Number.parseFloat(quantity) > 0 && /(^|[./-])gpu($|[./-])/i.test(name),
    )
  ) {
    return ["cloud"];
  }
  return ["unknown"];
}

export function hasNodeCategory(node: CRDNode, category: NodeCategory) {
  return getNodeCategories(node).includes(category);
}

// A node is its cluster's control plane by its Kubernetes role, as its
// agent reports the member Node's labels.
export function isControlPlaneLabels(labels: Record<string, string> | undefined): boolean {
  if (!labels) return false;
  const kubernetesRole = labels["kubernetes.io/role"];
  return (
    CONTROL_PLANE_ROLE_LABELS.some((label) => label in labels) ||
    kubernetesRole === "master" ||
    kubernetesRole === "control-plane"
  );
}

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

export function isBusinessWorkerNode(node: CRDNode): boolean {
  const labels = node.metadata.labels ?? {};
  const isControlPlane = isControlPlaneLabels(labels);
  const resources = {
    ...(node.status?.capacity ?? {}),
    ...(node.status?.allocatable ?? {}),
  };
  const hasWorkloadDevice = Object.entries(resources).some(
    ([name, value]) =>
      Number.parseFloat(value) > 0 &&
      (/(^|[./-])gpu($|[./-])/i.test(name) ||
        name === "rlinf.io/device" ||
        name.startsWith("rlinf.io/device-")),
  );
  return (
    (!isControlPlane || labels["rlark.io/enabled"] === "true") &&
    (getNodeCategories(node).some((category) => category !== "unknown") ||
      hasWorkloadDevice)
  );
}
