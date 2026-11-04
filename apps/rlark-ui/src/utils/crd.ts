import type { Job, JobTag, JobType, MountType, Phase } from "../data";
import type { CRDJob, CRDJobTask, CRDVolume, CRDWorkflow } from "../crdModels";

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

type JobMount = {
  type: MountType;
  objectStorage: string;
  mountPath: string;
  hostPath: string;
  pvcSizeGb: number;
  rawVolume?: CRDVolume;
};

// 把 Pod 模板里的一个 volume 映射成表单可消费的挂载项。
// 表单只能编辑 hostPath 与 ephemeral/PVC 两类卷；其余卷源（emptyDir、configMap、
// secret、projected 等）标记为 opaque 并原样保留 rawVolume，使"复制任务"不会把它们
// 降级成空的主机目录——例如 /dev/shm 的 emptyDir{medium:Memory} 必须保持内存 tmpfs。
export function toJobMount(
  vol: CRDVolume | undefined,
  mountPath: string,
): JobMount {
  if (vol?.ephemeral) {
    const spec = vol.ephemeral.volumeClaimTemplate?.spec;
    return {
      type: "storage",
      objectStorage: spec?.storageClassName ?? "",
      mountPath,
      hostPath: "",
      pvcSizeGb: storageGi(spec?.resources?.requests?.storage),
    };
  }
  if (vol?.persistentVolumeClaim) {
    return {
      type: "storage",
      objectStorage: vol.persistentVolumeClaim.claimName,
      mountPath,
      hostPath: "",
      pvcSizeGb: 10,
    };
  }
  if (vol?.hostPath) {
    return {
      type: "host",
      objectStorage: "",
      mountPath,
      hostPath: vol.hostPath.path,
      pvcSizeGb: 10,
    };
  }
  if (vol) {
    return {
      type: "opaque",
      objectStorage: "",
      mountPath,
      hostPath: "",
      pvcSizeGb: 10,
      rawVolume: vol,
    };
  }
  // volumeMount 找不到对应的 volume：按空主机目录展示，提示用户补全。
  return {
    type: "host",
    objectStorage: "",
    mountPath,
    hostPath: "",
    pvcSizeGb: 10,
  };
}

// 把表单无法编辑的卷源摘要成一行只读文案，例如 "emptyDir (Memory, 32Gi)"。
export function describeVolumeSource(vol?: CRDVolume): string {
  if (!vol) return "";
  const entry = Object.entries(vol).find(([key]) => key !== "name");
  if (!entry) return "";
  const [kind, source] = entry;
  if (kind === "emptyDir") {
    const empty = source as { medium?: string; sizeLimit?: string | number };
    const detail = [empty?.medium, empty?.sizeLimit].filter(Boolean).join(", ");
    return detail ? `emptyDir (${detail})` : "emptyDir";
  }
  const named = source as { name?: string; secretName?: string };
  const name = named?.name ?? named?.secretName;
  return name ? `${kind} (${name})` : kind;
}

function taskMounts(task: CRDJobTask | undefined): JobMount[] {
  const container = task?.workload.template?.spec?.containers?.find(
    (item) => item.name === "main",
  );
  const volumes = task?.workload.template?.spec?.volumes ?? [];
  return (container?.volumeMounts ?? []).map((vm) =>
    toJobMount(
      volumes.find((v) => v.name === vm.name),
      vm.mountPath,
    ),
  );
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
  // 角色数量必须与详情页和任务编辑器使用的角色名一致。`task.role` 只是用于分组
  // 展示的粗粒度分类（Actor/Rollout/Env），多个业务角色会被折叠成同一个取值，
  // 据此计数会少报角色数。
  const roleCount = new Set(roles.filter(Boolean)).size;
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
    const taskMountList = taskMounts(t);
    return {
      role: taskRoleName(t),
      // Job CRD 的 task 不携带集群标识，集群由 CreateJob 页面按 nodeSelector
      // 反查节点所属 namespace 推断，这里只能留空。
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
      mounts: taskMountList,
    };
  });
  const env =
    container?.env
      ?.filter((e) => e.name !== "RLARK_TASK_ROLE")
      .map((e) => ({
        key: e.name,
        value: e.value ?? "",
      })) ?? [];
  const mounts = taskMounts(tasks[0]);
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
