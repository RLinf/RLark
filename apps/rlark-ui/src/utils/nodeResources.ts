import type { CRDNode } from "../types";

export type NodeResourceLine = {
  key: string;
  kind: "gpu" | "device" | "other";
  label: string;
  amount: string;
  primary: string;
  secondary: string;
};

function resourceNumber(value?: string): number {
  const parsed = Number.parseFloat(value ?? "0");
  return Number.isFinite(parsed) ? parsed : 0;
}

function formatResourceNumber(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

function metadataValue(node: CRDNode, key: string): string {
  return node.metadata.annotations?.[key] ?? node.metadata.labels?.[key] ?? "";
}

function isOtherDeviceResourceKey(key: string): boolean {
  if (!key.includes("/") || /(^|[./-])gpu($|[./-])/i.test(key)) return false;
  return /(^|[./-])(camera|fpga|hca|infiniband|npu|rdma|robot|tpu)($|[./-])/i.test(
    key,
  );
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
  const keys = resources.flatMap(Object.keys);
  return (
    keys.find((key) => /\/gpu$/i.test(key)) ??
    keys.find(
      (key) =>
        /(^|[./-])gpu($|[./-])/i.test(key) &&
        !/(^|[./-])(share|spot|virtual|v)-?gpu($|[./-])/i.test(key) &&
        !/(^|[./-])gpu-?(core|mem|memory)($|[./-])/i.test(key),
    ) ??
    "nvidia.com/gpu"
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
): { primary: string; secondary: string; lines: NodeResourceLine[] } {
  const capacity = node.status?.capacity ?? node.status?.allocatable ?? {};
  const allocatable = node.status?.allocatable ?? capacity;
  const used = node.status?.used ?? {};
  const lines: NodeResourceLine[] = [];

  const gpuKey = getGPUResourceKey(node);
  const gpuTotal = resourceNumber(capacity[gpuKey] ?? allocatable[gpuKey]);
  if (gpuTotal > 0) {
    const allocatableGPU = resourceNumber(
      allocatable[gpuKey] ?? capacity[gpuKey],
    );
    const available = Math.max(
      0,
      allocatableGPU - resourceNumber(used[gpuKey]),
    );
    const label =
      metadataValue(node, "rlark.io/gpu-model") ||
      metadataValue(node, "rlark.io/model") ||
      (zh ? "未标注" : "Unlabeled");
    const amount = `${formatResourceNumber(available)} / ${formatResourceNumber(gpuTotal)} GPU`;
    lines.push({
      key: gpuKey,
      kind: "gpu",
      label,
      amount,
      primary: amount,
      secondary: label,
    });
  }

  const deviceKeys = Array.from(
    new Set([
      ...Object.keys(capacity),
      ...Object.keys(allocatable),
      ...Object.keys(used),
    ]),
  ).filter(
    (key) => key === "rlinf.io/device" || key.startsWith("rlinf.io/device-"),
  );
  const deviceTotal = deviceKeys.reduce(
    (sum, key) => sum + resourceNumber(capacity[key] ?? allocatable[key]),
    0,
  );
  if (deviceTotal > 0) {
    const available = deviceKeys.reduce(
      (sum, key) =>
        sum +
        Math.max(
          0,
          resourceNumber(allocatable[key] ?? capacity[key]) -
            resourceNumber(used[key]),
        ),
      0,
    );
    const resourceModels = deviceKeys
      .map((key) => key.replace(/^rlinf\.io\/device-?/, ""))
      .filter(Boolean);
    const label =
      metadataValue(node, "rlark.io/device-model") ||
      metadataValue(node, "rlark.io/model") ||
      resourceModels.join(" / ") ||
      (zh ? "未标注" : "Unlabeled");
    const amount = `${formatResourceNumber(available)} / ${formatResourceNumber(deviceTotal)} ${zh ? "设备" : "devices"}`;
    lines.push({
      key: "rlinf.io/device",
      kind: "device",
      label,
      amount,
      primary: amount,
      secondary: label,
    });
  }

  const allResourceKeys = Array.from(
    new Set([
      ...Object.keys(capacity),
      ...Object.keys(allocatable),
      ...Object.keys(used),
    ]),
  );
  const otherDeviceKeys = allResourceKeys.filter(
    (key) =>
      isOtherDeviceResourceKey(key) &&
      key !== gpuKey &&
      !deviceKeys.includes(key) &&
      resourceNumber(capacity[key] ?? allocatable[key]) > 0,
  );
  otherDeviceKeys.forEach((key) => {
    const total = resourceNumber(capacity[key] ?? allocatable[key]);
    const available = Math.max(
      0,
      resourceNumber(allocatable[key] ?? capacity[key]) -
        resourceNumber(used[key]),
    );
    const label = key.split("/").pop() || key;
    const amount = `${formatResourceNumber(available)} / ${formatResourceNumber(total)} ${zh ? "设备" : "devices"}`;
    lines.push({
      key,
      kind: "other",
      label,
      amount,
      primary: amount,
      secondary: label,
    });
  });

  return {
    primary: lines[0]?.primary ?? (zh ? "无设备" : "No devices"),
    secondary: lines.map((line) => line.secondary).join(" / ") || "—",
    lines,
  };
}
