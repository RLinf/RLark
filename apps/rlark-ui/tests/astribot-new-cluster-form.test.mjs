import assert from "node:assert/strict";
import test from "node:test";
import { newClusterProblem } from "../dist/test/astribot/lib/clusterProvision.js";
import { firstNewClusterProblem, newClusterFieldProblems, newClusterSummary, provisionProgress } from "../dist/test/astribot/lib/newClusterForm.js";

const form = { clusterType: "gpu", site: "baidu", cluster: "", address: "1.2.3.4", port: "22", user: "root", password: "secret", tunnelEndpoint: "" };

test("field problems identify all actionable errors and preserve the first focus order", () => {
  const problems = newClusterFieldProblems({ ...form, cluster: "bad name", address: "", port: "70000", user: "", password: "", tunnelEndpoint: "bad" }, "en");
  assert.ok(problems.cluster);
  assert.ok(problems.address);
  assert.ok(problems.port);
  assert.ok(problems.user);
  assert.ok(problems.password);
  assert.ok(problems.tunnelEndpoint);
  assert.equal(firstNewClusterProblem(problems), "cluster");
  assert.equal(firstNewClusterProblem(newClusterFieldProblems({ ...form, clusterType: "" }, "en")), "clusterType");
});

test("valid and invalid form results agree with the final provision guard", () => {
  assert.deepEqual(newClusterFieldProblems(form, "en"), {});
  assert.equal(newClusterProblem(form, "en"), "");
  const invalid = { ...form, tunnelEndpoint: "180.76.174.256:8100" };
  assert.equal(newClusterFieldProblems(invalid, "en").tunnelEndpoint, newClusterProblem(invalid, "en"));
});

test("summary reports generated cluster defaults and number of remaining fields", () => {
  const summary = newClusterSummary({ ...form, site: "", address: "", password: "" }, "zh");
  assert.equal(summary.cluster, "rlark-<site>-gpu");
  assert.equal(summary.remaining, 3);
});

test("provision progress counts done and skipped stages and exposes failure detail", () => {
  const result = provisionProgress({ status: { phase: "Failed", stage: "control-plane", stages: [
    { name: "precheck", state: "Done" }, { name: "bundle", state: "Skipped" }, { name: "control-plane", state: "Failed", message: "init failed" },
  ] } });
  assert.deepEqual(result, { completed: 2, total: 8, percent: 25, stageNumber: 4, failedStage: "init failed" });
});
