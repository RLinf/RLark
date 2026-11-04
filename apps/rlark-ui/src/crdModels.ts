import type { IJob } from "./generated/rlark/rlinf.io/v1alpha1/Job.js";
import type { ITask } from "./generated/rlark/rlinf.io/v1alpha1/Task.js";
import type { IWorkflow } from "./generated/rlark/rlinf.io/v1alpha1/Workflow.js";

export type CRDJob = IJob;
export type CRDTask = ITask;
export type CRDWorkflow = IWorkflow;

export type CRDJobTask = NonNullable<
  NonNullable<IJob["spec"]>["tasks"]
>[number];

/** A single entry of the Pod template `spec.volumes` list carried by a Job task. */
export type CRDVolume = NonNullable<
  NonNullable<
    NonNullable<CRDJobTask["workload"]["template"]>["spec"]
  >["volumes"]
>[number];
