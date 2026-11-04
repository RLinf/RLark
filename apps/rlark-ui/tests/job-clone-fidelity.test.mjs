import assert from "node:assert/strict";
import test from "node:test";
import { generateJobCRD } from "../dist/test/utils/job.js";
import { crdToJob, describeVolumeSource } from "../dist/test/utils/crd.js";

const SHM_VOLUME = {
  name: "shm",
  emptyDir: { medium: "Memory", sizeLimit: "32Gi" },
};

function actorResource(mounts) {
  return {
    role: "actor",
    cluster: "cluster-a",
    nodeSelector: "",
    replicas: 1,
    cpu: "8",
    memory: "64Gi",
    gpu: "4",
    devices: [],
    image: "busybox:latest",
    prepareScript: "",
    envs: [],
    mounts,
  };
}

function buildJobCRD(mounts) {
  return generateJobCRD({
    name: "jo-0123456789abcdef",
    type: "Custom",
    headerRole: "actor",
    roles: ["actor"],
    roleResources: { actor: actorResource(mounts) },
    runScript: "echo ready",
    domain: "",
  });
}

// 模拟通过 Gateway 接口创建的任务：Pod 模板里带一个表单无法表达的 emptyDir 卷。
function crdWithSharedMemory() {
  const crd = buildJobCRD([
    {
      type: "host",
      objectStorage: "",
      mountPath: "/mnt/data",
      hostPath: "/data",
      pvcSizeGb: 10,
    },
  ]);
  const podSpec = crd.spec.tasks[0].workload.template.spec;
  podSpec.volumes.push(SHM_VOLUME);
  podSpec.containers[0].volumeMounts.push({
    name: SHM_VOLUME.name,
    mountPath: "/dev/shm",
  });
  return crd;
}

function cloneRoleResource(resource) {
  return {
    ...resource,
    envs: resource.env,
    prepareScript: resource.prepareScript ?? "",
  };
}

test("maps volumes the form cannot edit to opaque mounts", () => {
  const job = crdToJob(crdWithSharedMemory());

  assert.deepEqual(job.resources[0].mounts[0], {
    type: "host",
    objectStorage: "",
    mountPath: "/mnt/data",
    hostPath: "/data",
    pvcSizeGb: 10,
  });
  const opaque = job.resources[0].mounts[1];
  assert.equal(opaque.type, "opaque");
  assert.equal(opaque.mountPath, "/dev/shm");
  assert.deepEqual(opaque.rawVolume, SHM_VOLUME);
});

test("keeps cpu and memory requests when reading a job CRD", () => {
  const job = crdToJob(crdWithSharedMemory());

  assert.equal(job.resources[0].cpu, "8");
  assert.equal(job.resources[0].memory, "64Gi");
});

test("writes opaque volumes back verbatim when a job is cloned", () => {
  const source = crdToJob(crdWithSharedMemory());
  const resource = source.resources[0];

  const crd = generateJobCRD({
    name: "jo-fedcba9876543210",
    type: "Custom",
    headerRole: resource.role,
    roles: [resource.role],
    roleResources: { [resource.role]: cloneRoleResource(resource) },
    runScript: "echo ready",
    domain: "",
  });

  const podSpec = crd.spec.tasks[0].workload.template.spec;
  const shm = podSpec.volumes.find((volume) => volume.emptyDir);
  assert.deepEqual(shm.emptyDir, SHM_VOLUME.emptyDir);
  assert.ok(
    podSpec.containers[0].volumeMounts.some(
      (mount) => mount.name === shm.name && mount.mountPath === "/dev/shm",
    ),
    "the opaque volume must stay mounted at its original path",
  );
});

test("skips opaque mounts that lost their original volume definition", () => {
  const crd = buildJobCRD([
    {
      type: "opaque",
      objectStorage: "",
      mountPath: "/dev/shm",
      hostPath: "",
      pvcSizeGb: 10,
    },
  ]);

  const podSpec = crd.spec.tasks[0].workload.template.spec;
  assert.deepEqual(podSpec.volumes, []);
  assert.equal(podSpec.containers[0].volumeMounts, undefined);
});

test("summarises opaque volume sources for read-only display", () => {
  assert.equal(describeVolumeSource(SHM_VOLUME), "emptyDir (Memory, 32Gi)");
  assert.equal(
    describeVolumeSource({ name: "cfg", configMap: { name: "train-config" } }),
    "configMap (train-config)",
  );
  assert.equal(describeVolumeSource(undefined), "");
});
