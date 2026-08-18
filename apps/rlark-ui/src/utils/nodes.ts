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

function resourceNumber(value?: string): number {
  const parsed = Number.parseFloat(value ?? "0");
  return Number.isFinite(parsed) ? parsed : 0;
}

function formatResourceNumber(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

export function getGPUResourceKey(node: CRDNode): string {
  const resources = [
    node.status?.capacity ?? {},
    node.status?.allocatable ?? {},
    node.status?.used ?? {},
  ];
  if (resources.some((values) => "nvidia.com/gpu" in values)) {
    return "nvidia.com/gpu";
  }
  return (
    resources
      .flatMap(Object.keys)
      .find((key) => /(^|[./-])gpu($|[./-])/i.test(key)) ?? "nvidia.com/gpu"
  );
}

export function parseResourceQuantity(
  key: string,
  raw?: string,
): number | null {
  if (!raw) return null;
  const match = raw.trim().match(/^([+-]?(?:\d+(?:\.\d+)?|\.\d+))([a-zA-Z]*)$/);
  if (!match) return null;
  const value = Number.parseFloat(match[1]);
  if (!Number.isFinite(value)) return null;
  const suffix = match[2];
  if (key === "cpu") {
    const factors: Record<string, number> = { n: 1e-9, u: 1e-6, m: 1e-3 };
    return value * (factors[suffix] ?? 1);
  }
  if (key === "memory" || key === "ephemeral-storage") {
    const factors: Record<string, number> = {
      Ki: 1024,
      Mi: 1024 ** 2,
      Gi: 1024 ** 3,
      Ti: 1024 ** 4,
      K: 1000,
      M: 1000 ** 2,
      G: 1000 ** 3,
      T: 1000 ** 4,
    };
    return value * (factors[suffix] ?? 1);
  }
  return value;
}

export function formatResourceQuantity(key: string, raw?: string): string {
  const value = parseResourceQuantity(key, raw);
  if (value === null) return "—";
  if (key === "memory" || key === "ephemeral-storage") {
    const gb = value / 1000 ** 3;
    return `${gb >= 100 ? gb.toFixed(0) : gb.toFixed(1)} GB`;
  }
  if (key === "cpu") return `${formatResourceNumber(value)} 核`;
  return formatResourceNumber(value);
}

export function getNodeResourceSummary(
  node: CRDNode,
  zh: boolean,
): { primary: string; secondary: string } {
  const category = getNodeCategory(node);
  const capacity = node.status?.capacity ?? node.status?.allocatable ?? {};
  const allocatable = node.status?.allocatable ?? capacity;
  const used = node.status?.used ?? {};
  const model =
    category === "cloud" ? getNodeGPUModel(node) : getNodeDeviceModel(node);

  // Existing GPU nodes may predate the RLark category label. The Kubernetes
  // extended resource is authoritative, so show it regardless of category.
  const gpuKey = getGPUResourceKey(node);
  const gpuTotal = resourceNumber(capacity[gpuKey] ?? allocatable[gpuKey]);
  if (gpuTotal > 0 && (category === "cloud" || category === "unknown")) {
    const allocatableGPU = resourceNumber(
      allocatable[gpuKey] ?? capacity[gpuKey],
    );
    const hasUsage = Object.prototype.hasOwnProperty.call(used, gpuKey);
    const available = Math.max(
      0,
      allocatableGPU - resourceNumber(used[gpuKey]),
    );
    return {
      primary: hasUsage
        ? `${formatResourceNumber(gpuTotal)} GPU · ${zh ? "可用" : "free"} ${formatResourceNumber(available)}`
        : `${formatResourceNumber(gpuTotal)} GPU · ${zh ? "可分配" : "allocatable"} ${formatResourceNumber(allocatableGPU)}`,
      secondary: model || "—",
    };
  }

  if (category === "cloud") {
    const total = resourceNumber(capacity["nvidia.com/gpu"]);
    const hasUsage = Object.prototype.hasOwnProperty.call(
      used,
      "nvidia.com/gpu",
    );
    const available = Math.max(
      0,
      resourceNumber(allocatable["nvidia.com/gpu"]) -
        resourceNumber(used["nvidia.com/gpu"]),
    );
    return {
      primary:
        total > 0
          ? hasUsage
            ? `${formatResourceNumber(total)} GPU · ${zh ? "空闲" : "free"} ${formatResourceNumber(available)}`
            : `${formatResourceNumber(total)} GPU · ${zh ? `可分配 ${formatResourceNumber(resourceNumber(allocatable["nvidia.com/gpu"]))}` : `allocatable ${formatResourceNumber(resourceNumber(allocatable["nvidia.com/gpu"]))}`}`
          : zh
            ? "暂无 GPU"
            : "No GPU",
      secondary: model || "—",
    };
  }

  if (category === "edge" || category === "robot") {
    const resourceKeys = Array.from(
      new Set([
        ...Object.keys(capacity),
        ...Object.keys(allocatable),
        ...Object.keys(used),
      ]),
    ).filter(
      (key) => key === "rlinf.io/device" || key.startsWith("rlinf.io/device-"),
    );
    const total = resourceKeys.reduce(
      (sum, key) => sum + resourceNumber(capacity[key] ?? allocatable[key]),
      0,
    );
    const available = Math.max(
      0,
      resourceKeys.reduce(
        (sum, key) =>
          sum +
          Math.max(
            0,
            resourceNumber(allocatable[key] ?? capacity[key]) -
              resourceNumber(used[key]),
          ),
        0,
      ),
    );
    const hasUsage = resourceKeys.some((key) =>
      Object.prototype.hasOwnProperty.call(used, key),
    );
    const resourceModels = resourceKeys
      .map((key) => key.replace(/^rlinf\.io\/device-?/, ""))
      .filter(Boolean);
    return {
      primary:
        total > 0
          ? hasUsage
            ? `${formatResourceNumber(total)} ${zh ? "台设备" : "devices"} · ${zh ? "空闲" : "free"} ${formatResourceNumber(available)}`
            : `${formatResourceNumber(total)} ${zh ? "台设备" : "devices"} · ${zh ? "空闲未知" : "free unknown"}`
          : zh
            ? "暂无设备"
            : "No devices",
      secondary: model || resourceModels.join(" / ") || "—",
    };
  }

  return { primary: zh ? "暂无资源" : "No resources", secondary: model || "—" };
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
        annotations: {
          "rlark.io/ip-location": JSON.stringify({ city }),
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
