import type { ClusterSummary } from "../../types";
import { clusterLabel } from "../lib/displayName.js";
import { clusterForStation } from "../lib/stationDisplay.js";
import { RealName } from "./NodeName.js";

// A namespace's cluster as the console shows it: its display name, followed
// by its ID in muted monospace when the two differ.
export function ClusterName({ clusters, namespace }: { clusters: ClusterSummary[]; namespace: string }) {
  return (
    <span className="cluster-name">
      <RealName label={clusterLabel(clusters, namespace)} name={clusterForStation(clusters, namespace)?.id ?? namespace} />
    </span>
  );
}
