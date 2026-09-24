import { request, requestJson, withQuery, type LoginResponse } from "./api.js";
import type {
  AgentCertListItem,
  CRDDomain,
  CRDJob,
  CRDNode,
  CRDPod,
  CRDTask,
  CRDWorkflow,
  SignAgentCertResponse,
} from "./types.js";

type ItemList<T> = { items?: T[] };
type DataResponse<T> = { data?: T };
const crdRoot = "/api/v1/rlinf.io/v1alpha1";
const resourcePath = (resource: string, name?: string) =>
  `${crdRoot}/${resource}${name ? `/${encodeURIComponent(name)}` : ""}`;

export interface SystemConfigResponse {
  ssh?: { jumpHost?: string; jumpPort?: string };
  sshJumpHost?: string;
  sshJumpPort?: string;
  log?: {
    backend?: string;
    config?: {
      endpoint?: string;
      project?: string;
      logstore?: string;
      accessKeyId?: string;
      accessKeySecret?: string;
    };
  };
  deployment?: DeploymentConfig;
}

export interface DeploymentConfig {
  apiVersion?: string;
  kind?: string;
  plane?: "data";
  controlPlaneAddress?: string;
  sshAddress?: string;
  insecureSkipTlsVerify?: boolean;
  kubernetes?: {
    kubeconfig?: string;
    agentImage?: string;
    image?: string;
    imagePullPolicy?: "" | "Always" | "IfNotPresent" | "Never";
    imagePullSecrets?: string[];
    containerdSocket?: string;
  };
}

let systemConfigCache: SystemConfigResponse | undefined;
let systemConfigRequest: Promise<SystemConfigResponse> | undefined;

export interface SSHKeyItem {
  index: number;
  user: string;
  public_key: string;
  added_at: string;
}

export interface ImageRegistryItem {
  id: string;
  name: string;
  registry: string;
  username: string;
  clusterSelection: {
    mode: "None" | "Selected" | "All";
    clusters: string[];
  };
}

export interface LocalizedText {
  zh: string;
  en: string;
}

export interface ApiReferenceEndpoint {
  method: string;
  path: string;
  description: LocalizedText;
  example: unknown;
}

export interface ApiReferenceSection {
  id: string;
  title: LocalizedText;
  description: LocalizedText;
  endpoints: ApiReferenceEndpoint[] | null;
}

export interface ApiReferenceResponse {
  title: LocalizedText;
  description: LocalizedText;
  sections: ApiReferenceSection[];
}

export const authApi = {
  login(username: string, password: string) {
    return requestJson<LoginResponse>("/api/v1/auth/login", {
      method: "POST",
      body: { username, password },
    });
  },
};

export const apiReferenceApi = {
  get() {
    return requestJson<ApiReferenceResponse>("/api/v1/api-reference");
  },
};

export const clustersApi = {
  async list<T = unknown>() {
    const response = await requestJson<DataResponse<T[]>>("/api/v1/clusters");
    return response.data ?? [];
  },
  async get<T = unknown>(id: string) {
    const response = await requestJson<DataResponse<T>>(
      `/api/v1/clusters/${encodeURIComponent(id)}`,
    );
    return response.data;
  },
};

function resourceApi<T>(resource: string) {
  return {
    async list(query: Record<string, string | undefined> = {}) {
      const response = await requestJson<ItemList<T>>(
        withQuery(resourcePath(resource), query),
      );
      return response.items ?? [];
    },
    get(name: string, query: Record<string, string | undefined> = {}) {
      return requestJson<T>(withQuery(resourcePath(resource, name), query));
    },
    create(body: object) {
      return requestJson<T>(resourcePath(resource), { method: "POST", body });
    },
    replace(name: string, body: object) {
      return requestJson<T>(resourcePath(resource, name), {
        method: "PUT",
        body,
      });
    },
    patch(name: string, body: object, namespace?: string) {
      return requestJson<T>(
        withQuery(resourcePath(resource, name), { namespace }),
        { method: "PATCH", body },
      );
    },
    remove(name: string) {
      return request(resourcePath(resource, name), { method: "DELETE" });
    },
  };
}

export const nodesApi = resourceApi<CRDNode>("nodes");
export const tasksApi = resourceApi<CRDTask>("tasks");
export const podsApi = {
  ...resourceApi<CRDPod>("pods"),
  events<T = unknown>(name: string) {
    return requestJson<T>(`${resourcePath("pods", name)}/events`);
  },
};
export const domainsApi = resourceApi<CRDDomain>("domains");
export const workflowsApi = {
  ...resourceApi<CRDWorkflow>("workflows"),
  setStopped(name: string, stopped: boolean) {
    return request(resourcePath("workflows", name), {
      method: "PATCH",
      body: { spec: { stopped } },
    });
  },
};

export const jobsApi = {
  ...resourceApi<CRDJob>("jobs"),
  async listTags<T = { key: string; values: string[] }>() {
    const response = await requestJson<ItemList<T>>(
      `${resourcePath("jobs")}/tags`,
    );
    return response.items ?? [];
  },
  setStopped(name: string, stopped: boolean) {
    return request(resourcePath("jobs", name), {
      method: "PATCH",
      body: { spec: { stopped } },
    });
  },
  logs<T>(name: string, query: Record<string, string | undefined>) {
    return requestJson<T>(
      withQuery(`${resourcePath("jobs", name)}/logs`, query),
    );
  },
  logLabelValues<T>(name: string, query: Record<string, string | undefined>) {
    return requestJson<T>(
      withQuery(`${resourcePath("jobs", name)}/logs/label-values`, query),
    );
  },
};

