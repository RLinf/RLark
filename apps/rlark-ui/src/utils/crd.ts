import type { Job, JobTag, JobType, Phase } from "../data";
import type { CRDJob, CRDJobTask, CRDWorkflow } from "../crdModels";

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

function storageGi(quantity?: string | number): number {
  const match = String(quantity ?? "").match(/^(\d+)(?:Gi)?$/);
  return match ? Number(match[1]) : 10;
}

export function crdToJob(crd: CRDJob): Job {
  const tasks = crd.spec?.tasks ?? [];
  const mainContainer = (task?: CRDJobTask) =>
    task?.workload.template?.spec?.containers?.find(
      (container) => container.name === "main",
    );
  const container = mainContainer(tasks[0]);
  // Stopped 优先于 deletionTimestamp：
  // archive 删除 KCP CR 时 deletionTimestamp 非空，但任务实际已停止。
  const rawPhase = crd.status?.phase ?? "Pending";
  const phase = (
    crd.metadata?.deletionTimestamp && rawPhase !== "Stopped"
      ? "Deleting"
      : rawPhase
  ) as Phase;
  const allTaskStatuses = crd.status?.tasks ?? [];
  const runningTasks = allTaskStatuses.filter(
    (t) => t.phase === "Running",
  ).length;
  const headerTask = tasks.find((t) => t.head) ?? tasks[0];
  const taskRoleName = (task: CRDJobTask) =>
    mainContainer(task)?.env?.find((env) => env.name === "RLARK_TASK_ROLE")
      ?.value ??
    task.name ??
    "";
  const roles = tasks.map(taskRoleName);
  const roleCount = new Set(tasks.map((task) => task.role).filter(Boolean))
    .size;
  const displayName =
    crd.metadata?.annotations?.["rlark.io/display-name"] ??
    crd.metadata?.labels?.["rlark.io/display-name"] ??
    crd.metadata?.name ??
    "";
  const resources = tasks.map((t) => {
    const c = mainContainer(t);
    const res = c?.resources?.requests ?? {};
    const gpu = res["nvidia.com/gpu"] ?? "0";
    const devices = Object.entries(res)
      .filter(([k]) => k.startsWith("rlinf.io/"))
      .map(([name, quantity]) => ({ name, quantity: String(quantity) }));
    const ns = t.placement.nodeSelector ?? {};
    const nsStr = Object.entries(ns)
      .map(([k, v]) => `${k}=${v}`)
      .join(",");
    const taskEnv =
      c?.env
        ?.filter((e) => e.name !== "RLARK_TASK_ROLE")
        .map((e) => ({
          key: e.name,
          value: e.value ?? "",
        })) ?? [];
    const taskMounts = (c?.volumeMounts ?? []).map((vm) => {
      const vol = t.workload.template?.spec?.volumes?.find(
        (v) => v.name === vm.name,
      );
      if (vol?.ephemeral) {
        const spec = vol.ephemeral.volumeClaimTemplate?.spec;
        return {
          type: "storage" as const,
          objectStorage: spec?.storageClassName ?? "",
          mountPath: vm.mountPath,
          hostPath: "",
          pvcSizeGb: storageGi(spec?.resources?.requests?.storage),
        };
      }
      if (vol?.persistentVolumeClaim) {
        return {
          type: "storage" as const,
          objectStorage: vol.persistentVolumeClaim.claimName,
          mountPath: vm.mountPath,
          hostPath: "",
          pvcSizeGb: 10,
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
      replicas: carrierReplicas(t),
      cpu: String(res.cpu ?? ""),
      memory: String(res.memory ?? ""),
      gpu: String(gpu),
      devices,
      image: c?.image ?? "",
      prepareScript: rayScript(t, "prepare"),
      env: taskEnv,
      mounts: taskMounts,
    };
  });
  const env =
    container?.env
      ?.filter((e) => e.name !== "RLARK_TASK_ROLE")
      .map((e) => ({
        key: e.name,
        value: e.value ?? "",
      })) ?? [];
  const mounts = (container?.volumeMounts ?? []).map((vm) => {
    const vol = tasks[0]?.workload.template?.spec?.volumes?.find(
      (v) => v.name === vm.name,
    );
    if (vol?.ephemeral) {
      const spec = vol.ephemeral.volumeClaimTemplate?.spec;
      return {
        type: "storage" as const,
        objectStorage: spec?.storageClassName ?? "",
        mountPath: vm.mountPath,
        hostPath: "",
        pvcSizeGb: storageGi(spec?.resources?.requests?.storage),
      };
    }
    if (vol?.persistentVolumeClaim) {
      return {
        type: "storage" as const,
        objectStorage: vol.persistentVolumeClaim.claimName,
        mountPath: vm.mountPath,
        hostPath: "",
        pvcSizeGb: 10,
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
    id: crd.metadata?.name ?? "",
    name: crd.metadata?.name ?? "",
    displayName,
    type: mapRoleToJobType(tasks),
    phase,
    owner: "",
    cluster: tasks
      .map((t) =>
        t.placement.nodeSelector
          ? Object.values(t.placement.nodeSelector)[0]
          : "",
      )
      .filter(Boolean)
      .join(", "),
    target: roles.join(" / "),
    workers: tasks.length,
    runningWorkers: runningTasks,
    startedAt: crd.metadata?.creationTimestamp ?? "—",
    submittedAt: crd.metadata?.creationTimestamp ?? "—",
    stoppedAt: crd.status?.endTime ?? "—",
    roleCount,
    duration: "—",
    progress:
      phase === "Succeeded"
        ? 100
        : Math.round((runningTasks / Math.max(tasks.length, 1)) * 100),
    defaultRoles: roles,
    image: container?.image ?? "",
    command: rayScript(headerTask, "run"),
    tensorBoardDir: tensorBoardLogDir(headerTask),
    env,
    mounts,
    headerRole: headerTask ? taskRoleName(headerTask) : "",
    headerWorker: headerTask ? taskRoleName(headerTask) : "",
    sshAddress: "",
    stopped: crd.spec?.suspended ?? false,
    domain: crd.spec?.domain ?? "",
    sshPublicKey: crd.spec?.sshPublicKey ?? "",
    tags:
      crd.spec?.tags && crd.spec.tags.length > 0
        ? crd.spec.tags.flatMap((tag) =>
            tag.values.map((value): JobTag => ({
              id: tagId(tag.key, value),
              key: tag.key,
              value,
            })),
          )
        : [],
    resources,
    taskStatuses: allTaskStatuses.map((status) => ({
      name: status.name ?? "",
      phase: status.phase ?? "Pending",
      message: status.message ?? "",
    })),
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
  const phase = crd.metadata?.deletionTimestamp
    ? "Deleting"
    : crd.spec?.suspended &&
        (crd.status?.phase === "Pending" || crd.status?.phase === "Running")
      ? "Stopping"
      : (crd.status?.phase ?? "Pending");
  return {
    name: crd.metadata?.name ?? "",
    phase: phase as Phase,
    jobCount: crd.spec?.jobTemplates?.length ?? 0,
    runningJobs: running,
    created: crd.metadata?.creationTimestamp ?? "—",
    templates: crd.spec?.jobTemplates ?? [],
    jobStatuses: jobs,
    stopped: crd.spec?.suspended ?? false,
  };
}

function carrierReplicas(task: CRDJobTask): number {
  const parameters = task.workload.carrier.parameters;
  if (!parameters || typeof parameters !== "object") return 1;
  const replicas = Number((parameters as { replicas?: unknown }).replicas);
  return Number.isFinite(replicas) ? replicas : 1;
}

function rayScript(
  task: CRDJobTask | undefined,
  field: "prepare" | "run",
): string {
  const parameters = task?.workload.parameters;
  if (!parameters || typeof parameters !== "object") return "";
  const script = (parameters as Record<string, unknown>)[field];
  if (!script || typeof script !== "object") return "";
  const inline = (script as { inline?: unknown }).inline;
  return typeof inline === "string" ? inline : "";
}

function tensorBoardLogDir(task: CRDJobTask | undefined): string {
  const environment = task?.environments?.find(
    (item) => item.type === "rlinf.io/tensorboard" && item.version === "v1",
  );
  if (!environment?.parameters || typeof environment.parameters !== "object")
    return "";
  const logDir = (environment.parameters as { logDir?: unknown }).logDir;
  return typeof logDir === "string" ? logDir : "";
}
