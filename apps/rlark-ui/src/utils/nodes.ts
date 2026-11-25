import { useEffect, useState } from "react";
import { Bot, CircleDot, CloudCog, Server } from "lucide-react";
import { nodes as mockNodes, type NodeKind } from "../data";
import type { CRDNode, CRDNodeLite, NodeCategory } from "../types";
import {
  getNodeAllLabels,
  getNodeCategories,
  getNodeCategory,
  hasNodeCategory,
  isBusinessWorkerNode,
  NODE_CATEGORY_LABEL,
} from "./nodeVisibility";

export {
  getNodeAllLabels,
  getNodeCategories,
  getNodeCategory,
  hasNodeCategory,
  isBusinessWorkerNode,
};
export {
  formatResourceQuantity,
  getGPUResourceKey,
  getNodeDiskUsage,
  getNodeResourceSummary,
  getResourceUsagePercent,
  isDiskUsageWarning,
  parseResourceQuantity,
  selectDeviceResourceKey,
} from "./nodeResources";

// Administrator metadata is authored on the management Node CR as annotations,
// so annotations take precedence over legacy values still present on the
// data-plane Node's observed labels.
export function getNodeLocation(node: CRDNode): string {
  const annotations = node.metadata.annotations ?? {};
  const labels = getNodeAllLabels(node);
  return annotations["rlark.io/city"] ?? labels["rlark.io/city"] ?? "";
}

export function getNodeGPUModel(node: CRDNode): string {
  const annotations = node.metadata.annotations ?? {};
  const labels = getNodeAllLabels(node);
  return (
    annotations["rlark.io/gpu-model"] ??
    labels["rlark.io/gpu-model"] ??
    annotations["rlark.io/model"] ??
    labels["rlark.io/model"] ??
    ""
  );
}

export function getNodeDeviceModel(node: CRDNode): string {
  const annotations = node.metadata.annotations ?? {};
  const labels = getNodeAllLabels(node);
  return (
    annotations["rlark.io/device-model"] ??
    labels["rlark.io/device-model"] ??
    annotations["rlark.io/model"] ??
    labels["rlark.io/model"] ??
    ""
  );
}

// node.status.reason values emitted by the node controller. Unknown values are
// returned verbatim so server-provided reasons still surface in the UI.
const nodeStatusReasonLabels: Record<string, { zh: string; en: string }> = {
  AgentLeaseActive: { zh: "节点代理心跳正常", en: "Agent lease active" },
  AgentLeaseExpired: { zh: "节点代理心跳已过期", en: "Agent lease expired" },
  NodeAgentLeaseMissing: {
    zh: "缺少节点代理心跳",
    en: "Node agent lease missing",
  },
};

export function nodeStatusReasonLabel(
  reason: string | undefined,
  zh: boolean,
): string {
  if (!reason) return "";
  const label = nodeStatusReasonLabels[reason];
  if (!label) return reason;
  return zh ? label.zh : label.en;
}

export const categoryLabels: Record<
  NodeCategory,
  { zh: string; en: string; icon: typeof CloudCog }
> = {
  cloud: { zh: "云算力", en: "Cloud", icon: CloudCog },
  edge: { zh: "端算力", en: "Edge", icon: Server },
  robot: { zh: "端真机", en: "Robot", icon: Bot },
  unknown: { zh: "其他", en: "Other", icon: CircleDot },
};

export { NODE_CATEGORY_LABEL };

// Mirrors the fixed English messages the Node Controller writes so the mock
// data renders the same localized explanations as a real control plane.
const mockConditionLastTransitionTime = "2026-11-16T08:00:00Z";

type MockNodeCondition = {
  type: string;
  status: "True" | "False" | "Unknown";
  reason: string;
  message: string;
  lastTransitionTime: string;
};

