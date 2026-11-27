import { useEffect, useRef, useState } from "react";
import { ApiError } from "../../api";
import { nodesApi } from "../../backend";
import type { CRDNode } from "../../types";
import { stationsApi } from "../api";
import {
  newRestartRequestID, outstandingRestart, readNodeRestart, restartPhaseOf, restartUIDMatches, restartWhy,
  type NodeRestartObservation, type NodeRestartProgress,
} from "../lib/nodeRestart";

// The request is a one-shot Node label, not a command: the Gateway writes it, the robot's agent syncs it down and the
// tuner acts on it once under its own guards. The console follows the Node's own records (rlark.io/restart-request-state
// and rlark.io/maintenance) with a bounded series of by-name reads, never a page refresh loop; when the outcome is
// terminal it asks the page to reload once.
export const RESTART_OBSERVE_INTERVAL_MS = 5000;
// A restart lowers the robot, waits about ten seconds, boots the host, and only then can the tuner record "done". The
// window is longer than a reboot path, not a claim about how long the machine takes.
export const RESTART_WINDOW_MS = 8 * 60 * 1000;
const LINGER_MS = 60_000;

export interface NodeRestartRequest extends NodeRestartProgress {
  key: string;
  node: string;
  namespace: string;
  startedAt: number;
  // The Node's records as the last by-name read showed them.
  observation?: NodeRestartObservation;
}

export interface NodeRestartTarget {
  key: string;
  node: string;
  namespace: string;
  // The UID the station record is bound to: the restart is not written to a same-named replacement machine.
  nodeUID: string;
}

