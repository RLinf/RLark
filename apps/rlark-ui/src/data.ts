import type { CRDWorkflow } from "./types";

export type Phase =
  | "Running"
  | "Pending"
  | "Succeeded"
  | "Failed"
  | "Stopped"
  | "Online"
  | "Offline";

export type ClusterType = "Cloud" | "Embodied";
export type NodeKind = "CloudCompute" | "EmbodiedCompute" | "Robot";
export type JobType = "RL" | "DataCollection" | "Evaluation" | "Custom";

export interface Cluster {
  id: string;
  name: string;
  type: ClusterType;
  region: string;
  location: string;
  phase: Phase;
  cloudNodes: number;
  embodiedNodes: number;
  robots: number;
  gpuModels: string[];
  robotModels: string[];
  cpuUsage: number;
  gpuUsage: number;
  robotUsage: number;
  runningJobs: number;
  description: string;
}

export interface NodeItem {
  id: string;
  name: string;
  cluster: string;
  kind: NodeKind;
  phase: Phase;
  address: string;
  model: string;
  cpu: number;
  memory: number;
  gpu: string;
  robotState?: string;
  cameraUrl?: string;
  telemetryUrl?: string;
  controlUrl?: string;
  battery?: number;
  tasks: number;
}

export interface Worker {
  id: string;
  name: string;
  jobId: string;
  role: string;
  node: string;
  phase: Phase;
  cpu: number;
  memory: number;
  gpu?: string;
  latency?: string;
  fps?: number;
  webTerminalUrl?: string;
  logs: string[];
}

export interface Job {
  id: string;
  name: string;
  displayName: string;
  type: JobType;
  phase: Phase;
  owner: string;
  cluster: string;
  target: string;
  workers: number;
  runningWorkers: number;
  startedAt: string;
  submittedAt: string;
  stoppedAt: string;
  roleCount: number;
  duration: string;
  progress: number;
  defaultRoles: string[];
  image: string;
  command: string;
  env: Array<{ key: string; value: string }>;
  mounts: Array<{
    type: "host" | "storage";
    objectStorage: string;
    mountPath: string;
    hostPath: string;
  }>;
  headerRole: string;
  headerWorker: string;
  sshAddress: string;
  stopped?: boolean;
  domain?: string;
  tensorBoardDir?: string;
  sshPublicKey?: string;
  resources: Array<{
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
    env: Array<{ key: string; value: string }>;
    mounts: Array<{
      type: "host" | "storage";
      objectStorage: string;
      mountPath: string;
      hostPath: string;
    }>;
    pvcStorageMap?: Record<string, string>;
  }>;
  taskStatuses: Array<{
    name: string;
    phase: string;
    message: string;
    observedNodes?: string[];
  }>;
}

export interface PodInfo {
  name: string;
  namespace: string;
  taskName: string;
  taskNamespace: string;
  podName: string;
  podNamespace: string;
  domain: string;
  phase: string;
  node: string;
  ip: string;
  message: string;
}

export interface Domain {
  id: string;
  name: string;
  cidr: string;
  ipAllocations: Array<{ ip: string; job: string; task: string; pod: string }>;
  createdAt: string;
}

export type StorageProvider =
  | "AWS S3"
  | "Google Cloud"
  | "Azure Blob"
  | "MinIO"
  | "Aliyun OSS"
  | "Tencent COS";

export interface StorageClass {
  id: string;
  name: string;
  namespace: string;
  provider: StorageProvider;
  clusters: string[];
  endpoint: string;
  region: string;
  bucket: string;
  accessKeyId: string;
  pathStyle: boolean;
  description: string;
  createdAt: string;
}

export interface StorageClassCreateRequest {
  name: string;
  namespace: string;
  provider: StorageProvider;
  clusters: string[];
  endpoint: string;
  region: string;
  bucket: string;
  access_key_id: string;
  access_key_secret: string;
  path_style: boolean;
  description: string;
}

