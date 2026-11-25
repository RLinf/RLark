// The mock console's answers to the Gateway routes this fork adds or changes. mockBackend.ts asks
// astribotMockRoute first and adds the fields of our storages to the ones it saves.
import type { CRDNode } from "../types.js";
import { displayNameError } from "./lib/displayName.js";
import type { StorageClass } from "./lib/storageClass.js";
import { mockAdmissions, mockSiteNodes, mockStationCameraFrame, mockStationCameraReport, mockStations } from "./mockStations.js";

// The demo storages: RLark's two and an SMB share.
export const mockStorageClasses: StorageClass[] = [
  {
    id: "training-datasets",
    name: "training-datasets",
    namespace: "default",
    clusters: ["cloud-east-a"],
    type: "s3",
    provider: "MinIO",
    endpoint: "https://minio.mock.local",
    region: "local",
    bucket: "rlark-training",
    accessKeyId: "mock-access-key",
    pathStyle: true,
    host: "",
    share: "",
    subPath: "",
    username: "",
    domain: "",
    description: "用于训练数据集的 Mock 对象存储",
    createdAt: "2026-08-01T08:00:00Z",
  },
  {
    id: "evaluation-results",
    name: "evaluation-results",
    namespace: "default",
    clusters: ["cloud-east-a"],
    type: "s3",
    provider: "AWS S3",
    endpoint: "https://s3.mock.local",
    region: "cn-east-1",
    bucket: "rlark-evaluation",
    accessKeyId: "mock-access-key",
    pathStyle: false,
    host: "",
    share: "",
    subPath: "",
    username: "",
    domain: "",
    description: "用于评估产物的 Mock 对象存储",
    createdAt: "2026-08-02T08:00:00Z",
  },
  {
    id: "lab-nas",
    name: "lab-nas",
    namespace: "default",
    clusters: ["cloud-east-a"],
    type: "smb",
    provider: "",
    endpoint: "",
    region: "",
    bucket: "",
    accessKeyId: "",
    pathStyle: false,
    host: "10.0.0.10",
    share: "shared",
    subPath: "datasets",
    username: "mock-user",
    domain: "",
    description: "用于共享数据集的 Mock SMB 存储",
    createdAt: "2026-08-03T08:00:00Z",
  },
];

// The fields of our storages in a created or updated one, from the request body.
export function mockStorageFields(payload: Record<string, any>) {
  return {
    type: payload.type === "smb" ? "smb" : "s3",
    provider: payload.type === "smb" ? "" : payload.provider || "MinIO",
    mount_endpoint: payload.mount_endpoint || "",
    prefix: payload.prefix || "",
    mount_profile: payload.mount_profile || "default",
    mount_options: payload.mount_options || [],
    vfs_cache_mode: payload.vfs_cache_mode === "off" ? "off" : "writes",
    host: payload.host || "",
    share: payload.share || "",
    subPath: payload.sub_path || payload.subPath || "",
    username: payload.username || "",
    domain: payload.domain || "",
  };
}

// A display name set through PUT …/display-name, by cluster ID (the demo clusters' names).
const clusterDisplayNames = new Map<string, string>();

// A cluster as the Gateway lists it: its display name as its name when one is set.
export function withMockDisplayName<T extends { id: string }>(cluster: T) {
  const displayName = clusterDisplayNames.get(cluster.id);
  return { ...cluster, name: displayName || cluster.id, displayName, agentNamespace: cluster.id };
}

const mockClusterProvisions = [
  {
    apiVersion: "rlinf.io/v1alpha1",
    kind: "ClusterProvision",
    metadata: { name: "rlark-baidu-gpu" },
    spec: { site: "baidu", machine: { address: "1.2.3.4", port: 22, user: "root" } },
    status: {
      phase: "Failed",
      stage: "control-plane",
      stages: [
        { name: "precheck", state: "Done", readback: "bd-gpu-1, Ubuntu 22.04.5 LTS, kernel 6.8.0; 8 × NVIDIA RTX PRO 6000; /var/lib 3500 GiB free; private address 192.168.0.10" },
        { name: "bundle", state: "Done", readback: "installer sha256 f8528e6d0aa1…; tunnel public key K1aR…=" },
        { name: "declare", state: "Done", readback: "site baidu; cp.rlark-baidu-gpu.rlark.internal → 10.254.240.20; Pod 10.200.0.0/16, Service 10.201.0.0/16" },
        { name: "control-plane", state: "Failed", message: "the management plane does not reach the API at 10.254.240.20:6443 through the tunnel (is UDP 51820 open to 1.2.3.4?)" },
      ],
    },
  },
];

