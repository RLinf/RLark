// Code generated from api/config/crd/bases. DO NOT EDIT.

import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData, setValidateFunc, createTypeMetaGuard } from "@kubernetes-models/base";
import { ValidateFunc } from "@kubernetes-models/validate";
import { validate } from "../../_schemas/RlinfIoV1alpha1Node.js";

export interface INode {
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
  "kind": "Node";
  "metadata"?: IObjectMeta;
  /**
   * NodeSpec contains only management-owned durable desired state. Agents must not
   * write this section; administrative APIs own Unschedulable and Operations.
   */
  "spec"?: {
    /**
     * AgentType is a reserved compatibility field for the Node access form.
     *
     * Deprecated: use Runtime.Type for new integrations. This field must remain in
     * the API and generated CRD while non-Kubernetes runtime implementations are
     * incomplete, so existing resources and clients retain their declared intent.
     */
    "agentType"?: "Kubernetes" | "Docker" | "Raw";
    /**
     * Identity is set by the management API or control plane when the Node is registered.
     */
    "identity"?: {
      /**
       * Domain is the owning Domain identifier.
       */
      "domain"?: string;
      /**
       * ID is the stable RLark business identity of the Node.
       */
      "id"?: string;
      /**
       * Owner is the management principal or controller that owns the Node.
       */
      "owner"?: string;
      /**
       * Tenant is the owning tenant identifier, independent of Kubernetes namespace placement.
       */
      "tenant"?: string;
    };
    /**
     * Operations contains uniquely identified one-shot maintenance or drain requests.
     */
    "operations"?: Array<{
      /**
       * ID is a stable operation identifier supplied by the requester. It is
       * required and must be non-empty: an operation without an identity cannot be
       * tracked, reported, or retried idempotently, so it is rejected at the API
       * boundary rather than dropped by each consumer.
       */
      "id": string;
      /**
       * Type selects the requested one-shot operation.
       */
      "type": "Drain" | "EnterMaintenance";
    }>;
    /**
     * Runtime is the management-selected runtime reference for this Node.
     */
    "runtime"?: {
      /**
       * Name identifies the runtime instance or provider configuration.
       */
      "name"?: string;
      /**
       * Type identifies the configured runtime implementation.
       */
      "type"?: "Kubernetes" | "Docker" | "Raw";
    };
    /**
     * Unschedulable durably blocks placement of new workloads without stopping existing ones.
     */
    "unschedulable"?: boolean;
  };
  /**
   * NodeStatus separates Agent-observed facts from Node Controller-derived lifecycle
   * conclusions. Job and scheduler components consume this status read-only.
   *
   * NodeStatus has two writers, and each one writes a disjoint set of fields:
   *
   *   - The control-plane Node Controller owns Phase, Conditions, DrainOperations,
   *     LastHeartbeatTime and LastStatusChangeTime. They are its conclusions about
   *     the Agent heartbeat Lease and the facts reported by the data plane.
   *   - The data-plane node-agent owns NodeInfo, Addresses, DiskPressure, Storage,
   *     Capacity, Allocatable, Used, CapabilitySnapshot, ActiveWorkloads,
   *     ObservedNodeLabels, PullProgress and Events. They are Node-local observations.
   *
   * Both writers Patch the status with client.MergeFrom (no optimistic lock), so
   * concurrent writes merge deterministically and do not depend on write order.
   * That only holds while the ownership above is respected: a new field must be
   * assigned to exactly one writer and must never be written by both.
   */
  "status"?: {
    /**
     * ActiveWorkloads is the Agent-observed bounded summary of current workloads.
     */
    "activeWorkloads"?: {
      "pending"?: number;
      /**
       * RLarkManaged is the count of Pods on this Node that are managed by RLark
       * (identified by the RLark task-management annotations). Agent populates it
       * so that controllers can distinguish "Agent hasn't scanned the node yet"
       * from "the node genuinely has zero RLark-managed workloads", which matters
       * for deciding whether a Node Drain operation can complete.
       */
      "rlarkManaged"?: number;
      "running"?: number;
      "stopping"?: number;
      "total"?: number;
    };
    /**
     * Addresses are Agent-observed node addresses.
     */
    "addresses"?: Array<{
      /**
       * The node address.
       */
      "address": string;
      /**
       * Node address type, one of Hostname, ExternalIP or InternalIP.
       */
      "type": string;
    }>;
    /**
     * Allocatable is the Kubernetes Node's scheduler-declared capacity after system reservations.
     * Schedulers use this value, together with Used, for placement decisions.
     */
    "allocatable"?: {
      [key: string]: number | string;
    };
    /**
     * CapabilitySnapshot is the Agent-observed structured capability declaration.
     * Transient capability availability is reported by readiness Conditions.
     */
    "capabilitySnapshot"?: {
      "devices"?: Array<{
        /**
         * Available is allocatable capacity less than non-terminal Pod resource requests.
         */
        "available"?: number;
        /**
         * Category is the Kubernetes extended resource name, for example "nvidia.com/gpu".
         */
        "category": string;
        /**
         * Count is the capacity reported by the Kubernetes Node.
         */
        "count"?: number;
      }>;
      /**
       * NodeNetworkCapability describes Node-local networking features supported by the runtime.
       */
      "network"?: {
        "crossCluster"?: boolean;
        "hostNetwork"?: boolean;
        "serviceExposure"?: boolean;
      };
      /**
       * NodeOperationsCapability is reserved for operations whose support is observed locally.
       */
      "operations"?: {
        "exec"?: boolean;
        "logs"?: boolean;
        "terminal"?: boolean;
      };
      /**
       * NodeRuntimeCapability describes the runtime and workload kinds the Agent can run.
       */
      "runtime"?: {
        /**
         * Type identifies the runtime implementation.
         */
        "type"?: "Kubernetes" | "Docker" | "Raw";
        /**
         * Version is the runtime implementation version.
         */
        "version"?: string;
        /**
         * WorkloadKinds lists the workload kinds supported by this runtime.
         */
        "workloadKinds"?: Array<string>;
      };
      /**
       * NodeStorageCapability is reserved for storage support observed locally on the Node.
       */
      "storage"?: {
        "storageClasses"?: Array<string>;
        "volumeTypes"?: Array<string>;
      };
    };
    /**
     * Capacity is the Kubernetes Node's physical resource inventory. It is an informational upper bound,
     * not the schedulable capacity.
     */
    "capacity"?: {
      [key: string]: number | string;
    };
    /**
     * Conditions are calculated only by the Node Controller from desired state and Agent facts.
     * They are observation-driven and therefore carry no per-condition
     * ObservedGeneration; use the status-level ObservedGeneration to tell whether
     * they already reflect the latest Spec.
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
     * DiskPressure is the raw kubelet observation used by the Node Controller to derive ResourcePressure.
     */
    "diskPressure"?: boolean;
    /**
     * DrainOperations is the Node Controller's bounded summary of requested drain operations.
     */
    "drainOperations"?: Array<{
      "blocked"?: number;
      "completed"?: boolean;
      "operationID": string;
      "waiting"?: number;
    }>;
    /**
     * Events is a bounded node-agent-reported diagnostic event summary.
     */
    "events"?: Array<{
      "count"?: number;
      "lastTime"?: string;
      "message"?: string;
      "objectKind"?: string;
      "objectName"?: string;
      "reason": string;
      "source"?: string;
      "type": string;
    }>;
    /**
     * KubernetesCordoned is the raw observation of the local Kubernetes Node's
     * spec.unschedulable reported by the Agent. RLark does not manage that field:
     * a cordoned Kubernetes Node conceals placement from every Pod, not only RLark
     * workloads, so the Agent reports the observation and the Node Controller
     * derives the KubernetesCordoned condition from it without changing RLark's own
     * placement decision. Absence means the Agent has not reported the value.
     */
    "kubernetesCordoned"?: boolean;
    /**
     * KubernetesNodeUID is the Agent-observed UID of the local Kubernetes Node object.
     * It identifies one Node incarnation and is used to correlate Node Drain requests.
     */
    "kubernetesNodeUID"?: string;
    /**
     * KubernetesReady is the raw Kubernetes Node Ready observation reported by the Agent.
     */
    "kubernetesReady"?: boolean;
    /**
     * LastHeartbeatTime is the Agent heartbeat renewal the Node Controller read
     * from the coordination Lease. It is republished through the control-plane
     * clock so that it stays comparable across Nodes, and it only ever moves
     * forward. Agents must not write it.
     */
    "lastHeartbeatTime"?: string;
    /**
     * LastStatusChangeTime is the latest time an Agent-observed or controller-derived state changed.
     */
    "lastStatusChangeTime"?: string;
    /**
     * NodeInfo is Agent-reported operating system, architecture, Agent, and runtime version information.
     */
    "nodeInfo"?: {
      "agentVersion"?: string;
      "architecture"?: string;
      "kernelVersion"?: string;
      "operatingSystem"?: string;
      "runtimeVersion"?: string;
    };
    /**
     * ObservedNodeLabels is the Agent-observed label dictionary read from the
     * local Kubernetes Node object. It is the user-facing source of truth for
     * what labels are actually present on the data-plane Node. RLark currently
     * writes only RLark-owned system labels (rlark.io/\*) to metadata.labels on
     * the management Node CR; other labels found there are not the data-plane
     * label set. Frontends and operator consoles should display
     * ObservedNodeLabels, not metadata.labels.
     */
    "observedNodeLabels"?: {
      [key: string]: string;
    };
    /**
     * Phase is a concise lifecycle summary calculated only by the Node Controller.
     */
    "phase"?: "Registering" | "Online" | "Degraded" | "Offline" | "Cordoned" | "Draining";
    /**
     * PullProgress is node-agent-reported progress for in-flight image pulls.
     */
    "pullProgress"?: Array<{
      "downloaded": number;
      "image": string;
      "message"?: string;
      "speed": number;
      "status": string;
      "total": number;
    }>;
    /**
     * Storage is a bounded monitoring snapshot and must not be used for placement or capacity decisions.
     */
    "storage"?: {
      "availableBytes"?: number;
      "capacityBytes"?: number;
      "usedBytes"?: number;
    };
    /**
     * Used is the sum of non-terminal Pod resource requests already allocated for scheduling.
     * It is not actual runtime utilization.
     */
    "used"?: {
      [key: string]: number | string;
    };
  };
}

export class Node extends Model<INode> implements INode {
  "apiVersion": INode["apiVersion"];
  "kind": INode["kind"];
  "metadata"?: INode["metadata"];
  "spec"?: INode["spec"];
  "status"?: INode["status"];

static apiVersion: INode["apiVersion"] = "rlinf.io/v1alpha1";
static kind: INode["kind"] = "Node";
static is = createTypeMetaGuard<INode>(Node);

constructor(data?: ModelData<INode>) {
  super();

  this.setDefinedProps({
    apiVersion: Node.apiVersion,
    kind: Node.kind,
    ...data
  } as INode);
}
}


setValidateFunc(Node, validate as ValidateFunc<INode>);
