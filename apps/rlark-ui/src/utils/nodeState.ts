export type NodeConditionStatus = "True" | "False" | "Unknown";

export type NodeStateKind =
  | "connected"
  | "disconnected"
  | "neverConnected"
  | "connectionUnknown"
  | "ready"
  | "notReady"
  | "schedulable"
  | "cordoned"
  | "draining"
  | "notSchedulable"
  | "pressure"
  | "noPressure"
  | "pressureUnknown"
  | "degraded"
  | "healthy"
  | "kubernetesCordoned"
  | "kubernetesNotCordoned"
  | "kubernetesCordonUnknown"
  | "available"
  | "unavailable"
  | "serviceDegraded";

type NodeCondition = {
  type: string;
  status: NodeConditionStatus;
  reason: string;
  message?: string;
  lastTransitionTime?: string;
  observedGeneration?: number;
};

/**
 * Minimal node shape needed to derive state. Both the full CRDNode and the
 * lighter node records used by the task placement picker satisfy it.
 */
export type NodeStateSource = {
  spec?: {
    unschedulable?: boolean;
    operations?: Array<{ id?: string; type?: string }>;
  };
  status?: {
    conditions?: NodeCondition[];
    drainOperations?: Array<{ operationID?: string; completed?: boolean }>;
  };
};

export type NodeState = {
  kind: NodeStateKind;
  condition?: NodeCondition;
};

function condition(
  node: NodeStateSource,
  type: string,
): NodeCondition | undefined {
  return node.status?.conditions?.find((item) => item.type === type);
}

export function nodeConnectionState(node: NodeStateSource): NodeState {
  const value = condition(node, "AgentConnected");
  if (!value || value.status === "Unknown") {
    return { kind: "connectionUnknown", condition: value };
  }
  if (value.status === "True" && value.reason === "LeaseValid") {
    return { kind: "connected", condition: value };
  }
  if (value.status === "False" && value.reason === "HeartbeatNeverReceived") {
    return { kind: "neverConnected", condition: value };
  }
  return { kind: "disconnected", condition: value };
}

export function nodeReadinessState(node: NodeStateSource): NodeState {
  // Readiness is two-state: only an explicit Ready=True counts as ready, so a
  // missing or unobserved Ready condition is reported as not ready.
  const value = condition(node, "Ready");
  return {
    kind: value?.status === "True" ? "ready" : "notReady",
    condition: value,
  };
}

export function nodeSchedulingState(node: NodeStateSource): NodeState {
  // The drain request is durable scheduling intent carried in spec.operations and
  // is written synchronously by the drain/undrain API, while the Draining
  // condition is only rewritten on a later Node Controller pass. Trust the
  // request so the console neither lags behind a drain nor keeps showing a drain
  // after an undrain; this mirrors the Node Controller's own rule, where an
  // uncompleted Drain operation means draining and a removed (or completed) one
  // falls back to the durable cordon intent.
  if (nodeDrainRequestActive(node))
    return { kind: "draining", condition: condition(node, "Draining") };
  // NodeSpec.unschedulable is a non-pointer bool serialized with omitempty, so a
  // schedulable node omits the field. Absence means the default scheduling intent.
  if (node.spec?.unschedulable === true) return { kind: "cordoned" };
  return { kind: "schedulable" };
}

/**
 * Whether the Node has a Drain operation that is not yet complete.
 *
 * A Drain operation stays in `spec.operations` until an operator removes it, so
 * its presence alone is not enough: the Node Controller reports a completed
 * drain as Cordoned, not Draining. Match that rule by treating a Drain request
 * as active only while `status.drainOperations` has not marked it completed.
 * This lets the console reflect a just-requested drain immediately, before the
 * controller has rewritten the Draining condition.
 */
function nodeDrainRequestActive(node: NodeStateSource): boolean {
  const drainIDs = (node.spec?.operations ?? [])
    .filter((operation) => operation.type === "Drain" && operation.id)
    .map((operation) => operation.id as string);
  if (drainIDs.length === 0) return false;
  const completed = new Set(
    (node.status?.drainOperations ?? [])
      .filter((operation) => operation.completed && operation.operationID)
      .map((operation) => operation.operationID as string),
  );
  return drainIDs.some((id) => !completed.has(id));
}