const mockSiteNetwork = {
  resourceVersion: "mock-rv-12", generation: 12,
  sites: [
    { name: "office", clusters: ["rlark-local-dev-gpu"], discovery: { enabled: false, ranges: ["10.11.12.0/22"], intervalSeconds: 30, from: { clusterID: "rlark-local-dev-gpu", nodeName: "cailiang" } } },
    { name: "lab", clusters: ["rlark-robot-s1u-01"], discovery: { enabled: true, ranges: ["192.168.31.0/24", "10.11.12.0/22"], intervalSeconds: 30, from: { clusterID: "rlark-local-dev-gpu", nodeName: "cailiang" } } },
    { name: "ksyun", clusters: ["rlark-ksyun-gpu"] },
  ],
  members: [
    { clusterID: "rlark-local-dev-gpu", nodeName: "cailiang", role: "Hub", site: "office", address: "10.254.240.1" },
    { clusterID: "rlark-local-dev-gpu", nodeName: "wendychen", role: "Hub", site: "office", address: "10.254.240.6" },
    { clusterID: "rlark-robot-s1u-01", nodeName: "alexfu", role: "Hub", site: "lab", address: "10.254.240.5" },
    { clusterID: "rlark-ksyun-gpu", nodeName: "10-0-2-57", role: "Hub", site: "ksyun", address: "10.254.240.2" },
  ],
  nodeClasses: [{ name: "s1u", hasAccess: true, hasDiscovery: true, entryPort: 22, hostKeyFingerprints: ["SHA256:mock-factory-key"] }],
  status: { observedGeneration: 11, members: 4, unreachable: [], conditions: [
    { type: "Ready", status: "True", reason: "Computed", message: "" },
    { type: "FullyConnected", status: "True", reason: "AllPairsReachable", message: "" },
    { type: "DiscoveryStale", status: "False", reason: "HubsReport", message: "" },
  ] },
};

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

const refusal = (error: { status: number; message: string }) =>
  json({ code: error.status === 409 ? "RESOURCE_CONFLICT" : "INVALID_REQUEST", message: error.message }, error.status);

// The mock's state these routes read and change.
export interface MockState {
  clusterPayload: () => Array<{ id: string; displayName?: string }>;
  nodes: CRDNode[];
  imageRegistries: Array<{ id: string }>;
}

