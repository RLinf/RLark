// The mock console's answers to the Gateway routes this fork adds or changes. mockBackend.ts asks
// astribotMockRoute first and adds the fields of our storages to the ones it saves.
import type { CRDNode } from "../types.js";
import { displayNameError } from "./lib/displayName.js";
import type { StorageClass } from "./lib/storageClass.js";

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
