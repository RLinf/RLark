import assert from "node:assert/strict";
import test from "node:test";
import { effectiveJobPhase, isActiveJob } from "../dist/test/utils/jobPhase.js";

function job(phase, stopped, taskPhases) {
  return {
    phase,
    stopped,
    taskStatuses: taskPhases.map((taskPhase) => ({ phase: taskPhase })),
  };
}

test("uses the aggregate Job phase regardless of task phases", () => {
  assert.equal(
    effectiveJobPhase(job("Pending", false, ["Running", "Running"])),
    "Pending",
  );
  assert.equal(
    effectiveJobPhase(job("Stopped", true, ["Stopped", "Stopped"])),
    "Stopped",
  );
  assert.equal(
    effectiveJobPhase(job("Running", false, ["Running", "Pending"])),
    "Running",
  );
  assert.equal(
    effectiveJobPhase(job("Running", true, ["Stopped", "Running"])),
    "Stopping",
  );
});

test("distinguishes stopping from normal Pending states", () => {
  assert.equal(effectiveJobPhase(job("Pending", true, [])), "Stopping");
  assert.equal(
    effectiveJobPhase(job("Pending", true, ["Running", "Pending"])),
    "Stopping",
  );
  assert.equal(
    effectiveJobPhase(job("Pending", false, ["Running", "Pending"])),
    "Pending",
  );
  assert.equal(effectiveJobPhase(job("Pending", false, [])), "Pending");
});

test("deleting takes precedence over stopping", () => {
  assert.equal(
    effectiveJobPhase(job("Deleting", true, ["Stopped"])),
    "Deleting",
  );
});

test("does not infer terminal states from task phases", () => {
  assert.equal(
    effectiveJobPhase(job("Pending", false, ["Pending", "Failed"])),
    "Pending",
  );
  assert.equal(
    effectiveJobPhase(job("Pending", false, ["Succeeded", "Succeeded"])),
    "Pending",
  );
});

test("isActiveJob keeps running and stopping jobs, drops terminal ones", () => {
  assert.equal(isActiveJob(job("Running", false, ["Running"])), true);
  assert.equal(isActiveJob(job("Pending", false, [])), true);
  assert.equal(isActiveJob(job("Stopped", true, ["Stopped"])), false);
  assert.equal(isActiveJob(job("Succeeded", false, ["Succeeded"])), false);
  assert.equal(isActiveJob(job("Failed", false, ["Failed"])), false);
  assert.equal(isActiveJob(job("Deleting", true, ["Stopped"])), false);
});

test("isActiveJob treats the Stopping transition as still active", () => {
  const stopping = job("Running", true, ["Stopped", "Running"]);
  assert.equal(effectiveJobPhase(stopping), "Stopping");
  assert.equal(isActiveJob(stopping), true);
});
