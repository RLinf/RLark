import { useEffect, useMemo, useState } from "react";
import { requestJson, withQuery } from "../../api";
import { clustersApi } from "../../backend";
import { siteNodesApi } from "../api";
import type { ClusterSummary, CRDNode } from "../../types";
import { forEachLimited } from "../lib/forEachLimited";
import type { NodeNetworkDisplay, SiteNodeLinks } from "../lib/nodeNetwork";
import { clusterForStation } from "../lib/stationDisplay";

type Summary = Omit<NonNullable<NodeNetworkDisplay["wireguard"]>, "state"> & { state: "current" | "stale" | "unreported" };

const unavailable: NodeNetworkDisplay = { wireguard: { state: "unavailable" } };

// The networks of the nodes, read again every 30 s. Every node gets an entry once read, so a page tells "read,
// nothing reported" from "not read yet"; a node whose cluster or report cannot be read is unavailable, and a
// cluster whose Station API does not answer shows no link state.
export function useNodeNetworks(nodes: CRDNode[]) {
  const [networks, setNetworks] = useState<Record<string, NodeNetworkDisplay>>({});
  const keys = useMemo(
    () => JSON.stringify(nodes.map((node) => [node.metadata.namespace ?? "", node.metadata.name]).sort()),
    [nodes],
  );
  useEffect(() => {
    const targets = JSON.parse(keys) as Array<[string, string]>;
    const controller = new AbortController();
    const { signal } = controller;
    let timer = 0;
    async function refresh() {
      const clusters = await clustersApi.list<ClusterSummary>().catch(() => []);
      const clusterOf = (namespace: string) => clusterForStation(clusters, namespace)?.id;
      const next: Record<string, NodeNetworkDisplay> = {};
      const sites: Record<string, SiteNodeLinks> = {};
      const read = async ([namespace, name]: [string, string]) => {
        const clusterID = clusterOf(namespace);
        next[`${namespace}/${name}`] = clusterID
          ? await requestJson<Summary>(withQuery(`/api/v1/node-networks/${encodeURIComponent(name)}`, { cluster_id: clusterID }), { signal }).then(
              (summary) => (summary.state === "unreported" ? {} : { wireguard: summary as NodeNetworkDisplay["wireguard"] }),
              () => unavailable,
            )
          : unavailable;
      };
      const readSites = async (namespace: string) => {
        const clusterID = clusterOf(namespace);
        if (!clusterID) return;
        const { items } = await siteNodesApi.list(clusterID, signal).catch(() => ({ items: [] }));
        for (const item of items)
          sites[`${namespace}/${item.node_name}`] = { online: item.online, links: item.network?.links ?? [], summary: item.network?.link_summary ?? "" };
      };
      await Promise.all([forEachLimited(targets, 6, signal, read), ...[...new Set(targets.map(([namespace]) => namespace))].map(readSites)]);
      for (const [key, site] of Object.entries(sites)) next[key] = { ...next[key], site };
      if (!signal.aborted) {
        setNetworks(next);
        timer = window.setTimeout(refresh, 30000);
      }
    }
    setNetworks({});
    void refresh();
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [keys]);
  return networks;
}