export function useNodeRestart(zh: boolean, onChanged?: () => unknown) {
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [activeLabel, setActiveLabel] = useState("");
  const [failed, setFailed] = useState(false);
  const [requests, setRequests] = useState<Record<string, NodeRestartRequest>>({});
  const busy = useRef(false);
  const mounted = useRef(true);
  const timers = useRef<number[]>([]);
  // A key's request id: an older observation stops touching a key a newer request took over.
  const generations = useRef(new Map<string, string>());

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      timers.current.forEach((timer) => window.clearTimeout(timer));
      timers.current = [];
    };
  }, []);

  const schedule = (callback: () => void, delay: number) => {
    const timer = window.setTimeout(callback, delay);
    timers.current.push(timer);
  };

  const update = (key: string, requestID: string, patch: Partial<NodeRestartRequest>) => {
    if (!mounted.current || generations.current.get(key) !== requestID) return;
    setRequests((current) => (current[key] ? { ...current, [key]: { ...current[key], ...patch } } : current));
  };

  const linger = (key: string, requestID: string) => {
    schedule(() => {
      if (!mounted.current || generations.current.get(key) !== requestID) return;
      generations.current.delete(key);
      setRequests((current) => {
        if (!current[key]) return current;
        const next = { ...current };
        delete next[key];
        return next;
      });
    }, LINGER_MS);
  };

  const observe = (target: NodeRestartTarget, requestID: string) => {
    const deadline = performance.now() + RESTART_WINDOW_MS;
    const step = async () => {
      if (!mounted.current || generations.current.get(target.key) !== requestID) return;
      let observation: NodeRestartObservation | undefined;
      let observedUID: string | undefined;
      let readable = false;
      try {
        const node = await nodesApi.get(target.node, { namespace: target.namespace });
        readable = true;
        // The records only speak for the machine the station record is bound to: a same-named replacement is not
        // mistaken for the robot that was asked to reboot.
        observedUID = node.status?.kubernetesNodeUID;
        if (restartUIDMatches(target.nodeUID, observedUID)) observation = readNodeRestart(node.metadata?.labels, node.metadata?.annotations);
      } catch {
        // An unreadable Node is not a failed restart; the bounded series tries again.
      }
      if (!mounted.current || generations.current.get(target.key) !== requestID) return;
      if (readable && !restartUIDMatches(target.nodeUID, observedUID)) {
        update(target.key, requestID, {
          phase: "failed",
          error: zh
            ? `观察期间管理 Node 的 UID（${observedUID || "未上报"}）与记录绑定的 ${target.nodeUID} 不一致，这台机器可能已被替换；不再把它的记录当作原机器人。`
            : `The management Node's UID (${observedUID || "not reported"}) no longer matches the record's ${target.nodeUID}; the machine may have been replaced, so its records are no longer attributed to the original robot.`,
        });
        linger(target.key, requestID);
        return;
      }
      if (observation) {
        const phase = restartPhaseOf(observation, requestID);
        update(target.key, requestID, { observation });
        if (phase === "done") {
          update(target.key, requestID, { phase: "done", doneAt: observation.record?.done, waiting: undefined, error: undefined });
          linger(target.key, requestID);
          void onChanged?.();
          return;
        }
        if (phase === "failed") {
          update(target.key, requestID, { phase: "failed", error: restartWhy(observation, requestID) || (zh ? "这次重启失败" : "The restart failed") });
          linger(target.key, requestID);
          return;
        }
        if (observation.request && observation.request !== requestID) {
          update(target.key, requestID, {
            phase: "failed",
            error: zh
              ? `节点上的重启请求已变为 ${observation.request}，这次请求不再有结果`
              : `The Node's restart request changed to ${observation.request}; this request will not be carried out`,
          });
          linger(target.key, requestID);
          return;
        }
        update(target.key, requestID, {
          phase: phase === "acting" ? "acting" : phase === "waiting" ? "waiting" : "submitted",
          waiting: restartWhy(observation, requestID) || undefined,
        });
      }
      if (performance.now() >= deadline) {
        update(target.key, requestID, { phase: "unconfirmed" });
        linger(target.key, requestID);
        return;
      }
      schedule(() => void step(), RESTART_OBSERVE_INTERVAL_MS);
    };
    schedule(() => void step(), RESTART_OBSERVE_INTERVAL_MS);
  };

  const start = async (target: NodeRestartTarget) => {
    if (busy.current) return;
    busy.current = true;
    setPending(true);
    setActiveLabel(target.node);
    setMessage("");
    setFailed(false);
    const fail = (text: string) => {
      setFailed(true);
      setMessage(text);
    };
    try {
      let node: CRDNode;
      try {
        node = await nodesApi.get(target.node, { namespace: target.namespace });
      } catch (reason) {
        fail(zh
          ? `读不到管理 Node ${target.node}，无法确认这台机器：${(reason as Error).message}`
          : `The management Node ${target.node} cannot be read, so this machine cannot be confirmed: ${(reason as Error).message}`);
        return;
      }
      // Only the machine the station record is bound to may be restarted: a node of the same name that reports another
      // UID is a replacement, and an old label on it must not be used as identity.
      if (!restartUIDMatches(target.nodeUID, node.status?.kubernetesNodeUID)) {
        fail(zh
          ? `管理 Node 的 UID（${node.status?.kubernetesNodeUID || "未上报"}）与场站记录的 ${target.nodeUID || "（空）"} 不一致；这台机器可能已经更换，请到现场核对并重新入网，不能按旧身份重启。`
          : `The management Node's UID (${node.status?.kubernetesNodeUID || "not reported"}) does not match the station record's ${target.nodeUID || "(empty)"}; the machine may have been replaced. Check it on site and enroll it again instead of restarting under the old identity.`);
        return;
      }
      const observation = readNodeRestart(node.metadata?.labels, node.metadata?.annotations);
      const outstanding = outstandingRestart(observation);
      if (outstanding) {
        fail(zh
          ? `节点上已有未完成的重启请求 ${outstanding.request}（${outstanding.phase}）${outstanding.why ? `：${outstanding.why}` : ""}。等它结束后再提交新的请求，避免无人跟踪的排队重启。`
          : `The Node already carries an unfinished restart request ${outstanding.request} (${outstanding.phase})${outstanding.why ? `: ${outstanding.why}` : ""}. Wait for it to finish before asking again, so no untracked restart is queued.`);
        return;
      }
      const requestID = newRestartRequestID();
      // The read that passed the outstanding-request check pins the object: the merge patch carries its version, so a
      // second administrator who passed the same check cannot overwrite this request. A 409 is reported, not retried.
      const resourceVersion = node.metadata?.resourceVersion;
      generations.current.set(target.key, requestID);
      setRequests((current) => ({
        ...current,
        [target.key]: { key: target.key, node: target.node, namespace: target.namespace, requestID, phase: "sending", startedAt: performance.now(), observation },
      }));
      try {
        await stationsApi.requestRestart(target.node, target.namespace, requestID, resourceVersion);
      } catch (reason) {
        const error = reason instanceof ApiError && reason.status === 409
          ? (zh
            ? "节点在这次提交前已被其他请求修改（409）。请重新检测，确认没有别人正在重启这台机器后再提交；本次不会自动重试写入。"
            : "The Node changed before this submission (409). Re-check, confirm nobody else is restarting this machine, then submit again; the write is not retried automatically.")
          : (reason as Error).message;
        update(target.key, requestID, { phase: "failed", error });
        fail(`${target.node}: ${error}`);
        return;
      }
      if (!mounted.current) return;
      update(target.key, requestID, { phase: "submitted" });
      setMessage(zh
        ? `已写入整机重启请求 ${requestID}（${target.node}）。Tuner 的节点配置、占用与急停守卫仍然生效，可能排队或拒绝；卡片会显示等待、重启中或已重启。`
        : `The restart request ${requestID} is written for ${target.node}. The tuner's node-profile, occupancy and emergency-stop guards still apply, so it may queue or refuse; the card shows waiting, restarting, or restarted.`);
      await onChanged?.();
      observe(target, requestID);
    } finally {
      busy.current = false;
      if (mounted.current) setPending(false);
    }
  };

  const clear = () => {
    setMessage("");
    setFailed(false);
    setActiveLabel("");
  };

  // A refusal before the write (the robot could not be confirmed stopped, or the administrator cancelled) is reported
  // on the page's restart notice; nothing is written.
  const reject = (label: string, text: string) => {
    setActiveLabel(label);
    setFailed(true);
    setMessage(text);
  };

  return { pending, message, activeLabel, failed, requests, start, reject, clear };
}

export type NodeRestart = ReturnType<typeof useNodeRestart>;
