// Code generated from api/config/crd/bases. DO NOT EDIT.

import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData, setValidateFunc, createTypeMetaGuard } from "@kubernetes-models/base";
import { ValidateFunc } from "@kubernetes-models/validate";
import { validate } from "../../_schemas/RlinfIoV1alpha1UpgradeJob.js";

export interface IUpgradeJob {
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
  "kind": "UpgradeJob";
  "metadata"?: IObjectMeta;
  /**
   * UpgradeJobSpec defines the desired state of an upgrade job.
   */
  "spec"?: {
    /**
     * ClusterNames is the list of target cluster names for upgrade.
     */
    "clusterNames"?: Array<string>;
    /**
     * ClusterSelector selects the target clusters for upgrade.
     */
    "clusterSelector"?: {
      /**
       * matchExpressions is a list of label selector requirements. The requirements are ANDed.
       */
      "matchExpressions"?: Array<{
        /**
         * key is the label key that the selector applies to.
         */
        "key": string;
        /**
         * operator represents a key's relationship to a set of values.
         * Valid operators are In, NotIn, Exists and DoesNotExist.
         */
        "operator": string;
        /**
         * values is an array of string values. If the operator is In or NotIn,
         * the values array must be non-empty. If the operator is Exists or DoesNotExist,
         * the values array must be empty. This array is replaced during a strategic
         * merge patch.
         */
        "values"?: Array<string>;
      }>;
      /**
       * matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
       * map is equivalent to an element of matchExpressions, whose key field is "key", the
       * operator is "In", and the values array contains only "value". The requirements are ANDed.
       */
      "matchLabels"?: {
        [key: string]: string;
      };
    };
    /**
     * ConfigPatch is the full deployment configuration for this upgrade.
     * When empty, the current configuration from rlark-state is used.
     * When set, it must be a complete DeployConfig including apiVersion, kind, and plane.
     */
    "configPatch"?: string;
    /**
     * Image is the target rlarkadm image for upgrade.
     */
    "image": string;
    /**
     * MaintenanceWindow defines the maintenance window for the upgrade.
     */
    "maintenanceWindow"?: {
      /**
       * EndTime is the end time of the maintenance window.
       */
      "endTime"?: string;
      /**
       * StartTime is the start time of the maintenance window.
       */
      "startTime"?: string;
    };
    /**
     * Strategy defines the upgrade strategy.
     */
    "strategy"?: {
      /**
       * AutoRollback indicates whether to automatically rollback on failure.
       */
      "autoRollback"?: boolean;
      /**
       * BackoffLimit is the number of retries before declaring the upgrade failed.
       */
      "backoffLimit"?: number;
      /**
       * Concurrency is the number of clusters to upgrade concurrently.
       */
      "concurrency"?: number;
      /**
       * FailureTolerate is the percentage of clusters that can fail before stopping the upgrade.
       */
      "failureTolerate"?: number;
      /**
       * TimeoutSeconds is the timeout for each cluster upgrade.
       */
      "timeoutSeconds"?: number;
    };
  };
  /**
   * UpgradeJobStatus defines the observed state of an upgrade job.
   */
  "status"?: {
    /**
     * CompletionTime is the time when the upgrade job completed.
     */
    "completionTime"?: string;
    /**
     * Conditions is the list of conditions for each cluster.
     */
    "conditions"?: Array<{
      /**
       * Cluster is the name of the cluster.
       */
      "cluster": string;
      /**
       * Reason is a human-readable message about the current status.
       */
      "reason"?: string;
      /**
       * RollbackImage is the image the cluster was rolled back to.
       */
      "rollbackImage"?: string;
      /**
       * Status is the phase of the ClusterUpgradeJob (Pending, Upgrading, Succeeded, Failed, RolledBack).
       */
      "status": string;
      /**
       * TargetImage is the image the cluster was asked to upgrade to.
       */
      "targetImage"?: string;
      /**
       * Type is the type of the condition. Reserved for future use.
       */
      "type": string;
      /**
       * UpgradeTime is when the cluster upgrade completed.
       */
      "upgradeTime"?: string;
    }>;
    /**
     * FailedClusters is the number of clusters that failed to upgrade.
     */
    "failedClusters"?: number;
    /**
     * Phase is the current phase of the upgrade job.
     */
    "phase"?: "Pending" | "Upgrading" | "Succeeded" | "Failed" | "RollingBack";
    /**
     * RollbackClusters is the number of clusters that were rolled back.
     */
    "rollbackClusters"?: number;
    /**
     * StartTime is the time when the upgrade job started.
     */
    "startTime"?: string;
    /**
     * TotalClusters is the total number of target clusters.
     */
    "totalClusters"?: number;
    /**
     * UpgradedClusters is the number of clusters that have been upgraded successfully.
     */
    "upgradedClusters"?: number;
    /**
     * UpgradingClusters is the number of clusters currently being upgraded.
     */
    "upgradingClusters"?: number;
  };
}

export class UpgradeJob extends Model<IUpgradeJob> implements IUpgradeJob {
  "apiVersion": IUpgradeJob["apiVersion"];
  "kind": IUpgradeJob["kind"];
  "metadata"?: IUpgradeJob["metadata"];
  "spec"?: IUpgradeJob["spec"];
  "status"?: IUpgradeJob["status"];

static apiVersion: IUpgradeJob["apiVersion"] = "rlinf.io/v1alpha1";
static kind: IUpgradeJob["kind"] = "UpgradeJob";
static is = createTypeMetaGuard<IUpgradeJob>(UpgradeJob);

constructor(data?: ModelData<IUpgradeJob>) {
  super();

  this.setDefinedProps({
    apiVersion: UpgradeJob.apiVersion,
    kind: UpgradeJob.kind,
    ...data
  } as IUpgradeJob);
}
}


setValidateFunc(UpgradeJob, validate as ValidateFunc<IUpgradeJob>);
