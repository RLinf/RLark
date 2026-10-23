// Code generated from api/config/crd/bases. DO NOT EDIT.

import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData, setValidateFunc, createTypeMetaGuard } from "@kubernetes-models/base";
import { ValidateFunc } from "@kubernetes-models/validate";
import { validate } from "../../_schemas/RlinfIoV1alpha1DomainPeer.js";

export interface IDomainPeer {
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
  "kind": "DomainPeer";
  "metadata"?: IObjectMeta;
  "spec"?: {
    "cert"?: string;
    "key"?: string;
    "pods"?: Array<{
      "globalNamespace"?: string;
      "ip"?: string;
      "localIP"?: string;
      "name"?: string;
      "namespace"?: string;
      "node"?: string;
      "uid"?: string;
    }>;
    "prefixLen"?: number;
  };
  "status"?: {
  };
}

export class DomainPeer extends Model<IDomainPeer> implements IDomainPeer {
  "apiVersion": IDomainPeer["apiVersion"];
  "kind": IDomainPeer["kind"];
  "metadata"?: IDomainPeer["metadata"];
  "spec"?: IDomainPeer["spec"];
  "status"?: IDomainPeer["status"];

static apiVersion: IDomainPeer["apiVersion"] = "rlinf.io/v1alpha1";
static kind: IDomainPeer["kind"] = "DomainPeer";
static is = createTypeMetaGuard<IDomainPeer>(DomainPeer);

constructor(data?: ModelData<IDomainPeer>) {
  super();

  this.setDefinedProps({
    apiVersion: DomainPeer.apiVersion,
    kind: DomainPeer.kind,
    ...data
  } as IDomainPeer);
}
}


setValidateFunc(DomainPeer, validate as ValidateFunc<IDomainPeer>);
