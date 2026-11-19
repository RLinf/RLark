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

function storageJob(mounts) {
  return generateJobCRD({
    name: "jo-0123456789abcdef",
    displayName: "Readable Job",
    type: "Custom",
    headerRole: "actor",
    roles: ["actor"],
    roleResources: { actor: storageResource(mounts) },
    runScript: "echo ready",
    domain: "",
  });
}

test("a storage mount names the storage instead of a StorageClass", () => {
  const crd = storageJob([
    {
      type: "storage",
      objectStorage: "ks3",
      mountPath: "/data",
      hostPath: "",
      profile: "",
      readOnly: false,
    },
  ]);
  const podSpec = crd.spec.tasks[0].workload.template.spec;
  const volume = podSpec.volumes[0];
  assert.equal(volume.name, "data");
  assert.equal(volume.ephemeral, undefined);
  assert.deepEqual(volume.csi, {
    driver: "storage.rlark.io",
    volumeAttributes: { storage: "ks3" },
  });
  assert.equal(podSpec.containers[0].volumeMounts[0].name, volume.name);
});

test("a storage mount carries its profile and read-only switch", () => {
  const crd = storageJob([
    {
      type: "storage",
      objectStorage: "ks3",
      mountPath: "/data",
      hostPath: "",
      profile: "archive",
      readOnly: true,
    },
  ]);
  assert.deepEqual(crd.spec.tasks[0].workload.template.spec.volumes[0].csi, {
    driver: "storage.rlark.io",
    readOnly: true,
    volumeAttributes: { storage: "ks3", profile: "archive" },
  });
});

test("reads a storage mount back from a job CRD", () => {
  const mount = {
    type: "storage",
    objectStorage: "kpfs",
    mountPath: "/data",
    hostPath: "",
    profile: "read",
    readOnly: true,
  };
  assert.deepEqual(crdToJob(storageJob([mount])).mounts[0], mount);
});

test("keeps a volume with a fixed StorageClass verbatim", () => {
  const crd = storageJob([]);
  const task = crd.spec.tasks[0];
  const legacy = {
    name: "data",
    ephemeral: {
      volumeClaimTemplate: {
        spec: {
          accessModes: ["ReadWriteOnce"],
          storageClassName: "sc-0123456789abcdef",
          resources: { requests: { storage: "10Gi" } },
        },
      },
    },
  };
  task.workload.template.spec.volumes = [legacy];
  task.workload.template.spec.containers[0].volumeMounts = [
    { name: "data", mountPath: "/data" },
  ];
  const mount = crdToJob(crd).mounts[0];
  assert.equal(mount.type, "opaque");
  assert.deepEqual(mount.rawVolume, legacy);
});
