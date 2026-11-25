import type { CRDNode, NodeCategory } from "../types";

export const NODE_CATEGORY_LABEL = "rlark.io/node-category";
const NODE_CATEGORY_PREFIX = "rlark.io/node-category-";

// RLark-managed Node marker. The data-plane Agent copies this label from the
// local Kubernetes Node onto the management Node CR, so its presence is the
// authoritative signal that a node belongs to the RLark fleet. Visibility keys
// off this marker rather than the Kubernetes role: an operator may manage a
// control-plane node, and the Gateway data-plane view marks it managed too.
export const NODE_ENABLED_LABEL = "rlark.io/enabled";

// getNodeAllLabels returns the labels the data-plane Agent observed on the local
// Kubernetes Node (status.observedNodeLabels). This is the only source for label
// display and editing: metadata.labels on the management CR only receives RLark
// system labels written by the Agent, so it is never used as a fallback.
// Administrator metadata such as categories and hardware models lives in
// metadata.annotations instead.
export function getNodeAllLabels(node: CRDNode): Record<string, string> {
  return node.status?.observedNodeLabels ?? {};
}

export function getNodeCategory(node: CRDNode): NodeCategory {
  return getNodeCategories(node)[0] ?? "unknown";
}

// Category metadata is authored on the management Node CR as annotations. The
// legacy label forms stay readable so Nodes created before the migration keep
// their category until an administrator saves the node again.
function nodeCategoryValue(node: CRDNode, key: string): string | undefined {
  return node.metadata.annotations?.[key] ?? getNodeAllLabels(node)[key];
}

export function getNodeCategories(node: CRDNode): NodeCategory[] {
  const explicit = (["cloud", "edge", "robot"] as const).filter(
    (category) =>
      nodeCategoryValue(node, `${NODE_CATEGORY_PREFIX}${category}`) === "true",
  );
  const values = (nodeCategoryValue(node, NODE_CATEGORY_LABEL) ?? "")
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

// Reports whether a Node belongs to the RLark fleet and therefore appears in
// the Worker view. RLark-managed nodes are marked with NODE_ENABLED_LABEL;
// category labels only group Workers, and the Kubernetes role does not gate
// visibility.
export function isBusinessWorkerNode(node: CRDNode): boolean {
  const labels = getNodeAllLabels(node);
  return labels[NODE_ENABLED_LABEL] === "true";
}
