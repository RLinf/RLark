import assert from "node:assert/strict";
import test from "node:test";

import { joinStageLabel, joinStageNeedsPassword, joinStages, nodeJoinView } from "../dist/test/astribot/lib/nodeJoin.js";

const join = (phase, stage, stages) => ({
  apiVersion: "rlinf.io/v1alpha1",
  kind: "NodeJoin",
  spec: { clusterID: "rlark-baidu-gpu", nodeName: "gpu-2", machine: { address: "180.76.174.250", port: 8023, user: "root" } },
  status: { phase, stage, stages },
});

test("an SSH join shows its eight stages in order, a skipped one with why", () => {
  const view = nodeJoinView(join("Running", "install", [
    { name: "precheck", state: "Done", readback: "root@180.76.174.250:8023" },
    { name: "bundle", state: "Skipped", readback: "机器直接到得了 Station API，不需要隧道" },
    { name: "install", state: "Running", message: "10:00:02 admitted as gpu-2" },
  ]), "zh");
  assert.deepEqual(view.stages.map((s) => s.name), ["precheck", "bundle", "declare", "tunnel", "install", "node-ready", "station", "gpu"]);
  assert.deepEqual(view.stages.map((s) => s.state), ["Done", "Skipped", "Pending", "Pending", "Running", "Pending", "Pending", "Pending"]);
  assert.equal(view.stages[0].label, "登录和预检");
  assert.equal(view.stages[1].readback, "机器直接到得了 Station API，不需要隧道");
  assert.equal(view.stages[4].message, "10:00:02 admitted as gpu-2");
  assert.equal(view.canRetry, false);
  assert.equal(view.canClear, false);
  assert.equal(joinStageLabel("tunnel", "en"), "Tunnel");
  assert.equal(joinStages.length, 8);
});

test("a retry logs in again up to install; after it, it does not", () => {
  for (const stage of ["precheck", "bundle", "declare", "tunnel", "install"]) assert.equal(joinStageNeedsPassword(stage), true, stage);
  for (const stage of ["node-ready", "station", "gpu"]) assert.equal(joinStageNeedsPassword(stage), false, stage);
  assert.equal(nodeJoinView(join("Failed", "tunnel", [{ name: "tunnel", state: "Failed" }]), "zh").retryNeedsPassword, true);
  const ready = nodeJoinView(join("Failed", "node-ready", [{ name: "node-ready", state: "Failed" }]), "zh");
  assert.equal(ready.canRetry, true);
  assert.equal(ready.retryNeedsPassword, false);
});

test("clearing logs in to the machine only to take away a tunnel its code's line has not used", () => {
  const atTunnel = nodeJoinView(join("Failed", "tunnel", [{ name: "tunnel", state: "Failed" }]), "zh");
  assert.equal(atTunnel.canClear, true);
  assert.equal(atTunnel.clearReachesMachine, true);
  const direct = nodeJoinView(join("Failed", "install", [{ name: "tunnel", state: "Skipped" }, { name: "install", state: "Failed" }]), "zh");
  assert.equal(direct.clearReachesMachine, false);
  const installed = nodeJoinView(join("Failed", "station", [{ name: "tunnel", state: "Done" }, { name: "install", state: "Done" }, { name: "station", state: "Failed" }]), "zh");
  assert.equal(installed.clearReachesMachine, false);
  assert.equal(nodeJoinView(join("Failed", "precheck", [{ name: "precheck", state: "Failed" }]), "zh").clearReachesMachine, false);
});
