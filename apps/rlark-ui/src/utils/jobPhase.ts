import type { Job, Phase } from "../data";

export type JobDisplayPhase = Phase | "Stopping";

export function effectiveJobPhase(
  job: Job,
  workerPhases?: string[],
): JobDisplayPhase {
  const phases = (
    workerPhases ?? job.taskStatuses.map((task) => task.phase)
  ).filter(Boolean);
  if (job.stopped) {
    if (job.phase === "Stopped") return "Stopped";
    return phases.length > 0 && phases.every((phase) => phase === "Stopped")
      ? "Stopped"
      : "Stopping";
  }
  if (phases.length === 0) return job.phase;
  if (phases.some((phase) => phase === "Failed")) return "Failed";
  if (phases.every((phase) => phase === "Succeeded")) return "Succeeded";
  if (phases.every((phase) => phase === "Running")) return "Running";
  if (job.phase === "Stopped" && phases.every((phase) => phase === "Stopped")) {
    return "Stopped";
  }
  return "Pending";
}
