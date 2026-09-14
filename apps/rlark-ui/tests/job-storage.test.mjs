import assert from "node:assert/strict";
import test from "node:test";
import {
  generateJobCRD,
  generateJobResourceName,
} from "../dist/test/utils/job.js";

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
  const claimName =
    workload.template.spec.volumes[0].persistentVolumeClaim.claimName;
  assert.equal(workload.pvcSizeGbMap[claimName], 200);
});

test("maps a selected storage class to the generated PVC claim name", () => {
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
  const claimName =
    workload.template.spec.volumes[0].persistentVolumeClaim.claimName;

  assert.equal(claimName, "pvc-jo-0123456789abcdef-actor-data");
  assert.equal(workload.pvcStorageMap[claimName], "fast-storage");
  assert.equal(workload.pvcSizeGbMap[claimName], 20);
});
