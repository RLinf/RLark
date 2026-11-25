export type HardwareReading<T> = {
  value: T | null;
  capability: "available" | "unsupported" | "unavailable";
};

export type NodeHardwareSnapshot = {
  state: "ready" | "stale" | "error" | "no-gpu";
  observedAt: string;
  source: "node-exporter + DCGM" | "node-exporter + tegrastats" | "node-exporter";
  cpu: { model: string; cores: number; usagePercent: number | null };
  memory: { capacityBytes: number; usedBytes: number | null; usagePercent: number | null };
  disks: Array<{ filesystem: string; usedBytes: number | null; capacityBytes: number }>;
  network: { rxBytesPerSecond: number | null; txBytesPerSecond: number | null };
  allocatedGPUs: number;
  gpus: Array<{
    index: number;
    model: string;
    memoryUsedBytes: HardwareReading<number>;
    memoryTotalBytes: number;
    utilizationPercent: HardwareReading<number>;
    temperatureC: HardwareReading<number>;
    powerWatts: HardwareReading<number>;
  }>;
  tegra?: {
    gr3dPercent: number | null;
    ramUsedBytes: number | null;
    ramTotalBytes: number;
    temperatureC: number | null;
    railPowerWatts: number | null;
    source: "tegrastats";
  };
  error?: string;
};

const minuteAgo = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString();
const gib = (value: number) => value * 1024 ** 3;
const reading = <T,>(value: T | null, capability: HardwareReading<T>["capability"] = value === null ? "unsupported" : "available"): HardwareReading<T> => ({ value, capability });

export const mockNodeHardware: Record<string, NodeHardwareSnapshot> = {
  "深圳公司S1U调试集群/s1u-01-x86": {
    state: "ready", observedAt: minuteAgo(1), source: "node-exporter + DCGM",
    cpu: { model: "AMD Ryzen 9 5950X", cores: 16, usagePercent: 27 },
    memory: { capacityBytes: gib(128), usedBytes: gib(64.2), usagePercent: 50.2 },
    disks: [{ filesystem: "/", usedBytes: gib(182), capacityBytes: gib(476) }, { filesystem: "/var/lib/kubelet", usedBytes: gib(91), capacityBytes: gib(476) }],
    network: { rxBytesPerSecond: 18.4 * 1024 ** 2, txBytesPerSecond: 6.8 * 1024 ** 2 },
    allocatedGPUs: 1,
    gpus: [
      { index: 0, model: "NVIDIA GeForce RTX 3090", memoryUsedBytes: reading(gib(19.2)), memoryTotalBytes: gib(24), utilizationPercent: reading(68), temperatureC: reading(null), powerWatts: reading(null) },
      { index: 1, model: "NVIDIA GeForce RTX 3090", memoryUsedBytes: reading(gib(4.1)), memoryTotalBytes: gib(24), utilizationPercent: reading(12), temperatureC: reading(null), powerWatts: reading(null) },
    ],
  },
  "深圳公司本地开发集群/wendychen": {
    state: "ready", observedAt: minuteAgo(1), source: "node-exporter + tegrastats",
    cpu: { model: "NVIDIA Carmel ARMv8.2", cores: 12, usagePercent: 18 },
    memory: { capacityBytes: gib(32), usedBytes: gib(9.6), usagePercent: 30 },
    disks: [{ filesystem: "/", usedBytes: gib(39), capacityBytes: gib(119) }],
    network: { rxBytesPerSecond: 2.4 * 1024 ** 2, txBytesPerSecond: 1.1 * 1024 ** 2 },
    allocatedGPUs: 0, gpus: [],
    tegra: { gr3dPercent: 42, ramUsedBytes: gib(9.6), ramTotalBytes: gib(32), temperatureC: 52, railPowerWatts: 7.6, source: "tegrastats" },
  },
  "深圳公司S1U调试集群/alexfu": {
    state: "no-gpu", observedAt: minuteAgo(2), source: "node-exporter",
    cpu: { model: "Intel Xeon Silver 4314", cores: 32, usagePercent: 8 },
    memory: { capacityBytes: gib(64), usedBytes: gib(21.4), usagePercent: 33.4 },
    disks: [{ filesystem: "/", usedBytes: gib(84), capacityBytes: gib(238) }],
    network: { rxBytesPerSecond: 420 * 1024, txBytesPerSecond: 780 * 1024 },
    allocatedGPUs: 0, gpus: [],
  },
  "深圳公司S1U调试集群/s1u-02-x86": {
    state: "stale", observedAt: minuteAgo(48), source: "node-exporter + DCGM",
    cpu: { model: "AMD Ryzen 9 5950X", cores: 16, usagePercent: 4 },
    memory: { capacityBytes: gib(128), usedBytes: gib(58), usagePercent: 45.3 },
    disks: [{ filesystem: "/", usedBytes: gib(180), capacityBytes: gib(476) }],
    network: { rxBytesPerSecond: null, txBytesPerSecond: null },
    allocatedGPUs: 0,
    gpus: [{ index: 0, model: "NVIDIA GeForce RTX 3090", memoryUsedBytes: reading(null, "unavailable"), memoryTotalBytes: gib(24), utilizationPercent: reading(null, "unavailable"), temperatureC: reading(null, "unsupported"), powerWatts: reading(null, "unsupported") }],
  },
  "深圳公司S1U调试集群/s1u-03-x86": {
    state: "error", observedAt: "", source: "node-exporter + DCGM",
    cpu: { model: "", cores: 0, usagePercent: null },
    memory: { capacityBytes: 0, usedBytes: null, usagePercent: null }, disks: [],
    network: { rxBytesPerSecond: null, txBytesPerSecond: null }, allocatedGPUs: 0, gpus: [],
    error: "Prometheus 查询超时，请检查该集群的采集服务。",
  },
};

export function hardwareForNode(clusterName: string | undefined, nodeName: string): NodeHardwareSnapshot | undefined {
  return clusterName ? mockNodeHardware[`${clusterName}/${nodeName}`] : undefined;
}

export function formatHardwareValue(value: number | null | undefined, suffix = ""): string {
  return value === null || value === undefined || !Number.isFinite(value) ? "—" : `${value}${suffix}`;
}

export function formatHardwareBytes(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return "—";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) { amount /= 1024; unit += 1; }
  return `${amount >= 10 || unit === 0 ? amount.toFixed(0) : amount.toFixed(1)} ${units[unit]}`;
}

export function hardwareFieldLabel(capability: HardwareReading<unknown>["capability"], zh: boolean): string {
  if (capability === "unsupported") return zh ? "不支持" : "Unsupported";
  if (capability === "unavailable") return zh ? "暂无数据" : "Unavailable";
  return zh ? "可用" : "Available";
}
