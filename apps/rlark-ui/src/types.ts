import type { Activity, LayoutDashboard } from "lucide-react";
import type { JobType } from "./data";

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
  name: string;
  type: "Cloud" | "Embodied" | "Hybrid" | string;
  region: string;
  location: string;
  phase: "Online" | "Degraded" | "Offline" | string;
  totalNodes: number;
  onlineNodes: number;
  offlineNodes: number;
  cloudNodes: number;
  embodiedNodes: number;
  robots: number;
  gpuModels: string[];
  robotModels: string[];
  runningJobs: number;
  description: string;
}

export interface CRDWorkload {
  kind: string;
  replicas: number;
  pvcStorageMap?: Record<string, string>;
  template: {
    spec: {
      containers: Array<{
        name: string;
        image: string;
        env: Array<{ name: string; value: string }>;
        volumeMounts?: Array<{ name: string; mountPath: string }>;
        resources?: {
          requests?: Record<string, string>;
          limits?: Record<string, string>;
        };
      }>;
      volumes?: Array<{
        name: string;
        hostPath?: { path: string };
        persistentVolumeClaim?: { claimName: string };
      }>;
    };
  };
}

export interface CRDJobTask {
  name: string;
  head: boolean;
  agentType: string;
  role: string;
  nodeSelector: Record<string, string>;
  prepareScript?: string;
  runScript?: string;
  tensorBoardDir?: string;
  kubernetes?: {
    workload?: CRDWorkload;
  };
}

export interface CRDJob {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    creationTimestamp?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec: {
    domain?: string;
    stopped?: boolean;
    sshPublicKey?: string;
    tasks: CRDJobTask[];
  };
  status?: {
    phase: string;
    startTime?: string;
    endTime?: string;
    tasks?: Array<{
      name: string;
      phase: string;
      message: string;
      observedNodes?: string[];
    }>;
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

export interface CRDWorkflowJobTemplate {
  name: string;
  dependencies?: string[];
  spec: { domain?: string; tasks: CRDJobTask[] };
}

export interface CRDWorkflow {
  apiVersion: string;
  kind: string;
  metadata: { name: string; creationTimestamp?: string };
  spec: { jobTemplates: CRDWorkflowJobTemplate[] };
  status?: {
    phase: string;
    jobs?: Array<{ name: string; phase: string; message: string }>;
    startTime?: string;
    endTime?: string;
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
    type: "host" | "storage";
    objectStorage: string;
    mountPath: string;
    hostPath: string;
  }>;
  pvcStorageMap?: Record<string, string>;
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
  };
  status?: {
    phase?: string;
    allocatable?: Record<string, string>;
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

export interface CRDNode {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    creationTimestamp?: string;
  };
  spec: {
    agentType?: string;
    unschedulable?: boolean;
  };
  status?: {
    phase?: string;
    reason?: string;
    nodeInfo?: {
      architecture?: string;
      kernelVersion?: string;
      agentVersion?: string;
      operatingSystem?: string;
    };
    addresses?: Array<{ type: string; address: string }>;
    diskPressure?: boolean;
    allocatable?: Record<string, string>;
    capacity?: Record<string, string>;
    used?: Record<string, string>;
  };
}

export type AdminNavItem = {
  id: string;
  icon: typeof Activity;
  zh: string;
  en: string;
  children?: AdminNavItem[];
};
