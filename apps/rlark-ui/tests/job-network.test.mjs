import assert from "node:assert/strict";
import test from "node:test";
import {
  automaticNetworkDomain,
  generateJobCRD,
} from "../dist/test/utils/job.js";

function resource(cluster) {
  return {
    role: "worker",
    cluster,
    nodeSelector: "",
    replicas: 1,
    cpu: "1",
    memory: "1Gi",
    gpu: "0",
    devices: [],
    image: "busybox:latest",
    prepareScript: "",
    envs: [],
    mounts: [],
  };
}

const roles = ["actor", "worker"];

const base = {
  name: "jo-0123456789abcdef",
  type: "Custom",
  headerRole: "actor",
  roles,
  runScript: "echo ready",
};

test("selects the configured network domain regardless of task placement", () => {
  assert.equal(automaticNetworkDomain([]), "");
  assert.equal(
    automaticNetworkDomain([{ name: "network-b" }, { name: "network-a" }]),
    "network-a",
  );
});

test("enables the configured network domain for same-cluster jobs", () => {
  const sameCluster = {
    actor: resource("cluster-a"),
    worker: resource("cluster-a"),
  };

  assert.equal(
    generateJobCRD({
      ...base,
      roleResources: sameCluster,
      domain: "network-a",
    }).spec.domain,
    "network-a",
  );
});

test("omits the network domain when none is configured", () => {
  const differentClusters = {
    actor: resource("cluster-a"),
    worker: resource("cluster-b"),
  };

  assert.equal(
    generateJobCRD({
      ...base,
      roleResources: differentClusters,
      domain: "",
    }).spec.domain,
    undefined,
  );
});
