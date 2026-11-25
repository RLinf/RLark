import type { Activity, LayoutDashboard } from "lucide-react";
import type { JobType, MountType, PullProgressEntry } from "./data";
import type { CRDVolume } from "./crdModels";

export type Page =
  | "overview"
  | "clusters-management"
  | "clusters-nodes"
  | "jobs"
  | "workflows"
  | "domains"
  | "storageClass"
  | "files"
  | "api"
  | "ssh-keys";

export type NavParent = {
  id: Page;
  icon: typeof LayoutDashboard;
  children?: { id: Page; icon: typeof LayoutDashboard }[];
};

export interface ResourceRow {
  label: string;
  count: number;
  models: string;
  color: string;
}

export interface ClusterSummary {
  id: string;
  agentNamespace: string;
  name: string;
  type: "Cloud" | "Embodied" | "Hybrid" | string;
  region: string;
  location: string;
  phase:
    | "Online"
    | "Offline"
    | "Downgraded"
    | "Delisted"
    | "Cordoned"
    | "Draining"
    | string;
  totalNodes: number;
  onlineNodes: number;
  offlineNodes: number;
  readyNodes?: number;
  degradedNodes?: number;
  disconnectedNodes?: number;
  unschedulableNodes?: number;
  dominantFailureReason?: string;
  cloudNodes: number;
  embodiedNodes: number;
  robots: number;
  gpuModels: string[];
  robotModels: string[];
  runningJobs: number;
  description: string;
  agentNamespaceConflict?: boolean;
}

export interface CRDPod {
  metadata?: { name?: string; namespace?: string };
  spec?: {
    taskName?: string;
    taskNamespace?: string;
    podName?: string;
    podNamespace?: string;
    domain?: string;
  };
  status?: {
    phase?: string;
    node?: string;
    ip?: string;
    message?: string;
  };
}

export interface CRDDomain {
  apiVersion: string;
  kind: string;
  metadata: { name: string; creationTimestamp?: string };
  spec: { cidr: string };
  status?: {
    ipAllocations?: Array<{
      ip: string;
      job: string;
      task: string;
      pod: string;
    }>;
  };
}

export interface DAGNode {
  id: string;
  name: string;
  x: number;
  y: number;
}

export interface DAGEdge {
  from: string;
  to: string;
}

export interface RoleResource {
  role: string;
  cluster: string;
  nodeSelector: string;
  replicas: number;
  cpu: string;
  memory: string;
  gpu: string;
  devices: Array<{ name: string; quantity: string }>;
  image: string;
  prepareScript: string;
  envs: Array<{ key: string; value: string }>;
  mounts: Array<{
    type: MountType;
    objectStorage: string;
    mountPath: string;
    hostPath: string;
    pvcSizeGb: number | "";
    rawVolume?: CRDVolume;
  }>;
  advanced?: {
    sharedMemorySize?: string;
  };
}

export interface WorkflowJobDef {
  id: string;
  name: string;
  dependencies: string[];
  type: JobType;
  roles: string[];
  headerRole: string;
  roleResources: Record<string, RoleResource>;
  runScript: string;
  domain: string;
}

