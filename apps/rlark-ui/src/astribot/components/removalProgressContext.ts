import { createContext, useContext } from "react";
import type { NodeOperation } from "../api.js";

export interface RemovalEntry {
  clusterID: string;
  nodeName: string;
  nodeUID: string;
  attempt: number;
  startedAt: number;
  operation?: NodeOperation;
  operationName?: string;
  error?: string;
}

export interface RemovalNotification {
  key: string;
  outcome: "failed" | "succeeded";
  message: string;
}

export interface RemovalProgressValue {
  entries: RemovalEntry[];
  entryFor: (clusterID: string, nodeName: string) => RemovalEntry | undefined;
  registerClusters: (ids: string[]) => void;
  registerNodes: (clusterID: string, nodes: Array<{ name: string; uid?: string; creationTimestamp?: string }>) => void;
  start: (input: { clusterID: string; nodeName: string; nodeUID: string }) => Promise<void>;
  retry: (clusterID: string, nodeName: string) => Promise<void>;
  openDrawer: (clusterID: string, nodeName: string) => void;
  closeDrawer: () => void;
  dismiss: (clusterID: string, nodeName: string) => void;
  completionRevision: number;
  notifications: RemovalNotification[];
  drawerKey?: string;
  zh: boolean;
}

const noop = () => undefined;

export const removalProgressDefault: RemovalProgressValue = {
  entries: [],
  entryFor: () => undefined,
  registerClusters: noop,
  registerNodes: noop,
  start: async () => undefined,
  retry: async () => undefined,
  openDrawer: noop,
  closeDrawer: noop,
  dismiss: noop,
  completionRevision: 0,
  notifications: [],
  zh: true,
};

export const RemovalProgressContext = createContext<RemovalProgressValue>(removalProgressDefault);

export function useRemovalProgress(): RemovalProgressValue {
  return useContext(RemovalProgressContext);
}
