import type { Job, JobType, Phase } from "../data";
import type { CRDJob, CRDJobTask, CRDWorkflow } from "../types";

export function crdToJob(crd: CRDJob): Job {
  const tasks = crd.spec.tasks ?? [];
  const container =
    tasks[0]?.kubernetes?.workload?.template.spec.containers?.[0];
  const phase = (crd.status?.phase ?? "Pending") as Phase;
  const allTaskStatuses = crd.status?.tasks ?? [];
  const runningTasks = allTaskStatuses.filter(
    (t) => t.phase === "Running",
  ).length;
  const headerTask = tasks.find((t) => t.head) ?? tasks[0];
  const roles = tasks.map((t) => t.name);
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
      if (vol?.persistentVolumeClaim) {
        const claimName = vol.persistentVolumeClaim.claimName;
        const storageClass =
          t.kubernetes?.workload?.pvcStorageMap?.[claimName] ?? "";
        return {
          type: "storage" as const,
          objectStorage: storageClass,
          mountPath: vm.mountPath,
          hostPath: "",
        };
      }
      const hostPath = vol?.hostPath?.path ?? "";
      return {
        type: "host" as const,
        objectStorage: "",
        mountPath: vm.mountPath,
        hostPath,
      };
    });
    return {
      role: t.name,
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
      pvcStorageMap: t.kubernetes?.workload?.pvcStorageMap,
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
    if (vol?.persistentVolumeClaim) {
      const claimName = vol.persistentVolumeClaim.claimName;
      const storageClass =
        tasks[0]?.kubernetes?.workload?.pvcStorageMap?.[claimName] ?? "";
      return {
        type: "storage" as const,
        objectStorage: storageClass,
        mountPath: vm.mountPath,
        hostPath: "",
      };
    }
    const hostPath = vol?.hostPath?.path ?? "";
    return {
      type: "host" as const,
      objectStorage: "",
      mountPath: vm.mountPath,
      hostPath,
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
    headerRole: headerTask?.name ?? "",
    headerWorker: headerTask?.name ?? "",
    sshAddress: "",
    stopped: crd.spec.stopped ?? false,
    domain: crd.spec.domain ?? "",
    sshPublicKey: crd.spec.sshPublicKey ?? "",
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
  const phase = crd.status?.phase ?? "Pending";
  return {
    name: crd.metadata.name,
    phase: phase as Phase,
    jobCount: crd.spec.jobTemplates.length,
    runningJobs: running,
    created: crd.metadata.creationTimestamp ?? "—",
    templates: crd.spec.jobTemplates,
    jobStatuses: jobs,
  };
}
