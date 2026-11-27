import { useEffect, useRef, useState } from "react";
import { stationsApi } from "../api";
import type { CRDNode } from "../../types";
import { robotPowerState, robotPowerWaiting, type RobotPowerAction, type RobotPowerDesired } from "../lib/siteNodeDisplay";

// The desired power of robots' Nodes, which the Addon's power service converges on: one request per robot, each
// recorded by the Gateway with who made it. onChanged reloads what the page shows once they are set. watch, when
// the page has one, is how the request is followed to the robot's own report: one bounded, slow series of reads,
// never a full page refresh.
export type RobotPowerRequestPhase = "sending" | "accepted" | "observed" | "unconfirmed" | "failed";

export interface RobotPowerRequest {
  key: string;
  label: string;
  action?: RobotPowerAction;
  desired: RobotPowerDesired;
  phase: RobotPowerRequestPhase;
  startedAt: number;
  // The power service's rlark.io/power waiting text, when it reports one.
  waiting?: string;
  error?: string;
}

export interface RobotPowerTarget {
  key: string;
  node: string;
  namespace: string;
  desired: RobotPowerDesired;
  action?: RobotPowerAction;
}

export type RobotPowerNodes = Partial<Record<string, CRDNode>>;
type RobotPowerObservationTarget = Pick<RobotPowerTarget, "key" | "node" | "namespace">;

export interface RobotPowerWatch {
  observe: (targets: RobotPowerObservationTarget[]) => Promise<RobotPowerNodes>;
}

// The power service reconciles every 5 s and publishes its state on the Node only when it changes, so reads follow
// that cadence rather than racing it. A power-off also holds power-on back for about 10 s, which the shorter driver
// window does not have to cover.
const OBSERVE_INTERVAL_MS = 5000;
const OBSERVED_LINGER_MS = 20000;
const ACTION_WINDOW_MS: Record<RobotPowerAction, number> = {
  driver_on: 30000,
  driver_off: 30000,
  power_on: 60000,
  power_off: 75000,
};
const DEFAULT_WINDOW_MS = 45000;

const windowFor = (targets: RobotPowerTarget[]) =>
  Math.max(...targets.map((target) => (target.action ? ACTION_WINDOW_MS[target.action] : DEFAULT_WINDOW_MS)));

export type RobotPowerBatchItem = { key: string; node: string; namespace: string; desired: RobotPowerDesired; action?: RobotPowerAction; status: "queued" | "running" | "succeeded" | "failed" | "cancelled"; error?: string };
export type RobotPowerBatchState = {
  id: number; action: string; total: number; succeeded: number; failed: number; running: number; queued: number;
  cancelled: number; items: RobotPowerBatchItem[];
};
type RobotPowerOptions = { concurrency?: number; onCoalescedRefresh?: () => Promise<RobotPowerNodes> };

