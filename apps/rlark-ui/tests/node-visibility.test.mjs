import test from "node:test";
import assert from "node:assert/strict";
import {
  getNodeCategories,
  getNodeCategory,
  isBusinessWorkerNode,
} from "../dist/test/utils/nodeVisibility.js";

function node(name, labels = {}) {
  return { metadata: { name, labels } };
}

test("shows categorized workload workers", () => {
  for (const category of ["cloud", "edge", "robot"]) {
    assert.equal(
      isBusinessWorkerNode(
        node(`${category}-worker`, { "rlark.io/node-category": category }),
      ),
      true,
    );
  }
});

test("supports multiple Kubernetes-safe category labels", () => {
  const hybrid = node("hybrid-worker", {
    "rlark.io/node-category-cloud": "true",
    "rlark.io/node-category-robot": "true",
  });
  assert.deepEqual(getNodeCategories(hybrid), ["cloud", "robot"]);
  assert.equal(isBusinessWorkerNode(hybrid), true);
});

test("hides uncategorized management nodes", () => {
  assert.equal(isBusinessWorkerNode(node("management-node")), false);
  assert.equal(getNodeCategory(node("management-node")), "unknown");
});

test("shows uncategorized nodes that advertise workload devices", () => {
  const gpuNode = node("gpu-worker");
  gpuNode.status = { allocatable: { "nvidia.com/gpu": "8" } };
  assert.equal(isBusinessWorkerNode(gpuNode), true);
  assert.equal(getNodeCategory(gpuNode), "cloud");

  const robotNode = node("robot-worker");
  robotNode.status = { capacity: { "rlinf.io/device-franka": "1" } };
  assert.equal(isBusinessWorkerNode(robotNode), true);
});

test("hides Kubernetes control-plane nodes even when categorized", () => {
  const category = { "rlark.io/node-category": "cloud" };
  assert.equal(
    isBusinessWorkerNode(
      node("master", {
        ...category,
        "node-role.kubernetes.io/master": "",
      }),
    ),
    false,
  );
  assert.equal(
    isBusinessWorkerNode(
      node("control-plane", {
        ...category,
        "node-role.kubernetes.io/control-plane": "",
      }),
    ),
    false,
  );
  assert.equal(
    isBusinessWorkerNode(
      node("legacy-control-plane", {
        ...category,
        "kubernetes.io/role": "control-plane",
      }),
    ),
    false,
  );
});
