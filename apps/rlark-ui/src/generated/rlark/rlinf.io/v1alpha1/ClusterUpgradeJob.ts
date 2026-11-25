// Code generated from api/config/crd/bases. DO NOT EDIT.

import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData, setValidateFunc, createTypeMetaGuard } from "@kubernetes-models/base";
import { ValidateFunc } from "@kubernetes-models/validate";
import { validate } from "../../_schemas/RlinfIoV1alpha1ClusterUpgradeJob.js";

export interface IClusterUpgradeJob {
  /**
   * APIVersion defines the versioned schema of this representation of an object.
   * Servers should convert recognized schemas to the latest internal value, and
   * may reject unrecognized values.
   * More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
   */
  "apiVersion": "rlinf.io/v1alpha1";
  /**
   * Kind is a string value representing the REST resource this object represents.
   * Servers may infer this from the endpoint the client submits requests to.
   * Cannot be updated.
   * In CamelCase.
   * More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
   */
  "kind": "ClusterUpgradeJob";
  "metadata"?: IObjectMeta;
  /**
   * ClusterUpgradeJobSpec defines the desired state of a cluster upgrade job.
   * This CR is created by the control plane for each target cluster.
   */
  "spec"?: {
    /**
     * AutoRollback indicates whether to automatically rollback on failure.
     */
    "autoRollback"?: boolean;
    /**
     * ConfigPatch is the configuration patch to apply.
     */
    "configPatch"?: string;
    /**
     * Image is the target rlarkadm image for upgrade.
     */
    "image": string;
    /**
     * TimeoutSeconds is the timeout for the upgrade.
     */
    "timeoutSeconds"?: number;
    /**
     * UpgradeJobName is the name of the parent UpgradeJob.
     */
    "upgradeJobName": string;
  };
  /**
   * ClusterUpgradeJobStatus defines the observed state of a cluster upgrade job.
   * This is updated by the data plane agent.
   */
  "status"?: {
    /**
     * CompletionTime is the time when the upgrade completed.
     */
    "completionTime"?: string;
    /**
     * JobName is the name of the local upgrade Job.
     */
    "jobName"?: string;
    /**
     * JobNamespace is the namespace of the local upgrade Job.
     */
    "jobNamespace"?: string;
    /**
     * Message provides additional information about the current phase.
     */
    "message"?: string;
    /**
     * Phase is the current phase of the upgrade.
     */
    "phase"?: "Pending" | "Upgrading" | "Succeeded" | "Failed" | "RolledBack";
    /**
     * RollbackJobName is the name of the rollback Job if rollback was performed.
     */
    "rollbackJobName"?: string;
    /**
     * StartTime is the time when the upgrade started.
     */
    "startTime"?: string;
  };
}

export class ClusterUpgradeJob extends Model<IClusterUpgradeJob> implements IClusterUpgradeJob {
  "apiVersion": IClusterUpgradeJob["apiVersion"];
  "kind": IClusterUpgradeJob["kind"];
  "metadata"?: IClusterUpgradeJob["metadata"];
  "spec"?: IClusterUpgradeJob["spec"];
  "status"?: IClusterUpgradeJob["status"];

static apiVersion: IClusterUpgradeJob["apiVersion"] = "rlinf.io/v1alpha1";
static kind: IClusterUpgradeJob["kind"] = "ClusterUpgradeJob";
static is = createTypeMetaGuard<IClusterUpgradeJob>(ClusterUpgradeJob);

constructor(data?: ModelData<IClusterUpgradeJob>) {
  super();

  this.setDefinedProps({
    apiVersion: ClusterUpgradeJob.apiVersion,
    kind: ClusterUpgradeJob.kind,
    ...data
  } as IClusterUpgradeJob);
}
}


setValidateFunc(ClusterUpgradeJob, validate as ValidateFunc<IClusterUpgradeJob>);
