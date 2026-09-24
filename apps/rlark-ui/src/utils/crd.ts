import type { Job, JobTag, JobType, Phase } from "../data";
import type { CRDJob, CRDJobTask, CRDWorkflow } from "../types";

// 生成稳定的 tag id：用 key:value 组合哈希，保证同一 CRD 多次转换得到相同 id。
function tagId(key: string, value: string): string {
  const s = `${key}|${value}`;
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return `tag-${key.replace(/[^a-zA-Z0-9]/g, "_")}-${(h >>> 0)
    .toString(16)
    .padStart(8, "0")}`;
}

function storageGi(quantity?: string): number {
  const match = quantity?.match(/^(\d+)(?:Gi)?$/);
  return match ? Number(match[1]) : 10;
}

export function crdToJob(crd: CRDJob): Job {
  const tasks = crd.spec.tasks ?? [];
  const container =
    tasks[0]?.kubernetes?.workload?.template.spec.containers?.[0];
  const phase = (
    crd.metadata.deletionTimestamp
      ? "Deleting"
      : (crd.status?.phase ?? "Pending")
  ) as Phase;
  const allTaskStatuses = crd.status?.tasks ?? [];
  const runningTasks = allTaskStatuses.filter(
    (t) => t.phase === "Running",
  ).length;
  const headerTask = tasks.find((t) => t.head) ?? tasks[0];
  const taskRoleName = (task: CRDJobTask) =>
    task.kubernetes?.workload?.template.spec.containers?.[0]?.env?.find(
      (env) => env.name === "RLARK_TASK_ROLE",
    )?.value ?? task.name;
  const roles = tasks.map(taskRoleName);
  const roleCount = new Set(tasks.map((task) => task.role).filter(Boolean))
    .size;
  const displayName =
    crd.metadata.annotations?.["rlark.io/display-name"] ??
    crd.metadata.labels?.["rlark.io/display-name"] ??
    crd.metadata.name;
  const resources = tasks.map((t) => {
    const c = t.kubernetes?.workload?.template.spec.containers?.[0];
    const res = c?.resources?.requests ?? {};
    const gpu = res["nvidia.com/gpu"] ?? "0";
    const devices = Object.entries(res)
      .filter(([k]) => k.startsWith("rlinf.io/"))
      .map(([name, quantity]) => ({ name, quantity: String(quantity) }));
    const ns = t.nodeSelector ?? {};
    const nsStr = Object.entries(ns)
      .map(([k, v]) => `${k}=${v}`)
      .join(",");
    const taskEnv =
      c?.env
        ?.filter((e) => e.name !== "RLARK_TASK_ROLE")
        .map((e) => ({
          key: e.name,
          value: e.value,
        })) ?? [];
    const taskMounts = (c?.volumeMounts ?? []).map((vm) => {
      const vol = t.kubernetes?.workload?.template.spec.volumes?.find(
        (v) => v.name === vm.name,
      );
      if (vol?.ephemeral) {
        const spec = vol.ephemeral.volumeClaimTemplate.spec;
        return {
          type: "storage" as const,
          objectStorage: spec.storageClassName ?? "",
          mountPath: vm.mountPath,
          hostPath: "",
          pvcSizeGb: storageGi(spec.resources?.requests?.storage),
        };
      }
      if (vol?.persistentVolumeClaim) {
        const claimName = vol.persistentVolumeClaim.claimName;
        const storageClass =
          t.kubernetes?.workload?.pvcStorageMap?.[claimName] ?? "";
        return {
          type: "storage" as const,
          objectStorage: storageClass,
          mountPath: vm.mountPath,
          hostPath: "",
          pvcSizeGb: t.kubernetes?.workload?.pvcSizeGbMap?.[claimName] ?? 10,
        };
      }
      const hostPath = vol?.hostPath?.path ?? "";
      return {
        type: "host" as const,
        objectStorage: "",
        mountPath: vm.mountPath,
        hostPath,
        pvcSizeGb: 10,
      };
    });
    return {
      role: taskRoleName(t),
      cluster: "",
      nodeSelector: nsStr,
      replicas: t.kubernetes?.workload?.replicas ?? 1,
      cpu: res.cpu ?? "",
      memory: res.memory ?? "",
      gpu,
      devices,
      image: c?.image ?? "",
      prepareScript: t.prepareScript ?? "",
      env: taskEnv,
      mounts: taskMounts,
    };
  });
  const env =
    container?.env
      ?.filter((e) => e.name !== "RLARK_TASK_ROLE")
      .map((e) => ({
        key: e.name,
        value: e.value,
      })) ?? [];
  const mounts = (container?.volumeMounts ?? []).map((vm) => {
    const vol = tasks[0]?.kubernetes?.workload?.template.spec.volumes?.find(
      (v) => v.name === vm.name,
    );
    if (vol?.ephemeral) {
      const spec = vol.ephemeral.volumeClaimTemplate.spec;
      return {
        type: "storage" as const,
        objectStorage: spec.storageClassName ?? "",
        mountPath: vm.mountPath,
        hostPath: "",
        pvcSizeGb: storageGi(spec.resources?.requests?.storage),
      };
    }
    if (vol?.persistentVolumeClaim) {
      const claimName = vol.persistentVolumeClaim.claimName;
      const storageClass =
        tasks[0]?.kubernetes?.workload?.pvcStorageMap?.[claimName] ?? "";
      return {
        type: "storage" as const,
        objectStorage: storageClass,
        mountPath: vm.mountPath,
        hostPath: "",
        pvcSizeGb:
          tasks[0]?.kubernetes?.workload?.pvcSizeGbMap?.[claimName] ?? 10,
      };
    }
    const hostPath = vol?.hostPath?.path ?? "";
    return {
      type: "host" as const,
      objectStorage: "",
      mountPath: vm.mountPath,
      hostPath,
      pvcSizeGb: 10,
    };
  });
  return {
    id: crd.metadata.name,
    name: crd.metadata.name,
    displayName,
    type: mapRoleToJobType(tasks),
    phase,
    owner: "",
    cluster: tasks
      .map((t) => (t.nodeSelector ? Object.values(t.nodeSelector)[0] : ""))
      .filter(Boolean)
      .join(", "),
    target: roles.join(" / "),
    workers: tasks.length,
    runningWorkers: runningTasks,
    startedAt: crd.metadata.creationTimestamp ?? "—",
    submittedAt: crd.metadata.creationTimestamp ?? "—",
    stoppedAt: crd.status?.endTime ?? "—",
    roleCount,
    duration: "—",
    progress:
      phase === "Succeeded"
        ? 100
        : Math.round((runningTasks / Math.max(tasks.length, 1)) * 100),
    defaultRoles: roles,
    image: container?.image ?? "",
    command: headerTask?.runScript ?? "",
    tensorBoardDir: headerTask?.tensorBoardDir ?? "",
    env,
    mounts,
    headerRole: headerTask ? taskRoleName(headerTask) : "",
    headerWorker: headerTask ? taskRoleName(headerTask) : "",
    sshAddress: "",
    stopped: crd.spec.stopped ?? false,
    domain: crd.spec.domain ?? "",
    sshPublicKey: crd.spec.sshPublicKey ?? "",
    tags:
      crd.spec.tags && crd.spec.tags.length > 0
        ? crd.spec.tags.flatMap((tag) => {
            // 兼容旧格式 {key, value} 与新格式 {key, values[]}
            const values =
              tag.values ?? (tag.value !== undefined ? [tag.value] : []);
            return values.map((value): JobTag => ({
              id: tagId(tag.key, value),
              key: tag.key,
              value,
            }));
          })
        : [],
    resources,
    taskStatuses: allTaskStatuses,
  };
}