export function nodePressureState(node: NodeStateSource): NodeState {
  const value = condition(node, "ResourcePressure");
  if (!value || value.status === "Unknown") {
    return { kind: "pressureUnknown", condition: value };
  }
  return {
    kind: value.status === "True" ? "pressure" : "noPressure",
    condition: value,
  };
}

export function nodeDegradationState(node: NodeStateSource): NodeState {
  // Degraded is a two-state conclusion: the Node Controller only writes True or
  // False, so anything other than True means "not degraded".
  const value = condition(node, "Degraded");
  return {
    kind: value?.status === "True" ? "degraded" : "healthy",
    condition: value,
  };
}

export type NodeAvailability = {
  kind: NodeStateKind;
  reasons: NodeState[];
};

/**
 * Cordon state of the underlying Kubernetes Node, read from the controller-written
 * `KubernetesCordoned` condition. RLark never writes that field, so `True` means
 * an operator cordoned the node outside RLark and the Kubernetes scheduler now
 * excludes it for every Pod, not only RLark workloads. The condition also makes
 * `Schedulable` False, so `nodeAvailabilityState` reports the Node as unavailable
 * instead of hiding a Node that can no longer accept Pods.
 */
export function nodeKubernetesCordonState(node: NodeStateSource): NodeState {
  const value = condition(node, "KubernetesCordoned");
  if (!value || value.status === "Unknown") {
    return { kind: "kubernetesCordonUnknown", condition: value };
  }
  return {
    kind:
      value.status === "True" ? "kubernetesCordoned" : "kubernetesNotCordoned",
    condition: value,
  };
}

/**
 * Placement availability, read directly from the controller-written
 * `Schedulable` condition. The Node Controller is the single source of truth:
 * the Job scheduler and the console both consume this condition instead of
 * independently re-deriving placement from connection, readiness, and pressure.
 * A missing condition is not schedulable, matching a fail-closed scheduler.
 *
 * `serviceDegraded` remains a display-only distinction for a schedulable Node
 * with a non-fatal `Degraded` condition.
 */
export function nodeAvailabilityState(node: NodeStateSource): NodeAvailability {
  const schedulable = condition(node, "Schedulable");
  if (!schedulable || schedulable.status !== "True") {
    return {
      kind: "unavailable",
      reasons: [{ kind: "notSchedulable", condition: schedulable }],
    };
  }
  const degradation = nodeDegradationState(node);
  if (degradation.kind === "degraded") {
    return { kind: "serviceDegraded", reasons: [degradation] };
  }
  return { kind: "available", reasons: [] };
}

export function nodeStateLabel(kind: NodeStateKind, zh: boolean): string {
  const labels: Record<NodeStateKind, [string, string]> = {
    connected: ["在线", "Online"],
    disconnected: ["离线", "Offline"],
    neverConnected: ["离线", "Offline"],
    connectionUnknown: ["状态未知", "Status unknown"],
    ready: ["已就绪", "Ready"],
    notReady: ["未就绪", "Not ready"],
    schedulable: ["可调度", "Schedulable"],
    cordoned: ["已暂停调度", "Cordoned"],
    draining: ["排空中", "Draining"],
    notSchedulable: ["不可调度", "Not schedulable"],
    pressure: ["存在资源压力", "Resource pressure"],
    noPressure: ["无资源压力", "No resource pressure"],
    pressureUnknown: ["资源压力未知", "Resource pressure unknown"],
    degraded: ["已降级", "Degraded"],
    healthy: ["未降级", "Not degraded"],
    kubernetesCordoned: ["被外部 cordon", "Cordoned externally"],
    kubernetesNotCordoned: ["未被外部 cordon", "Not cordoned externally"],
    kubernetesCordonUnknown: [
      "外部 cordon 状态未知",
      "External cordon state unknown",
    ],
    available: ["可用", "Available"],
    unavailable: ["不可用", "Unavailable"],
    serviceDegraded: ["服务降级", "Service degraded"],
  };
  return labels[kind][zh ? 0 : 1];
}

