import { useEffect, useState } from "react";
import { Bot, CircleDot, CloudCog, Server } from "lucide-react";
import { nodes as mockNodes, type NodeKind } from "../data";
import type { CRDNode, CRDNodeLite, NodeCategory } from "../types";

const NODE_CATEGORY_LABEL = "rlark.io/node-category";

export function getNodeCategory(node: CRDNode): NodeCategory {
  const v = node.metadata.labels?.[NODE_CATEGORY_LABEL];
  if (v === "cloud" || v === "edge" || v === "robot") return v;
  return "unknown";
}

export function getNodeLocation(node: CRDNode): string {
  const raw = node.metadata.annotations?.["rlark.io/ip-location"];
  if (raw) {
    try {
      const location = JSON.parse(raw) as {
        city?: string;
        province?: string;
        country?: string;
      };
      const parts = [location.province, location.city].filter(
        (value, index, values): value is string =>
          Boolean(value) && values.indexOf(value) === index,
      );
      if (parts.length > 0) return parts.join(" · ");
      if (location.country) return location.country;
    } catch {
      // Fall back to the legacy city label below.
    }
  }
  return node.metadata.labels?.["rlark.io/city"] ?? "";
}

function resourceNumber(value?: string): number {
  const parsed = Number.parseFloat(value ?? "0");
  return Number.isFinite(parsed) ? parsed : 0;
}

function formatResourceNumber(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

export function getNodeResourceSummary(
  node: CRDNode,
  zh: boolean,
): { primary: string; secondary: string } {
  const category = getNodeCategory(node);
  const capacity = node.status?.capacity ?? node.status?.allocatable ?? {};
  const allocatable = node.status?.allocatable ?? capacity;
  const used = node.status?.used ?? {};
  const model = node.metadata.labels?.["rlark.io/model"] ?? "";

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