export const storageClasses: StorageClass[] = [];
export const clusters: Cluster[] = [
  {
    id: "cloud-east-a",
    name: "云训练集群 East-A",
    type: "Cloud",
    region: "华东 · cn-shanghai",
    location: "上海",
    phase: "Online",
    cloudNodes: 42,
    embodiedNodes: 0,
    robots: 0,
    gpuModels: ["NVIDIA H800", "NVIDIA A100"],
    robotModels: [],
    cpuUsage: 64,
    gpuUsage: 78,
    robotUsage: 0,
    runningJobs: 7,
    description: "承载强化学习训练和大规模 rollout 的主力云 GPU 集群。",
  },
  {
    id: "cloud-north-b",
    name: "云评测集群 North-B",
    type: "Cloud",
    region: "华北 · cn-beijing",
    location: "北京",
    phase: "Online",
    cloudNodes: 18,
    embodiedNodes: 0,
    robots: 0,
    gpuModels: ["NVIDIA L40S", "NVIDIA A10"],
    robotModels: [],
    cpuUsage: 42,
    gpuUsage: 51,
    robotUsage: 0,
    runningJobs: 3,
    description: "用于离线回归评测、仿真评测和批量指标聚合。",
  },
  {
    id: "robot-lab-sh",
    name: "上海具身实验集群",
    type: "Embodied",
    region: "上海机器人实验室",
    location: "上海",
    phase: "Online",
    cloudNodes: 0,
    embodiedNodes: 9,
    robots: 26,
    gpuModels: ["Jetson Orin", "RTX 4090 Edge"],
    robotModels: ["Unitree G1", "Franka Panda", "AgileX Scout"],
    cpuUsage: 55,
    gpuUsage: 46,
    robotUsage: 68,
    runningJobs: 5,
    description: "连接实验室内真机、边缘算力和传感器通道的具身实验集群。",
  },
  {
    id: "robot-warehouse-hz",
    name: "杭州仓储真机场",
    type: "Embodied",
    region: "杭州 · Warehouse A2",
    location: "杭州",
    phase: "Online",
    cloudNodes: 0,
    embodiedNodes: 6,
    robots: 18,
    gpuModels: ["Jetson AGX Orin"],
    robotModels: ["Robodog OT-T12", "AMR Forklift"],
    cpuUsage: 48,
    gpuUsage: 39,
    robotUsage: 72,
    runningJobs: 2,
    description: "面向仓储巡检、导航采集和 AMR 任务的现场真机场。",
  },
];

export const nodes: NodeItem[] = [
  {
    id: "gpu-cloud-01",
    name: "gpu-cloud-01",
    cluster: "云训练集群 East-A",
    kind: "CloudCompute",
    phase: "Online",
    address: "10.20.0.11",
    model: "H800 SXM · 8 GPU",
    cpu: 62,
    memory: 71,
    gpu: "6 / 8",
    tasks: 14,
  },
  {
    id: "gpu-cloud-03",
    name: "gpu-cloud-03",
    cluster: "云训练集群 East-A",
    kind: "CloudCompute",
    phase: "Online",
    address: "10.20.0.13",
    model: "A100 80G · 8 GPU",
    cpu: 78,
    memory: 69,
    gpu: "8 / 8",
    tasks: 18,
  },
  {
    id: "eval-l40s-07",
    name: "eval-l40s-07",
    cluster: "云评测集群 North-B",
    kind: "CloudCompute",
    phase: "Online",
    address: "10.32.4.17",
    model: "L40S · 4 GPU",
    cpu: 44,
    memory: 52,
    gpu: "2 / 4",
    tasks: 6,
  },
  {
    id: "edge-orin-02",
    name: "edge-orin-02",
    cluster: "上海具身实验集群",
    kind: "EmbodiedCompute",
    phase: "Online",
    address: "192.168.7.42",
    model: "Jetson Orin",
    cpu: 56,
    memory: 61,
    gpu: "1 / 1",
    tasks: 4,
  },
  {
    id: "robot-g1-12",
    name: "robot-g1-12",
    cluster: "上海具身实验集群",
    kind: "Robot",
    phase: "Online",
    address: "192.168.7.112",
    model: "Unitree G1",
    cpu: 38,
    memory: 45,
    gpu: "0 / 1",
    robotState: "执行抓取序列",
    cameraUrl: "rtsp://192.168.7.112/front-camera",
    telemetryUrl: "ws://192.168.7.112/telemetry",
    controlUrl: "grpc://192.168.7.112:9090/control",
    battery: 82,
    tasks: 1,
  },
  {
    id: "robot-dog-08",
    name: "robot-dog-08",
    cluster: "杭州仓储真机场",
    kind: "Robot",
    phase: "Online",
    address: "172.18.4.88",
    model: "Robodog OT-T12",
    cpu: 41,
    memory: 48,
    gpu: "0 / 1",
    robotState: "巡检中",
    cameraUrl: "rtsp://172.18.4.88/inspection-cam",
    telemetryUrl: "ws://172.18.4.88/state",
    controlUrl: "grpc://172.18.4.88:9090/control",
    battery: 67,
    tasks: 1,
  },
  {
    id: "robot-franka-03",
    name: "robot-franka-03",
    cluster: "上海具身实验集群",
    kind: "Robot",
    phase: "Offline",
    address: "192.168.7.203",
    model: "Franka Panda",
    cpu: 0,
    memory: 0,
    gpu: "0 / 1",
    robotState: "维护中",
    cameraUrl: "rtsp://192.168.7.203/wrist-camera",
    telemetryUrl: "ws://192.168.7.203/telemetry",
    controlUrl: "grpc://192.168.7.203:9090/control",
    battery: 0,
    tasks: 0,
  },
];

export const jobs: Job[] = [];

export const workflowCRDs: CRDWorkflow[] = [];

export const workers: Worker[] = [];

export const activity: {
  time: string;
  type: string;
  title: string;
  meta: string;
}[] = [];
