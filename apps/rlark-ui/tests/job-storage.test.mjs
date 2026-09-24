import assert from "node:assert/strict";
import test from "node:test";
import {
  generateJobCRD,
  generateJobResourceName,
} from "../dist/test/utils/job.js";
import { crdToJob } from "../dist/test/utils/crd.js";

function storageResource(mounts) {
  return {
    role: "actor",
    cluster: "cluster-a",
    nodeSelector: "",
    replicas: 1,
    cpu: "1",
    memory: "1Gi",
    gpu: "0",
    devices: [],
    image: "busybox:latest",
    prepareScript: "",
    envs: [],
    mounts,
  };
}

test("generates browser-compatible job resource IDs", () => {
  assert.match(generateJobResourceName(), /^jo-[0-9a-f]{16}$/);
});

test("caps generated PVC storage size at 200 Gi", () => {
  const crd = generateJobCRD({
    name: "jo-0123456789abcdef",
    displayName: "Readable Job",
    type: "Custom",
    headerRole: "actor",
    roles: ["actor"],
    roleResources: {
      actor: storageResource([
        {
          type: "storage",
          objectStorage: "fast-storage",
          mountPath: "/data",
          hostPath: "",
          pvcSizeGb: 201,
        },
      ]),
    },
    runScript: "echo ready",
    domain: "",
  });

  const workload = crd.spec.tasks[0].kubernetes.workload;
  const claimSpec =
    workload.template.spec.volumes[0].ephemeral.volumeClaimTemplate.spec;
  assert.equal(claimSpec.resources.requests.storage, "200Gi");
});

test("generates an ephemeral volume claim for selected storage", () => {
  const resourceName = "jo-0123456789abcdef";
  const mounts = [
    {
      type: "storage",
      objectStorage: "fast-storage",
      mountPath: "/data",
      hostPath: "",
      pvcSizeGb: 20,
    },
  ];
  const crd = generateJobCRD({
    name: resourceName,
    displayName: "Readable Job",
    type: "Custom",
    headerRole: "actor",
    roles: ["actor"],
    roleResources: { actor: storageResource(mounts) },
    runScript: "echo ready",
    domain: "",
  });

  const workload = crd.spec.tasks[0].kubernetes.workload;
  const volume = workload.template.spec.volumes[0];
  const claimSpec = volume.ephemeral.volumeClaimTemplate.spec;

  assert.equal(volume.name, "data");
  assert.equal(volume.persistentVolumeClaim, undefined);
  assert.deepEqual(claimSpec.accessModes, ["ReadWriteOnce"]);
  assert.equal(claimSpec.storageClassName, "fast-storage");
  assert.equal(claimSpec.resources.requests.storage, "20Gi");
  assert.equal(workload.pvcStorageMap, undefined);
  assert.equal(workload.pvcSizeGbMap, undefined);
  assert.equal(
    workload.template.spec.containers[0].volumeMounts[0].name,
    volume.name,
  );
});

test("reads ephemeral volume storage settings from a job CRD", () => {
  const crd = generateJobCRD({
    name: "jo-0123456789abcdef",
    type: "Custom",
    headerRole: "actor",
    roles: ["actor"],
    roleResources: {
      actor: storageResource([
        {
          type: "storage",
          objectStorage: "fast-storage",
          mountPath: "/data",
          hostPath: "",
          pvcSizeGb: 20,
        },
      ]),
    },
    runScript: "echo ready",
    domain: "",
  });

  const job = crdToJob(crd);
  assert.deepEqual(job.mounts[0], {
    type: "storage",
    objectStorage: "fast-storage",
    mountPath: "/data",
    hostPath: "",
    pvcSizeGb: 20,
  });
});
