import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { CheckCircle2, Circle, ChevronDown, ChevronUp, CircleX, LoaderCircle, X } from "lucide-react";
import { selfHealApi, stationsApi, type NodeOperation } from "../api";
import {
  latestRemovalOperation,
  removalAppliesToNode,
  removalFailureMessage,
  removalIsActive,
  removalLooksStuck,
  removalNoProgressMs,
  removalNextPollDelayMs,
  removalOperationKey,
  removalOutcome,
  removalRequestID,
  removalStageLabel,
  removalStageProgress,
  removalStages,
  removalStagesDone,
  parseStoredRemovals,
  type StoredRemoval,
} from "../lib/nodeRemoval";
import { RemovalOperationDrawer } from "./RemovalOperationDrawer";
import { RemovalProgressContext, type RemovalEntry, type RemovalNotification } from "./removalProgressContext";
import "./RemovalProgress.css";

const STORAGE_KEY = "rlark-node-removals-v1";

function readStored(): StoredRemoval[] {
  try { return parseStoredRemovals(sessionStorage.getItem(STORAGE_KEY)); } catch { return []; }
}

function elapsed(startedAt: number): string {
  const seconds = Math.max(0, Math.floor((Date.now() - startedAt) / 1000));
  return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m${String(seconds % 60).padStart(2, "0")}s`;
}

function durationLabel(milliseconds: number, zh: boolean): string {
  const totalMinutes = Math.floor(milliseconds / 60_000);
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (zh) return hours ? `${hours} 小时 ${minutes} 分钟` : `${totalMinutes} 分钟`;
  return hours ? `${hours}h ${minutes}m` : `${totalMinutes}m`;
}

// Owns node-removal operations for the authenticated administrator shell.
export function RemovalProgressProvider({ children, zh, compact = false }: { children: ReactNode; zh: boolean; compact?: boolean }) {
  const [entries, setEntries] = useState<RemovalEntry[]>(() => readStored().map((entry) => ({ ...entry, nodeUID: "" })));
  const entriesRef = useRef(entries);
  const [clusterIDs, setClusterIDs] = useState<string[]>(() => [...new Set(readStored().map((entry) => entry.clusterID))]);
  const nodeInstances = useRef<Record<string, Record<string, { uid?: string; creationTimestamp?: string }>>>({});
  const [nodeRevision, setNodeRevision] = useState(0);
  const [drawerKey, setDrawerKey] = useState<string>();
  const [collapsed, setCollapsed] = useState(compact);
  // The task card follows the administrator across admin pages. Away from
  // the node list it starts collapsed so it does not cover a form or a
  // drawer's fixed footer; the header still shows the stage and a stall.
  useEffect(() => {
    if (compact) setCollapsed(true);
  }, [compact]);
  const [completionRevision, setCompletionRevision] = useState(0);
  const [pollRevision, setPollRevision] = useState(0);
  const [notifications, setNotifications] = useState<RemovalNotification[]>([]);
  const controllers = useRef(new Set<AbortController>());
  const failedPolls = useRef<Record<string, number>>({});
  const finished = useRef(new Set<string>());

  const save = useCallback((next: RemovalEntry[]) => {
    entriesRef.current = next;
    setEntries(next);
    try {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify(next.map(({ clusterID, nodeName, operation, operationName, attempt, startedAt }) => ({
        clusterID, nodeName, operationName: operation?.metadata.name ?? operationName ?? "", attempt, startedAt,
      })).slice(-10)));
    } catch { /* Storage can be unavailable in private browsing. */ }
  }, []);

  const mergeOperation = useCallback((clusterID: string, operation: NodeOperation) => {
    if (operation.spec.action !== "Remove") return;
    const nodeName = operation.spec.nodeName;
    const key = removalOperationKey(clusterID, nodeName);
    const node = nodeInstances.current[clusterID]?.[nodeName];
    if (!removalAppliesToNode(operation, node?.uid, node?.creationTimestamp)) {
      save(entriesRef.current.filter((entry) => removalOperationKey(entry.clusterID, entry.nodeName) !== key));
      return;
    }
    const outcome = removalOutcome(operation);
    const operationKey = `${key}:${operation.metadata.name}`;
    const current = entriesRef.current;
    const existing = current.find((entry) => removalOperationKey(entry.clusterID, entry.nodeName) === key);
    // Notify on a transition this session saw, or on a terminal operation
    // created just now. A historical Blocked operation loaded from the list
    // must not raise a toast every time the page opens.
    const transitioned = !!existing?.operation && removalIsActive(existing.operation);
    const createdAt = Date.parse(operation.metadata.creationTimestamp ?? "");
    const justCreated = Number.isFinite(createdAt) && Date.now() - createdAt < 60_000;
    const notify = transitioned || justCreated;
    if (outcome === "succeeded" && notify && !finished.current.has(operationKey)) {
      finished.current.add(operationKey);
      setCompletionRevision((revision) => revision + 1);
      setNotifications((current) => [...current.filter((item) => item.key !== key), { key, outcome, message: zh ? `${nodeName} 已删除` : `${nodeName} was removed` }]);
      window.setTimeout(() => setNotifications((current) => current.filter((item) => item.key !== key)), 6000);
    }
    if (outcome === "failed" && notify && !finished.current.has(operationKey)) {
      finished.current.add(operationKey);
      setNotifications((current) => [...current.filter((item) => item.key !== key), { key, outcome, message: removalFailureMessage(operation) }]);
    }
    const nextEntry: RemovalEntry = {
      clusterID, nodeName, nodeUID: operation.spec.nodeUID ?? existing?.nodeUID ?? "",
      attempt: existing?.attempt ?? 0, startedAt: existing?.startedAt ?? (Date.parse(operation.metadata.creationTimestamp ?? "") || Date.now()), operation, operationName: operation.metadata.name,
    };
    save([...current.filter((entry) => removalOperationKey(entry.clusterID, entry.nodeName) !== key), nextEntry]);
  }, [save, zh]);

  useEffect(() => {
    const stored = readStored();
    if (!stored.length) return;
    save(stored.map((item) => ({ ...item, nodeUID: "" })));
  // Hydrate once when the provider mounts.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const registerClusters = useCallback((ids: string[]) => {
    setClusterIDs((current) => [...new Set([...current, ...ids.filter(Boolean)])]);
  }, []);

  const registerNodes = useCallback((clusterID: string, nodes: Array<{ name: string; uid?: string; creationTimestamp?: string }>) => {
    const next = Object.fromEntries(nodes.map((node) => [node.name, { uid: node.uid, creationTimestamp: node.creationTimestamp }]));
    const previous = nodeInstances.current[clusterID] ?? {};
    const previousNames = Object.keys(previous);
    const nextNames = Object.keys(next);
    const unchanged = previousNames.length === nextNames.length && nextNames.every((name) => previous[name]?.uid === next[name]?.uid && previous[name]?.creationTimestamp === next[name]?.creationTimestamp);
    if (unchanged) return;
    nodeInstances.current = { ...nodeInstances.current, [clusterID]: next };
    setNodeRevision((revision) => revision + 1);
  }, []);

  const entryFor = useCallback((clusterID: string, nodeName: string) => entries.find((entry) => entry.clusterID === clusterID && entry.nodeName === nodeName), [entries]);

  const submit = useCallback(async (input: { clusterID: string; nodeName: string; nodeUID: string; attempt: number; startedAt: number }) => {
    const { clusterID, nodeName, nodeUID, attempt, startedAt } = input;
    const requestID = removalRequestID(clusterID, nodeName, nodeUID, attempt);
    const key = removalOperationKey(clusterID, nodeName);
    setPollRevision((revision) => revision + 1);
    setNotifications((current) => current.filter((item) => item.key !== key));
    const provisional: RemovalEntry = { clusterID, nodeName, nodeUID, attempt, startedAt };
    finished.current.delete(`${key}:${entriesRef.current.find((entry) => removalOperationKey(entry.clusterID, entry.nodeName) === key)?.operation?.metadata.name ?? ""}`);
    save([...entriesRef.current.filter((entry) => removalOperationKey(entry.clusterID, entry.nodeName) !== key), provisional]);
    try {
      const operation = await selfHealApi.removeNode(clusterID, nodeName, requestID, nodeUID || undefined);
      mergeOperation(clusterID, operation);
    } catch (reason) {
      const message = (reason as Error).message;
      save([...entriesRef.current.filter((entry) => removalOperationKey(entry.clusterID, entry.nodeName) !== key), { ...provisional, error: message }]);
      // Keep the same attempt and request ID. The next list poll resolves an uncertain POST.
      throw reason;
    }
  }, [mergeOperation, save]);

  const start = useCallback(async (input: { clusterID: string; nodeName: string; nodeUID: string }) => {
    const existing = entryFor(input.clusterID, input.nodeName);
    if (existing?.operation && removalIsActive(existing.operation)) {
      setDrawerKey(removalOperationKey(input.clusterID, input.nodeName));
      return;
    }
    const attempt = existing?.operation && (existing.operation.status?.phase === "Failed" || existing.operation.status?.phase === "Blocked") ? existing.attempt + 1 : existing?.attempt ?? 0;
    await submit({ ...input, attempt, startedAt: Date.now() });
  }, [entryFor, submit]);

  const retry = useCallback(async (clusterID: string, nodeName: string) => {
    const entry = entryFor(clusterID, nodeName);
    if (!entry) return;
    const terminalFailure = entry.operation?.status?.phase === "Failed" || entry.operation?.status?.phase === "Blocked";
    await submit({ clusterID, nodeName, nodeUID: entry.nodeUID || entry.operation?.spec.nodeUID || "", attempt: terminalFailure ? entry.attempt + 1 : entry.attempt, startedAt: Date.now() });
  }, [entryFor, submit]);

  const dismiss = useCallback((clusterID: string, nodeName: string) => {
    const key = removalOperationKey(clusterID, nodeName);
    save(entriesRef.current.filter((entry) => removalOperationKey(entry.clusterID, entry.nodeName) !== key));
    setNotifications((current) => current.filter((item) => item.key !== key));
  }, [save]);

  useEffect(() => {
    const controller = new AbortController();
    const activeControllers = controllers.current;
    activeControllers.add(controller);
    const timers = new Map<string, number>();
    const poll = async (clusterID: string) => {
      if (controller.signal.aborted || document.hidden) {
        if (!controller.signal.aborted) timers.set(clusterID, window.setTimeout(() => void poll(clusterID), 1000));
        return;
      }
      try {
        const { items } = await stationsApi.listNodeOperations(clusterID, controller.signal);
        failedPolls.current[clusterID] = 0;
        const relevant = items.filter((item) => item.spec.action === "Remove");
        for (const name of new Set(relevant.map((item) => item.spec.nodeName))) {
          const latest = latestRemovalOperation(relevant, name);
          if (latest) mergeOperation(clusterID, latest);
        }
        const returnedNames = new Set(relevant.map((item) => item.metadata.name));
        const current = entriesRef.current;
        const unresolved = current.filter((entry) => entry.clusterID === clusterID && !entry.operation && entry.operationName && !returnedNames.has(entry.operationName));
        if (unresolved.length) save(current.filter((entry) => !unresolved.includes(entry)));
      } catch {
        if (!controller.signal.aborted) failedPolls.current[clusterID] = (failedPolls.current[clusterID] ?? 0) + 1;
      }
      if (!controller.signal.aborted && (failedPolls.current[clusterID] || entriesRef.current.some((entry) => entry.clusterID === clusterID && (!entry.operation || removalIsActive(entry.operation))))) {
        const delay = failedPolls.current[clusterID] ? removalNextPollDelayMs(failedPolls.current[clusterID]) : 3000;
        timers.set(clusterID, window.setTimeout(() => void poll(clusterID), delay));
      }
    };
    clusterIDs.forEach((clusterID) => void poll(clusterID));
    return () => {
      controller.abort();
      activeControllers.delete(controller);
      timers.forEach((timer) => window.clearTimeout(timer));
    };
  }, [clusterIDs, mergeOperation, nodeRevision, pollRevision, save]);

  useEffect(() => () => controllers.current.forEach((controller) => controller.abort()), []);

  const activeEntries = useMemo(() => entries.filter((entry) => !entry.operation || removalIsActive(entry.operation)), [entries]);
  const value = useMemo(() => ({ entries, entryFor, registerClusters, registerNodes, start, retry, openDrawer: (clusterID: string, nodeName: string) => setDrawerKey(removalOperationKey(clusterID, nodeName)), closeDrawer: () => setDrawerKey(undefined), dismiss, completionRevision, notifications, drawerKey, zh }), [entries, entryFor, registerClusters, registerNodes, start, retry, dismiss, completionRevision, notifications, drawerKey, zh]);
  const visibleNotification = notifications[notifications.length - 1];
  const drawerEntry = drawerKey ? entries.find((entry) => removalOperationKey(entry.clusterID, entry.nodeName) === drawerKey) : undefined;
  const primary = activeEntries[0] ?? entries.find((entry) => entry.operation?.status?.phase === "Failed" || entry.operation?.status?.phase === "Blocked");
  const currentStage = primary?.operation?.status?.stage;
  const primaryOutcome = primary?.operation ? removalOutcome(primary.operation) : "active";
  const primaryStuck = !!primary?.operation && removalLooksStuck(primary.operation);
  const noProgressMs = primary?.operation ? removalNoProgressMs(primary.operation) : undefined;

  return <RemovalProgressContext.Provider value={value}>
    {children}
    {primary && <section className={`removal-task-card${collapsed ? " is-collapsed" : ""}`} aria-label={zh ? "节点删除任务" : "Node removal task"}>
      <header><span className="removal-task-icon">{primaryOutcome === "succeeded" ? <CheckCircle2 size={17} /> : primaryOutcome === "failed" ? <CircleX size={17} /> : <LoaderCircle size={17} />}</span><div><strong>{primaryOutcome === "succeeded" ? (zh ? `${primary.nodeName} 已删除` : `${primary.nodeName} was removed`) : primaryOutcome === "failed" ? (zh ? `${primary.nodeName} 删除失败` : `${primary.nodeName} removal failed`) : (zh ? `正在删除 ${primary.nodeName}` : `Removing ${primary.nodeName}`)}</strong><small>{removalStageLabel(currentStage ?? "Preflight", zh)} · {zh ? "已用" : "Elapsed"} {elapsed(primary.startedAt)}</small>{primaryStuck && primary.operation && noProgressMs !== undefined && <small className="removal-stuck-inline">{zh ? `长时间无进展 · 已用 ${durationLabel(noProgressMs, true)} · 最后完成阶段：${removalStageLabel(primary.operation.status?.stage ?? "Preflight", true)}` : `No progress for ${durationLabel(noProgressMs, false)} · Last completed stage: ${removalStageLabel(primary.operation.status?.stage ?? "Preflight", false)}`}</small>}</div><button className="icon-button" type="button" aria-label={collapsed ? (zh ? "展开" : "Expand") : (zh ? "收起" : "Collapse")} onClick={() => setCollapsed(!collapsed)}>{collapsed ? <ChevronDown size={16} /> : <ChevronUp size={16} />}</button></header>
      {!collapsed && <><div className="removal-task-progress"><i style={{ width: `${removalStageProgress(currentStage)}%` }} /></div><ol>{removalStages.map((stage, index) => <li className={index < removalStagesDone(currentStage) ? "is-done" : ""} key={stage}>{index < removalStagesDone(currentStage) ? <CheckCircle2 size={13} /> : <Circle size={13} />}{removalStageLabel(stage, zh)}</li>)}</ol>{primaryStuck && <div className="removal-stuck-callout" role="status">{zh ? "成员集群或机器可能不可达；操作仍在服务端等待。" : "The member cluster or machine may be unreachable. The operation is still waiting on the server."}</div>}<footer><span>{zh ? `阶段 ${Math.min(removalStagesDone(currentStage) + 1, removalStages.length)}/7` : `Stage ${Math.min(removalStagesDone(currentStage) + 1, removalStages.length)}/7`}</span><button className="secondary-button" type="button" onClick={() => setDrawerKey(removalOperationKey(primary.clusterID, primary.nodeName))}>{zh ? "查看详情" : "View details"}</button></footer></>}
    </section>}
    {visibleNotification && <div className={`removal-toast ${visibleNotification.outcome}${primary ? " has-task-card" : ""}`} role="alert"><span>{visibleNotification.message}</span><button className="secondary-button" type="button" onClick={() => { const entry = entries.find((item) => removalOperationKey(item.clusterID, item.nodeName) === visibleNotification.key); if (entry) setDrawerKey(visibleNotification.key); }}>{zh ? "查看" : "View"}</button>{visibleNotification.outcome === "failed" && <button className="secondary-button" type="button" onClick={() => { const entry = entries.find((item) => removalOperationKey(item.clusterID, item.nodeName) === visibleNotification.key); if (entry) void retry(entry.clusterID, entry.nodeName); }}>{zh ? "重试" : "Retry"}</button>}<button className="icon-button" aria-label={zh ? "关闭提示" : "Dismiss notification"} type="button" onClick={() => setNotifications((current) => current.filter((item) => item.key !== visibleNotification.key))}><X size={14} /></button></div>}
    {drawerEntry && <RemovalOperationDrawer entry={drawerEntry} onClose={() => setDrawerKey(undefined)} onRetry={() => void retry(drawerEntry.clusterID, drawerEntry.nodeName)} onDismiss={() => dismiss(drawerEntry.clusterID, drawerEntry.nodeName)} zh={zh} />}
  </RemovalProgressContext.Provider>;
}
