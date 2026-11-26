import assert from "node:assert/strict";
import test from "node:test";

import {
  buildNodeWorkloads,
  nodeWorkloadKey,
} from "../dist/test/utils/nodeWorkloads.js";

function task(name, job) {
  return { metadata: { name, labels: job ? { "rlinf.io/job": job } : {} } };
}

function pod({ namespace = "agent-a", node, taskName, phase = "Running" }) {
  return {
    metadata: { namespace, name: `${taskName ?? "pod"}-${node}-${phase}` },
    spec: { taskName },
    status: { phase, node },
  };
}

test("keys workloads by namespace and name so equal node names do not collide", () => {
  const workloads = buildNodeWorkloads(
    [task("rollout-a", "job-a"), task("rollout-b", "job-b")],
    [
      pod({ namespace: "agent-a", node: "node-1", taskName: "rollout-a" }),
      pod({ namespace: "agent-b", node: "node-1", taskName: "rollout-b" }),
    ],
  );

  assert.deepEqual(workloads["agent-a/node-1"], {
    jobs: ["job-a"],
    workers: 1,
  });
  assert.deepEqual(workloads["agent-b/node-1"], {
    jobs: ["job-b"],
    workers: 1,
  });
});

test("counts non-terminal pods and deduplicates their jobs", () => {
  const workloads = buildNodeWorkloads(
    [task("rollout-a", "job-b"), task("rollout-c", "job-a")],
    [
      pod({ node: "node-1", taskName: "rollout-a" }),
      pod({ node: "node-1", taskName: "rollout-a" }),
      pod({ node: "node-1", taskName: "rollout-c" }),
      pod({ node: "node-1", taskName: "rollout-a", phase: "Pending" }),
      pod({ node: "node-2", taskName: "rollout-a", phase: "Failed" }),
      pod({ node: "node-2", taskName: "rollout-a", phase: "Succeeded" }),
    ],
  );

  assert.deepEqual(workloads["agent-a/node-1"], {
    jobs: ["job-a", "job-b"],
    workers: 4,
  });
  assert.equal(workloads["agent-a/node-2"], undefined);
});

test("still counts workers for pods whose Task mirror is missing", () => {
  const workloads = buildNodeWorkloads(
    [],
    [pod({ node: "node-1", taskName: "unknown" })],
  );

  assert.deepEqual(workloads["agent-a/node-1"], { jobs: [], workers: 1 });
});

test("nodeWorkloadKey matches the key produced from pods", () => {
  assert.equal(
    nodeWorkloadKey({ metadata: { name: "node-1", namespace: "agent-a" } }),
    "agent-a/node-1",
  );
});
