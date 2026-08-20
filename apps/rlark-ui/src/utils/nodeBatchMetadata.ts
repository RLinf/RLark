import type { CRDNode } from "../types";

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