export function useRobotPower(zh: boolean, onChanged?: () => unknown, watch?: RobotPowerWatch, options: RobotPowerOptions = {}) {
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [activeLabel, setActiveLabel] = useState("");
  const [failed, setFailed] = useState(false);
  const [requests, setRequests] = useState<Record<string, RobotPowerRequest>>({});
  const [batch, setBatch] = useState<RobotPowerBatchState | null>(null);
  const watchRef = useRef(watch);
  watchRef.current = watch;
  const timers = useRef<number[]>([]);
  // A request id per robot: a newer request makes an older observation stop touching that robot's state.
  const generations = useRef(new Map<string, number>());
  const sequence = useRef(0);
  const mounted = useRef(true);
  const batchSequence = useRef(0);
  const batchItems = useRef<RobotPowerBatchItem[]>([]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      timers.current.forEach((timer) => window.clearTimeout(timer));
      timers.current = [];
    };
  }, []);

  const updateRequest = (key: string, generation: number, patch: Partial<RobotPowerRequest>) => {
    if (!mounted.current || generations.current.get(key) !== generation) return;
    setRequests((current) => (current[key] ? { ...current, [key]: { ...current[key], ...patch } } : current));
  };

  const schedule = (callback: () => void, delay: number) => {
    const timer = window.setTimeout(callback, delay);
    timers.current.push(timer);
    return timer;
  };

  const linger = (key: string, generation: number) => {
    schedule(() => {
      if (!mounted.current || generations.current.get(key) !== generation) return;
      setRequests((current) => {
        if (!current[key]) return current;
        const next = { ...current };
        delete next[key];
        return next;
      });
    }, OBSERVED_LINGER_MS);
  };

  // Reads the robot's own report on the page's existing Node source until it matches what was requested, the window
  // closes, or a newer request supersedes this one. The button is not kept disabled meanwhile: a newer request simply
  // replaces the desired state.
  const observe = (generation: number, targets: RobotPowerTarget[]) => {
    const observer = watchRef.current?.observe;
    if (!observer || !targets.length) return;
    const deadline = performance.now() + windowFor(targets);
    const unresolved = new Set(targets.map((target) => target.key));
    const waiting = new Map<string, string>();
    const step = async () => {
      if (!mounted.current) return;
      const live = targets.filter((target) => unresolved.has(target.key) && generations.current.get(target.key) === generation);
      if (!live.length) return;
      let nodes: RobotPowerNodes = {};
      try {
        nodes = live.length > 3 && options.onCoalescedRefresh
          ? await options.onCoalescedRefresh()
          : await observer(live);
      } catch {
        // An unreadable sample is not a failed device action; the bounded series tries again.
      }
      if (!mounted.current) return;
      for (const target of live) {
        if (generations.current.get(target.key) !== generation) {
          unresolved.delete(target.key);
          continue;
        }
        const node = nodes[target.key];
        if (!node) continue;
        const waitingReason = robotPowerWaiting(node.metadata.annotations?.["rlark.io/power"]);
        if (waitingReason) waiting.set(target.key, waitingReason);
        if (robotPowerState(node.metadata.labels) === target.desired) {
          unresolved.delete(target.key);
          updateRequest(target.key, generation, { phase: "observed", waiting: undefined, error: undefined });
          linger(target.key, generation);
        } else {
          updateRequest(target.key, generation, { phase: "accepted", waiting: waitingReason || undefined });
        }
      }
      if (!unresolved.size) return;
      if (performance.now() >= deadline) {
        for (const target of live) {
          if (unresolved.has(target.key) && generations.current.get(target.key) === generation) {
            updateRequest(target.key, generation, { phase: "unconfirmed", waiting: waiting.get(target.key) || undefined });
          }
        }
        return;
      }
      schedule(step, OBSERVE_INTERVAL_MS);
    };
    schedule(step, OBSERVE_INTERVAL_MS);
  };

  const set = async (targets: RobotPowerTarget[], label: string) => {
    if (!targets.length || pending) return;
    const names = targets.map((entry) => entry.node).join(zh ? "、" : ", ");
    if (!window.confirm(zh ? `${label}：${names}\n\n请确认这些机器人周围安全、现场有人守着急停。` : `${label}: ${names}\n\nMake sure the robots' surroundings are safe and someone is at their emergency stops.`)) return;
    const generation = ++sequence.current;
    setPending(true);
    setActiveLabel(label);
    setMessage("");
    setFailed(false);
    // Each robot starts as the request that is being sent; a failure below settles its own entry.
    for (const target of targets) generations.current.set(target.key, generation);
    setRequests((current) => {
      const next = { ...current };
      for (const target of targets) {
        next[target.key] = {
          key: target.key,
          label,
          action: target.action,
          desired: target.desired,
          phase: "sending",
          startedAt: performance.now(),
        };
      }
      return next;
    });
    const id = ++batchSequence.current;
    batchItems.current = targets.map((target) => ({ key: target.key, node: target.node, namespace: target.namespace, desired: target.desired, action: target.action, status: "queued" }));
    const publishBatch = () => {
      if (!mounted.current) return;
      const items = batchItems.current.map((item) => ({ ...item }));
      setBatch({ id, action: label, total: items.length, succeeded: items.filter((item) => item.status === "succeeded").length,
        failed: items.filter((item) => item.status === "failed").length, running: items.filter((item) => item.status === "running").length,
        queued: items.filter((item) => item.status === "queued").length, cancelled: items.filter((item) => item.status === "cancelled").length, items });
    };
    const failures: string[] = [];
    const concurrency = Math.max(1, Math.floor(options.concurrency ?? 6));
    let next = 0;
    const worker = async () => {
      while (next < targets.length) {
        const index = next++;
        const target = targets[index];
        const item = batchItems.current[index];
        if (item.status !== "queued") continue;
        item.status = "running";
        publishBatch();
        try {
          await stationsApi.setRobotPower(target.node, target.namespace, target.desired);
          item.status = "succeeded";
          updateRequest(target.key, generation, { phase: "accepted" });
        } catch (reason) {
          const message = (reason as Error).message;
          item.status = "failed";
          item.error = message;
          failures.push(`${target.node}: ${message}`);
          updateRequest(target.key, generation, { phase: "failed", error: message });
        }
        publishBatch();
      }
    };
    await Promise.all(Array.from({ length: Math.min(concurrency, targets.length) }, () => worker()));
    if (!mounted.current) return;
    setFailed(failures.length > 0);
    setMessage(failures.length
      ? failures.join("; ")
      : watchRef.current?.observe
        ? (zh
          ? `已受理（${label}，${targets.length} 台）。卡片先显示「已受理」，观测到机器人真实状态后变为「已生效」；限时内没跟上的原因见节点注解 rlark.io/power 的 waiting。`
          : `Accepted (${label}, ${targets.length}). The cards show "Accepted" until the robots' own report confirms the state, then "Applied"; why one does not follow in time is in its Node's annotation rlark.io/power (waiting).`)
        : (zh
          ? `期望状态已改（${label}，${targets.length} 台），机器人几秒内跟上；没跟上的原因见节点注解 rlark.io/power 的 waiting。`
          : `Desired state set (${label}, ${targets.length}); the robots follow within seconds. Why one does not is in its Node's annotation rlark.io/power (waiting).`));
    setPending(false);
    const accepted = new Set(batchItems.current.filter((item) => item.status === "succeeded").map((item) => item.key));
    observe(generation, targets.filter((target) => accepted.has(target.key)));
    await onChanged?.();
  };
  const cancelBatch = () => {
    batchItems.current = batchItems.current.map((item) => item.status === "queued" ? { ...item, status: "cancelled" } : item);
    if (mounted.current) setBatch((current) => current ? { ...current, items: batchItems.current.map((item) => ({ ...item })), queued: 0, cancelled: batchItems.current.filter((item) => item.status === "cancelled").length } : current);
  };
  const retryFailed = async () => {
    const failedItems = batchItems.current.filter((item) => item.status === "failed");
    if (!failedItems.length) return;
    const targets = failedItems.flatMap((item) => {
      return [{ key: item.key, node: item.node, namespace: item.namespace, desired: item.desired, action: item.action }];
    });
    await set(targets, batch?.action ?? activeLabel);
  };
  const dismissBatch = () => setBatch(null);
  const clear = () => {
    setMessage("");
    setFailed(false);
    setActiveLabel("");
  };
  return { pending, message, activeLabel, failed, requests, set, clear, batch, cancelBatch, retryFailed, dismissBatch };
}

export type RobotPower = ReturnType<typeof useRobotPower>;
