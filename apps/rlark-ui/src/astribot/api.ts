// The console's requests to the Gateway routes this fork adds. backend.ts keeps RLark's own.
import { ApiError, request, requestJson, withQuery } from "../api.js";
import type { AdmittedMachine, IssuedNodeAdmission, MachineList, NodeAdmission, NodeClassAccess, RobotPowerDesired, SiteNode } from "./lib/siteNodeDisplay.js";
import type { NodeJoin } from "./lib/nodeJoin.js";
import type { ClusterProvision, NewClusterForm } from "./lib/clusterProvision.js";
import type { CRDNode } from "../types.js";

interface Condition {
  type: string;
  status: string;
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
}

// A page of a Station API list: continue asks for the next.
type Page<T> = { items: T[]; continue?: string };

// A site node: the NetworkEnrollment of the node its name is, as GET /api/v1/station/stations lists it.
export interface StationItem {
  metadata: { name: string; namespace: string };
  spec: {
    nodeUID: string;
    // The node was removed: the record stays until it is deleted or the node is admitted again.
    retired?: boolean;
  };
  status?: {
    // The node's last report; nodeUID and systemUUID are the Node's that made it.
    observation?: { nodeUID: string; systemUUID?: string };
    node?: {
      state?: "Bound" | "Offline" | "Conflict" | "Retired";
      bootTime?: string;
      lastSeenTime?: string;
      reboots?: number;
      conditions?: Condition[];
    };
  };
}

export interface StationRunningImage {
  namespace: string;
  pod: string;
  container: string;
  image: string;
  imageID?: string;
}

export interface NodeOperation {
  // annotations: what a removal's PowerOff stage found (station.rlark.io/power-off).
  metadata: { name: string; namespace: string; annotations?: Record<string, string>; creationTimestamp?: string; uid?: string };
  spec: { action: "Join" | "Remove"; nodeName: string; clusterID?: string; requestID?: string; nodeUID?: string };
  status?: {
    phase?: "Pending" | "Running" | "Blocked" | "Succeeded" | "Failed";
    stage?: string;
    conditions?: Condition[];
  };
}

// A cluster's type, the Cluster's label rlark.io/cluster-type; the Gateway's cluster list and detail (ClusterInfo)
// carry it as clusterType, absent on a cluster whose type was never set. Not the official ClusterInfo.type
// (Cloud / Embodied / Hybrid, from its nodes). A Node's kind stays the official rlark.io/node-category-* label.
export type ClusterType = "gpu" | "robot";
export const clusterTypes: { value: ClusterType; zh: string; en: string }[] = [
  { value: "gpu", zh: "GPU 集群", en: "GPU cluster" },
  { value: "robot", zh: "机器人集群", en: "Robot cluster" },
];
export type WithClusterType = { clusterType?: ClusterType };

export const clusterTypeApi = {
  // An existing cluster's type (administrator); the answer is the cluster as GET /api/v1/clusters/{id} gives it.
  async set<T = unknown>(id: string, clusterType: ClusterType) {
    const response = await requestJson<{ data?: T }>(`/api/v1/clusters/${encodeURIComponent(id)}/cluster-type`, {
      method: "PUT",
      body: { clusterType },
    });
    return response.data;
  },
};

export const clusterProvisionsApi = {
  async list() {
    const response = await requestJson<{ data?: ClusterProvision[] }>("/api/v1/cluster-provisions");
    return response.data ?? [];
  },
  // The new cluster's name, left empty, is rlark-<site>-<clusterType>.
  async create(form: NewClusterForm & { clusterType: ClusterType }) {
    const response = await requestJson<{ data: ClusterProvision }>("/api/v1/cluster-provisions", {
      method: "POST",
      body: {
        type: form.clusterType,
        site: form.site.trim(),
        cluster: form.cluster.trim(),
        address: form.address.trim(),
        port: Number(form.port),
        user: form.user.trim(),
        password: form.password,
        ...(form.tunnelEndpoint.trim() ? { tunnel_endpoint: form.tunnelEndpoint.trim() } : {}),
      },
    });
    return response.data;
  },
  async retry(name: string, password: string, user: string, sudoPassword = "") {
    const response = await requestJson<{ data: ClusterProvision }>(
      `/api/v1/cluster-provisions/${encodeURIComponent(name)}/retry`,
      { method: "POST", body: { password, user: user.trim() } },
    );
    return response.data;
  },
};

