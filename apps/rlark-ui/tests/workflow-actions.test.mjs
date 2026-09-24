import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { crdToWorkflow } from "../dist/test/utils/crd.js";

const workflowSource = await readFile(
  new URL("../src/pages/Workflows.tsx", import.meta.url),
  "utf8",
);
const backendSource = await readFile(
  new URL("../src/backend.ts", import.meta.url),
  "utf8",
);
const mockSource = await readFile(
  new URL("../src/mockBackend.ts", import.meta.url),
  "utf8",
);
const dataSource = await readFile(new URL("../src/data.ts", import.meta.url), "utf8");

test("stopped workflow requests display Stopping until observed", () => {
  const workflow = crdToWorkflow({
    apiVersion: "rlinf.io/v1alpha1",
    kind: "Workflow",
    metadata: { name: "workflow" },
    spec: { stopped: true, jobTemplates: [] },
    status: { phase: "Running" },
  });
  assert.equal(workflow.phase, "Stopping");
  assert.equal(workflow.stopped, true);
});

test("stopping intent does not hide terminal workflow phases", () => {
  for (const phase of ["Succeeded", "Failed"]) {
    const workflow = crdToWorkflow({
      apiVersion: "rlinf.io/v1alpha1",
      kind: "Workflow",
      metadata: { name: "workflow" },
      spec: { stopped: true, jobTemplates: [] },
      status: { phase },
    });
    assert.equal(workflow.phase, phase);
  }
});

test("workflow list and detail expose stop and resume PATCH actions", () => {
  assert.match(workflowSource, /workflowsApi\.setStopped\(name, stopped\)/);
  assert.match(
    backendSource,
    /setStopped\(name: string, stopped: boolean\)[\s\S]*?body: \{ spec: \{ stopped \} \}/,
  );
  assert.match(workflowSource, /onSetStopped/);
  assert.match(workflowSource, /<Play size=\{15\}/);
  assert.match(workflowSource, /<Square size=\{14\}/);
  assert.match(mockSource, /patch\.spec\?\.stopped/);
});

test("shared phases include stopping and unknown", () => {
  assert.match(dataSource, /\| "Stopping"/);
  assert.match(dataSource, /\| "Unknown"/);
});
