import type { Job, Phase } from "../data";

export type JobDisplayPhase = Phase | "Stopping";

// 任务状态统一以 Job 自身的 phase 为准。
// 当 job.stopped 已置位但 phase 尚未推进到 Stopped 时，展示过渡状态 Stopping。
export function effectiveJobPhase(job: Job): JobDisplayPhase {
  if (job.stopped && job.phase !== "Stopped") {
    return "Stopping";
  }
  return job.phase || "Pending";
}
