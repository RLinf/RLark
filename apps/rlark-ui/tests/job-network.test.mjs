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

  const job = generateJobCRD({
    ...base,
    roleResources: sameCluster,
    domain: "network-a",
  });
  assert.equal(job.spec.domain, "network-a");
  assert.equal(job.spec.tasks[0].runtime.kind, "Kubernetes");
  assert.equal(job.spec.tasks[0].workload.kind, "rlinf.io/ray");
  assert.equal(job.spec.tasks[0].workload.version, "v1");
  assert.equal(
    job.spec.tasks[0].workload.carrier.type,
    "rlinf.io/kubernetes-statefulset",
  );
  assert.equal(job.spec.tasks[0].workload.carrier.version, "v1");
  assert.equal(
    job.spec.tasks[0].workload.template.spec.containers[0].name,
    "main",
  );
  assert.equal("agentType" in job.spec.tasks[0], false);
  assert.equal("kubernetes" in job.spec.tasks[0], false);
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
