import assert from "node:assert/strict";
import test from "node:test";

import {
  defaultClusterName,
  newClusterProblem,
  provisionView,
  stageNeedsPassword,
  tunnelEndpointProblem,
} from "../dist/test/astribot/lib/clusterProvision.js";

const form = { site: "baidu", cluster: "", address: "1.2.3.4", port: "22", user: "root", password: "p", tunnelEndpoint: "" };

test("a new cluster is named after its site unless named", () => {
  assert.equal(defaultClusterName("baidu"), "rlark-baidu-gpu");
  assert.equal(newClusterProblem(form, "en"), "");
  assert.match(newClusterProblem({ ...form, cluster: "baidu" }, "en"), /rlark-/);
  assert.match(newClusterProblem({ ...form, site: "Baidu" }, "en"), /site/);
  assert.match(newClusterProblem({ ...form, address: "root@1.2.3.4" }, "en"), /IP/);
  assert.match(newClusterProblem({ ...form, password: "" }, "zh"), /密码/);
  assert.match(newClusterProblem({ ...form, port: "70000" }, "zh"), /SSH 端口/);
});

test("machines behind one public IP give the UDP port forwarded to each", () => {
  assert.equal(newClusterProblem({ ...form, port: "8023", tunnelEndpoint: "180.76.174.250:8100" }, "zh"), "");
  assert.equal(tunnelEndpointProblem("", "en"), "");
  assert.match(tunnelEndpointProblem("180.76.174.250", "zh"), /公网IP:UDP端口/);
  assert.match(tunnelEndpointProblem("180.76.174.256:8100", "en"), /public-ip:udp-port/);
  assert.match(tunnelEndpointProblem("180.76.174.250:0", "en"), /public-ip:udp-port/);
});

test("a retry logs in again up to Station's stage", () => {
  assert.equal(stageNeedsPassword("precheck"), true);
  assert.equal(stageNeedsPassword("station"), true);
  assert.equal(stageNeedsPassword("addons"), false);
  assert.equal(stageNeedsPassword("accept"), false);
});

test("the view shows every stage, where it stopped and why", () => {
  const view = provisionView(
    {
      apiVersion: "rlinf.io/v1alpha1",
      kind: "ClusterProvision",
      spec: { site: "baidu", machine: { address: "1.2.3.4", user: "root" } },
      status: {
        phase: "Failed",
        stage: "control-plane",
        stages: [
          { name: "precheck", state: "Done", readback: "bd-gpu-1, 8 × RTX PRO 6000" },
          { name: "control-plane", state: "Failed", message: "kubeadm init failed" },
        ],
      },
    },
    "zh",
  );
  assert.equal(view.stages.length, 8);
  assert.deepEqual(
    view.stages.map((s) => s.state),
    ["Done", "Pending", "Pending", "Failed", "Pending", "Pending", "Pending", "Pending"],
  );
  assert.equal(view.stages[0].label, "预检");
  assert.equal(view.stages[3].message, "kubeadm init failed");
  assert.equal(view.canRetry, true);
  assert.equal(view.retryNeedsPassword, true);
});

test("a making that does not run can be taken away; one that reached the machine puts it back", () => {
  const provision = (phase, stages) => ({ apiVersion: "rlinf.io/v1alpha1", kind: "ClusterProvision", spec: { site: "baidu", machine: { address: "1.2.3.4", user: "root" } }, status: { phase, stage: "precheck", stages } });
  const stoppedEarly = provisionView(provision("Failed", [{ name: "precheck", state: "Failed" }]), "zh");
  assert.equal(stoppedEarly.canClear, true);
  assert.equal(stoppedEarly.clearReachesMachine, false);
  const stoppedLate = provisionView(provision("Failed", [{ name: "precheck", state: "Done" }, { name: "bundle", state: "Done" }, { name: "declare", state: "Failed" }]), "zh");
  assert.equal(stoppedLate.clearReachesMachine, true);
  assert.equal(provisionView(provision("Running", []), "zh").canClear, false);
});

test("a stage the making had nothing to do in shows as skipped, with why", () => {
  const view = provisionView(
    {
      apiVersion: "rlinf.io/v1alpha1",
      kind: "ClusterProvision",
      spec: { site: "baidu", machine: { address: "1.2.3.4", user: "root" } },
      status: { phase: "Running", stage: "declare", stages: [{ name: "bundle", state: "Skipped", readback: "the installer is there" }] },
    },
    "en",
  );
  assert.equal(view.stages[1].state, "Skipped");
  assert.equal(view.stages[1].readback, "the installer is there");
  assert.equal(view.stages[0].label, "Pre-check");
});
