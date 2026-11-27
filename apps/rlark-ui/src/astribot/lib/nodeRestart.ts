// A whole-machine restart is asked for with the Node label rlark.io/restart-request, whose value is the request's own
// id. The robot's node tuner runs it once per value, as one of its maintenance steps, and keeps its own guards: the
// node profile has to hold (a restart shows what the disk says), the robot must be free (no Run, no device session,
// no pressed emergency stop) and its wave must be up. What became of the request is on the Node annotation
// rlark.io/restart-request-state: the record's "profile" is the request id, its "was" the boot it was asked in, and
// "done" the time the host booted after it. The console only writes that label on the management Node and reads these
// records; it never writes the claim holder's rlark.io/robot-restart annotation.
import { formatRelativeTime } from "./stationDisplay.js";

export const RESTART_REQUEST_LABEL = "rlark.io/restart-request";
export const RESTART_STATE_ANNOTATION = "rlark.io/restart-request-state";
export const MAINTENANCE_ANNOTATION = "rlark.io/maintenance";
export const PROFILE_STATE_LABEL = "rlark.io/node-profile-state";
export const PROFILE_REPORT_ANNOTATION = "rlark.io/node-profile-report";

// submitted: the label is written and the tuner has not picked it up; waiting: the tuner will do it but is held by
// one of its guards (why says which); acting: the robot is being lowered or rebooted; done/failed are terminal.
export type NodeRestartPhase = "submitted" | "waiting" | "acting" | "done" | "failed";
// The console's own request adds sending (the label is being written) and unconfirmed (the observation window closed
// without a terminal record, which does not mean the robot will not do it).
export type NodeRestartRequestPhase = NodeRestartPhase | "sending" | "unconfirmed";

export interface NodeRestartRecord {
  kind?: string;
  profile?: string;
  attempts?: number;
  was?: string;
  at?: number;
  said?: string;
  done?: string;
}

export interface NodeRestartMaintenance {
  kind?: string;
  of?: string;
  state?: string;
  why?: string;
  since?: number;
}

export interface NodeRestartObservation {
  // The label's value: the request the Node asks for, whatever its id.
  request?: string;
  // rlark.io/restart-request-state, the last request record.
  record?: NodeRestartRecord;
  // rlark.io/maintenance, the maintenance the tuner is on now (of "request <id>" for a requested restart).
  maintenance?: NodeRestartMaintenance;
  // rlark.io/node-profile-state: ok, or what a restart would be refused for.
  profileState?: string;
  // rlark.io/node-profile-report, which carries the refusal of a request as "restart request <id> refused, ...".
  report?: string;
}

export interface NodeRestartOutstanding {
  request: string;
  phase: NodeRestartPhase;
  why: string;
}

export interface NodeRestartProgress {
  requestID: string;
  phase: NodeRestartRequestPhase;
  waiting?: string;
  error?: string;
  doneAt?: string;
}

function parseRecord<T>(value: string | undefined): T | undefined {
  if (!value) return undefined;
  try {
    const parsed = JSON.parse(value) as T;
    return parsed && typeof parsed === "object" ? parsed : undefined;
  } catch {
    return undefined;
  }
}

// The restart records of a Node as its GET returns them, from labels and annotations only.
export function readNodeRestart(labels?: Record<string, string>, annotations?: Record<string, string>): NodeRestartObservation {
  return {
    request: labels?.[RESTART_REQUEST_LABEL] || undefined,
    record: parseRecord<NodeRestartRecord>(annotations?.[RESTART_STATE_ANNOTATION]),
    maintenance: parseRecord<NodeRestartMaintenance>(annotations?.[MAINTENANCE_ANNOTATION]),
    profileState: labels?.[PROFILE_STATE_LABEL] ?? "",
    report: annotations?.[PROFILE_REPORT_ANNOTATION] ?? "",
  };
}

const refused = (observation: NodeRestartObservation, requestID: string) =>
  (observation.report ?? "").includes(`restart request ${requestID} refused`);

// Where a request stands, from the Node's records; "" when the records say nothing of it (not picked up yet, or stale).
export function restartPhaseOf(observation: NodeRestartObservation, requestID: string | undefined): NodeRestartPhase | "" {
  if (!requestID) return "";
  const record = observation.record;
  if (record?.profile === requestID) {
    if (record.done) return "done";
    const maintenance = observation.maintenance;
    if (maintenance?.of === `request ${requestID}` && maintenance.state === "failed") return "failed";
    return "acting";
  }
  const maintenance = observation.maintenance;
  if (maintenance?.of === `request ${requestID}`) {
    if (maintenance.state === "failed") return "failed";
    if (maintenance.state === "lowering" || maintenance.state === "acting") return "acting";
    if (maintenance.state === "waiting" || maintenance.state === "queued") return "waiting";
  }
  if (observation.request === requestID && refused(observation, requestID)) return "failed";
  return "";
}

// Why the tuner is holding or failed a request: its maintenance explanation, the attempt's last words, or the
// refusal it published against the node profile.
export function restartWhy(observation: NodeRestartObservation, requestID: string | undefined): string {
  if (!requestID) return "";
  if (observation.maintenance?.of === `request ${requestID}`) return observation.maintenance.why ?? "";
  if (observation.record?.profile === requestID && !observation.record.done) return observation.maintenance?.of === `request ${requestID}` ? (observation.maintenance.why ?? "") : (observation.record.said ?? "");
  const said = observation.report?.trim();
  return refused(observation, requestID) ? (said || "") : "";
}

