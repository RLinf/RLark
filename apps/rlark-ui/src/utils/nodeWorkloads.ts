import type { CRDPod } from "../types";

export type NodeWorkload = {
  /** Distinct Job names owning the active Pods, sorted for stable rendering. */
  jobs: string[];
  /** Number of active Pods scheduled to the Node. */
  workers: number;
};

/**
 * A scheduled Pod counts as a Worker until it reaches a terminal phase. Pods
 * that are briefly Pending during startup or a restart are still placed on the
 * Node, so excluding them would make the node list disagree with the detail.
 */
export function isActiveWorkerPhase(phase?: string): boolean {
  return phase !== "Succeeded" && phase !== "Failed";
}

type TaskLike = {
  metadata?: { name?: string; labels?: Record<string, string> };
};

type NodeRef = { metadata?: { name?: string; namespace?: string } };

/**
 * Workload map key. Node names are only unique inside an Agent namespace, so the
 * namespace must stay part of the key: two clusters may both run a "node-1".
 */
export function nodeWorkloadKey(node: NodeRef): string {
  return `${node.metadata?.namespace ?? ""}/${node.metadata?.name ?? ""}`;
}

/**
 * Aggregates the mirrored Task and Pod resources into per-Node workload counts
 * for display. Placement and action gating must use the controller-written Node
 * status instead, because this mirror can lag behind the Agent.
 */
export function buildNodeWorkloads(
  tasks: readonly TaskLike[],
  pods: readonly CRDPod[],
): Record<string, NodeWorkload> {
  const taskJobs = new Map<string, string>();
  for (const task of tasks) {
    const name = task.metadata?.name;
    if (!name) continue;
    taskJobs.set(name, task.metadata?.labels?.["rlinf.io/job"] ?? "");
  }

  const workloads = new Map<string, { jobs: Set<string>; workers: number }>();
  for (const pod of pods) {
    const nodeName = pod.status?.node;
    if (!nodeName || !isActiveWorkerPhase(pod.status?.phase)) continue;
    const key = `${pod.metadata?.namespace ?? ""}/${nodeName}`;
    const current = workloads.get(key) ?? {
      jobs: new Set<string>(),
      workers: 0,
    };
    const jobName = taskJobs.get(pod.spec?.taskName ?? "");
    if (jobName) current.jobs.add(jobName);
    current.workers += 1;
    workloads.set(key, current);
  }

  return Object.fromEntries(
    [...workloads].map(([key, workload]) => [
      key,
      { jobs: [...workload.jobs].sort(), workers: workload.workers },
    ]),
  );
}
