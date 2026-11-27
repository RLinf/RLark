// Code generated from api/config/crd/bases. DO NOT EDIT.

import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData, setValidateFunc, createTypeMetaGuard } from "@kubernetes-models/base";
import { ValidateFunc } from "@kubernetes-models/validate";
import { validate } from "../../_schemas/RlinfIoV1alpha1ClusterProvision.js";

/**
 * ClusterProvision is the making of a cluster from a machine: the record the
 * console shows while the Gateway makes it and where it stopped. Its name is
 * the cluster's.
 */
export interface IClusterProvision {
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
  "kind": "ClusterProvision";
  "metadata"?: IObjectMeta;
  /**
   * ClusterProvisionSpec is what the cluster is made of: a new site, and its
   * first machine, which becomes the site's first hub and the cluster's control
   * plane. The machine's password is never part of it: it is given with the
   * request that starts or retries the making, and lives only while that runs.
   */
  "spec": {
    /**
     * Machine is how the Gateway reaches the first machine over SSH.
     */
    "machine": {
      /**
       * Address is the machine's address as the management plane reaches it.
       */
      "address": string;
      "port"?: number;
      /**
       * TunnelEndpoint is the IPv4 ip:port other sites dial for the machine's
       * tunnel, and the machine listens on its port: Address, port 51820, for a
       * machine with an address of its own; the address and port its cloud
       * forwards to it for machines behind one address.
       */
      "tunnelEndpoint": string;
      "user": string;
    };
    /**
     * RequestedBy is the user who asked for the cluster.
     */
    "requestedBy"?: string;
    /**
     * Site is the new site's name in the SiteNetwork.
     */
    "site": string;
  };
  /**
   * ClusterProvisionStatus is how the making went, stage by stage.
   */
  "status"?: {
    /**
     * ClusterProvisionFacts are what the stages found and chose; later stages,
     * and a retry, go on from them.
     */
    "facts"?: {
      "bundleSHA256"?: string;
      "cacheVolume"?: boolean;
      "controlPlaneName"?: string;
      "diskFree"?: string;
      "gpus"?: Array<{
        "count": number;
        "model": string;
      }>;
      "kernel"?: string;
      "nodeName"?: string;
      "os"?: string;
      "podSubnet"?: string;
      "privateAddress"?: string;
      "publicKey"?: string;
      /**
       * RegistryCA is the CA of the site registry the machine's installer trusts (PEM).
       */
      "registryCA"?: string;
      "release"?: string;
      "routes"?: Array<string>;
      "serviceSubnet"?: string;
      "tunnelAddress"?: string;
    };
    /**
     * HostKey is the SHA256 fingerprint the machine showed the first time;
     * every later connection must show the same.
     */
    "hostKey"?: string;
    /**
     * ClusterProvisionPhase is where the making of a cluster stands.
     */
    "phase"?: "Running" | "Succeeded" | "Failed";
    /**
     * Stage is the stage running, or the one the making stopped at.
     */
    "stage"?: string;
    "stages"?: Array<{
      "finishedAt"?: string;
      /**
       * Message is why the stage stopped, or the last line of its progress.
       */
      "message"?: string;
      "name": string;
      /**
       * Readback is what was read back once the stage had done its work.
       */
      "readback"?: string;
      "startedAt"?: string;
      /**
       * ClusterProvisionStageState is where one stage stands.
       */
      "state": "Pending" | "Running" | "Done" | "Skipped" | "Failed";
    }>;
  };
}

/**
 * ClusterProvision is the making of a cluster from a machine: the record the
 * console shows while the Gateway makes it and where it stopped. Its name is
 * the cluster's.
 */
export class ClusterProvision extends Model<IClusterProvision> implements IClusterProvision {
  "apiVersion": IClusterProvision["apiVersion"];
  "kind": IClusterProvision["kind"];
  "metadata"?: IClusterProvision["metadata"];
  "spec": IClusterProvision["spec"];
  "status"?: IClusterProvision["status"];

static apiVersion: IClusterProvision["apiVersion"] = "rlinf.io/v1alpha1";
static kind: IClusterProvision["kind"] = "ClusterProvision";
static is = createTypeMetaGuard<IClusterProvision>(ClusterProvision);

constructor(data?: ModelData<IClusterProvision>) {
  super();

  this.setDefinedProps({
    apiVersion: ClusterProvision.apiVersion,
    kind: ClusterProvision.kind,
    ...data
  } as IClusterProvision);
}
}


setValidateFunc(ClusterProvision, validate as ValidateFunc<IClusterProvision>);
