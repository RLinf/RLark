import type { ClusterSummary } from "../../types";
import { clusterLabel } from "../lib/displayName.js";
import { clusterForStation } from "../lib/stationDisplay.js";

// A namespace's cluster as the console shows it: its display name, followed
// by its ID in muted monospace when the two differ.
export function ClusterName({ clusters, namespace }: { clusters: ClusterSummary[]; namespace: string }) {
  const label = clusterLabel(clusters, namespace);
  const id = clusterForStation(clusters, namespace)?.id ?? namespace;
  return (
    <span className="cluster-name">
      {label}
      {label !== id && <span className="real-name"> · {id}</span>}
    </span>
  );
}