export const stationsApi = {
  list(query: Record<string, string | undefined> = {}, signal?: AbortSignal) {
    return requestJson<Page<StationItem>>(withQuery("/api/v1/station/stations", query), { signal });
  },
  runningImages(name: string, clusterID: string, nodeUID: string, signal?: AbortSignal) {
    const path = `/api/v1/station/stations/${encodeURIComponent(name)}/running-images`;
    return requestJson<{ items: StationRunningImage[] }>(withQuery(path, { cluster_id: clusterID, node_uid: nodeUID }), { signal });
  },
  listNodeOperations(clusterID: string, signal?: AbortSignal) {
    return requestJson<Page<NodeOperation>>(withQuery("/api/v1/station/node-operations", { cluster_id: clusterID, limit: 100 }), { signal });
  },
  getNodeOperation(name: string, clusterID: string, signal?: AbortSignal) {
    return requestJson<NodeOperation>(withQuery(`/api/v1/station/node-operations/${encodeURIComponent(name)}`, { cluster_id: clusterID }), { signal });
  },
  // The desired power of a robot's Node, which its Addon converges on; the Gateway records who set it.
  setRobotPower(node: string, namespace: string, desired: RobotPowerDesired) {
    return request(withQuery(`/api/v1/rlinf.io/v1alpha1/nodes/${encodeURIComponent(node)}/robot-power`, { namespace }), { method: "PUT", body: { desired } });
  },
  // Hold-to-move to a pose (初始姿势 is home; administrator or operator): hold about every 200 ms while the button is
  // held, stop when it is let go; the robot stops by itself 0.5 s after the last hold. A refusal is an ApiError of
  // status 409 whose body is RobotPoseRefusal; 502 when the vendor's web server failed or the robot is not reached.
  robotPose(node: string, namespace: string, action: "hold" | "stop", pose: RobotPose = "home", signal?: AbortSignal) {
    return requestJson<RobotPoseAnswer>(withQuery(`/api/v1/rlinf.io/v1alpha1/nodes/${encodeURIComponent(node)}/robot-pose`, { namespace }), {
      method: "POST",
      signal,
      body: { pose, action },
    });
  },
};

export type RobotPose = "home";
export interface RobotPoseAnswer {
  pose: RobotPose;
  moving: boolean;
  // This hold started the movement (the vendor was sent pose_running).
  started: boolean;
  // The vendor's reply to pose_running or pose_stop; null when none was sent.
  vendor: unknown;
}

export interface SiteNetworkMember {
  clusterID: string;
  nodeName: string;
  role: string;
  site: string;
  address: string;
}

export interface SiteNetworkNodeClass {
  name: string;
  hasAccess: boolean;
  hasDiscovery: boolean;
  entryPort?: number;
  hostKeyFingerprints?: string[];
}

export interface SiteNetworkDiscovery {
  enabled: boolean;
  ranges: string[];
  intervalSeconds: number;
  from: { clusterID: string; nodeName: string };
  includeUnclassified?: boolean;
  sshPorts?: number[];
}

export interface SiteNetworkSite {
  name: string;
  clusters: string[];
  discovery?: SiteNetworkDiscovery;
}

export interface SiteNetworkView {
  resourceVersion: string;
  generation: number;
  sites: SiteNetworkSite[];
  members: SiteNetworkMember[];
  nodeClasses: SiteNetworkNodeClass[];
  status: {
    observedGeneration?: number;
    members?: number;
    unreachable?: string[];
    conditions?: Condition[];
  };
}

export interface SiteNetworkDiscoveryInput {
  resourceVersion: string;
  discovery: SiteNetworkDiscovery | null;
}

export const siteNetworkApi = {
  get(signal?: AbortSignal) {
    return requestJson<SiteNetworkView>("/api/v1/station/site-network", { signal });
  },
  updateDiscovery(site: string, input: SiteNetworkDiscoveryInput) {
    return requestJson<SiteNetworkView>(
      `/api/v1/station/site-network/sites/${encodeURIComponent(site)}/discovery`,
      { method: "PUT", body: input },
    );
  },
};

export interface RobotModelItem {
  name: string;
  displayName?: string;
  vendor?: string;
  description?: string;
  references: { nodeClasses: number; nodes: number; machines: number; nodeAdmissions: number; nodeJoins: number };
}