// Builds the full condition set the Node Controller would write for a node in
// the given phase, so local development can exercise connectivity, readiness,
// resource pressure, cordon, drain, and placement states without a backend.
function mockNodeConditions(phase: string): MockNodeCondition[] {
  const connected = ["Online", "Degraded", "Draining"].includes(phase);
  const draining = phase === "Draining";
  const pressure = phase === "Degraded";
  const unschedulable = draining || phase === "Offline";
  const capabilityStatus: MockNodeCondition["status"] = connected
    ? "True"
    : "Unknown";

  const condition = (
    type: string,
    status: MockNodeCondition["status"],
    reason: string,
    message: string,
  ): MockNodeCondition => ({
    type,
    status,
    reason,
    message,
    lastTransitionTime: mockConditionLastTransitionTime,
  });

  return [
    condition(
      "AgentConnected",
      connected ? "True" : "False",
      connected ? "LeaseValid" : "LeaseExpired",
      "Agent connectivity is derived from the node heartbeat Lease.",
    ),
    condition(
      "RuntimeReady",
      capabilityStatus,
      connected ? "RuntimeAvailable" : "RuntimeUnavailable",
      "Runtime availability is derived from the Agent-reported Kubernetes Node readiness.",
    ),
    condition(
      "NetworkReady",
      capabilityStatus,
      connected ? "NetworkAvailable" : "NetworkUnavailable",
      "Network availability is derived from the Agent-reported Kubernetes Node readiness.",
    ),
    condition(
      "StorageReady",
      connected ? "True" : "Unknown",
      connected ? "StorageAvailable" : "StorageUnavailable",
      "Storage availability is derived from the Agent-reported storage snapshot and disk pressure.",
    ),
    condition(
      "DeviceReady",
      capabilityStatus,
      connected ? "DeviceAvailable" : "DeviceUnavailable",
      "Device availability is derived from the Agent-reported Kubernetes Node readiness.",
    ),
    condition(
      "ResourcePressure",
      phase === "Offline" ? "Unknown" : pressure ? "True" : "False",
      phase === "Offline"
        ? "ResourcePressureUnknown"
        : pressure
          ? "ResourcePressureDetected"
          : "NoResourcePressure",
      "Resource pressure is derived from the Agent-reported Kubernetes disk pressure.",
    ),
    condition(
      "KubernetesCordoned",
      phase === "Offline" ? "Unknown" : "False",
      phase === "Offline"
        ? "KubernetesCordonUnobserved"
        : "KubernetesNotCordoned",
      "Cordon state is the Agent-observed Kubernetes Node spec.unschedulable. RLark does not manage that field, but a cordoned Node blocks every Pod on it, so it makes the Node unschedulable for RLark placement as well.",
    ),
    condition(
      "Draining",
      draining ? "True" : "False",
      draining ? "DrainRequested" : "NoDrainRequest",
      "Draining reflects the current Node drain operation request and Task observations.",
    ),
    condition(
      "Ready",
      connected && !draining ? "True" : "False",
      !connected
        ? "LeaseExpired"
        : draining
          ? "DrainRequested"
          : "KubernetesNodeReady",
      "Readiness is derived from Agent connectivity, Kubernetes Node readiness, drain state, and critical runtime, network, and device availability; resource pressure does not change readiness.",
    ),
    condition(
      "Degraded",
      pressure ? "True" : "False",
      pressure ? "ResourcePressureDetected" : "NoResourcePressure",
      "Degraded reports resource pressure or failed capability observations for diagnosis; it does not change readiness, and resource pressure separately excludes the Node from new placement.",
    ),
    condition(
      "Schedulable",
      unschedulable || pressure ? "False" : "True",
      unschedulable
        ? "Unschedulable"
        : pressure
          ? "ResourcePressureDetected"
          : "Schedulable",
      "Schedulability is the single placement decision; it combines spec.unschedulable with Agent connectivity, readiness, an external Kubernetes cordon, and scheduler-relevant resource pressure.",
    ),
  ];
}