export const systemConfigApi = {
  get(options: { refresh?: boolean } = {}) {
    if (!options.refresh && systemConfigCache) {
      return Promise.resolve(systemConfigCache);
    }
    if (!options.refresh && systemConfigRequest) return systemConfigRequest;

    const pending = requestJson<SystemConfigResponse>("/api/v1/system-config")
      .then((config) => {
        systemConfigCache = config;
        return config;
      })
      .finally(() => {
        if (systemConfigRequest === pending) systemConfigRequest = undefined;
      });
    systemConfigRequest = pending;
    return pending;
  },
  async update(body: object) {
    const config = await requestJson<SystemConfigResponse>(
      "/api/v1/system-config",
      { method: "PUT", body },
    );
    systemConfigCache = config;
    return config;
  },
};

export const sshKeysApi = {
  list(signal?: AbortSignal) {
    return requestJson<SSHKeyItem[]>("/api/v1/ssh-user-keys", { signal });
  },
  create(user: string, publicKey: string) {
    return request("/api/v1/ssh-user-keys", {
      method: "POST",
      body: { user, public_key: publicKey },
    });
  },
  remove(user: string, index: number) {
    return request(withQuery(`/api/v1/ssh-user-keys/${index}`, { user }), {
      method: "DELETE",
    });
  },
};

export const certificatesApi = {
  list() {
    return requestJson<AgentCertListItem[]>("/api/v1/certificates/agent");
  },
  sign(clusterId: string) {
    return requestJson<SignAgentCertResponse>("/api/v1/certificates/agent", {
      method: "POST",
      body: { cluster_id: clusterId },
    });
  },
  get(clusterId: string) {
    return requestJson<SignAgentCertResponse>(
      `/api/v1/certificates/agent/${encodeURIComponent(clusterId)}`,
    );
  },
};

export const imagesApi = {
  async list<T = unknown>() {
    const response = await requestJson<ItemList<T>>("/api/v1/images");
    return response.items ?? [];
  },
};

export const imageRegistriesApi = {
  list() {
    return requestJson<ImageRegistryItem[]>("/api/v1/image-registries");
  },
  create(body: object) {
    return request("/api/v1/image-registries", { method: "POST", body });
  },
  update(id: string, body: object) {
    return request(`/api/v1/image-registries/${encodeURIComponent(id)}`, {
      method: "PUT",
      body,
    });
  },
  remove(id: string) {
    return request(`/api/v1/image-registries/${encodeURIComponent(id)}`, {
      method: "DELETE",
    });
  },
};

export const storageClassesApi = {
  async list<T = unknown>(cluster?: string) {
    const response = await requestJson<DataResponse<Record<string, T>>>(
      withQuery("/api/v1/storage/storageclass", { clusters: cluster }),
    );
    return response.data ?? {};
  },
  create(body: object) {
    return request("/api/v1/storage/storageclass", { method: "POST", body });
  },
  update(name: string, body: object) {
    return request(`/api/v1/storage/storageclass/${encodeURIComponent(name)}`, {
      method: "PUT",
      body,
    });
  },
  remove(name: string) {
    return request(`/api/v1/storage/storageclass/${encodeURIComponent(name)}`, {
      method: "DELETE",
    });
  },
};

const storageObjectPath = (storageClass: string, cluster: string) =>
  `/api/v1/storage/storageclass/${encodeURIComponent(storageClass)}/${encodeURIComponent(cluster)}`;

export const storageObjectsApi = {
  list<T>(storageClass: string, cluster: string, prefix: string) {
    return requestJson<T>(
      withQuery(`${storageObjectPath(storageClass, cluster)}/list`, {
        prefix,
        maxKeys: 100,
      }),
    );
  },
  upload(storageClass: string, cluster: string, formData: FormData) {
    return request(`${storageObjectPath(storageClass, cluster)}/upload`, {
      method: "POST",
      body: formData,
    });
  },
  download<T>(storageClass: string, cluster: string, key: string) {
    return requestJson<T>(
      withQuery(
        `${storageObjectPath(storageClass, cluster)}/object/${encodeURIComponent(key)}`,
        { expire: 3600 },
      ),
    );
  },
  remove(storageClass: string, cluster: string, key: string) {
    return request(
      `${storageObjectPath(storageClass, cluster)}/object/${encodeURIComponent(key)}`,
      { method: "DELETE" },
    );
  },
};

export const addonsApi = {
  async catalog<T = unknown>() {
    const response = await requestJson<DataResponse<T[]>>("/api/v1/addons");
    return response.data ?? [];
  },
  async installed<T = unknown>(cluster?: string) {
    const response = await requestJson<DataResponse<T[]>>(
      withQuery("/api/v1/installed-addons", { cluster }),
    );
    return response.data ?? [];
  },
  install(clusterId: string, body: object) {
    return request(`/api/v1/clusters/${encodeURIComponent(clusterId)}/addons`, {
      method: "POST",
      body,
    });
  },
  update(clusterId: string, name: string, body: object) {
    return request(
      `/api/v1/clusters/${encodeURIComponent(clusterId)}/addons/${encodeURIComponent(name)}`,
      { method: "PUT", body },
    );
  },
  remove(clusterId: string, name: string) {
    return request(
      `/api/v1/clusters/${encodeURIComponent(clusterId)}/addons/${encodeURIComponent(name)}`,
      { method: "DELETE" },
    );
  },
};

export const terminalApi = {
  createSocket(podName: string) {
    const protocol = location.protocol === "https:" ? "wss:" : "ws:";
    return new WebSocket(
      `${protocol}//${location.host}${resourcePath("pods", podName)}/terminal`,
    );
  },
};
