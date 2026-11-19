import test from "node:test";
import assert from "node:assert/strict";
import { isBusinessWorkerNode } from "../dist/test/utils/nodeVisibility.js";

function node(name, labels = {}) {
  return { metadata: { name, labels } };
}

test("shows explicitly managed control-plane nodes with workload resources", () => {
  const controlPlane = node("gpu-control-plane", {
    "node-role.kubernetes.io/control-plane": "",
    "rlark.io/enabled": "true",
  });
  controlPlane.status = { allocatable: { "nvidia.com/gpu": "8" } };
  assert.equal(isBusinessWorkerNode(controlPlane), true);
  assert.equal(isBusinessWorkerNode(node("management", {
    "node-role.kubernetes.io/control-plane": "",
    "rlark.io/enabled": "true",
  })), false);
});

test("a cluster's control plane is the node its Kubernetes role says, with its InternalIP", async () => {
  const { controlPlanesOf, isControlPlaneLabels } = await import("../dist/test/utils/nodeVisibility.js");
  const node = (name, namespace, labels, addresses) => ({ metadata: { name, namespace, labels }, spec: {}, status: { addresses } });
  const nodes = [
    node("cailiang", "rlark-local-dev-gpu", { "node-role.kubernetes.io/control-plane": "" }, [{ type: "Hostname", address: "cailiang" }, { type: "InternalIP", address: "10.11.14.128" }]),
    node("wendychen", "rlark-local-dev-gpu", {}, [{ type: "InternalIP", address: "10.11.14.135" }]),
    node("alexfu", "rlark-robot-s1u-01", { "node-role.kubernetes.io/control-plane": "" }, [{ type: "InternalIP", address: "10.254.240.5" }]),
    node("10-0-2-196", "managed", {}, []),
  ];
  assert.deepEqual(controlPlanesOf(nodes, "rlark-local-dev-gpu"), [{ name: "cailiang", displayName: undefined, address: "10.11.14.128" }]);
  assert.deepEqual(controlPlanesOf(nodes, "managed"), []);
  assert.equal(isControlPlaneLabels({ "kubernetes.io/role": "master" }), true);
  assert.equal(isControlPlaneLabels(undefined), false);
});