export const robotModelsApi = {
  list(signal?: AbortSignal) {
    return requestJson<{ items: RobotModelItem[] }>("/api/v1/station/robot-models", { signal });
  },
  delete(name: string) {
    return request(`/api/v1/station/robot-models/${encodeURIComponent(name)}`, { method: "DELETE" });
  },
};
export interface RobotPoseRefusal {
  reason: "estop" | "fault" | "not_powered" | "driver_not_running" | "claimed" | "in_use" | "vendor_monitor_down" | `maintenance: ${string}`;
  state: Record<string, unknown>;
}

export const siteNodesApi = {
  list(clusterID: string, signal?: AbortSignal) {
    return requestJson<{ items: SiteNode[] }>(
      withQuery("/api/v1/site-nodes", { cluster_id: clusterID }),
      { signal },
    );
  },
};

// Joins over SSH: RLark logs in to the machine with the account given and joins it in stages, which the NodeJoin
// records; the password is sent with the request that starts or retries a join only.
export const nodeJoinsApi = {
  async list(clusterID: string, signal?: AbortSignal) {
    const response = await requestJson<{ items?: NodeJoin[] }>(withQuery("/api/v1/station/node-joins", { cluster_id: clusterID }), { signal });
    return response.items ?? [];
  },
  async create(input: {
    cluster_id: string;
    machine_name?: string;
    host_key?: string;
    host_key_algorithm?: string;
    machine_type?: "Robot" | "Server";
    machine_model?: string;
    labels?: Record<string, string>;
    address: string;
    port?: number;
    user: string;
    password: string;
    sudo_password?: string;
    node_name?: string;
    tunnel_endpoint?: string;
  }) {
    const response = await requestJson<{ data: NodeJoin }>("/api/v1/station/node-joins", { method: "POST", body: input });
    return response.data;
  },
  async retry(name: string, password: string, user: string, sudoPassword = "") {
    const response = await requestJson<{ data: NodeJoin }>(`/api/v1/station/node-joins/${encodeURIComponent(name)}/retry`, {
      method: "POST",
      body: { password, user: user.trim(), ...(sudoPassword ? { sudo_password: sudoPassword } : {}) },
    });
    return response.data;
  },
  async clear(name: string, password: string, user: string, machineGone: boolean, sudoPassword = "") {
    const response = await requestJson<{ steps?: string[] }>(`/api/v1/station/node-joins/${encodeURIComponent(name)}/clear`, {
      method: "POST",
      body: { password, user: user.trim(), machine_gone: machineGone, ...(sudoPassword ? { sudo_password: sudoPassword } : {}) },
    });
    return response.steps ?? [];
  },
};

// Join codes. The code is in the answer of issue and reissue only; the
// management plane keeps its hash.
export const nodeAdmissionsApi = {
  list(clusterID: string, signal?: AbortSignal) {
    return requestJson<{ items: NodeAdmission[] }>(
      withQuery("/api/v1/station/node-admissions", { cluster_id: clusterID }),
      { signal },
    );
  },
  issue(input: {
    cluster_id: string;
    node_name: string;
    site?: string;
    address?: string;
    uplink?: string;
    node_class?: string;
    machine?: { category: "Robot" | "Server"; model?: string; body?: string };
    labels?: Record<string, string>;
    taints?: string[];
    ttl_seconds?: number;
  }) {
    return requestJson<IssuedNodeAdmission>("/api/v1/station/node-admissions", {
      method: "POST",
      body: input,
    });
  },
  reissue(name: string, clusterID: string) {
    return requestJson<IssuedNodeAdmission>(
      withQuery(`/api/v1/station/node-admissions/${encodeURIComponent(name)}/reissue`, { cluster_id: clusterID }),
      { method: "POST", body: {} },
    );
  },
  revoke(name: string, clusterID: string) {
    return requestJson<{ admission: NodeAdmission }>(
      withQuery(`/api/v1/station/node-admissions/${encodeURIComponent(name)}/revoke`, { cluster_id: clusterID }),
      { method: "POST" },
    );
  },
};

export const machinesApi = {
  // site scopes the list to the machines that site's hubs found; empty lists every site's.
  list(site = "", signal?: AbortSignal) {
    return requestJson<MachineList>(withQuery("/api/v1/station/machines", { site }), { signal });
  },
  admit(input: { cluster_id: string; machines: string[]; node_class?: string; machine_type?: "Robot" | "Server"; machine_model?: string; model?: string; labels?: Record<string, string> }) {
    return requestJson<{ machines: AdmittedMachine[] }>("/api/v1/station/machines/admit", {
      method: "POST",
      body: input,
    });
  },
  // The node's passwords of a NodeClass: written, never read back.
  setClassAccess(nodeClass: string, input: { node_password: string; sudo_password?: string }) {
    return requestJson<NodeClassAccess>(`/api/v1/station/node-classes/${encodeURIComponent(nodeClass)}/access`, {
      method: "PUT",
      body: input,
    });
  },
};

