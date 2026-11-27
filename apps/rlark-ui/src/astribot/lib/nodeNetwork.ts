import type { CRDNode } from "../../types";

// What the node pages show of a node's network: its WireGuard addresses as
// station-agent last reported them (GET /api/v1/node-networks/{name};
// absent when nothing was reported, unavailable when they cannot be read),
// and its site-network view from GET /api/v1/site-nodes (absent for a node
// that is not in the site network).
export interface NodeNetworkDisplay {
  wireguard?: {
    state: "current" | "stale" | "unavailable";
    interface?: string;
    addresses?: string[];
    reportedAt?: string;
  };
  site?: SiteNodeLinks;
}

// What each Hub a Leaf dials sees of it (NetworkEnrollment status.links):
// Unanswered = the Leaf's requests reach the Hub and no session forms,
// Silent = nothing reaches the Hub, Unknown = the Hub has no recent report.
export interface SiteNodeLink {
  hub: string;
  state: "Connected" | "Unanswered" | "Silent" | "Unknown";
  since: string;
  latest_handshake?: string;
}

export interface SiteNodeLinks {
  online: boolean;
  links: SiteNodeLink[];
  // Empty while every link is Connected.
  summary: string;
}

// down: a Hub hears the node badly or not at all, or the node's heartbeat
// stopped; unknown: no Hub has a recent report; connected otherwise.
export function nodeLinkHealth(site: SiteNodeLinks): "connected" | "down" | "unknown" {
  if (!site.online || site.links.some((link) => link.state === "Unanswered" || link.state === "Silent"))
    return "down";
  if (site.links.some((link) => link.state === "Unknown")) return "unknown";
  return "connected";
}

export function nodeNetworkKey(node: CRDNode): string {
  return `${node.metadata.namespace ?? ""}/${node.metadata.name}`;
}

export function nodeNetworkModel(node: CRDNode, network?: NodeNetworkDisplay) {
  const primary =
    node.status?.addresses?.find((item) => item.type === "InternalIP")?.address ?? node.status?.addresses?.[0]?.address;
  const addresses: Array<{ kind: "node" | "wg"; address: string; interface?: string; cidr?: string }> = primary
    ? [{ kind: "node", address: primary }]
    : [];
  const wg = network?.wireguard;
  if (wg && wg.state !== "unavailable") {
    for (const cidr of new Set((wg.addresses ?? []).map((raw) => raw.trim()))) {
      const address = cidr.split("/")[0];
      if (address) addresses.push({ kind: "wg", address, interface: wg.interface, cidr });
    }
  }
  const message: "unreported" | "stale" | "unavailable" | null =
    wg?.state === "unavailable" || wg?.state === "stale"
      ? wg.state
      : addresses.some((entry) => entry.kind === "wg")
        ? null
        : "unreported";
  return { addresses, message, reportedAt: wg?.reportedAt };
}

export function nodeNetworkSearchText(node: CRDNode, network?: NodeNetworkDisplay): string {
  return nodeNetworkModel(node, network)
    .addresses.map((entry) => `${entry.address} ${entry.cidr ?? ""}`)
    .join(" ");
}

// One short line for a list row: the worst state first, the Hubs it names.
export function nodeLinkLine(site: SiteNodeLinks, zh: boolean): string {
  const hubs = (state: SiteNodeLink["state"]) =>
    site.links.filter((link) => link.state === state).map((link) => link.hub.split("/")[1] ?? link.hub).join(zh ? "、" : ", ");
  if (!site.online) return zh ? "心跳中断" : "Heartbeat lost";
  const silent = hubs("Silent");
  if (silent) return zh ? `${silent} 收不到它的包` : `Nothing arrives at ${silent}`;
  const unanswered = hubs("Unanswered");
  if (unanswered) return zh ? `请求到了 ${unanswered}，回包回不来` : `Requests reach ${unanswered}, answers do not get back`;
  const unknown = hubs("Unknown");
  if (unknown) return zh ? `${unknown} 没有近况` : `No current report from ${unknown}`;
  if (!site.links.length) return zh ? "在线" : "Online";
  return zh ? `${site.links.length} 个 Hub 都已连通` : `Connected to all ${site.links.length} hubs`;
}
