import { useEffect, useState } from "react";
import { Bot, CircleDot, CloudCog, Server } from "lucide-react";
import { nodes as mockNodes, type NodeKind } from "../data";
import type { CRDNode, CRDNodeLite, NodeCategory } from "../types";
import {
  getNodeCategories,
  getNodeCategory,
  hasNodeCategory,
  isBusinessWorkerNode,
  NODE_CATEGORY_LABEL,
} from "./nodeVisibility";

export {
  getNodeCategories,
  getNodeCategory,
  hasNodeCategory,
  isBusinessWorkerNode,
};
export {
  formatResourceQuantity,
  getGPUResourceKey,
  getNodeResourceSummary,
  parseResourceQuantity,
} from "./nodeResources";

export function getNodeLocation(node: CRDNode): string {
  return (
    node.metadata.annotations?.["rlark.io/city"] ??
    node.metadata.labels?.["rlark.io/city"] ??
    ""
  );
}

export function getNodeGPUModel(node: CRDNode): string {
  return (
    node.metadata.annotations?.["rlark.io/gpu-model"] ??
    node.metadata.labels?.["rlark.io/gpu-model"] ??
    node.metadata.annotations?.["rlark.io/model"] ??
    node.metadata.labels?.["rlark.io/model"] ??
    ""
  );
}

export function getNodeDeviceModel(node: CRDNode): string {
  return (
    node.metadata.annotations?.["rlark.io/device-model"] ??
    node.metadata.labels?.["rlark.io/device-model"] ??
    node.metadata.annotations?.["rlark.io/model"] ??
    node.metadata.labels?.["rlark.io/model"] ??
    ""
  );
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

export function buildMockCRDNodes(): CRDNode[] {
  const categoryByKind: Record<NodeKind, NodeCategory> = {
    CloudCompute: "cloud",
    EmbodiedCompute: "edge",
    Robot: "robot",
  };

  return mockNodes.map((node) => {
    const city = node.cluster.includes("上海")
      ? "上海市"
      : node.cluster.includes("杭州")
        ? "杭州市"
        : node.cluster.includes("North")
          ? "北京市"
          : "深圳市";
    return {
      apiVersion: "rlinf.io/v1alpha1",
      kind: "Node",
      metadata: {
        name: node.name,
        namespace: node.cluster,
        labels: {
          [NODE_CATEGORY_LABEL]: categoryByKind[node.kind],
          "rlark.io/model": node.model,
          "rlark.io/city": city,
          ...(node.kind !== "CloudCompute" && node.tasks > 0
            ? {
                "rlark.io/embodied-task": "true",
                "rlark.io/embodied-task-name":
                  node.kind === "Robot"
                    ? `robot-operation-${node.id.replace(/^robot-/, "")}`
                    : `edge-inference-${node.id.replace(/^edge-/, "")}`,
              }
            : {}),
        },
        creationTimestamp: "2026-06-29T10:00:00Z",
      },
      spec: {
        agentType:
          node.kind === "Robot"
            ? "Robot"
            : node.kind === "EmbodiedCompute"
              ? "Edge"
              : "Kubernetes",
        unschedulable: node.phase === "Offline",
      },
      status: {
        phase: node.phase,
        reason: node.robotState,
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
          "nvidia.com/gpu": node.gpu.split(" / ")[1] ?? "0",
          ...(node.kind !== "CloudCompute"
            ? {
                [`rlinf.io/device-${node.model.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`]:
                  "1",
              }
            : {}),
        },
        used: {
          cpu: `${node.cpu}%`,
          memory: `${node.memory}%`,
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
    fetch("/api/v1/rlinf.io/v1alpha1/nodes")
      .then((r) =>
        r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)),
      )
      .then((data) => setNodes(data.items ?? []))
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
