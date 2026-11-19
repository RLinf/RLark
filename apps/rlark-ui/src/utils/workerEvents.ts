import type { NodeEventEntry } from "../types";

// 一旦 Pod 已绑定节点，这些 reason 就只能是绑定之前产生的历史事件。
// kube-scheduler 的 FailedScheduling 与卷绑定阶段的 FailedBinding 都只在 Pod
// 绑定前写入：使用 generic ephemeral volume 时首次调度必然先失败（PVC 尚未由
// ephemeral volume controller 创建），约一秒后自动调度成功，因此必须按当前状态
// 而不是事件流本身判断事件是否仍然有效。
const RESOLVED_AFTER_SCHEDULING_REASONS = new Set([
  "FailedScheduling",
  "FailedBinding",
]);

// kubelet 的 Pulling/Pulled 是"某个容器镜像在某一刻的拉取进展"，不是 Pod 的
// 持久状态。一个 Worker Pod 同时包含主容器与平台注入的 sidecar/init 容器，
// 每个容器各自上报一次，所以"镜像已拉取"很可能说的是 sidecar 镜像，而不是
// 用户填写的业务镜像；把它与"正在拉取镜像""启动失败"并列展示会让用户以为
// 错误镜像已经拉取成功。
const PULL_PROGRESS_REASONS = new Set(["Pulling", "Pulled"]);

// 镜像拉取处于失败状态时，任何历史拉取进展都不应再作为"当前等待原因"展示。
const IMAGE_PULL_FAILURE_REASONS = new Set([
  "ErrImagePull",
  "ImagePullBackOff",
  "InvalidImageName",
  "FailedToRetrieveImagePullSecret",
]);

// kubelet 对镜像拉取失败和容器启动失败都使用 reason=Failed，仅凭 reason 会把
// "镜像拉取失败"显示成"Worker 启动失败"。message 才带有可区分的信息，因此按
// message 把 Failed 收敛到更精确的 reason；无法判定时保持原值。
const INVALID_IMAGE_NAME_PATTERN =
  /invalid reference format|couldn't parse image( reference)?|cannot parse image/i;
const IMAGE_PULL_FAILURE_PATTERN =
  /failed to pull (and unpack )?image|errimagepull|imagepullbackoff|pull access denied|manifest unknown|manifest for .* not found|repository .* not found|does not exist or no pull access|unauthorized: authentication required/i;

// workerEventTime 把事件时间解析为毫秒；缺失或非法时间返回 0，让排序与比较
// 退化为"最旧"而不是抛出 NaN 污染比较结果。
export function workerEventTime(event: NodeEventEntry): number {
  const parsed = Date.parse(event.lastTime ?? "");
  return Number.isNaN(parsed) ? 0 : parsed;
}

// refineWorkerEventReason 返回用于展示与去重的 reason。只收敛 kubelet 的
// 通用 Failed 事件，其余 reason 原样返回。
export function refineWorkerEventReason(
  reason?: string,
  message?: string,
): string {
  const current = reason ?? "";
  if (current !== "Failed" || !message) return current;
  if (INVALID_IMAGE_NAME_PATTERN.test(message)) return "InvalidImageName";
  if (IMAGE_PULL_FAILURE_PATTERN.test(message)) return "ErrImagePull";
  return current;
}

// supersedePullProgress 从"当前有效"事件里剔除已经没有参考价值的镜像拉取
// 进展：只保留最新的一条，并在出现不早于它的 Warning 事件、或镜像拉取已进入
// 失败状态时整条剔除。被剔除的事件进入 resolved 分区，仍可排查。
function supersedePullProgress(
  active: NodeEventEntry[],
  resolved: NodeEventEntry[],
): { active: NodeEventEntry[]; resolved: NodeEventEntry[] } {
  const progress = active.filter((event) =>
    PULL_PROGRESS_REASONS.has(event.reason ?? ""),
  );
  if (progress.length === 0) return { active, resolved };

  const others = active.filter(
    (event) => !PULL_PROGRESS_REASONS.has(event.reason ?? ""),
  );
  const pullFailed = others.some((event) =>
    IMAGE_PULL_FAILURE_REASONS.has(
      refineWorkerEventReason(event.reason, event.message),
    ),
  );
  const newestWarning = others.reduce(
    (latest, event) =>
      event.type === "Normal"
        ? latest
        : Math.max(latest, workerEventTime(event)),
    Number.NEGATIVE_INFINITY,
  );
  const newestProgress = progress.reduce(
    (latest, event) =>
      workerEventTime(event) > workerEventTime(latest) ? event : latest,
    progress[0],
  );
  const keep =
    pullFailed || workerEventTime(newestProgress) < newestWarning
      ? undefined
      : newestProgress;

  const superseded = new Set(progress.filter((event) => event !== keep));
  return {
    active: active.filter((event) => !superseded.has(event)),
    resolved: [...resolved, ...superseded],
  };
}

// partitionWorkerEvents 把 worker 事件拆成"当前有效"与"已被后续进展取代"
// 两组。Gateway 只放行 Warning 与 Pulling/Pulled 事件，Normal 的 Scheduled
// 事件拿不到，所以调用方以 scheduled（Pod 已分配节点）作为调度完成的判据。
// resolved 事件不会被丢弃，调用方以弱化样式单独分区展示，保证可排查性。
export function partitionWorkerEvents(
  events: NodeEventEntry[],
  scheduled: boolean,
): { active: NodeEventEntry[]; resolved: NodeEventEntry[] } {
  const active: NodeEventEntry[] = [];
  const resolved: NodeEventEntry[] = [];
  for (const event of events) {
    if (
      scheduled &&
      RESOLVED_AFTER_SCHEDULING_REASONS.has(event.reason ?? "")
    ) {
      resolved.push(event);
    } else {
      active.push(event);
    }
  }
  return supersedePullProgress(active, resolved);
}
