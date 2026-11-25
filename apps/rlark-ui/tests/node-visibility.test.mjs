import test from "node:test";
import assert from "node:assert/strict";
import {
  getNodeCategories,
  getNodeCategory,
  isBusinessWorkerNode,
} from "../dist/test/utils/nodeVisibility.js";

function node(name, labels = {}) {
  return { metadata: { name }, status: { observedNodeLabels: labels } };
}

test("shows RLark-managed nodes", () => {
  for (const managed of ["cloud", "edge", "robot"]) {
    assert.equal(
      isBusinessWorkerNode(
        node(`${managed}-worker`, { "rlark.io/enabled": "true" }),
      ),
      true,
    );
  }
});

test("shows a managed Kubernetes control-plane node", () => {
  assert.equal(
    isBusinessWorkerNode(
      node("test-master01", {
        "rlark.io/enabled": "true",
        "rlark.io/node-category-cloud": "true",
        "node-role.kubernetes.io/control-plane": "true",
        "node-role.kubernetes.io/master": "true",
      }),
    ),
    true,
  );
});

test("hides nodes without the RLark managed marker", () => {
  assert.equal(isBusinessWorkerNode(node("unmanaged")), false);
  assert.equal(
    isBusinessWorkerNode(
      node("unmanaged-categorized", { "rlark.io/node-category": "cloud" }),
    ),
    false,
  );
  assert.equal(
    isBusinessWorkerNode(
      node("unmanaged-legacy-control-plane", {
        "kubernetes.io/role": "control-plane",
      }),
    ),
    false,
  );
});

test("supports multiple Kubernetes-safe category labels", () => {
  const hybrid = node("hybrid-worker", {
    "rlark.io/node-category-cloud": "true",
    "rlark.io/node-category-robot": "true",
  });
  assert.deepEqual(getNodeCategories(hybrid), ["cloud", "robot"]);
});

test("infers a cloud category from advertised GPU resources", () => {
  const gpuNode = node("gpu-worker");
  gpuNode.status = { allocatable: { "nvidia.com/gpu": "8" } };
  assert.equal(getNodeCategory(gpuNode), "cloud");
});

test("reports an unknown category for uncategorized nodes", () => {
  assert.equal(getNodeCategory(node("plain-node")), "unknown");
  assert.deepEqual(getNodeCategories(node("plain-node")), ["unknown"]);
});