// The answer to a request on one of our routes; undefined for RLark's own.
export async function astribotMockRoute(method: string, path: string, request: Request, state: MockState) {
  if (method === "GET" && path === "/api/v1/station/site-network") return json(mockSiteNetwork);
  const discoveryRoute = path.match(/^\/api\/v1\/station\/site-network\/sites\/([^/]+)\/discovery$/);
  if (method === "PUT" && discoveryRoute) {
    const body = await request.json();
    if (body.resourceVersion !== mockSiteNetwork.resourceVersion) return json({ code: "RESOURCE_CONFLICT", message: "The site network changed meanwhile." }, 409);
    const siteName = decodeURIComponent(discoveryRoute[1]);
    const site = mockSiteNetwork.sites.find((item) => item.name === siteName);
    if (!site) return json({ code: "RESOURCE_NOT_FOUND", message: "site not found" }, 404);
    site.discovery = body.discovery;
    mockSiteNetwork.resourceVersion = `mock-rv-${Number(mockSiteNetwork.resourceVersion.split("-").at(-1)) + 1}`;
    mockSiteNetwork.generation += 1;
    return json({ resourceVersion: mockSiteNetwork.resourceVersion, site });
  }
  // The site-node page's fixtures: the same shapes the Gateway serves, so the real cards and detail render.
  const query = new URL(request.url).searchParams;
  if (method === "GET" && path === "/api/v1/station/stations") return json({ items: mockStations });
  if (method === "GET" && path === "/api/v1/site-nodes") {
    const cluster = query.get("cluster_id");
    return json({ items: mockSiteNodes.filter((node) => !cluster || node.cluster_id === cluster) });
  }
  if (method === "GET" && path === "/api/v1/station/node-admissions") {
    const cluster = query.get("cluster_id");
    return json({ items: mockAdmissions.filter((item) => !cluster || item.cluster_id === cluster) });
  }
  if (method === "GET" && path === "/api/v1/station/node-operations") return json({ items: [] });
  const cameraReport = path.match(/^\/api\/v1\/rlinf\.io\/v1alpha1\/nodes\/([^/]+)\/cameras$/);
  if (method === "GET" && cameraReport) return json(mockStationCameraReport(decodeURIComponent(cameraReport[1])));
  const cameraFrame = path.match(/^\/api\/v1\/rlinf\.io\/v1alpha1\/nodes\/[^/]+\/cameras\/([^/]+)\.jpg$/);
  if (method === "GET" && cameraFrame)
    return new Response(mockStationCameraFrame(cameraFrame[1]), { headers: { "Content-Type": "image/svg+xml" } });
  const admissionAction = path.match(/^\/api\/v1\/station\/node-admissions\/([^/]+)\/(reissue|revoke)$/);
  if (method === "POST" && admissionAction) {
    const name = decodeURIComponent(admissionAction[1]);
    const cluster = query.get("cluster_id") ?? "";
    if (admissionAction[2] === "revoke") return json({ admission: { cluster_id: cluster, node_name: name, state: "Expired", expires_at: new Date().toISOString() } });
    return json({ node_name: name, cluster_id: cluster, code: "MOCK-JOIN-CODE", expires_at: new Date(Date.now() + 86400_000).toISOString(), public_key: "", tunnel_address: "" });
  }
  if (method === "DELETE" && /^\/api\/v1\/station\/stations\/[^/]+$/.test(path)) return json({});
  if (method === "PUT" && /\/robot-power$/.test(path)) return json({});
  if (method === "POST" && /\/robot-pose$/.test(path)) return json({});
  if (path.startsWith("/api/v1/station/") || path.startsWith("/api/v1/site-nodes"))
    return json({ error: "Station is unavailable in mock mode" }, 503);
  if (method === "GET" && path === "/api/v1/cluster-provisions")
    return json({ success: true, data: mockClusterProvisions });
  if (method === "PUT" && /^\/api\/v1\/clusters\/[^/]+\/display-name$/.test(path)) {
    const id = decodeURIComponent(path.split("/")[4]);
    const clusters = state.clusterPayload();
    if (!clusters.some((cluster) => cluster.id === id))
      return json({ code: "RESOURCE_NOT_FOUND", message: "not found" }, 404);
    const { displayName = "" } = await request.json();
    const error = displayNameError(
      displayName,
      id,
      clusters.map((cluster) => ({ id: cluster.id, name: cluster.id, displayName: cluster.displayName })),
    );
    if (error) return refusal(error);
    if (displayName.trim()) clusterDisplayNames.set(id, displayName.trim());
    else clusterDisplayNames.delete(id);
    return json({ data: state.clusterPayload().find((cluster) => cluster.id === id) });
  }
  if (method === "PUT" && /^\/api\/v1\/rlinf\.io\/v1alpha1\/nodes\/[^/]+\/display-name$/.test(path)) {
    const name = decodeURIComponent(path.split("/")[6]);
    const node = state.nodes.find((item) => item.metadata.name === name);
    if (!node) return json({ code: "RESOURCE_NOT_FOUND", message: "not found" }, 404);
    const { displayName = "" } = await request.json();
    const error = displayNameError(
      displayName,
      `${node.metadata.namespace}/${name}`,
      state.nodes.map((item) => ({
        id: `${item.metadata.namespace}/${item.metadata.name}`,
        name: item.metadata.name,
        displayName: item.spec.displayName,
      })),
    );
    if (error) return refusal(error);
    node.spec.displayName = displayName.trim() || undefined;
    return json(node);
  }
  const registry = path.match(/^\/api\/v1\/image-registries\/([^/]+)\/(password|repositories|tags|manifest)$/);
  if (method === "GET" && registry) {
    if (!state.imageRegistries.some((item) => item.id === decodeURIComponent(registry[1])))
      return json({ code: "RESOURCE_NOT_FOUND", message: "not found" }, 404);
    if (registry[2] === "password") return json({ password: "example-password" });
    if (registry[2] === "manifest") return json({ digest: `sha256:${"a".repeat(64)}` });
    return json({ items: [registry[2] === "tags" ? "demo" : "example/image"], next: "" });
  }
  return undefined;
}
