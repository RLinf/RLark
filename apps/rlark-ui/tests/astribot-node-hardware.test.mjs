import assert from "node:assert/strict";
import test from "node:test";
import { formatHardwareBytes, formatHardwareValue, hardwareFieldLabel, hardwareForNode } from "../dist/test/astribot/lib/mockNodeHardware.js";

const robotCluster = "深圳公司S1U调试集群";
const localCluster = "深圳公司本地开发集群";

test("3090 fixture exposes current snapshot and explicit unsupported DCGM fields", () => {
  const hardware = hardwareForNode(robotCluster, "s1u-01-x86");
  assert.equal(hardware.state, "ready");
  assert.equal(hardware.gpus.length, 2);
  assert.equal(hardware.gpus[0].model, "NVIDIA GeForce RTX 3090");
  assert.equal(hardware.gpus[0].utilizationPercent.value, 68);
  assert.equal(hardware.gpus[0].temperatureC.value, null);
  assert.equal(hardware.gpus[0].temperatureC.capability, "unsupported");
  assert.equal(hardware.allocatedGPUs, 1);
  assert.notEqual(hardware.allocatedGPUs, hardware.gpus[0].utilizationPercent.value);
});

test("Orin fixture carries tegrastats fields and labels its source", () => {
  const hardware = hardwareForNode(localCluster, "wendychen");
  assert.equal(hardware.tegra.source, "tegrastats");
  assert.equal(hardware.tegra.gr3dPercent, 42);
  assert.equal(hardware.tegra.railPowerWatts, 7.6);
});

test("no-GPU, stale, and exporter query failure are separate states", () => {
  assert.equal(hardwareForNode(robotCluster, "alexfu").state, "no-gpu");
  assert.equal(hardwareForNode(robotCluster, "s1u-02-x86").state, "stale");
  assert.equal(hardwareForNode(robotCluster, "s1u-03-x86").state, "error");
  assert.match(hardwareForNode(robotCluster, "s1u-03-x86").error, /超时/);
});

test("missing numeric values render as a dash instead of zero", () => {
  assert.equal(formatHardwareValue(null, "%"), "—");
  assert.equal(formatHardwareValue(Number.NaN), "—");
  assert.equal(formatHardwareBytes(null), "—");
  assert.equal(hardwareFieldLabel("unsupported", true), "不支持");
  assert.equal(hardwareFieldLabel("unavailable", false), "Unavailable");
});

test("fixtures are scoped to cluster and node identity", () => {
  assert.equal(hardwareForNode("other-cluster", "s1u-01-x86"), undefined);
  assert.equal(hardwareForNode(undefined, "s1u-01-x86"), undefined);
});
