export const NODE_CATEGORY_KEYS = [
  "rlark.io/node-category",
  "rlark.io/node-category-cloud",
  "rlark.io/node-category-edge",
  "rlark.io/node-category-robot",
] as const;

export function getRemovedLabelKeys(
  original: Record<string, string>,
  next: Record<string, string>,
) {
  return Object.keys(original).filter((key) => !(key in next));
}