export function buildMockCRDNodes(): CRDNode[] {
  const categoryByKind: Record<NodeKind, NodeCategory> = {
    CloudCompute: "cloud",
    EmbodiedCompute: "edge",
    Robot: "robot",
  };

  return mockNodes.map((node) => {
    const diskPercent = node.id === "gpu-cloud-01" ? 90 : 40;
    const diskCapacityBytes = 120 * 1024 ** 3;
    const diskUsedBytes = (diskCapacityBytes * diskPercent) / 100;
    const city = node.cluster.includes("上海")
      ? "上海市"
      : node.cluster.includes("杭州")
        ? "杭州市"
        : node.cluster.includes("North")
          ? "北京市"
          : "深圳市";
    const allLabels: Record<string, string> = {
      [NODE_CATEGORY_LABEL]: categoryByKind[node.kind],
      "rlark.io/model": node.model,
      "rlark.io/city": city,
      "rlark.io/enabled": "true",
      "kubernetes.io/hostname": node.name,
      ...(node.kind !== "CloudCompute" && node.tasks > 0
        ? {
            "rlark.io/embodied-task": "true",
            "rlark.io/embodied-task-name":
              node.kind === "Robot"
                ? `robot-operation-${node.id.replace(/^robot-/, "")}`
                : `edge-inference-${node.id.replace(/^edge-/, "")}`,
          }
        : {}),
    };
    return {
      apiVersion: "rlinf.io/v1alpha1",
      kind: "Node",
      metadata: {
        name: node.name,
        namespace: node.cluster,
        labels: allLabels,
        creationTimestamp: "2026-06-29T10:00:00Z",
      },
      spec: {
        agentType:
          node.kind === "Robot"
            ? "Robot"
            : node.kind === "EmbodiedCompute"
              ? "Edge"
              : "Kubernetes",
        unschedulable: node.phase === "Offline" || node.phase === "Draining",
      },
      status: {
        phase: node.phase,
        // Mock the Agent-reported observed labels so local dev sees the
        // same data shape as production. metadata.labels and
        // observedNodeLabels can differ in real clusters, but mock data
        // keeps them identical for simplicity.
        observedNodeLabels: allLabels,
        // Mock controller-written conditions so local development renders
        // connectivity, readiness, pressure, cordon, drain, and placement.
        conditions: mockNodeConditions(node.phase),
        nodeInfo: {
          architecture: "amd64",
          kernelVersion: "mock",
          agentVersion: "demo",
          operatingSystem: node.kind === "Robot" ? "robot-os" : "linux",
        },
        addresses: [{ type: "InternalIP", address: node.address }],
        allocatable: {
          cpu: "16",
          memory: "64Gi",
          "ephemeral-storage": "100Gi",
          "nvidia.com/gpu": node.gpu.split(" / ")[1] ?? "0",
          ...(node.kind !== "CloudCompute"
            ? {
                [`rlinf.io/device-${node.model.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`]:
                  "1",
              }
            : {}),
        },
        capacity: {
          cpu: "16",
          memory: "64Gi",
          "ephemeral-storage": "120Gi",
          "nvidia.com/gpu": node.gpu.split(" / ")[1] ?? "0",
          ...(node.kind !== "CloudCompute"
            ? {
                [`rlinf.io/device-${node.model.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`]:
                  "1",
              }
            : {}),
        },
        storage: {
          capacityBytes: diskCapacityBytes,
          usedBytes: diskUsedBytes,
          availableBytes: diskCapacityBytes - diskUsedBytes,
        },
        used: {
          cpu: `${node.cpu}%`,
          memory: `${node.memory}%`,
          "ephemeral-storage": `${diskPercent}Gi`,
          "nvidia.com/gpu": node.gpu.split(" / ")[0] ?? "0",
          ...(node.kind !== "CloudCompute"
            ? {
                [`rlinf.io/device-${node.model.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`]:
                  node.tasks > 0 ? "1" : "0",
              }
            : {}),
        },
      },
    };
  });
}

export function useNodeLabels() {
  const [nodes, setNodes] = useState<CRDNodeLite[]>([]);
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    setLoading(true);
    nodesApi
      .list()
      .then(setNodes)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);
  const clusterNames = Array.from(
    new Set(
      nodes.map((n) => n.metadata.namespace).filter((v): v is string => !!v),
    ),
  );
  const clusterDisplayNames = clusterNames;
  return { nodes, loading, clusterNames, clusterDisplayNames };
}
import { nodesApi } from "../backend";
