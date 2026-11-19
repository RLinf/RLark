import type { Job, Phase } from "../data";

export type JobDisplayPhase = Phase | "Stopping";

// 任务状态统一以 Job 自身的 phase 为准。
// 当 job.stopped 已置位但 phase 尚未推进到 Stopped 时，展示过渡状态 Stopping。
export function effectiveJobPhase(job: Job): JobDisplayPhase {
  if (job.phase === "Deleting") {
    return "Deleting";
  }
  if (job.stopped && job.phase !== "Stopped") {
    return "Stopping";
  }
  return job.phase || "Pending";
}

// 终态集合：进入这些状态后任务不再运行，不应计入「运行中」。
const TERMINAL_JOB_PHASES: readonly JobDisplayPhase[] = [
  "Stopped",
  "Succeeded",
  "Failed",
  "Deleting",
];

// 判定任务是否仍处于活跃（非终态）状态，过渡态 Stopping 仍视为活跃。
export function isActiveJob(job: Job): boolean {
  return !TERMINAL_JOB_PHASES.includes(effectiveJobPhase(job));
}
