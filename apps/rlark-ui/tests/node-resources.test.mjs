import assert from "node:assert/strict";
import test from "node:test";
import {
  formatResourceQuantity,
  getNodeDiskUsage,
  getNodeResourceSummary,
  getResourceUsagePercent,
  isDiskUsageWarning,
  selectDeviceResourceKey,
} from "../dist/test/utils/nodeResources.js";

test("formats Kubernetes binary quantities without inflating them", () => {
  assert.equal(
    formatResourceQuantity("ephemeral-storage", "3748906852Ki"),
    "3575 GiB",
  );
  assert.equal(formatResourceQuantity("memory", "1Gi"), "1.0 GiB");
  assert.equal(formatResourceQuantity("memory", "1000M"), "0.9 GiB");
});

test("calculates disk usage against allocatable capacity", () => {
  assert.equal(
    getResourceUsagePercent("ephemeral-storage", "90Gi", "100Gi"),
    90,
  );
  assert.equal(
    getResourceUsagePercent("ephemeral-storage", "89%", "100Gi"),
    89,
  );
});

test("warns when real disk usage reaches ninety percent", () => {
  const node = {
    metadata: { name: "disk-warning" },
    spec: {},
    status: {
      storage: {
        capacityBytes: 1000,
        usedBytes: 900,
        availableBytes: 100,
      },
    },
  };
  assert.equal(isDiskUsageWarning(node), true);
  node.status.storage.usedBytes = 899;
  node.status.storage.availableBytes = 100;
  assert.equal(isDiskUsageWarning(node), true);
  node.status.storage.usedBytes = 894;
  node.status.storage.availableBytes = 106;
  assert.equal(isDiskUsageWarning(node), false);
});

test("keeps disk usage below one hundred percent while space remains", () => {
  const usage = getNodeDiskUsage({
    metadata: { name: "nearly-full" },
    spec: {},
    status: {
      storage: {
        capacityBytes: 3838880616448,
        usedBytes: 3827780616448,
        availableBytes: 11100000000,
      },
    },
  });
  assert.equal(usage?.percent, 99);
});

test("uses available bytes as the source of truth for disk percentage", () => {
  const usage = getNodeDiskUsage({
    metadata: { name: "inconsistent-stats" },
    spec: {},
    status: {
      storage: {
        capacityBytes: 1000,
        usedBytes: 1000,
        availableBytes: 100,
      },
    },
  });
  assert.equal(usage?.percent, 90);
});

test("shows one hundred percent only when no disk space remains", () => {
  const usage = getNodeDiskUsage({
    metadata: { name: "full" },
    spec: {},
    status: {
      storage: {
        capacityBytes: 1000,
        usedBytes: 1000,
        availableBytes: 0,
      },
    },
  });
  assert.equal(usage?.percent, 100);
});

test("warns whenever kubelet reports disk pressure", () => {
  const node = {
    metadata: { name: "disk-pressure" },
    spec: {},
    status: {
      diskPressure: true,
      storage: {
        capacityBytes: 1000,
        usedBytes: 100,
        availableBytes: 900,
      },
    },
  };
  assert.equal(isDiskUsageWarning(node), true);
});

test("prefers a positive modeled device resource over zero-capacity keys", () => {
  const capacity = {
    "rlinf.io/device": "0",
    "rlinf.io/device-franka": "1",
    "rlinf.io/device-macvlan": "0",
  };
  assert.equal(
    selectDeviceResourceKey(capacity, capacity),
    "rlinf.io/device-franka",
  );
});

test("uses the positive generic device resource only as a fallback", () => {
  assert.equal(
    selectDeviceResourceKey(
      { "rlinf.io/device": "1", "rlinf.io/device-franka": "0" },
      { "rlinf.io/device": "1", "rlinf.io/device-franka": "0" },
    ),
    "rlinf.io/device",
  );
});

test("shows GPU and embodied device resources together on an edge node", () => {
  const summary = getNodeResourceSummary(
    {
      metadata: {
        name: "hybrid-edge",
        labels: { "rlark.io/node-category": "edge" },
        annotations: {
          "rlark.io/gpu-model": "NVIDIA RTX 4090",
          "rlark.io/device-model": "Franka",
        },
      },
      spec: {},
      status: {
        capacity: { "nvidia.com/gpu": "2", "rlinf.io/device-franka": "1" },
        allocatable: {
          "nvidia.com/gpu": "2",
          "rlinf.io/device-franka": "1",
        },
        used: { "nvidia.com/gpu": "1", "rlinf.io/device-franka": "0" },
      },
    },
    true,
  );

  assert.deepEqual(
    summary.lines.map(({ kind, primary, secondary }) => ({
      kind,
      primary,
      secondary,
    })),
    [
      { kind: "gpu", primary: "1 / 2 GPU", secondary: "NVIDIA RTX 4090" },
      { kind: "device", primary: "1 / 1 设备", secondary: "Franka" },
    ],
  );
});

test("does not fabricate capacity from a configured model", () => {
  const summary = getNodeResourceSummary(
    {
      metadata: {
        name: "metadata-only",
        annotations: { "rlark.io/gpu-model": "NVIDIA RTX 4090" },
      },
      spec: {},
    },
    true,
  );
  assert.deepEqual(summary.lines, []);
  assert.equal(summary.primary, "无设备");
});

test("marks a GPU resource without a configured model as unlabeled", () => {
  const summary = getNodeResourceSummary(
    {
      metadata: { name: "unlabeled-gpu" },
      spec: {},
      status: {
        capacity: { "nvidia.com/gpu": "1" },
        allocatable: { "nvidia.com/gpu": "1" },
      },
    },
    true,
  );
  assert.equal(summary.lines[0].label, "未标注");
  assert.equal(summary.lines[0].amount, "1 / 1 GPU");
});

test("includes other extended device resources", () => {
  const summary = getNodeResourceSummary(
    {
      metadata: { name: "fpga-node" },
      spec: {},
      status: {
        capacity: { "example.com/fpga": "4", cpu: "16" },
        allocatable: { "example.com/fpga": "3", cpu: "15" },
        used: { "example.com/fpga": "1" },
      },
    },
    true,
  );
  assert.equal(summary.lines.length, 1);
  assert.equal(summary.lines[0].kind, "other");
  assert.equal(summary.lines[0].label, "fpga");
  assert.equal(summary.lines[0].amount, "2 / 4 设备");
});

test("ignores virtual quota and derived GPU resources", () => {
  const summary = getNodeResourceSummary(
    {
      metadata: { name: "quota-node" },
      spec: {},
      status: {
        capacity: {
          "rlark.io/aicoder": "60",
          "rlark.io/vcpu": "64",
          "rlark.io/vmem": "503",
          "rlark.io/spot-gpu": "8",
          "rlark.io/share-gpu-3": "3",
        },
      },
    },
    true,
  );
  assert.deepEqual(summary.lines, []);
  assert.equal(summary.primary, "无设备");
});
