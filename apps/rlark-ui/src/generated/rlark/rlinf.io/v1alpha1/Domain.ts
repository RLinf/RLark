// Code generated from api/config/crd/bases. DO NOT EDIT.

import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData, setValidateFunc, createTypeMetaGuard } from "@kubernetes-models/base";
import { ValidateFunc } from "@kubernetes-models/validate";
import { validate } from "../../_schemas/RlinfIoV1alpha1Domain.js";

export interface IDomain {
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
  "kind": "Domain";
  "metadata"?: IObjectMeta;
  "spec"?: {
    "cidr"?: string;
  };
  "status"?: {
    /**
     * Conditions 记录 Domain 的结构化状态结论，当前仅包含 ConditionTypeValidated
     * （Spec.CIDR 是否合法）。它由 Spec 驱动，写入方必须填写 ObservedGeneration。
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
     * IPAllocations 由 Domain 控制器独占写入，其它组件不得修改。该列表规模随
     * Domain CIDR 增长（/16 可达数万条）。
     */
    "ipAllocations"?: Array<{
      "ip"?: string;
      "job"?: string;
      "pod"?: string;
      "task"?: string;
    }>;
    /**
     * ObservedGeneration 是 Domain 控制器最近一次完成调谐时所观测到的
     * metadata.generation。它是控制器契约而非 API Server 保证：消费者在依据
     * ipAllocations 做判断前，应比较本字段与 metadata.generation 以识别过期状态。
     */
    "observedGeneration"?: number;
  };
}

export class Domain extends Model<IDomain> implements IDomain {
  "apiVersion": IDomain["apiVersion"];
  "kind": IDomain["kind"];
  "metadata"?: IDomain["metadata"];
  "spec"?: IDomain["spec"];
  "status"?: IDomain["status"];

static apiVersion: IDomain["apiVersion"] = "rlinf.io/v1alpha1";
static kind: IDomain["kind"] = "Domain";
static is = createTypeMetaGuard<IDomain>(Domain);

constructor(data?: ModelData<IDomain>) {
  super();

  this.setDefinedProps({
    apiVersion: Domain.apiVersion,
    kind: Domain.kind,
    ...data
  } as IDomain);
}
}


setValidateFunc(Domain, validate as ValidateFunc<IDomain>);
