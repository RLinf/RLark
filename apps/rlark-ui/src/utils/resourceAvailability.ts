import type { CRDTask } from "../types";

export type ReclaimableResources = Record<string, Record<string, number>>;

function quantity(value?: string): number {
  const parsed = Number.parseFloat(value ?? "0");
  return Number.isFinite(parsed) ? parsed : 0;
}

export function availableResource(
  allocatable: string | undefined,
  used: string | undefined,
  reclaimable = 0,
): number {
  const capacity = quantity(allocatable);
  return Math.min(
    capacity,
    Math.max(0, capacity - quantity(used) + reclaimable),
  );
}

export function reclaimableResourcesForTasks(
  tasks: CRDTask[],
): ReclaimableResources {
  const reclaimable: ReclaimableResources = {};
  for (const task of tasks) {
    const requests =
      task.spec?.kubernetes?.workload?.template.spec.containers?.reduce<
        Record<string, number>
      >((totals, container) => {
        for (const [key, value] of Object.entries(
          container.resources?.requests ?? {},
        )) {
          totals[key] = (totals[key] ?? 0) + quantity(value);
        }
        return totals;
      }, {}) ?? {};

    for (const nodeName of task.status?.observedNodes ?? []) {
      const nodeKey = `${task.metadata.namespace ?? ""}/${nodeName}`;
      const nodeResources = (reclaimable[nodeKey] ??= {});
      for (const [key, value] of Object.entries(requests)) {
        nodeResources[key] = (nodeResources[key] ?? 0) + value;
      }
    }
  }
  return reclaimable;
}
