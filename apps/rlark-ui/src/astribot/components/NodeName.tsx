import { nodeLabel } from "../lib/displayName.js";

// A Node as the console shows it: its display name, followed by its
// Kubernetes name in muted monospace when the two differ.
export function NodeName({ node }: { node: { metadata: { name: string }; spec?: { displayName?: string } } }) {
  const label = nodeLabel(node);
  return (
    <span className="node-name">
      {label}
      {label !== node.metadata.name && <span className="real-name"> · {node.metadata.name}</span>}
    </span>
  );
}