// The request still to be seen through on this Node, or none. A label whose request is done or has failed is history
// (the tuner never repeats a value), so another request may be made with a new id; a label the records do not explain
// is left alone rather than overwritten.
export function outstandingRestart(observation: NodeRestartObservation): NodeRestartOutstanding | undefined {
  const request = observation.request;
  if (!request) return undefined;
  const phase = restartPhaseOf(observation, request);
  if (phase === "done" || phase === "failed") return undefined;
  return { request, phase: phase || "submitted", why: restartWhy(observation, request) };
}

export function restartPhaseLabel(phase: NodeRestartPhase, zh: boolean): string {
  return zh
    ? ({ submitted: "已提交", waiting: "等待中", acting: "重启中", done: "已重启", failed: "失败" } as const)[phase]
    : ({ submitted: "Submitted", waiting: "Waiting", acting: "Restarting", done: "Restarted", failed: "Failed" } as const)[phase];
}

// A new request id every time: the tuner does a label value once, so a second click must not reuse the first one's
// completed value. It is a Kubernetes label value: lowercase alphanumerics and dashes, well under 63 characters.
export function newRestartRequestID(now = Date.now(), random = Math.random): string {
  const stamp = Math.floor(now / 1000).toString(36);
  const suffix = Math.floor(random() * 0x1000000).toString(36).padStart(4, "0").slice(-4);
  return `ui-${stamp}-${suffix}`;
}

// The identity binding the request needs before it writes anything: the Node the name resolves to now must be the
// machine the station record is bound to (its UID reported by the agent), not a node of the same name that replaced it.
export function restartUIDMatches(recorded: string | undefined, observed: string | undefined): boolean {
  return Boolean(recorded) && recorded === observed;
}

// The long explanation of a request's state, for the tile's tooltip.
export function restartTooltip(progress: NodeRestartProgress, observation: NodeRestartObservation | undefined, zh: boolean): string {
  const why = observation ? restartWhy(observation, progress.requestID) : "";
  const whyLine = why ? (zh ? `Tuner：${why}` : `Tuner: ${why}`) : "";
  if (progress.phase === "sending") return zh ? "正在写入一次性重启请求标签" : "Writing the one-shot restart request label";
  if (progress.phase === "submitted") return [zh ? "已写入节点标签 rlark.io/restart-request，等待 robot tuner 接手。节点配置不成立时它会在 rlark.io/node-profile-report 里说明拒绝原因。" : "The Node label rlark.io/restart-request is written; the robot tuner has not picked it up yet. If the node profile does not hold it publishes the refusal in rlark.io/node-profile-report.", whyLine].filter(Boolean).join(" · ");
  if (progress.phase === "waiting") return [zh ? "Tuner 已排上这次重启，但被占用、轮次、急停或人员值守的条件挡住" : "The tuner has the restart scheduled but a guard (occupancy, wave, emergency stop, or attended power-off) holds it", whyLine].filter(Boolean).join(" · ");
  if (progress.phase === "acting") return [zh ? "Tuner 正在执行：停驱动、降下、下电并重启整机" : "The tuner is doing it: stopping the driver, lowering the robot, powering off and rebooting", whyLine].filter(Boolean).join(" · ");
  if (progress.phase === "done") return [zh ? `整机已重启，主机启动时间 ${formatRelativeTime(progress.doneAt, Date.now(), zh)}` : `The machine restarted; it booted ${formatRelativeTime(progress.doneAt, Date.now(), zh)}`, whyLine].filter(Boolean).join(" · ");
  if (progress.phase === "failed") return progress.error || whyLine || (zh ? "这次重启失败，原因见节点记录" : "The restart failed; see the Node's records");
  return [zh ? "观测窗口内没有看到完成或失败；请求标签仍在节点上，Tuner 仍可能按它执行。刷新详情查看最新记录。" : "No completion or failure was observed in the window; the request label is still on the Node and the tuner may still act on it. Refresh the details for the latest record.", whyLine].filter(Boolean).join(" · ");
}

// The tile's one-line state: a short chip whose tooltip carries the explanation.
export function restartProgressText(progress: NodeRestartProgress, zh: boolean, observation?: NodeRestartObservation, now = Date.now()): { text: string; title: string } {
  const title = restartTooltip(progress, observation, zh);
  const request = progress.requestID;
  if (progress.phase === "sending") return { text: zh ? "整机重启 · 请求中…" : "Restart · sending…", title };
  if (progress.phase === "submitted") return { text: zh ? `整机重启 · ${request} 已提交` : `Restart · ${request} submitted`, title };
  if (progress.phase === "waiting") return { text: [zh ? "整机重启 · 等待中" : "Restart · waiting", progress.waiting].filter(Boolean).join(" · "), title };
  if (progress.phase === "acting") return { text: zh ? "整机重启 · 正在重启" : "Restart · restarting", title };
  if (progress.phase === "done") return { text: [zh ? "整机重启 · 已重启" : "Restart · restarted", progress.doneAt ? formatRelativeTime(progress.doneAt, now, zh) : ""].filter(Boolean).join(" · "), title };
  if (progress.phase === "failed") return { text: zh ? "整机重启 · 失败" : "Restart · failed", title };
  return { text: zh ? "整机重启 · 未观测到结果" : "Restart · not observed", title };
}

// The line for a request that predates this page view (another administrator's, or one left from earlier): shown on
// the tile so nobody submits a second request over an untracked one.
export function outstandingRestartText(state: NodeRestartOutstanding, zh: boolean): { text: string; title: string } {
  const label = restartPhaseLabel(state.phase, zh);
  return {
    text: zh ? `整机重启 · ${state.request} ${label}` : `Restart · ${state.request} ${label}`,
    title: state.why || (zh ? "节点上已有这次重启请求；等它结束后再提交新的请求。" : "The Node already carries this restart request; wait for it to finish before asking again."),
  };
}