export interface CRDNodeLite {
  metadata: {
    name: string;
    namespace?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec?: {
    unschedulable?: boolean;
  };
  status?: {
    phase?: string;
    conditions?: Array<{
      type: string;
      status: "True" | "False" | "Unknown";
      reason: string;
      message?: string;
      lastTransitionTime?: string;
      observedGeneration?: number;
    }>;
    capacity?: Record<string, string>;
    allocatable?: Record<string, string>;
    used?: Record<string, string>;
    observedNodeLabels?: Record<string, string>;
  };
}

export interface SignAgentCertResponse {
  cluster_id: string;
  ca_cert: string;
  agent_cert: string;
  agent_key: string;
  server_addr: string;
}

export interface AgentCertListItem {
  cluster_id: string;
  created_at: string;
  server_addr: string;
}

export type NodeCategory = "cloud" | "edge" | "robot" | "unknown";

// NodeEventEntry 对应后端 api/rlark.io/v1alpha1 NodeEvent（参见
// node_types.go 中 NodeEvent 结构）：数据面 node-agent 采集的节点级
// Kubernetes Event，目前仅包含 Warning 类型（DiskPressure 等）。控制面
// Task reconciler 会聚合各节点 events 写入 Task.status.events。
export interface NodeEventEntry {
  type: string; // Warning / Normal
  reason: string; // DiskPressure, FailedScheduling, Pulling, ...
  message?: string;
  lastTime?: string; // RFC3339
  count?: number;
  source?: string; // 事件来源组件，如 kubelet
  objectKind?: string; // 涉及对象类型：Node / Pod
  objectName?: string; // 涉及对象名称
}

export interface CRDNode {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace?: string;
    // metadata.labels on the management Node CR only receives RLark system
    // labels written by the Agent; it is not a complete mirror of the
    // data-plane Node. The authoritative label set is status.observedNodeLabels,
    // which the Agent reads directly from the K8s Node.
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    creationTimestamp?: string;
    generation?: number;
  };
  spec: {
    agentType?: string;
    unschedulable?: boolean;
    operations?: Array<{
      id: string;
      type: "Drain" | "EnterMaintenance";
    }>;
  };
  status?: {
    phase?: string;
    conditions?: Array<{
      type: string;
      status: "True" | "False" | "Unknown";
      reason: string;
      message?: string;
      lastTransitionTime?: string;
      observedGeneration?: number;
    }>;
    // observedNodeLabels is the authoritative label dictionary the Agent reads
    // directly from the local Kubernetes Node. Frontend label display must use
    // this field, not metadata.labels — RLark only writes its own system labels
    // (rlark.io/*) to metadata.labels on the management Node CR.
    observedNodeLabels?: Record<string, string>;
    // drainOperations 是 Node Controller 对每个 Drain 操作的有界进度汇总，
    // 与控制面 nodePhase/drainingCondition 使用同一判定规则。前端据此在
    // Draining Condition 尚未写入时也能立即判断节点是否正在排空。
    drainOperations?: Array<{
      operationID: string;
      waiting?: number;
      blocked?: number;
      completed?: boolean;
    }>;
    lastHeartbeatTime?: string;
    lastStatusChangeTime?: string;
    nodeInfo?: {
      architecture?: string;
      kernelVersion?: string;
      agentVersion?: string;
      operatingSystem?: string;
    };
    addresses?: Array<{ type: string; address: string }>;
    diskPressure?: boolean;
    storage?: {
      capacityBytes?: number;
      usedBytes?: number;
      availableBytes?: number;
    };
    allocatable?: Record<string, string>;
    capacity?: Record<string, string>;
    used?: Record<string, string>;
    activeWorkloads?: {
      total?: number;
      pending?: number;
      // running 统计节点上全部非终态 Pod（含系统组件），不能用来判断节点上是否
      // 还有 RLark 工作负载；rlarkManaged 才是由 RLark 托管的 Pod 数量。
      running?: number;
      stopping?: number;
      rlarkManaged?: number;
    };
    // PullProgress 由数据面 node-agent 上报，cluster-agent 仅在 Pod 处于
    // ContainerCreating 期间聚合写入 Task.status.pullProgress；此处保留
    // node 原始上报值，前端用于在任务 Pending 时展示镜像拉取进度。
    pullProgress?: PullProgressEntry[];
    // Events 由数据面 node-agent 上报节点级 Warning 事件（DiskPressure 等）。
    // 前端在任务 Pending 期间聚合展示，以便定位 Pod 为何卡在 Pending。
    events?: NodeEventEntry[];
  };
}

export type AdminNavItem = {
  id: string;
  icon: typeof Activity;
  zh: string;
  en: string;
  children?: AdminNavItem[];
};
