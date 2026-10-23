// Code generated from api/config/crd/bases. DO NOT EDIT.

import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData, setValidateFunc, createTypeMetaGuard } from "@kubernetes-models/base";
import { ValidateFunc } from "@kubernetes-models/validate";
import { validate } from "../../_schemas/RlinfIoV1alpha1Pod.js";

export interface IPod {
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
  "kind": "Pod";
  "metadata"?: IObjectMeta;
  /**
   * PodSpec 包含 Pod 的标识和引用信息，由数据面上报时设置。
   */
  "spec"?: {
    "domain"?: string;
    "podName"?: string;
    "podNamespace"?: string;
    "taskName"?: string;
    "taskNamespace"?: string;
  };
  /**
   * PodStatus 包含 Pod 的运行状态信息（节点、IP、阶段等），
   * 对应 k8s 中由调度器和 kubelet 设置的观测状态。
   */
  "status"?: {
    "ip"?: string;
    "message"?: string;
    "node"?: string;
    "phase"?: "Pending" | "Running" | "Succeeded" | "Failed" | "Unknown";
  };
}

export class Pod extends Model<IPod> implements IPod {
  "apiVersion": IPod["apiVersion"];
  "kind": IPod["kind"];
  "metadata"?: IPod["metadata"];
  "spec"?: IPod["spec"];
  "status"?: IPod["status"];

static apiVersion: IPod["apiVersion"] = "rlinf.io/v1alpha1";
static kind: IPod["kind"] = "Pod";
static is = createTypeMetaGuard<IPod>(Pod);

constructor(data?: ModelData<IPod>) {
  super();

  this.setDefinedProps({
    apiVersion: Pod.apiVersion,
    kind: Pod.kind,
    ...data
  } as IPod);
}
}


setValidateFunc(Pod, validate as ValidateFunc<IPod>);