export function nodeReasonLabel(
  reason: string | undefined,
  zh: boolean,
): string | undefined {
  if (!reason) return undefined;
  const labels: Record<string, [string, string]> = {
    LeaseValid: ["心跳租约有效", "Heartbeat lease valid"],
    LeaseExpired: ["心跳租约已过期", "Heartbeat lease expired"],
    HeartbeatNeverReceived: ["尚未收到心跳", "No heartbeat received"],
    KubernetesNodeReady: ["Kubernetes 节点已就绪", "Kubernetes node ready"],
    KubernetesNodeNotReady: [
      "Kubernetes 节点未就绪",
      "Kubernetes node not ready",
    ],
    KubernetesNodeUnknown: [
      "Kubernetes 节点状态未知",
      "Kubernetes node status unknown",
    ],
    KubernetesNodeReadyUnobserved: [
      "Kubernetes 节点就绪状态未观测",
      "Kubernetes node readiness unobserved",
    ],
    RuntimeAvailable: ["运行时可用", "Runtime available"],
    RuntimeUnavailable: ["运行时不可用", "Runtime unavailable"],
    NetworkAvailable: ["网络可用", "Network available"],
    NetworkUnavailable: ["网络不可用", "Network unavailable"],
    StorageAvailable: ["存储可用", "Storage available"],
    StorageUnavailable: ["存储不可用", "Storage unavailable"],
    DeviceAvailable: ["设备可用", "Device available"],
    DeviceUnavailable: ["设备不可用", "Device unavailable"],
    ResourcePressureDetected: ["检测到资源压力", "Resource pressure detected"],
    NoResourcePressure: ["无资源压力", "No resource pressure"],
    ResourcePressureUnknown: ["资源压力未知", "Resource pressure unknown"],
    DrainRequested: ["已请求排空", "Drain requested"],
    DrainBlocked: ["排空受阻", "Drain blocked"],
    DrainComplete: ["排空完成", "Drain complete"],
    NoDrainRequest: ["未请求排空", "No drain request"],
    Schedulable: ["可调度", "Schedulable"],
    Unschedulable: ["已暂停调度", "Cordoned"],
    NotSchedulable: ["无法判定可调度性", "Schedulability unavailable"],
    KubernetesCordoned: [
      "Kubernetes 节点已被外部 cordon",
      "Kubernetes node cordoned externally",
    ],
    KubernetesNotCordoned: [
      "Kubernetes 节点未被 cordon",
      "Kubernetes node not cordoned",
    ],
    KubernetesCordonUnobserved: [
      "Kubernetes 节点 cordon 状态未观测",
      "Kubernetes node cordon state unobserved",
    ],
  };
  return labels[reason]?.[zh ? 0 : 1] ?? reason;
}

/**
 * Human-readable explanation of why a node is not available, for tooltips.
 * Returns an empty string when the node is available.
 */
export function nodeAvailabilityReasonText(
  availability: NodeAvailability,
  zh: boolean,
): string {
  return availability.reasons
    .map((reason) => {
      const detail = nodeReasonLabel(reason.condition?.reason, zh);
      const label = nodeStateLabel(reason.kind, zh);
      return detail ? `${label}${zh ? "：" : ": "}${detail}` : label;
    })
    .join(zh ? "；" : "; ");
}

/**
 * Notes-column text shown for a node in the node list. The node detail header
 * reuses it so both surfaces explain availability with the same wording.
 * Draining also sets `spec.unschedulable`, so the generic Schedulable reason
 * would only report "cordoned"; surface the eviction explicitly instead.
 */
export function nodeAvailabilityNote(
  node: NodeStateSource,
  zh: boolean,
): string {
  if (nodeSchedulingState(node).kind === "draining") {
    return zh ? "不可调度/驱逐任务中" : "Not schedulable / draining jobs";
  }
  const availability = nodeAvailabilityState(node);
  return availability.reasons.length > 0
    ? nodeAvailabilityReasonText(availability, zh)
    : "—";
}

