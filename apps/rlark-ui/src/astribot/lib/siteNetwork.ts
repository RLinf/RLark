import type { SiteNetworkDiscovery, SiteNetworkView } from "../api.js";

export function validateDiscovery(discovery: SiteNetworkDiscovery, zh: boolean) {
  const ranges = discovery.ranges.map((range) => range.trim()).filter(Boolean);
  if (ranges.length < 1 || ranges.length > 8) return zh ? "网段数量必须为 1 到 8 条。" : "Enter between 1 and 8 ranges.";
  for (const range of ranges) {
    const match = range.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\/(\d{1,2})$/);
    if (!match || match.slice(1, 5).some((octet) => Number(octet) > 255) || Number(match[5]) < 22 || Number(match[5]) > 32) {
      return zh ? `${range} 不是有效的 IPv4 /22 或更小网段。` : `${range} must be a valid IPv4 prefix of /22 or shorter.`;
    }
  }
  if (new Set(ranges).size !== ranges.length) return zh ? "网段不能重复。" : "Ranges must be unique.";
  if (!Number.isInteger(discovery.intervalSeconds) || discovery.intervalSeconds < 10 || discovery.intervalSeconds > 3600) {
    return zh ? "扫描间隔必须在 10 到 3600 秒之间。" : "Interval must be between 10 and 3600 seconds.";
  }
  if (!discovery.from.clusterID || !discovery.from.nodeName) return zh ? "请选择管理面拨号 Hub。" : "Select a management-plane Hub.";
  const ports = discovery.sshPorts ?? [];
  if (ports.length > 4 || ports.some((port) => !Number.isInteger(port) || port < 1 || port > 65535) || new Set(ports).size !== ports.length) {
    return zh ? "SSH 端口必须是 1–65535 的不重复整数，最多 4 个。" : "SSH ports must be unique integers from 1 to 65535, with at most 4 ports.";
  }
  return "";
}

export function isSiteNetworkConflict(error: unknown) {
  return typeof error === "object" && error !== null && "status" in error && error.status === 409;
}

// The site whose SiteNetwork entry lists the cluster; "" when no site lists it.
// Discovery is reported per site, so a cluster's machines are that site's.
export function siteOfCluster(view: SiteNetworkView | undefined, clusterID: string) {
  if (!clusterID) return "";
  return view?.sites.find((site) => site.clusters.includes(clusterID))?.name ?? "";
}
