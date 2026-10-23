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
  "spec"?: {
    "agentType"?: "Kubernetes" | "Docker" | "Raw";
    "unschedulable"?: boolean;
  };
  "status"?: {
    /**
     * Addresses 是节点地址列表（InternalIP / Hostname 等）。
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
     * ResourceList is a set of (resource name, quantity) pairs.
     */
    "allocatable"?: {
      [key: string]: number | string;
    };
    /**
     * ResourceList is a set of (resource name, quantity) pairs.
     */
    "capacity"?: {
      [key: string]: number | string;
    };
    /**
     * Conditions 记录节点的结构化状态结论，当前仅包含 ConditionTypeReady。
     * 它由观测（而非 Spec）驱动，因此不填写 ObservedGeneration。
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
    "diskPressure"?: boolean;
    /**
     * Events 由数据面 node-agent 上报节点相关 Kubernetes Event（如 DiskPressure
     * 等 Warning 事件及镜像拉取/调度相关事件）。控制面 Task reconciler 在 Task
     * 处于 Pending 期间聚合各节点事件到 Task.status.events，供前端展示。
     * 条目数量由 node-agent 侧裁剪（最多保留 50 条）。
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
     * LastHeartbeatTime 是控制面 Node Reconciler 从 Agent Lease 读取到的最近一次
     * 心跳续约时间；数据面不写入本字段。
     */
    "lastHeartbeatTime"?: string;
    /**
     * LastStatusChangeTime 是 Phase 最近一次发生变化的时间，与 LastHeartbeatTime
     * 同由控制面 Node Reconciler 写入，用于区分「长时间保持同一状态」与「刚刚翻转」。
     */
    "lastStatusChangeTime"?: string;
    /**
     * NodeInfo 是节点基础信息，由数据面 Agent 上报；控制面不写入本字段。
     */
    "nodeInfo"?: {
      "agentVersion"?: string;
      "architecture"?: string;
      "kernelVersion"?: string;
      "operatingSystem"?: string;
    };
    /**
     * Phase 是节点连通性聚合阶段，由数据面 node-agent 上报。
     */
    "phase"?: "Online" | "Offline";
    /**
     * PullProgress 是节点上正在进行的镜像拉取进度，由 node-agent 上报。
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
     * Reason 是 Phase 的机器可读原因（CamelCase），与 Conditions 中
     * ConditionTypeReady 的 Reason 同源，供列表视图直接展示。
     * 该字段与 Conditions 均由数据面 node-agent 独占写入。
     */
    "reason"?: string;
    /**
     * NodeStorageStatus reports kubelet filesystem usage in bytes.
     */
    "storage"?: {
      "availableBytes"?: number;
      "capacityBytes"?: number;
      "usedBytes"?: number;
    };
    /**
     * ResourceList is a set of (resource name, quantity) pairs.
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
