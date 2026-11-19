// Code generated from api/config/crd/bases. DO NOT EDIT.

import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData, setValidateFunc, createTypeMetaGuard } from "@kubernetes-models/base";
import { ValidateFunc } from "@kubernetes-models/validate";
import { validate } from "../../_schemas/RlinfIoV1alpha1Addon.js";

export interface IAddon {
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
  "kind": "Addon";
  "metadata"?: IObjectMeta;
  "spec"?: {
    "addonName": string;
    "values"?: {
      [key: string]: string;
    };
    "version"?: string;
  };
  "status"?: {
    /**
     * Conditions 记录 Addon 的结构化状态结论，当前仅包含 ConditionTypeReady。
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
    "message"?: string;
    /**
     * ObservedGeneration 是 Addon 控制器最近一次完成调谐时所观测到的
     * metadata.generation。它是控制器契约而非 API Server 保证：消费者在读取
     * phase / conditions 之前，应比较本字段与 metadata.generation 以识别过期状态。
     */
    "observedGeneration"?: number;
    /**
     * AddonPhase 是 Addon 安装与升级流程的聚合阶段，由数据面 Agent 观测后上报。
     * 取值集合在 CRD Schema 层封闭，新增取值必须同步更新枚举校验与文档。
     */
    "phase"?: "Pending" | "Installing" | "Ready" | "Failed" | "Upgrading";
    "version"?: string;
  };
}

export class Addon extends Model<IAddon> implements IAddon {
  "apiVersion": IAddon["apiVersion"];
  "kind": IAddon["kind"];
  "metadata"?: IAddon["metadata"];
  "spec"?: IAddon["spec"];
  "status"?: IAddon["status"];

static apiVersion: IAddon["apiVersion"] = "rlinf.io/v1alpha1";
static kind: IAddon["kind"] = "Addon";
static is = createTypeMetaGuard<IAddon>(Addon);

constructor(data?: ModelData<IAddon>) {
  super();

  this.setDefinedProps({
    apiVersion: Addon.apiVersion,
    kind: Addon.kind,
    ...data
  } as IAddon);
}
}


setValidateFunc(Addon, validate as ValidateFunc<IAddon>);