/**
 * Display state for the scheduling badge on the node detail header.
 *
 * The badge answers one question: can new work be placed on this Node? It
 * therefore follows the controller-written `Schedulable` decision, the same
 * source as the node list, instead of `spec.unschedulable`, which only records
 * administrative intent and stays false when a Node merely goes offline.
 * Draining is surfaced separately to match the node list note.
 */
export function nodeSchedulingBadgeState(node: NodeStateSource): NodeStateKind {
  if (nodeSchedulingState(node).kind === "draining") return "draining";
  return nodeAvailabilityState(node).kind === "unavailable"
    ? "notSchedulable"
    : "schedulable";
}

export function nodeConditionLabel(type: string, zh: boolean): string {
  const labels: Record<string, [string, string]> = {
    AgentConnected: ["Agent 连接", "Agent connection"],
    Ready: ["节点就绪", "Node readiness"],
    RuntimeReady: ["运行时就绪", "Runtime readiness"],
    NetworkReady: ["网络就绪", "Network readiness"],
    StorageReady: ["存储就绪", "Storage readiness"],
    DeviceReady: ["设备就绪", "Device readiness"],
    ResourcePressure: ["资源压力", "Resource pressure"],
    Draining: ["节点排空", "Node draining"],
    Degraded: ["降级状态", "Degradation"],
    Schedulable: ["可调度性", "Schedulability"],
    KubernetesCordoned: ["外部 cordon 状态", "External cordon state"],
  };
  return labels[type]?.[zh ? 0 : 1] ?? type;
}

/**
 * Localizes a condition status without inferring health from it: `True` means
 * different things per type (for example `ResourcePressure=True` reports
 * pressure), so the label stays a neutral boolean.
 */
export function nodeConditionStatusLabel(
  status: NodeConditionStatus,
  zh: boolean,
): string {
  const labels: Record<NodeConditionStatus, [string, string]> = {
    True: ["是", "True"],
    False: ["否", "False"],
    Unknown: ["未知", "Unknown"],
  };
  const label = labels[status];
  return label ? label[zh ? 0 : 1] : status;
}

/**
 * Localizes the fixed English explanation the Node Controller writes per
 * condition type. Unknown types fall back to the reported message so a new
 * controller text is shown verbatim instead of a stale translation.
 */
export function nodeConditionMessage(
  type: string,
  message: string | undefined,
  zh: boolean,
): string | undefined {
  if (!message || !zh) return message;
  const labels: Record<string, string> = {
    AgentConnected: "Agent 连接状态来自节点心跳租约。",
    RuntimeReady: "运行时可用性来自 Agent 上报的 Kubernetes 节点就绪状态。",
    NetworkReady: "网络可用性来自 Agent 上报的 Kubernetes 节点就绪状态。",
    StorageReady: "存储可用性来自 Agent 上报的存储快照与磁盘压力。",
    DeviceReady: "设备可用性来自 Agent 上报的 Kubernetes 节点就绪状态。",
    ResourcePressure: "资源压力来自 Agent 上报的 Kubernetes 磁盘压力。",
    KubernetesCordoned:
      "cordon 状态是 Agent 观测到的 Kubernetes 节点 spec.unschedulable。RLark 不管理该字段，因此被 cordon 的节点会阻止其上所有 Pod；它不改变 RLark 的可调度性。",
    Draining: "排空状态反映当前的节点排空请求与 Task 观测结果。",
    Ready:
      "就绪状态由 Agent 连接、Kubernetes 节点就绪、排空状态，以及运行时、网络、设备可用性共同决定；资源压力不影响就绪。",
    Degraded:
      "降级状态反映资源压力或能力观测失败，仅供诊断；它不改变就绪状态，资源压力会单独将该节点排除在新调度之外。",
    Schedulable:
      "可调度性是唯一的放置决策，综合 spec.unschedulable、Agent 连接、就绪状态与调度相关资源压力。",
  };
  return labels[type] ?? message;
}

export function formatDurationSince(
  time: string | undefined,
  now = Date.now(),
): string | undefined {
  if (!time) return undefined;
  const milliseconds = now - new Date(time).getTime();
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return undefined;
  const seconds = Math.floor(milliseconds / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}
