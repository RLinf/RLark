import assert from "node:assert/strict";
import test from "node:test";
import {
  availableResource,
  reclaimableResourcesForTasks,
} from "../dist/test/utils/resourceAvailability.js";

const task = (namespace, nodes, gpu) => ({
  metadata: { name: "job-actor", namespace },
  spec: {
    kubernetes: {
      workload: {
        kind: "StatefulSet",
        replicas: nodes.length,
        template: {
          spec: {
            containers: [
              {
                name: "main",
                image: "example/image",
                env: [],
                resources: { requests: { "nvidia.com/gpu": gpu } },
              },
            ],
          },
        },
      },
    },
  },
  status: { observedNodes: nodes },
});

test("keeps normal availability unchanged without reclaimable resources", () => {
  assert.equal(availableResource("8", "8"), 0);
});

test("restores the current task GPU allocation when restarting", () => {
  const reclaimable = reclaimableResourcesForTasks([
    task("cluster-a", ["node-a"], "2"),
  ]);
  assert.equal(reclaimable["cluster-a/node-a"]["nvidia.com/gpu"], 2);
  assert.equal(
    availableResource(
      "8",
      "2",
      reclaimable["cluster-a/node-a"]["nvidia.com/gpu"],
    ),
    8,
  );
});

test("does not restore resources occupied by other tasks", () => {
  const reclaimable = reclaimableResourcesForTasks([
    task("cluster-a", ["node-a", "node-b"], "2"),
  ]);
  assert.equal(
    availableResource(
      "8",
      "6",
      reclaimable["cluster-a/node-a"]["nvidia.com/gpu"],
    ),
    4,
  );
  assert.equal(
    availableResource(
      "8",
      "6",
      reclaimable["cluster-a/node-b"]["nvidia.com/gpu"],
    ),
    4,
  );
});

test("caps restored availability at allocatable capacity", () => {
  assert.equal(availableResource("8", "1", 4), 8);
});

test("keeps nodes with the same name in different clusters separate", () => {
  const reclaimable = reclaimableResourcesForTasks([
    task("cluster-a", ["worker"], "1"),
    task("cluster-b", ["worker"], "2"),
  ]);
  assert.equal(reclaimable["cluster-a/worker"]["nvidia.com/gpu"], 1);
  assert.equal(reclaimable["cluster-b/worker"]["nvidia.com/gpu"], 2);
});