// Joining again and starting over (astribot/docs/install-join.md): a node removed by its name, a cluster's
// making cleared, a site node's record deleted.
export const selfHealApi = {
  // Removes whatever is under the name: the Node its cluster has (nodeUID names the instance RLark verified, when
  // it has one), or, when its cluster has none, what RLark keeps of the name.
  removeNode(clusterID: string, nodeName: string, requestID: string, nodeUID?: string) {
    return requestJson<NodeOperation>("/api/v1/station/node-operations", {
      method: "POST",
      body: { cluster_id: clusterID, request_id: requestID, action: "Remove", node_name: nodeName, ...(nodeUID ? { node_uid: nodeUID } : {}) },
    });
  },
  // Takes away a making that stopped, or what the making of a deleted cluster left; answers what it did.
  async clearClusterProvision(name: string, password: string, user: string, machineGone: boolean) {
    const response = await requestJson<{ steps?: string[] }>(
      `/api/v1/cluster-provisions/${encodeURIComponent(name)}/clear`,
      { method: "POST", body: { password, user: user.trim(), machine_gone: machineGone } },
    );
    return response.steps ?? [];
  },
  // Deletes the record of a removed node, of a node in Conflict, or of a name no node reports under and RLark has
  // no Node of.
  deleteStation(name: string, clusterID: string) {
    return request(withQuery(`/api/v1/station/stations/${encodeURIComponent(name)}`, { cluster_id: clusterID }), { method: "DELETE" });
  },
};

/** One tag with image metadata read by the Gateway; `error` marks unreadable metadata. */
export interface RegistryTagDetail {
  tag: string;
  digest?: string;
  created?: string;
  size?: number;
  platforms?: string[];
  error?: "unavailable";
}

export interface RegistryTagDetailPage {
  items: RegistryTagDetail[];
  next: string;
}

// What a failed read of a registry says to the person browsing it.
export function registryError(error: unknown, zh: boolean): string {
  if (error instanceof ApiError && error.status === 403) {
    return zh ? "浏览镜像仓库需要管理员权限。" : "Registry browsing requires administrator access.";
  }
  return zh
    ? "读取仓库失败，请检查 Gateway 白名单、网络、证书和仓库认证后重试。"
    : "Cannot read the registry. Check the Gateway allowlist, network, certificate and registry credentials, then retry.";
}

// Display names: an empty one clears it; a cluster's ID and a Node's metadata.name never change.
export const displayNameApi = {
  async cluster<T = unknown>(id: string, displayName: string) {
    const response = await requestJson<{ data?: T }>(`/api/v1/clusters/${encodeURIComponent(id)}/display-name`, {
      method: "PUT",
      body: { displayName },
    });
    return response.data;
  },
  node(name: string, namespace: string, displayName: string) {
    return requestJson<CRDNode>(withQuery(`/api/v1/rlinf.io/v1alpha1/nodes/${encodeURIComponent(name)}/display-name`, { namespace }), {
      method: "PUT",
      body: { displayName },
    });
  },
};

// Read-only browsing of a configured image registry through the Gateway: its password, repositories and tags; pages continue from `next` as `last`.
export const registryBrowseApi = {
  password(id: string, signal?: AbortSignal) {
    return requestJson<{ password: string }>(`/api/v1/image-registries/${encodeURIComponent(id)}/password`, { signal });
  },
  repositories(id: string, last = "", signal?: AbortSignal) {
    return requestJson<{ items: string[]; next: string }>(withQuery(`/api/v1/image-registries/${encodeURIComponent(id)}/repositories`, { last }), { signal });
  },
  tags(id: string, repository: string, last = "", signal?: AbortSignal) {
    return requestJson<{ items: string[]; next: string }>(withQuery(`/api/v1/image-registries/${encodeURIComponent(id)}/tags`, { repository, last }), { signal });
  },
  tagDetails(id: string, repository: string, last = "", signal?: AbortSignal) {
    return requestJson<RegistryTagDetailPage>(withQuery(`/api/v1/image-registries/${encodeURIComponent(id)}/tag-details`, { repository, last }), { signal });
  },
};
