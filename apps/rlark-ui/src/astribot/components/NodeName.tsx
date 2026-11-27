import { nodeLabel } from "../lib/displayName.js";

// A Node as the console shows it: its display name, followed by its
// Kubernetes name in muted monospace when the two differ.
export type NodeRef = { metadata: { name: string }; spec?: { displayName?: string } };

export function NodeName({ node }: { node: NodeRef }) {
  return (
    <span className="node-name">
      <RealName label={nodeLabel(node)} name={node.metadata.name} />
    </span>
  );
}

// A display name, followed by the real name (cluster ID, Kubernetes name) in
// muted monospace when the two differ.
export function RealName({ label, name }: { label: string; name?: string }) {
  return (
    <>
      {label}
      {label !== name && <span className="real-name"> · {name}</span>}
    </>
  );
}
