import assert from "node:assert/strict";
import test from "node:test";
import { getNodeResourceSummary } from "../dist/test/utils/nodeResources.js";

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
