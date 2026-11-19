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
    /**
     * DisplayName is a human-readable name; metadata.name remains the stable cluster ID.
     */
    "displayName"?: string;
    "drainPreviousCordoned"?: boolean;
    "draining"?: boolean;
    /**
     * NodeAgents declares the cluster's node Agents. The cluster Agent keeps them as one DaemonSet,
     * so a Kubernetes Node that joins and matches the selector gets its node Agent without further action.
     */
    "nodeAgents"?: {
      /**
       * Enabled makes the cluster Agent keep the node Agent DaemonSet; false makes it remove the DaemonSet.
       */
      "enabled": boolean;
      /**
       * Image is the node Agents' image. It is the node Agents' own: neither the cluster Agent's image nor the
       * image of the tools the cluster Agent gives Task Pods (its --image), so that each of the three is replaced
       * by itself. Required while Enabled; without it the cluster Agent leaves the DaemonSet as it is.
       */
      "image"?: string;
      /**
       * NodeSelector selects the Kubernetes Nodes that run a node Agent. It defaults to rlark.io/enabled=true,
       * the label that makes RLark manage a Kubernetes Node.
       */
      "nodeSelector"?: {
        [key: string]: string;
      };
      /**
       * Rollout is how the node Agents are replaced when their DaemonSet changes. Absent, the defaults of
       * RolloutPolicy: one node first, then a tenth of the ready nodes at a time.
       */
      "rollout"?: {
        /**
         * Canary is how many nodes get the new Pod first. Until that many new Pods
         * are available (ready for the DaemonSet's minReadySeconds), no other node
         * is touched, so a template that does not work stops there. Defaults to 1.
         */
        "canary"?: number;
        /**
         * MaxUnavailable bounds how many ready nodes may be without an available
         * Pod at once after the canary: a number, or a percentage of the ready
         * nodes rounded up. Defaults to 10%.
         */
        "maxUnavailable"?: number | string;
        /**
         * WhenIdle replaces a Pod only while its Node's rlark.io/in-use annotation
         * is reported and empty: for a workload that must not be interrupted while
         * its machine works, such as what runs on a robot.
         */
        "whenIdle"?: boolean;
      };
    };
  };
  /**
   * ClusterStatus reports the controller's current observation of a data-plane cluster.
   */
  "status"?: {
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
    "lastHeartbeatTime"?: string;
    "onlineNodes"?: number;
    /**
     * ClusterPhase summarizes the current lifecycle state of a data-plane cluster.
     * The set is closed in the CRD schema: adding a value requires updating the
     * enum marker, regenerating the CRD manifests, and updating the documentation.
     */
    "phase"?: "Online" | "Offline" | "Downgraded" | "Cordoned" | "Draining" | "Delisted";
    "totalNodes"?: number;
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
