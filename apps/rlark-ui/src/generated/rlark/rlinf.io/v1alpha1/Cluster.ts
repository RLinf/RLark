// Code generated from api/config/crd/bases. DO NOT EDIT.

import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData, setValidateFunc, createTypeMetaGuard } from "@kubernetes-models/base";
import { ValidateFunc } from "@kubernetes-models/validate";
import { validate } from "../../_schemas/RlinfIoV1alpha1Cluster.js";

export interface ICluster {
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
  "kind": "Cluster";
  "metadata"?: IObjectMeta;
  /**
   * ClusterSpec declares the durable operational state of a data-plane cluster.
   */
  "spec"?: {
    /**
     * AgentNamespace is the management-plane namespace that contains the agent heartbeat Lease and Node CRs.
     */
    "agentNamespace": string;
    "cordoned"?: boolean;
    "delisted"?: boolean;
    "drainPreviousCordoned"?: boolean;
    "draining"?: boolean;
  };
  /**
   * ClusterStatus reports the controller's current observation of a data-plane cluster.
   */
  "status"?: {
    /**
     * Conditions records the structured conclusions of the Cluster controller,
     * which is their only writer. Ready and Degraded are independent dimensions:
     * a cluster that lost part of its capacity reports Ready=True together with
     * Degraded=True. They are observation-driven and therefore carry no
     * per-condition ObservedGeneration; use the status-level ObservedGeneration
     * above to tell whether they already reflect the latest Spec.
     */
    "conditions"?: Array<{
      /**
       * lastTransitionTime is the last time the condition transitioned from one status to another.
       * This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.
       */
      "lastTransitionTime": string;
      /**
       * message is a human readable message indicating details about the transition.
       * This may be an empty string.
       */
      "message": string;
      /**
       * observedGeneration represents the .metadata.generation that the condition was set based upon.
       * For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
       * with respect to the current state of the instance.
       */
      "observedGeneration"?: number;
      /**
       * reason contains a programmatic identifier indicating the reason for the condition's last transition.
       * Producers of specific condition types may define expected values and meanings for this field,
       * and whether the values are considered a guaranteed API.
       * The value should be a CamelCase string.
       * This field may not be empty.
       */
      "reason": string;
      /**
       * status of the condition, one of True, False, Unknown.
       */
      "status": "True" | "False" | "Unknown";
      /**
       * type of condition in CamelCase or in foo.example.com/CamelCase.
       */
      "type": string;
    }>;
    /**
     * DegradedNodes is the number of connected Nodes that are not ready or report Degraded=True.
     */
    "degradedNodes"?: number;
    /**
     * DisconnectedNodes is the number of Nodes whose AgentConnected condition is not True.
     */
    "disconnectedNodes"?: number;
    /**
     * DominantFailureReason is the most frequent reason among unavailable candidate Nodes.
     */
    "dominantFailureReason"?: string;
    "lastHeartbeatTime"?: string;
    /**
     * LastStatusChangeTime is the time Phase last changed. It carries the same
     * semantics as the identically named field on NodeStatus and distinguishes
     * "has stayed in this phase for a long time" from "just flipped".
     */
    "lastStatusChangeTime"?: string;
    /**
     * ObservedGeneration is the metadata.generation observed by the Cluster
     * controller the last time it finished a reconciliation. ClusterSpec's
     * cordoned / draining are Spec-driven switches, so consumers must compare
     * this field with metadata.generation to tell whether phase and conditions
     * already reflect the latest Spec. It is a controller contract, not an API
     * server guarantee.
     */
    "observedGeneration"?: number;
    "onlineNodes"?: number;
    /**
     * ClusterPhase summarizes the current lifecycle state of a data-plane cluster.
     * The set is closed in the CRD schema: adding a value requires updating the
     * enum marker, regenerating the CRD manifests, and updating the documentation.
     */
    "phase"?: "Online" | "Offline" | "Downgraded" | "Cordoned" | "Draining" | "Delisted";
    /**
     * ReadyNodes is the number of connected Nodes with Ready=True that are not
     * cordoned by spec.unschedulable. A ready Node under resource pressure is
     * still counted here even though it is excluded from new placement.
     */
    "readyNodes"?: number;
    "totalNodes"?: number;
    /**
     * UnschedulableNodes is the number of Nodes with spec.unschedulable=true.
     */
    "unschedulableNodes"?: number;
  };
}

export class Cluster extends Model<ICluster> implements ICluster {
  "apiVersion": ICluster["apiVersion"];
  "kind": ICluster["kind"];
  "metadata"?: ICluster["metadata"];
  "spec"?: ICluster["spec"];
  "status"?: ICluster["status"];

static apiVersion: ICluster["apiVersion"] = "rlinf.io/v1alpha1";
static kind: ICluster["kind"] = "Cluster";
static is = createTypeMetaGuard<ICluster>(Cluster);

constructor(data?: ModelData<ICluster>) {
  super();

  this.setDefinedProps({
    apiVersion: Cluster.apiVersion,
    kind: Cluster.kind,
    ...data
  } as ICluster);
}
}


setValidateFunc(Cluster, validate as ValidateFunc<ICluster>);
