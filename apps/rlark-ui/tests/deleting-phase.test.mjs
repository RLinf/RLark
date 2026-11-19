import assert from "node:assert/strict";
import test from "node:test";
import { crdToJob, crdToWorkflow } from "../dist/test/utils/crd.js";

test("maps terminating jobs to Deleting", () => {
  const job = crdToJob({
    apiVersion: "rlinf.io/v1alpha1",
    kind: "Job",
    metadata: {
      name: "job",
      deletionTimestamp: "2026-09-18T00:00:00Z",
    },
    spec: { tasks: [] },
    status: { phase: "Running" },
  });

  assert.equal(job.phase, "Deleting");
});

test("maps terminating workflows to Deleting", () => {
  const workflow = crdToWorkflow({
    apiVersion: "rlinf.io/v1alpha1",
    kind: "Workflow",
    metadata: {
      name: "workflow",
      deletionTimestamp: "2026-09-18T00:00:00Z",
    },
    spec: { jobTemplates: [] },
    status: { phase: "Running" },
  });

  assert.equal(workflow.phase, "Deleting");
});
