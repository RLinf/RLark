import { nodeLabel } from "../lib/displayName.js";

// A Node as the console shows it: its display name, followed by its
// Kubernetes name in muted monospace when the two differ.
export type NodeRef = { metadata: { name: string }; spec?: { displayName?: string } };

export function NodeName({ node }: { node: NodeRef }) {
  const label = nodeLabel(node);
  return (
    <span className="node-name">
      {label}
      {label !== node.metadata.name && <span className="real-name"> · {node.metadata.name}</span>}
    </span>
  );
}
