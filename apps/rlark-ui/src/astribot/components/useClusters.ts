import { useEffect, useState } from "react";
import { clustersApi } from "../../backend";
import type { ClusterSummary } from "../../types";

// The clusters as the Gateway lists them, read once per mount: what
// clusterLabel shows a namespace as.
export function useClusters() {
  const [clusters, setClusters] = useState<ClusterSummary[]>([]);
  useEffect(() => {
    clustersApi
      .list<ClusterSummary>()
      .then(setClusters)
      .catch(() => {});
  }, []);
  return clusters;
}