export function mapRoleToJobType(tasks: CRDJobTask[]): JobType {
  const roles = new Set(tasks.map((t) => t.role.toLowerCase()));
  const hasEnv = roles.has("env");
  const hasRollout = roles.has("rollout");
  const hasActor = roles.has("actor");
  if (hasActor) return "RL";
  if (hasEnv && hasRollout) return "Evaluation";
  if (hasEnv) return "DataCollection";
  if (hasRollout) return "RL";
  return "Custom";
}

export function crdToWorkflow(crd: CRDWorkflow) {
  const jobs = crd.status?.jobs ?? [];
  const running = jobs.filter((j) => j.phase === "Running").length;
  const phase = crd.metadata.deletionTimestamp
    ? "Deleting"
    : crd.spec.stopped &&
        (crd.status?.phase === "Pending" || crd.status?.phase === "Running")
      ? "Stopping"
      : (crd.status?.phase ?? "Pending");
  return {
    name: crd.metadata.name,
    phase: phase as Phase,
    jobCount: crd.spec.jobTemplates.length,
    runningJobs: running,
    created: crd.metadata.creationTimestamp ?? "—",
    templates: crd.spec.jobTemplates,
    jobStatuses: jobs,
    stopped: crd.spec.stopped ?? false,
  };
}
