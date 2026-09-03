import type { CRDNode, NodeCategory } from "../types";

export const NODE_CATEGORY_KEYS = [
  "rlark.io/node-category",
  "rlark.io/node-category-cloud",
  "rlark.io/node-category-edge",
  "rlark.io/node-category-robot",
] as const;

export type NodeModelMetadataUpdates = {
  gpuModel?: string;
  deviceModel?: string;
};

export function updateNodeModelMetadata(
  node: CRDNode,
  updates: NodeModelMetadataUpdates,
) {
  const labels = { ...(node.metadata.labels ?? {}) };
  const annotations = { ...(node.metadata.annotations ?? {}) };
  const removedLabelKeys = new Set<string>();
  const removedAnnotationKeys = new Set<string>();

  const applyUpdate = (key: string, value: string | undefined) => {
    if (value === undefined) return;
    const normalizedValue = value.trim();
    if (normalizedValue) annotations[key] = normalizedValue;
    else {
      delete annotations[key];
      removedAnnotationKeys.add(key);
    }
    delete labels[key];
    removedLabelKeys.add(key);
  };

  applyUpdate("rlark.io/gpu-model", updates.gpuModel);
  applyUpdate("rlark.io/device-model", updates.deviceModel);
  return { labels, annotations, removedLabelKeys, removedAnnotationKeys };
}

export function updateNodeCategoryLabels(
  labels: Record<string, string>,
  categories: NodeCategory[],
) {
  const next = { ...labels };
  const removedKeys = new Set<string>();
  NODE_CATEGORY_KEYS.forEach((key) => {
    delete next[key];
    removedKeys.add(key);
  });
  (["cloud", "edge", "robot"] as const).forEach((category) => {
    if (categories.includes(category)) {
      next[`rlark.io/node-category-${category}`] = "true";
    }
  });
  return { labels: next, removedKeys };
}

export function getRemovedLabelKeys(
  original: Record<string, string>,
  next: Record<string, string>,
) {
  return Object.keys(original).filter((key) => !(key in next));
}
