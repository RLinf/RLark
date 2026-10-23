import type { IJob } from "./generated/rlark/rlinf.io/v1alpha1/Job.js";
import type { ITask } from "./generated/rlark/rlinf.io/v1alpha1/Task.js";
import type { IWorkflow } from "./generated/rlark/rlinf.io/v1alpha1/Workflow.js";

export type CRDJob = IJob;
export type CRDTask = ITask;
export type CRDWorkflow = IWorkflow;

export type CRDJobTask = NonNullable<
  NonNullable<IJob["spec"]>["tasks"]
>[number];
