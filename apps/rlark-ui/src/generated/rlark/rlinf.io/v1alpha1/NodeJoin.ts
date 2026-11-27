// Code generated from api/config/crd/bases. DO NOT EDIT.

import { IObjectMeta } from "@kubernetes-models/apimachinery/apis/meta/v1/ObjectMeta";
import { Model, ModelData, setValidateFunc, createTypeMetaGuard } from "@kubernetes-models/base";
import { ValidateFunc } from "@kubernetes-models/validate";
import { validate } from "../../_schemas/RlinfIoV1alpha1NodeJoin.js";

/**
 * NodeJoin is the joining of a machine to a cluster over SSH: the record the
 * console shows while the Gateway joins it and where it stopped. Its name is
 * <cluster>.<node>; label rlark.io/cluster holds the cluster.
 */
export interface INodeJoin {
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
  "kind": "NodeJoin";
  "metadata"?: IObjectMeta;
  /**
   * NodeJoinSpec is the machine the Gateway joins to a cluster over SSH. The
   * machine's password is never part of it: it is given with the request that
   * starts or retries the join, and lives only while that runs.
   */
  "spec": {
    "clusterID": string;
    /**
     * NodeJoinMachine is the SSH address and account of the machine.
     */
    "machine": {
      "address": string;
      "port"?: number;
      /**
       * TunnelEndpoint is the IPv4 ip:port other sites dial for the machine's
       * tunnel, when it joins through one: empty, the address at port 51820.
       */
      "tunnelEndpoint"?: string;
      "user": string;
    };
    /**
     * NodeName is the node's name: the one asked for, or the machine's host name.
     */
    "nodeName": string;
    /**
     * RequestedBy is the user who asked for the join.
     */
    "requestedBy"?: string;
  };
  /**
   * NodeJoinStatus is how the join went, stage by stage.
   */
  "status"?: {
    /**
     * NodeJoinFacts are what the stages found and chose; later stages, and a
     * retry, go on from them.
     */
    "facts"?: {
      /**
       * AdmissionIssuedAt is when the join's code was given out.
       */
      "admissionIssuedAt"?: string;
      /**
       * BundleSHA256 is the sha256 of the installer the machine holds.
       */
      "bundleSHA256"?: string;
      /**
       * DeclaredGeneration is the SiteNetwork's generation the declaration wrote.
       */
      "declaredGeneration"?: number;
      /**
       * DeclaredHub says this join declared the machine a hub in the SiteNetwork.
       */
      "declaredHub"?: boolean;
      "diskFree"?: string;
      "gpus"?: Array<{
        "count": number;
        "model": string;
      }>;
      "kernel"?: string;
      "os"?: string;
      "privateAddress"?: string;
      /**
       * PublicKey is the machine's tunnel key, made on the machine.
       */
      "publicKey"?: string;
      /**
       * NodeJoinRoute is how a machine reaches Station API for its code.
       */
      "route"?: "direct" | "tunnel" | "tunnel-up";
      /**
       * Timings are the moments of the join's steps, in order.
       */
      "timings"?: Array<{
        "at": string;
        "event": string;
      }>;
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
     * Stage is the stage running, or the one the join stopped at.
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
 * NodeJoin is the joining of a machine to a cluster over SSH: the record the
 * console shows while the Gateway joins it and where it stopped. Its name is
 * <cluster>.<node>; label rlark.io/cluster holds the cluster.
 */
export class NodeJoin extends Model<INodeJoin> implements INodeJoin {
  "apiVersion": INodeJoin["apiVersion"];
  "kind": INodeJoin["kind"];
  "metadata"?: INodeJoin["metadata"];
  "spec": INodeJoin["spec"];
  "status"?: INodeJoin["status"];

static apiVersion: INodeJoin["apiVersion"] = "rlinf.io/v1alpha1";
static kind: INodeJoin["kind"] = "NodeJoin";
static is = createTypeMetaGuard<INodeJoin>(NodeJoin);

constructor(data?: ModelData<INodeJoin>) {
  super();

  this.setDefinedProps({
    apiVersion: NodeJoin.apiVersion,
    kind: NodeJoin.kind,
    ...data
  } as INodeJoin);
}
}


setValidateFunc(NodeJoin, validate as ValidateFunc<INodeJoin>);
