import assert from "node:assert/strict";
import test from "node:test";
import { effectiveJobPhase } from "../dist/test/utils/jobPhase.js";

function job(phase, stopped, taskPhases) {
  return {
    phase,
    stopped,
    taskStatuses: taskPhases.map((taskPhase) => ({ phase: taskPhase })),
  };
}

test("derives Running and Stopped only when every task matches", () => {
  assert.equal(
    effectiveJobPhase(job("Pending", false, ["Running", "Running"])),
    "Running",
  );
  assert.equal(
    effectiveJobPhase(job("Pending", true, ["Stopped", "Stopped"])),
    "Stopped",
  );
  assert.equal(
    effectiveJobPhase(job("Running", false, ["Running", "Pending"])),
    "Pending",
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

test("derives terminal states while the aggregate is Pending", () => {
  assert.equal(
    effectiveJobPhase(job("Pending", false, ["Pending", "Failed"])),
    "Failed",
  );
  assert.equal(
    effectiveJobPhase(job("Pending", false, ["Succeeded", "Succeeded"])),
    "Succeeded",
  );
});
