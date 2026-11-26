import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import {
  Check,
  ChevronLeft,
  ChevronDown,
  KeyRound,
  ListChecks,
  Plus,
  RefreshCw,
  Search,
  Server,
  ShieldCheck,
  Terminal,
  TerminalSquare,
} from "lucide-react";
import { ApiError } from "../../api";
import { clustersApi } from "../../backend";
import { machinesApi, nodeAdmissionsApi, nodeJoinsApi, robotModelsApi, siteNetworkApi, type RobotModelItem, type SiteNetworkView } from "../api";
import { RetryClear } from "../components/RetryClear";
import { StageList } from "../components/StageList";
import { useAction } from "../components/useAction";
import type { ClusterSummary } from "../../types";
import { CopyButton } from "../components/CopyButton";
import { useAutoRefresh } from "../../hooks";
import { tunnelEndpointProblem } from "../lib/clusterProvision";
import { siteOfCluster } from "../lib/siteNetwork";
import { joinStageLabel, joinStages, nodeJoinView, type NodeJoin } from "../lib/nodeJoin";
import { machineJoinProgress, machineJoinSteps } from "../lib/machineJoin";
import {
  classPasswordLabel,
  hasRegisteredRobotClass,
  machineClassAccess,
  machineLabelChoices,
  machineNotTickable,
  machineReach,
  machineUnseen,
  parseLabelLines,
  parseTaintLines,
  reachLabel,
  robotIDOfNode,
  robotTaint,
  seenTime,
  sshJoinFailure,
  unjoinedMachines,
  unseenFor,
  type IssuedNodeAdmission,
  type Machine,
  type MachineList,
  type NodeAdmission,
  type Reach,
  type NodeClassAccess,
} from "../lib/siteNodeDisplay";
import { JoinMachinesDialog } from "./JoinMachinesDialog";
import "./JoinNode.css";

const ttlOptions = [
  { seconds: 7200, zh: "2 小时", en: "2 hours" },
  { seconds: 86400, zh: "24 小时", en: "24 hours" },
  { seconds: 604800, zh: "7 天", en: "7 days" },
];

type Method = "machines" | "ssh" | "code";

// What a join answered, laid out as the console's other results (a signed
// cluster's YAML): a header, then blocks with a copy button.
function Result({ title, note, children }: { title: string; note: string; children: ReactNode }) {
  const shown = useRef<HTMLElement>(null);
  useEffect(() => {
    shown.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, []);
  return <section ref={shown} className="join-node-result" aria-live="polite">
    <div className="join-node-result-header">
      <div><Check size={18} /><strong>{title}</strong></div>
      <small>{note}</small>
    </div>
    {children}
  </section>;
}

function Block({ title, value, zh }: { title: string; value: string; zh: boolean }) {
  return <div className="join-node-block">
    <div className="join-node-block-head">
      <div><TerminalSquare size={16} /><strong>{title}</strong></div>
      <CopyButton value={value} zh={zh} />
    </div>
    <pre>{value}</pre>
  </div>;
}

// The answer that carries the code: shown once, never fetched again.
function IssuedJoinCode({ issued, zh }: { issued: IssuedNodeAdmission; zh: boolean }) {
  return <Result
    title={zh ? `${issued.admission.node_name} 的入网码` : `Join code for ${issued.admission.node_name}`}
    note={zh
      ? `有效至 ${new Date(issued.admission.expires_at).toLocaleString()}，用过一次即失效`
      : `Valid until ${new Date(issued.admission.expires_at).toLocaleString()}, for one machine`}
  >
    <div className="cert-error join-code-once" role="note">
      {zh ? "入网码和命令只显示这一次，离开本页就看不到了；丢了就在入网记录里重发。" : "The code and the command are shown this once. If they are lost, reissue from the admission records."}
    </div>
    <p className="join-code-wait"><span className="status status-pending"><i />{zh ? "等待机器使用入网码" : "Waiting for the machine to use the code"}</span><small>{zh ? "机器执行命令后，入网操作会出现在下面的列表。" : "The join appears in the operations section below after the machine runs the command."}</small></p>
    <Block zh={zh} title={zh ? `在 ${issued.admission.node_name} 上以有 sudo 权限的用户执行` : `Run on ${issued.admission.node_name} as a user with sudo`} value={issued.command} />
    <Block zh={zh} title={zh ? "入网码" : "Join code"} value={issued.code} />
  </Result>;
}

// The passwords of one NodeClass: whether each has been entered, and the form
// that enters the node's (once for the class; what is typed is sent and forgotten).
function ClassPasswords({ zh, name, access, onSaved }: { zh: boolean; name: string; access: NodeClassAccess; onSaved: () => Promise<unknown> }) {
  const [editing, setEditing] = useState(false);
  const [password, setPassword] = useState("");
  const [sudo, setSudo] = useState("");
  const { pending, error, run } = useAction();
  const open = editing || !access.node.configured;

  const save = () => run(async () => {
    await machinesApi.setClassAccess(name, { node_password: password, ...(sudo ? { sudo_password: sudo } : {}) });
    setPassword("");
    setSudo("");
    setEditing(false);
    await onSaved();
  });

  return <div className="join-class-access">
    <div className="join-class-access-state">
      <strong>{zh ? `机型 ${name}` : `Class ${name}`}</strong>
      <span>{zh ? "Orin 口令：" : "Entry device password: "}<span className={access.entry.configured ? "" : "join-missing"}>{classPasswordLabel(access.entry, zh)}</span></span>
      <span>{zh ? "x86 口令：" : "Computer password: "}<span className={access.node.configured ? "" : "join-missing"}>{classPasswordLabel(access.node, zh)}</span></span>
      {access.node.configured && !editing && <button type="button" className="secondary-button" onClick={() => setEditing(true)}>{zh ? "更改" : "Change"}</button>}
    </div>
    {!access.entry.configured && <small className="muted">{zh ? `Orin 口令在“密钥”里的 ${access.entry.secret}（键 ${access.entry.key}）录入。` : `The entry device's password is entered under Secrets, in ${access.entry.secret} (key ${access.entry.key}).`}</small>}
    {open && <form onSubmit={(event) => { event.preventDefault(); void save(); }}>
      <div className="form-grid">
        <label>{zh ? "x86 登录口令" : "Computer login password"}
          <input type="password" value={password} onChange={(event) => setPassword(event.target.value)} maxLength={256} autoComplete="new-password" /></label>
        <label>{zh ? "sudo 口令（和登录口令相同就不填）" : "sudo password (empty: the login password)"}
          <input type="password" value={sudo} onChange={(event) => setSudo(event.target.value)} maxLength={256} autoComplete="new-password" /></label>
      </div>
      <small className="muted">{zh ? "每个机型填一次，存进 RLark 的密钥，之后只显示有没有、不显示内容。没填之前 RLark 不登录这个机型的机器，它们也不能勾选。" : "Entered once for the class and kept as an RLark secret; afterwards only whether it is there is shown. Until then RLark logs in to no machine of the class and none can be ticked."}</small>
      {error && <div className="cert-error" role="alert">{error}</div>}
      <div className="join-node-actions">
        {editing && <button type="button" className="secondary-button" onClick={() => { setEditing(false); setPassword(""); setSudo(""); }} disabled={pending}>{zh ? "取消" : "Cancel"}</button>}
        <button type="submit" className="primary-button" disabled={pending || !password}>{pending ? (zh ? "保存中…" : "Saving…") : (zh ? "保存" : "Save")}</button>
      </div>
    </form>}
  </div>;
}

// The machines admitted to the cluster, as their joins go: each one's step, where and why one stopped, and how
// many have joined.
type OperationItem = {
  key: string;
  name: string;
  source: "batch" | "ssh" | "code";
  status: "running" | "failed" | "waiting" | "success";
  step: string;
  number: number;
  total: number;
  at: number;
  updated: string;
  reason: string;
  detail: Array<{ label: string; state: "done" | "running" | "failed" | "pending" }>;
};

function JoinOperationRail({ zh, clusterID, onOpenSSH, onOpenMachines, onReissue }: { zh: boolean; clusterID: string; onOpenSSH: (name: string) => void; onOpenMachines: () => void; onReissue: (issued: IssuedNodeAdmission) => void }) {
  const [machines, setMachines] = useState<Machine[]>([]);
  const [sshJoins, setSSHJoins] = useState<NodeJoin[]>([]);
  const [admissions, setAdmissions] = useState<NodeAdmission[]>([]);
  const [open, setOpen] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [actionPending, setActionPending] = useState("");
  const clusterRef = useRef(clusterID);
  clusterRef.current = clusterID;
  useEffect(() => {
    setMachines([]);
    setSSHJoins([]);
    setAdmissions([]);
    setError("");
    setLoaded(false);
  }, [clusterID]);
  const lang = zh ? "zh" : "en";
  const refresh = useCallback(async () => {
    const requested = clusterID;
    const [machineResult, sshResult, admissionResult] = await Promise.allSettled([
      machinesApi.list(), clusterID ? nodeJoinsApi.list(clusterID) : Promise.resolve([]), clusterID ? nodeAdmissionsApi.list(clusterID) : Promise.resolve({ items: [] }),
    ]);
    // A slow response for the previous cluster must not populate this one:
    // revoke/reissue would then act on the current cluster with a stale name.
    if (clusterRef.current !== requested) return;
    if (machineResult.status === "fulfilled") setMachines(machineResult.value.items);
    if (sshResult.status === "fulfilled") setSSHJoins(sshResult.value);
    if (admissionResult.status === "fulfilled") setAdmissions(admissionResult.value.items);
    const failed = [machineResult, sshResult, admissionResult].find((result) => result.status === "rejected");
    setError(failed?.status === "rejected" ? (failed.reason as Error).message : "");
    setLoaded(true);
  }, [clusterID]);
  useAutoRefresh(refresh, 3000, [clusterID]);

  const now = Date.now();
  const machineProgress = machineJoinProgress(machines, clusterID, now, zh);
  const items: OperationItem[] = machineProgress.rows.map((row) => {
    const total = machineJoinSteps.length;
    const stepIndex = Math.min(row.done, total - 1);
    const failed = row.state === "failed";
    const machineSteps = machineJoinSteps.map((stage, index) => ({ label: stage[lang], state: row.state === "joined" || index < row.done ? "done" as const : index === stepIndex && row.state === "running" ? "running" as const : index === stepIndex && failed ? "failed" as const : "pending" as const }));
    return { key: `batch:${row.machine.name}`, name: row.machine.node_name || row.machine.serial || row.machine.name, source: "batch", status: row.state === "joined" ? "success" : failed ? "failed" : "running", step: row.text, number: row.state === "joined" ? total : Math.min(row.done + 1, total), total, at: row.machine.requested_at ? Date.parse(row.machine.requested_at) : 0, updated: row.machine.requested_at ? seenTime(row.machine.requested_at, now) : "—", reason: failed ? row.text : "", detail: machineSteps };
  });
  for (const join of sshJoins) {
    const view = nodeJoinView(join, lang);
    const total = view.stages.length;
    const current = view.stages.findIndex((stage) => stage.name === join.status?.stage);
    const number = join.status?.phase === "Succeeded" ? total : Math.max(1, current + 1);
    const status = join.status?.phase === "Succeeded" ? "success" : join.status?.phase === "Failed" ? "failed" : "running";
    const reason = view.stages.find((stage) => stage.state === "Failed")?.message ?? "";
    items.push({ key: `ssh:${join.metadata?.name}`, name: join.spec.nodeName || join.spec.machine.address, source: "ssh", status, step: join.status?.phase === "Succeeded" ? (zh ? "入网完成" : "Joined") : joinStageLabel(join.status?.stage ?? "", lang), number, total, at: join.metadata?.creationTimestamp ? Date.parse(join.metadata.creationTimestamp) : 0, updated: join.metadata?.creationTimestamp ? seenTime(join.metadata.creationTimestamp, now) : "—", reason, detail: view.stages.map((stage) => ({ label: stage.label, state: stage.state === "Done" || stage.state === "Skipped" ? "done" : stage.state === "Failed" ? "failed" : stage.state === "Running" ? "running" : "pending" })) });
  }
  for (const admission of admissions) {
    if (admission.ssh_login) continue;
    if (!["Pending", "Redeemed", "Joined", "Expired"].includes(admission.state)) continue;
    const waiting = admission.state === "Pending";
    const success = admission.state === "Joined";
    const expired = admission.state === "Expired";
    const sshStages = joinStages;
    const stageIndex = sshStages.findIndex((stage) => stage.name === admission.join_stage);
    const total = sshStages.length;
    const number = success ? total : waiting || expired ? 0 : Math.max(1, stageIndex + 1);
    const status = waiting ? "waiting" : success ? "success" : expired || admission.stuck || admission.join_phase === "Failed" ? "failed" : "running";
    items.push({ key: `code:${admission.cluster_id}:${admission.node_name}`, name: admission.node_name, source: "code", status, step: waiting ? (zh ? "等待机器使用入网码" : "Waiting for the machine to use the code") : success ? (zh ? "入网完成" : "Joined") : expired ? (zh ? "入网码已过期或被作废" : "Join code expired or revoked") : joinStageLabel(admission.join_stage ?? "", lang), number, total, at: admission.issued_at ? Date.parse(admission.issued_at) || 0 : 0, updated: seenTime(admission.issued_at, now), reason: status === "failed" ? admission.reason ?? (expired ? (zh ? "入网码已过期或被作废" : "Join code expired or revoked") : (zh ? "入网停住" : "Join stopped")) : "", detail: sshStages.map((stage, index) => ({ label: stage[lang], state: success || index < number - 1 ? "done" : index === number - 1 && status === "running" ? "running" : index === number - 1 && status === "failed" ? "failed" : "pending" })) });
  }
  const priority = { running: 0, failed: 1, waiting: 2, success: 3 };
  const active = items.filter((item) => item.status !== "success").sort((a, b) => priority[a.status] - priority[b.status] || a.name.localeCompare(b.name));
  const completed = items.filter((item) => item.status === "success").sort((a, b) => b.at - a.at);
  const seenNames = new Set<string>();
  const history = completed.filter((item) => {
    if (seenNames.has(item.name)) return false;
    seenNames.add(item.name);
    return true;
  });
  const count = (status: OperationItem["status"]) => status === "success" ? history.length : active.filter((item) => item.status === status).length;
  const toggle = (key: string) => setOpen((current) => current.includes(key) ? current.filter((item) => item !== key) : [...current, key]);
  const stateText = (status: OperationItem["status"]) => status === "running" ? zh ? "进行中" : "Running" : status === "failed" ? zh ? "失败" : "Failed" : status === "waiting" ? zh ? "待使用" : "Waiting" : zh ? "已加入" : "Joined";
  const itemView = (item: OperationItem) => <article key={item.key} className={`join-operation is-${item.status}`}>
      <div className="join-operation-title"><strong title={item.name}>{item.name}</strong><span className={`status ${item.status === "success" ? "status-online" : item.status === "failed" ? "status-failed" : "status-pending"}`}><i />{stateText(item.status)}</span></div>
      <div className="join-operation-meta"><span className="join-source">{item.source === "batch" ? zh ? "批量" : "Batch" : item.source === "ssh" ? "SSH" : zh ? "入网码" : "Join code"}</span><span>{item.step}</span></div>
      <div className="join-operation-progress"><span>{item.number}/{item.total}</span><progress value={item.number} max={item.total} aria-label={`${item.number}/${item.total}`} /></div>
      {item.reason && <p className="join-operation-reason">{item.reason}</p>}
      <div className="join-operation-actions"><small>{zh ? "提交于" : "Started"} {item.updated}</small>{item.source === "batch" && item.status === "failed" && <button type="button" className="table-action-button" onClick={onOpenMachines}>{zh ? "到机器列表" : "Open machines"}</button>}{item.source === "ssh" && item.status === "failed" && <button type="button" className="table-action-button" onClick={() => onOpenSSH(item.key.slice(4))}>{zh ? "打开重试记录" : "Open retry record"}</button>}{item.source === "code" && (item.status === "waiting" || item.status === "failed") && <button type="button" className="table-action-button" disabled={!!actionPending} onClick={() => { setActionPending(item.key); void nodeAdmissionsApi.reissue(item.name, clusterID).then(onReissue).catch((reason: unknown) => setError((reason as Error).message)).finally(() => setActionPending("")); }}>{zh ? "重发入网码" : "Reissue code"}</button>}{item.source === "code" && item.status === "waiting" && <button type="button" className="table-action-button danger" disabled={!!actionPending} onClick={() => { setActionPending(item.key); void nodeAdmissionsApi.revoke(item.name, clusterID).then(refresh).catch((reason: unknown) => setError((reason as Error).message)).finally(() => setActionPending("")); }}>{zh ? "作废" : "Revoke"}</button>}<button type="button" className="table-action-button" aria-expanded={open.includes(item.key)} onClick={() => toggle(item.key)}>{open.includes(item.key) ? zh ? "收起" : "Hide" : zh ? "阶段" : "Stages"}<ChevronDown size={13} /></button></div>
      {open.includes(item.key) && <ol className="join-operation-stages">{item.detail.map((stage, index) => <li className={`is-${stage.state}`} key={`${stage.label}-${index}`}><span>{stage.state === "done" ? "✓" : stage.state === "failed" ? "!" : stage.state === "running" ? "•" : ""}</span><strong>{stage.label}</strong><small>{stage.state === "done" ? zh ? "完成" : "Done" : stage.state === "failed" ? zh ? "失败" : "Failed" : stage.state === "running" ? zh ? "进行中" : "Running" : ""}</small></li>)}</ol>}
    </article>;

  return <section className="table-panel join-rail" aria-labelledby="join-rail-title" aria-busy={!loaded}>
    <header className="join-rail-head"><div><h2 id="join-rail-title">{zh ? "入网操作" : "Join operations"}</h2><p>{zh ? "操作在服务端继续，离开页面不会中断。" : "Operations continue on the server when you leave."}</p></div><button type="button" className="icon-button" onClick={() => void refresh()} aria-label={zh ? "刷新操作" : "Refresh operations"}><RefreshCw size={15} /></button></header>
    {active.length > 0 && <div className="join-rail-summary" aria-live="polite">{count("running") > 0 && <span className="join-rail-chip is-running">{zh ? "进行中" : "Running"} {count("running")}</span>}{count("failed") > 0 && <span className="join-rail-chip is-failed">{zh ? "失败" : "Failed"} {count("failed")}</span>}{count("waiting") > 0 && <span className="join-rail-chip is-waiting">{zh ? "待使用" : "Waiting"} {count("waiting")}</span>}</div>}
    {error && <div className="cert-error" role="alert">{zh ? `操作记录读取失败：${error}` : `Could not load operations: ${error}`}</div>}
    {active.length > 0 && <div className="join-rail-list">{active.map(itemView)}</div>}
    {!active.length && history.length > 0 && loaded && !error && <p className="join-rail-empty">{zh ? "没有正在进行的入网" : "No join in progress"}</p>}
    {history.length > 0 && <details className="join-rail-history"><summary><span>{zh ? `已加入 ${history.length}` : `${history.length} joined`}</span><ChevronDown size={14} /></summary><div className="join-rail-list">{history.map(itemView)}</div></details>}
    {!active.length && !history.length && loaded && !error && <p className="join-rail-empty">{zh ? "暂无入网操作" : "No join operations yet"}</p>}
  </section>;
}

// Whether RLark got through to one computer of a machine: the reason and when it last looked on hover, the reason
// beside it when it did not.
function ReachCell({ zh, reach, machine, now }: { zh: boolean; reach: Reach; machine: Machine; now: number }) {
  const looked = machine.inspected_at ? (zh ? `检查于 ${seenTime(machine.inspected_at, now)}` : `checked ${seenTime(machine.inspected_at, now)}`) : "";
  const title = [reach.reason, reach.state === "fail" ? machine.message : "", looked].filter(Boolean).join(zh ? "；" : "; ");
  const pill = { ok: "status-online", fail: "status-failed", checking: "status-pending", unknown: "" }[reach.state];
  return <span className="join-reach-cell" title={title}>
    {pill ? <span className={`status ${pill}`}><i />{reachLabel(reach.state, zh)}</span> : <span className="muted">{reachLabel(reach.state, zh)}</span>}
    {reach.state !== "ok" && <small className={reach.state === "fail" ? "join-missing join-reach-reason" : "muted join-reach-reason"}>{reach.reason}</small>}
    {reach.state === "ok" && looked && <small className="muted join-reach-reason">{looked}</small>}
  </span>;
}

// The machines the hubs of the sites have found and that are not nodes yet:
// ticking them is all it takes. RLark names each one, logs in to it with the
// accounts of its class and joins it; nobody types anything at the machine.
// SSH joins that are still running or stopped are listed beside them as manual
// entries; they are opened from their record, not batch-admitted.
type MachineSource = "all" | "scanned" | "manual";
type MachineKindFilter = "all" | "Robot" | "Server" | "Unknown";
type ManualJoinRow = { join: NodeJoin; record: string; name: string; address: string; user: string; phase: string; stage: string };

function UnjoinedMachines({ zh, clusterID, clusterName, site, ticked, setTicked, onViewProgress, onUseMethod, onOpenSSHJoin }: { zh: boolean; clusterID: string; clusterName: string; site: string; ticked: string[]; setTicked: (update: string[] | ((current: string[]) => string[])) => void; onViewProgress: () => void; onUseMethod: (method: Method) => void; onOpenSSHJoin: (record: string) => void }) {
  const [list, setList] = useState<MachineList>();
  const [joins, setJoins] = useState<NodeJoin[]>([]);
  const [labels, setLabels] = useState<Record<string, string>>({});
  const [query, setQuery] = useState("");
  const [source, setSource] = useState<MachineSource>("all");
  const [kindFilter, setKindFilter] = useState<MachineKindFilter>("all");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogMachines, setDialogMachines] = useState<Machine[]>([]);
  const [accessOpen, setAccessOpen] = useState<boolean>();
  const { error, setError } = useAction();

  const load = useCallback(async () => {
    const [machineResult, joinResult] = await Promise.allSettled([
      machinesApi.list(site),
      nodeJoinsApi.list(clusterID),
    ]);
    if (machineResult.status === "fulfilled") { setList(machineResult.value); setError(""); }
    if (joinResult.status === "fulfilled") setJoins(joinResult.value);
    const failed = [machineResult, joinResult].find((result) => result.status === "rejected");
    if (failed?.status === "rejected") setError((failed.reason as Error).message);
  }, [clusterID, setError, site]);
  const now = Date.now();
  const progress = machineJoinProgress(list?.items ?? [], clusterID, now, zh);
  const joining = progress.running > 0;
  const joinRunning = joins.some((join) => join.status?.phase === "Running");
  // A machine that was just plugged in shows up within a minute; a join is
  // followed step by step. The shared hook pauses while the tab is hidden.
  useAutoRefresh(load, joining || joinRunning ? 3000 : 10000, [joining, joinRunning, site, clusterID]);

  const scanned = [...unjoinedMachines(list?.items ?? [], clusterID), ...(list?.items ?? []).filter((machine) => machine.joining && machine.admit_cluster_id === clusterID)];
  const lang = zh ? "zh" : "en";
  const manual: ManualJoinRow[] = joins
    .filter((join) => join.status?.phase !== "Succeeded")
    .map((join) => ({
      join,
      record: join.metadata?.name ?? "",
      name: join.spec.nodeName || join.spec.machine.address,
      address: join.spec.machine.address,
      user: join.spec.machine.user,
      phase: join.status?.phase ?? "Running",
      stage: join.status?.stage ? joinStageLabel(join.status.stage, lang) : "",
    }));
  // A machine already joining this cluster is shown for context, but must
  // not be selectable again: a second admit would start a duplicate join.
  const selectable = (machine: Machine) => machine.admissible && !(machine.joining && machine.admit_cluster_id === clusterID);
  const chosen = scanned.filter((machine) => selectable(machine) && ticked.includes(machine.name));
  const choices = machineLabelChoices(chosen, list?.classes ?? {});
  const search = query.trim().toLocaleLowerCase();
  const matches = (values: Array<string | undefined>) => values.some((value) => value?.toLocaleLowerCase().includes(search));
  const visibleScanned = source === "manual" ? [] : scanned.filter((machine) => (kindFilter === "all" || machine.machine_type === kindFilter) && matches([machine.serial, machine.mac, machine.name, machine.node_class, machine.site, machine.address, machine.node_name]));
  const visibleManual = source === "scanned" ? [] : manual.filter((item) => matches([item.name, item.address, item.user, item.stage]));
  const visibleAdmissible = visibleScanned.filter((machine) => selectable(machine)).map((machine) => machine.name);
  const visibleCount = visibleScanned.length + visibleManual.length;
  const scanClasses = machineClassAccess(list?.classes ?? {});
  const allClassesConfigured = scanClasses.length > 0 && scanClasses.every(({ access }) => access.node.configured);
  const labelOverrides = Object.fromEntries(Object.entries(labels).filter(([key, value]) => key in choices && value.trim() && value.trim() !== choices[key]).map(([key, value]) => [key, value.trim()]));
  const toggle = (name: string) => setTicked((current) => current.includes(name) ? current.filter((item) => item !== name) : [...current, name]);

  return <>
    <section className="join-panel join-machines-panel" aria-busy={!list}>
      <header className="join-panel-head">
        <div>
          <h3>{zh ? "未加入的机器" : "Machines that have not joined"} <span className="count-pill">{visibleCount} {zh ? "台" : ""}</span></h3>
          <p>{zh ? <>发现候选 <strong>{scanned.length}</strong> 台；手动 SSH 入网 <strong>{manual.length}</strong> 台。选择类型后再决定使用机器人配置或 SSH 入网。</> : <>Discovered candidates: <strong>{scanned.length}</strong>. Manual SSH joins: <strong>{manual.length}</strong>. Choose a type before selecting a robot profile or SSH join.</>}</p>
        </div>
        <div className="join-panel-tools">
          <button type="button" className="secondary-button" onClick={() => onUseMethod("ssh")}><Plus size={14} />{zh ? "添加普通电脑/GPU" : "Add computer/GPU"}</button>
          <label className="join-machine-search"><Search size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={zh ? "搜索序列号、MAC、地址" : "Search serial, MAC, address"} aria-label={zh ? "搜索机器" : "Search machines"} /></label>
          <label className="join-select-all"><input type="checkbox" aria-label={zh ? "全选可加入的扫描机器" : "Select all joinable scanned machines"} checked={visibleAdmissible.length > 0 && visibleAdmissible.every((name) => ticked.includes(name))} disabled={visibleAdmissible.length === 0} onChange={(event) => setTicked((current) => event.target.checked ? [...new Set([...current, ...visibleAdmissible])] : current.filter((name) => !visibleAdmissible.includes(name)))} />{zh ? "全选扫描机器" : "Select scanned"}</label>
          <button type="button" className="icon-button" onClick={() => void load()} aria-label={zh ? "刷新机器列表" : "Refresh machines"}><RefreshCw size={14} /></button>
        </div>
      </header>

      <details className="join-class-access-panel" open={accessOpen ?? !allClassesConfigured} onToggle={(event) => setAccessOpen((event.currentTarget).open)}>
        <summary>
          <span className="join-class-access-title"><KeyRound size={14} />{zh ? "扫描机器型凭据" : "Scanned-machine credentials"}</span>
          <span className="join-class-access-summary">{scanClasses.length ? scanClasses.map(({ name, access }) => `${name === "s1u" ? "S1U" : name} ${access.node.configured ? (zh ? "已配置" : "configured") : (zh ? "未配置" : "missing")}`).join(" · ") : (zh ? "没有声明登录方式的机型" : "No NodeClass declares access")}</span>
          <span className="join-class-access-state"><span className={`status ${allClassesConfigured ? "status-online" : "status-pending"}`}><i />{allClassesConfigured ? (zh ? "可加入" : "Ready") : scanClasses.length ? (zh ? "有未配置" : "Needs setup") : (zh ? "无凭据" : "No credentials")}</span><ChevronDown size={14} /></span>
        </summary>
        <div className="join-class-access-body">
          {scanClasses.length ? scanClasses.map(({ name, access }) => <ClassPasswords key={name} zh={zh} name={name} access={access} onSaved={() => load()} />) : <p className="muted">{zh ? "没有声明登录方式的机型；扫描到的机器无法自动登录。" : "No NodeClass declares how to log in; scanned machines cannot be joined automatically."}</p>}
          <p className="join-class-access-note">{zh ? "手动添加的普通电脑/GPU 不使用机型凭据；它们用 SSH 表单里的账号和口令。" : "Manually added computers/GPUs do not use class credentials; their SSH account and password are in the SSH form."}</p>
        </div>
      </details>

      <div className="join-machine-filter" role="group" aria-label={zh ? "机器来源" : "Machine source"}>
        <button type="button" className={source === "all" ? "is-active" : ""} onClick={() => setSource("all")}>{zh ? "全部" : "All"} <span>{scanned.length + manual.length}</span></button>
        <button type="button" className={source === "scanned" ? "is-active" : ""} onClick={() => setSource("scanned")}>{zh ? "扫描发现" : "Scanned"} <span>{scanned.length}</span></button>
        <button type="button" className={source === "manual" ? "is-active" : ""} onClick={() => setSource("manual")}>{zh ? "手动添加" : "Manual"} <span>{manual.length}</span></button>
      </div>
      <div className="join-machine-filter" role="group" aria-label={zh ? "机器类型" : "Machine type"}>
        {(["all", "Robot", "Server", "Unknown"] as const).map((kind) => <button key={kind} type="button" className={kindFilter === kind ? "is-active" : ""} onClick={() => setKindFilter(kind)}>{kind === "all" ? (zh ? "全部类型" : "All types") : kind === "Robot" ? "Robot" : kind === "Server" ? "Server" : (zh ? "未知" : "Unknown")}<span>{kind === "all" ? scanned.length : scanned.filter((machine) => machine.machine_type === kind).length}</span></button>)}
      </div>

      <div className="join-machine-columns" aria-hidden="true">
        <span></span><span>{zh ? "机器" : "Machine"}</span><span>{zh ? "位置" : "Location"}</span><span>{zh ? "可达性" : "Reachability"}</span><span>{zh ? "状态" : "State"}</span><span>{zh ? "操作" : "Action"}</span>
      </div>

      {!list ? <div className="join-machines-empty muted" role="status">{error ? (zh ? "机器列表读取失败…" : "Could not load machines…") : (zh ? "正在读取机器…" : "Loading machines…")}</div>
        : visibleCount === 0 ? <div className="join-machines-empty muted">
          {source === "manual"
            ? (zh ? "没有手动添加的机器。用上方“添加普通电脑/GPU”打开 SSH 入网表单。" : "No manually added machines. Use Add computer/GPU above to open the SSH form.")
            : (zh ? "没有扫到未加入的机器。新机器插上网线后，站点下一次扫描时会出现在这里；普通电脑/GPU 可以用 SSH 手动添加，场地第一台也可以用入网码。" : "No machine that has not joined was found. A new machine shows up after the site's next scan; ordinary computers/GPUs can be added over SSH, and a site's first machine can use a join code.")}
        </div> : <div className="join-machine-grid">
          {visibleScanned.map((machine) => {
            const hasClassMatch = hasRegisteredRobotClass(machine, list?.classes ?? {});
            const reach = hasClassMatch ? machineReach(machine, zh) : undefined;
            const why = hasClassMatch ? machineNotTickable(machine, zh) : "";
            const inProgress = machine.joining && machine.admit_cluster_id === clusterID;
            const checked = selectable(machine) && ticked.includes(machine.name);
            return <article className={`join-machine-card${machine.admissible ? "" : " is-disabled"}${inProgress ? " is-joining" : ""}${checked ? " is-selected" : ""}`} key={machine.name}>
              <label className="join-machine-check"><input type="checkbox" aria-label={machine.serial || machine.name} title={why || undefined} checked={checked} disabled={!selectable(machine)} onChange={() => toggle(machine.name)} /></label>
              <div className="join-machine-identity">
                <strong title={machine.serial || machine.name}>{machine.serial || machine.name}</strong>
                <small>{machine.node_class || (zh ? "无 NodeClass" : "No NodeClass")} · <span className="mono">{machine.mac || "—"}</span><span className="join-source-tag">{zh ? "扫描" : "scanned"}</span></small>
                <span className={`join-machine-kind is-${machine.machine_type.toLowerCase()}`} title={machine.identity_reason}>{machine.machine_type === "Unknown" ? (zh ? "类型未知" : "Type unknown") : machine.machine_type}{machine.machine_model ? ` · ${machine.machine_model}` : ""}{machine.identity_verified === "False" ? (zh ? " · 身份不匹配" : " · identity mismatch") : ""}</span>
              </div>
              <div className="join-machine-location">
                <strong>{machine.site || "—"}</strong>
                <small className="mono">{machine.address || "—"}</small>
                <small>{zh ? "最近发现" : "Seen"} {seenTime(machine.last_seen, now)}{machineUnseen(machine, now) && <span className="join-missing"> · {unseenFor(machine.last_seen, now, zh)}</span>}</small>
              </div>
              <div className="join-machine-reach">
                {reach ? <>
                  <div className="join-reach-pair"><div><small>Orin {zh ? "上联" : "entry"}</small><ReachCell zh={zh} reach={reach.orin} machine={machine} now={now} /></div><div><small>x86 {zh ? "计算机" : "computer"}</small><ReachCell zh={zh} reach={reach.x86} machine={machine} now={now} /></div></div>
                  {why && <small className="join-reach-reason">{why}</small>}
                  {!inProgress && machine.phase === "Admitted" && <small className="join-reach-reason">{zh ? "入网停住了，勾选它再点加入，重新入网" : "Its join stopped: tick it and add it again to join it anew"}</small>}
                </> : machine.port ? <small className="muted">SSH · {zh ? `端口 ${machine.port}` : `port ${machine.port}`}</small> : null}
              </div>
              <div className="join-machine-state">
                {inProgress ? <span className="status status-pending"><i />{zh ? "已在入网中" : "Joining"}</span> : machine.admissible ? <span className="status status-online"><i />{zh ? "可选择" : "Selectable"}</span> : <span className="status status-failed"><i />{zh ? "不可选" : "Not selectable"}</span>}
                {machine.node_name && <small className="muted join-machine-node">{machine.node_name}</small>}
              </div>
              <div className="join-machine-action">
                {inProgress ? <button type="button" className="table-action-button" onClick={onViewProgress}>{zh ? "查看进度" : "View progress"}</button>
                  : machine.admissible ? <button type="button" className="table-action-button" onClick={() => toggle(machine.name)}>{checked ? (zh ? "已选择" : "Selected") : (zh ? "选择" : "Select")}</button>
                    : <span className="muted">—</span>}
              </div>
            </article>;
          })}
          {visibleManual.map((item) => <article className="join-machine-card is-manual" key={item.record}>
            <span className="join-machine-check join-machine-check-empty" aria-hidden="true">—</span>
            <div className="join-machine-identity"><strong>{item.address}</strong><small>{item.user}@{item.address}<span className="join-source-tag">{zh ? "手动" : "manual"}</span></small></div>
            <div className="join-machine-location"><strong>{site || "—"}</strong><small>{zh ? "SSH 手动添加" : "Added over SSH"}</small></div>
            <div className="join-machine-reach"><span className={`status ${item.phase === "Failed" ? "status-failed" : "status-pending"}`}><i />{item.phase === "Failed" ? (zh ? "SSH 停住" : "SSH stopped") : (zh ? "SSH 进行中" : "SSH joining")}</span>{item.stage && <small className="join-reach-reason">{item.phase === "Failed" ? (zh ? `停在 ${item.stage}` : `Stopped at ${item.stage}`) : (zh ? `当前 ${item.stage}` : `At ${item.stage}`)}</small>}</div>
            <div className="join-machine-state"><span className={`status ${item.phase === "Failed" ? "status-failed" : "status-pending"}`}><i />{item.phase === "Failed" ? (zh ? "失败" : "Failed") : (zh ? "进行中" : "Running")}</span>{item.name !== item.address && <small className="muted join-machine-node">{item.name}</small>}</div>
            <div className="join-machine-action"><button type="button" className="table-action-button" onClick={() => onOpenSSHJoin(item.record)}>{zh ? "打开 SSH 记录" : "Open SSH record"}</button></div>
          </article>)}
        </div>}

      {source !== "manual" && visibleScanned.length > 0 && visibleAdmissible.length === 0 && <div className="join-no-selectable" role="status">
        <div><strong>{zh ? "当前没有可勾选的机器" : "No machine can be selected right now"}</strong><small>{zh ? "先看机器行里的具体原因；检查网络或机型口令后刷新，也可以直接改用其他方式。" : "Check the reason on the machine; fix its network or class password and refresh, or use another method."}</small></div>
        <div className="join-no-selectable-actions"><button type="button" className="secondary-button" onClick={() => onUseMethod("ssh")}><Terminal size={14} />{zh ? "改用 SSH 入网" : "Use SSH join"}</button><button type="button" className="secondary-button" onClick={() => onUseMethod("code")}><KeyRound size={14} />{zh ? "生成入网码" : "Use a join code"}</button></div>
      </div>}

      {visibleAdmissible.length > 0 && <footer className={`join-selection-bar${chosen.length ? " is-floating" : ""}`}>
        <div><strong>{zh ? `已选 ${chosen.length} 台` : `${chosen.length} selected`}</strong><button type="button" className="table-action-button" onClick={() => setTicked([])} disabled={!ticked.length}>{zh ? "清空" : "Clear"}</button></div>
        <button type="button" className="primary-button" onClick={() => { setDialogMachines(chosen); setDialogOpen(true); }} disabled={chosen.length === 0 || !clusterID}>{zh ? "加入所选机器" : "Join selected machines"}</button>
      </footer>}
    </section>

    {Object.keys(choices).length > 0 && <details className="join-node-advanced">
      <summary>{zh ? "这几台和机型预设不一样的地方" : "Where these machines differ from their class"}</summary>
      <div className="form-grid">
        {Object.entries(choices).map(([key, value]) => <label key={key}>{key}
          <input value={labels[key] ?? value} onChange={(event) => setLabels((current) => ({ ...current, [key]: event.target.value }))} maxLength={63} autoComplete="off" />
        </label>)}
      </div>
    </details>}
    {error && <div className="cert-error" role="alert">{error}</div>}
    {dialogOpen && dialogMachines.length > 0 && <JoinMachinesDialog
      zh={zh}
      clusterID={clusterID}
      clusterName={clusterName}
      machines={dialogMachines}
      classes={list?.classes ?? {}}
      labels={labelOverrides}
      onClose={() => { setDialogOpen(false); setDialogMachines([]); setTicked([]); }}
      onJoined={async () => { await load(); }}
      onUseCode={() => { setDialogOpen(false); setDialogMachines([]); onUseMethod("code"); }}
    />}
  </>;
}

// A machine RLark logs in to with the account typed here, and joins in stages: the Gateway records them on the
// join (NodeJoin), so the list below shows every join of the cluster that runs or has lately run, as it goes, from
// any browser and after the page is left. The password is sent with the request that starts or retries a join, and
// forgotten.
function SSHJoin({ zh, clusterID, initial, selectedOperation }: { zh: boolean; clusterID: string; initial?: string; selectedOperation?: string }) {
  const lang = zh ? "zh" : "en";
  const [form, setForm] = useState({ address: "", user: "", password: "", sudoPassword: "", port: "22", nodeName: "", tunnelEndpoint: "" });
  const { pending, error, run } = useAction();
  // The last poll's failure, shown only until a poll succeeds again (a Gateway that restarts).
  const [pollError, setPollError] = useState("");
  const [joins, setJoins] = useState<NodeJoin[]>([]);
  const [selected, setSelected] = useState(initial ?? "");
  // Whether the shown join is the operator's to choose: from the first list on, or once a join is named.
  const chosen = useRef(Boolean(initial));
  const [cleared, setCleared] = useState<string[]>([]);
  useEffect(() => {
    if (selectedOperation && joins.some((join) => join.metadata?.name === selectedOperation)) {
      chosen.current = true;
      setSelected(selectedOperation);
    }
  }, [selectedOperation, joins]);
  const update = (patch: Partial<typeof form>) => setForm((current) => ({ ...current, ...patch }));
  const port = Number(form.port);
  const endpointProblem = tunnelEndpointProblem(form.tunnelEndpoint, lang);
  const ready = clusterID && form.address.trim() && form.user.trim() && form.password && Number.isInteger(port) && port >= 1 && port <= 65535 && !endpointProblem;

  const refresh = useCallback(async () => {
    try {
      const list = await nodeJoinsApi.list(clusterID);
      setJoins(list);
      setPollError("");
      // Coming back to the page, the join that runs is the one shown; after that, the one the operator opens.
      if (!chosen.current) {
        chosen.current = true;
        setSelected(list.find((j) => j.status?.phase === "Running")?.metadata?.name ?? "");
      }
    } catch (reason) {
      setPollError((reason as Error).message);
    }
  }, [clusterID]);
  useAutoRefresh(refresh, 3000, [refresh]);

  const submit = async () => {
    if (pending || !ready) return;
    setCleared([]);
    await run(async () => {
      try {
        const made = await nodeJoinsApi.create({
          cluster_id: clusterID,
          address: form.address.trim(),
          user: form.user.trim(),
          password: form.password,
          ...(form.sudoPassword ? { sudo_password: form.sudoPassword } : {}),
          ...(port !== 22 ? { port } : {}),
          ...(form.nodeName.trim() ? { node_name: form.nodeName.trim() } : {}),
          ...(form.tunnelEndpoint.trim() ? { tunnel_endpoint: form.tunnelEndpoint.trim() } : {}),
        });
        chosen.current = true;
        setSelected(made.metadata?.name ?? "");
        await refresh();
      } catch (reason) {
        // The Gateway's reason for a refused login, in words that say what to do.
        throw new Error(sshJoinFailure(reason instanceof ApiError ? (reason.body as { reason?: string } | undefined)?.reason : undefined, (reason as Error).message, zh));
      } finally {
        update({ password: "", sudoPassword: "" });
      }
    });
  };

  const current = joins.find((j) => j.metadata?.name === selected);
  const view = current ? nodeJoinView(current, lang) : undefined;
  const phaseText = (j: NodeJoin) => j.status?.phase === "Succeeded"
    ? (zh ? "已入网" : "Joined")
    : j.status?.phase === "Failed"
      ? (zh ? `停在 ${joinStageLabel(j.status.stage ?? "", lang)}` : `Stopped at ${joinStageLabel(j.status.stage ?? "", lang)}`)
      : (zh ? `进行中：${joinStageLabel(j.status?.stage ?? "", lang)}` : `Running: ${joinStageLabel(j.status?.stage ?? "", lang)}`);

  return <section className="join-method-panel">
    <header className="join-method-head">
      <div><h3>{zh ? "SSH 入网" : "SSH join"} <span className="pill">{zh ? "独立方式" : "Standalone"}</span></h3><p>{zh ? "提供地址和 SSH 凭据，由 RLark 远程完成入网。支持 S1U 机器人、普通电脑和 GPU 服务器。" : "Provide an address and SSH credentials for RLark to join remotely. Works for S1U robots, ordinary computers and GPU servers."}</p></div>
    </header>
    <div className="join-method-body">
      <div className="join-method-layout">
        <form className="join-method-form" onSubmit={(event) => { event.preventDefault(); void submit(); }}>
          <div className="join-method-fields">
            <label>{zh ? "IP 地址" : "IP address"} *<input value={form.address} onChange={(event) => update({ address: event.target.value })} maxLength={253} autoComplete="off" placeholder="10.11.107.154" disabled={pending} /></label>
            <label>{zh ? "账号" : "Account"} *<input value={form.user} onChange={(event) => update({ user: event.target.value })} maxLength={64} autoComplete="off" disabled={pending} /></label>
            <label>{zh ? "密码" : "Password"} *<input type="password" value={form.password} onChange={(event) => update({ password: event.target.value })} maxLength={4096} autoComplete="new-password" disabled={pending} /></label>
            <label>{zh ? "sudo 密码（可选）" : "sudo password (optional)"}<input type="password" value={form.sudoPassword} onChange={(event) => update({ sudoPassword: event.target.value })} maxLength={4096} autoComplete="new-password" disabled={pending} /><small>{zh ? "和 SSH 密码不同才填写。" : "Only enter when it differs from the SSH password."}</small></label>
          </div>
          <details className="join-node-advanced">
            <summary>{zh ? "高级：节点名、端口、隧道端点" : "Advanced: node name, port, tunnel"}</summary>
            <div className="form-grid">
              <label>{zh ? "节点名" : "Node name"}<input value={form.nodeName} onChange={(event) => update({ nodeName: event.target.value })} maxLength={253} autoComplete="off" placeholder={zh ? "不填取机器的主机名" : "the machine's host name"} disabled={pending} /><small className="muted">{zh ? "Kubernetes 节点名（调度用，加入后不可改，之后只能改显示名）" : "The Kubernetes Node name (used for scheduling; it cannot change after the join, only the display name can)"}</small></label>
              <label>{zh ? "SSH 端口" : "SSH port"}<input type="number" min={1} max={65535} value={form.port} onChange={(event) => update({ port: event.target.value })} disabled={pending} /></label>
              <label title={zh ? "机器到不了场地的 Station API、要先建自己的隧道时用：几台机器共用一个公网 IP，填云上转发给这台的 公网IP:UDP端口" : "For a machine that gets a tunnel of its own first: behind one public IP with others, the public IP and the UDP port the cloud forwards to it"}>
                {zh ? "隧道公网端点（UDP）" : "Tunnel endpoint (UDP)"}
                <input value={form.tunnelEndpoint} onChange={(event) => update({ tunnelEndpoint: event.target.value })} maxLength={21} autoComplete="off" placeholder={zh ? "不填为 IP:51820" : "IP:51820 when empty"} disabled={pending} />
              </label>
            </div>
          </details>
          {endpointProblem && <small className="cert-error">{endpointProblem}</small>}
          {error && <div className="cert-error" role="alert">{error}</div>}
          {pollError && <div className="cert-error" role="status">{zh ? `读不到入网记录：${pollError}` : `The joins cannot be read: ${pollError}`}</div>}
          {cleared.length > 0 && <ol className="provision-cleared">{cleared.map((step) => <li key={step}>{step}</li>)}</ol>}
          <div className="join-method-actions">
            <button type="submit" className="primary-button" disabled={pending || !ready}>{pending ? (zh ? "提交中…" : "Starting…") : (zh ? "一键入网" : "Join")}</button>
          </div>
        </form>
        <aside className="join-method-side">
          <section className="join-method-side-card">
            <h4>{zh ? "入网流程" : "Join flow"}</h4>
            <ol className="join-method-steps">
              <li><b>1</b><span>{zh ? "登录并预检 x86" : "Log in and pre-check the computer"}</span></li>
              <li><b>2</b><span>{zh ? "安装组件、建立隧道" : "Install components and bring up the tunnel"}</span></li>
              <li><b>3</b><span>{zh ? "节点 Ready 并登记到 RLark" : "Node Ready and known to RLark"}</span></li>
            </ol>
          </section>
          <section className="join-method-side-card">
            <h4>{zh ? "说明" : "Notes"}</h4>
            <span className="join-method-note"><KeyRound size={14} />{zh ? "密码只用于这次 SSH 入网，不保存。" : "The password is used by this join only and kept nowhere."}</span>
            <span className="join-method-note"><ShieldCheck size={14} />{zh ? "首次连接会记录主机指纹。" : "The host key is recorded on first connect."}</span>
            <span className="join-method-note"><Terminal size={14} />{zh ? "分步入网；离开页面后服务端继续。" : "Joins in stages and continues on the server when this page is left."}</span>
            <span className="join-method-note"><Plus size={14} />{zh ? "提交后机器出现在列表的“手动添加”里。" : "The machine appears under Manual in the list after submission."}</span>
          </section>
        </aside>
      </div>
      {joins.length > 0 && <section className="join-method-records">
        <header className="join-method-records-head"><h4>{zh ? "最近 SSH 入网" : "Recent SSH joins"}</h4><span>{zh ? `${joins.length} 条记录` : `${joins.length} records`}</span></header>
        <div className="join-ssh-records">
          <div className="cert-list">
            {joins.map((j) => <div key={j.metadata?.name} className="cert-list-item">
              <div className={"cert-list-row" + (j === current ? " expanded" : "")} onClick={() => { chosen.current = true; setSelected(j === current ? "" : j.metadata?.name ?? ""); }}>
                <span className="cert-list-icon"><Server size={15} /></span>
                <span className="cert-list-name">{j.spec.nodeName} · {j.spec.machine.address}</span>
                <small className={"cert-list-date" + (j.status?.phase === "Failed" ? " cert-error" : "")}>{phaseText(j)}</small>
              </div>
              {j === current && view && <div className="provision-progress">
                <small className="muted">{zh ? `主机指纹 ${j.status?.hostKey ?? ""} · 由 ${j.spec.requestedBy ?? ""} 发起` : `Host key ${j.status?.hostKey ?? ""} · by ${j.spec.requestedBy ?? ""}`}</small>
                <StageList stages={view.stages} zh={zh} />
                <RetryClear zh={zh} view={view} clearReach={zh ? "要登录机器撤掉隧道" : "to take the tunnel away"} user={j.spec.machine.user} pending={pending} run={run} showSudo={j.metadata?.annotations?.["station.rlark.io/machine-type"] === "Server"}
                  retry={async (user, password, sudoPassword) => {
                    await nodeJoinsApi.retry(selected, password, user, sudoPassword);
                    await refresh();
                  }}
                  clear={async (user, password, machineGone, sudoPassword) => {
                    setCleared(await nodeJoinsApi.clear(selected, password, user, machineGone, sudoPassword));
                    setSelected("");
                    await refresh();
                  }}
                  clearNote={view.phase === "Succeeded"
                    ? (zh ? "清掉这条记录。节点还在集群里；要删节点，到场站节点页删。" : "Clear this record. The node stays in its cluster; remove it on the Site Nodes page.")
                    : (zh ? "不从这一步重试（机器或名字填错）：清掉这次入网。还没执行入网码的，撤掉这次的 Hub 声明和机器上的隧道；已执行的，按名字删除节点。之后可以重新入网。" : "Not retrying (a wrong machine or name): clear this join. Before its code's line ran, its hub declaration and the tunnel on the machine go; after, the node is removed by name. Then it can join again.")}
                  clearLabel={view.phase === "Succeeded" ? (zh ? "清掉这条记录" : "Clear this record") : (zh ? "清掉这次入网" : "Clear this join")} />
              </div>}
            </div>)}
          </div>
        </div>
      </section>}
    </div>
  </section>;
}

// A one-time code for a machine RLark cannot log in to: one line on the
// machine joins it. A robot's labels and taint follow from its node name;
// they are under Advanced, for the rare machine that differs.
function JoinCode({ zh, clusterID, reissued }: { zh: boolean; clusterID: string; reissued?: IssuedNodeAdmission }) {
  const [form, setForm] = useState({
    nodeName: "", kind: "Robot" as "Robot" | "Server", site: "", ttl: 7200,
    robotID: "", robotModel: "", nodeClass: "", labels: undefined as string | undefined, taints: undefined as string | undefined, address: "", uplink: "",
  });
  const { pending, error, run } = useAction();
  const [issued, setIssued] = useState(reissued);
  const [admissions, setAdmissions] = useState<NodeAdmission[]>([]);
  const [robotModels, setRobotModels] = useState<RobotModelItem[]>([]);
  const [classProfiles, setClassProfiles] = useState<MachineList["classes"]>({});
  const refreshAdmissions = useCallback(async () => {
    try {
      const result = await nodeAdmissionsApi.list(clusterID);
      setAdmissions(result.items.filter((admission) => !admission.ssh_login));
    } catch {
      setAdmissions([]);
    }
  }, [clusterID]);
  useEffect(() => { void refreshAdmissions(); }, [refreshAdmissions]);
  useEffect(() => {
    void Promise.all([robotModelsApi.list(), machinesApi.list()]).then(([models, machines]) => { setRobotModels(models.items); setClassProfiles(machines.classes); }).catch(() => { setRobotModels([]); setClassProfiles({}); });
  }, []);
  useAutoRefresh(refreshAdmissions, 10000, [clusterID]);
  const update = (patch: Partial<typeof form>) => setForm((current) => ({ ...current, ...patch }));

  const robot = form.kind === "Robot";
  const robotID = form.robotID.trim() || robotIDOfNode(form.nodeName);
  const profile = classProfiles[form.nodeClass];
  const model = profile?.model || profile?.labels?.["rlark.io/robot-model"] || form.robotModel;
  const profileLabels = profile?.labels ?? {};
  const defaultRobotLabels = Object.entries({ ...profileLabels, "rlark.io/node-category": "robot", ...(model ? { "rlark.io/robot-model": model } : {}), ...(robotID ? { "rlark.io/robot-id": robotID } : {}) }).map(([key, value]) => `${key}=${value}`).join("\n");
  const labelText = form.labels ?? (robot ? defaultRobotLabels : "");
  const taintText = form.taints ?? (robot ? robotTaint : "");
  const labels = parseLabelLines(labelText);
  const taints = parseTaintLines(taintText);
  const ready = clusterID && form.nodeName.trim() && (!robot || (model && form.nodeClass)) && !labels.error && !taints.error;

  const submit = () => !pending && ready && run(async () => {
    const answer = await nodeAdmissionsApi.issue({
      cluster_id: clusterID,
      node_name: form.nodeName.trim(),
      ...(form.site.trim() ? { site: form.site.trim() } : {}),
      ...(form.address.trim() ? { address: form.address.trim() } : {}),
      ...(form.uplink.trim() ? { uplink: form.uplink.trim() } : {}),
      ...(robot ? { node_class: form.nodeClass, machine: { category: "Robot" as const, model } } : { machine: { category: "Server" as const } }),
      ...(Object.keys(labels.labels).length ? { labels: labels.labels } : {}),
      ...(taints.taints.length ? { taints: taints.taints } : {}),
      ttl_seconds: form.ttl,
    });
    setIssued(answer);
    await refreshAdmissions();
  });
  const admissionState = (admission: NodeAdmission) => admission.state === "Joined"
    ? { label: zh ? "已加入" : "Joined", tone: "status-online" }
    : admission.state === "Pending"
      ? { label: zh ? "待使用" : "Waiting", tone: "status-pending" }
      : admission.state === "Expired" || admission.stuck
        ? { label: zh ? "已过期" : "Expired", tone: "status-failed" }
        : { label: zh ? "已兑换" : "Redeemed", tone: "status-pending" };

  return <section className="join-method-panel">
    <header className="join-method-head">
      <div><h3>{zh ? "入网码" : "Join code"} <span className="pill">{zh ? "独立方式" : "Standalone"}</span></h3><p>{zh ? "生成一次性入网码，在机器上执行一行命令；适合不能由 RLark 直接登录的机器。" : "Generate a one-time code and run one command on the machine; useful when RLark cannot log in directly."}</p></div>
    </header>
    <div className="join-method-body">
      <div className="join-method-layout">
        <form className="join-method-form" onSubmit={(event) => { event.preventDefault(); void submit(); }}>
          <div className="join-method-fields is-code">
            <label>{zh ? "节点名" : "Node name"} *<input value={form.nodeName} onChange={(event) => update({ nodeName: event.target.value })} maxLength={253} autoComplete="off" placeholder="s1u-09-x86" /><small className="muted">{zh ? "Kubernetes 节点名（调度用，加入后不可改，之后只能改显示名）" : "The Kubernetes Node name (used for scheduling; it cannot change after the join, only the display name can)"}</small></label>
            <label>{zh ? "机器类型" : "Machine kind"}
              <select value={form.kind} onChange={(event) => update({ kind: event.target.value as "Robot" | "Server", labels: undefined, taints: undefined })}>
              <option value="Robot">Robot</option>
                <option value="Server">Server</option>
              </select>
            </label>
            <label>{zh ? "场站" : "Site"}<input value={form.site} onChange={(event) => update({ site: event.target.value })} maxLength={63} autoComplete="off" placeholder={zh ? "不填用集群所在的" : "the cluster's"} /></label>
            <label>{zh ? "入网码有效期" : "Code valid for"}
              <select value={form.ttl} onChange={(event) => update({ ttl: Number(event.target.value) })}>
                {ttlOptions.map((option) => <option key={option.seconds} value={option.seconds}>{zh ? option.zh : option.en}</option>)}
              </select>
            </label>
          </div>
          <details className="join-node-advanced" open={!!(labels.error || taints.error) || undefined}>
            <summary>{zh ? "高级：机器人编号、隧道、标签与污点" : "Advanced: robot ID, tunnel, labels and taints"}</summary>
            <div className="form-grid">
              {robot && <>
                <label>{zh ? "机器人型号" : "Robot model"}<select value={model || form.robotModel} onChange={(event) => { const matching = Object.entries(classProfiles).find(([, item]) => (item.model || item.labels?.["rlark.io/robot-model"]) === event.target.value)?.[0] ?? ""; update({ robotModel: event.target.value, nodeClass: matching, labels: undefined }); }}><option value="">{zh ? "选择型号" : "Select model"}</option>{robotModels.map((item) => <option key={item.name} value={item.name}>{item.displayName || item.name}</option>)}</select></label>
                <label>{zh ? "机器人配置" : "Robot class profile"}<select value={form.nodeClass} onChange={(event) => { const item = classProfiles[event.target.value]; update({ nodeClass: event.target.value, robotModel: item?.model || item?.labels?.["rlark.io/robot-model"] || form.robotModel, labels: undefined }); }}><option value="">{zh ? "选择 NodeClass" : "Select NodeClass"}</option>{Object.entries(classProfiles).filter(([, item]) => (item.kind === "Robot" || Boolean(item.model || item.labels?.["rlark.io/robot-model"])) && (!form.robotModel || (item.model || item.labels?.["rlark.io/robot-model"]) === form.robotModel)).map(([name, item]) => <option key={name} value={name}>{name}{item.model ? ` · ${item.model}` : ""}</option>)}</select></label>
                <label>{zh ? "机器人编号" : "Robot ID"}<input value={form.robotID} onChange={(event) => update({ robotID: event.target.value, labels: undefined })} maxLength={63} autoComplete="off" placeholder={robotIDOfNode(form.nodeName) || "robot-01"} /></label>
              </>}
              <label>{zh ? "固定隧道地址" : "Pinned tunnel address"}<input value={form.address} onChange={(event) => update({ address: event.target.value })} maxLength={15} autoComplete="off" placeholder={zh ? "不填自动分配" : "allocated when empty"} /></label>
              <label>{zh ? "上联网关" : "Uplink gateway"}<input value={form.uplink} onChange={(event) => update({ uplink: event.target.value })} maxLength={15} autoComplete="off" placeholder={zh ? "默认路由出不去时才填" : "when the default route does not lead out"} /></label>
              <label className="join-node-wide">{zh ? "节点标签（每行 key=value）" : "Node labels (key=value per line)"}
                <textarea value={labelText} onChange={(event) => update({ labels: event.target.value })} rows={4} spellCheck={false} />
                {labels.error && <small className="join-missing">{zh ? `看不懂这一行：${labels.error}` : `Cannot read: ${labels.error}`}</small>}
              </label>
              <label className="join-node-wide">{zh ? "污点（每行 key=value:Effect）" : "Taints (key=value:Effect per line)"}
                <textarea value={taintText} onChange={(event) => update({ taints: event.target.value })} rows={2} spellCheck={false} />
                {taints.error && <small className="join-missing">{zh ? `看不懂这一行：${taints.error}` : `Cannot read: ${taints.error}`}</small>}
              </label>
            </div>
          </details>
          {error && <div className="cert-error" role="alert">{error}</div>}
          <div className="join-method-actions">
            <button type="submit" className="primary-button" disabled={pending || !ready}>{pending ? (zh ? "发码中…" : "Issuing…") : (zh ? "发入网码" : "Issue code")}</button>
          </div>
          {issued && <IssuedJoinCode key={issued.code} issued={issued} zh={zh} />}
        </form>
        <aside className="join-method-side">
          <section className="join-method-side-card">
            <h4>{zh ? "入网流程" : "Join flow"}</h4>
            <ol className="join-method-steps">
              <li><b>1</b><span>{zh ? "生成一次性入网码" : "Generate a one-time code"}</span></li>
              <li><b>2</b><span>{zh ? "在机器上执行一行命令" : "Run one command on the machine"}</span></li>
              <li><b>3</b><span>{zh ? "节点 Ready 并登记到 RLark" : "Node Ready and known to RLark"}</span></li>
            </ol>
          </section>
          <section className="join-method-side-card">
            <h4>{zh ? "说明" : "Notes"}</h4>
            <span className="join-method-note"><KeyRound size={14} />{zh ? "RLark 只保存入网码的哈希。" : "Only the code's hash is kept."}</span>
            <span className="join-method-note"><RefreshCw size={14} />{zh ? "同名节点再发码会替换原来的入网记录。" : "A new code for a node replaces its admission."}</span>
            <span className="join-method-note"><Terminal size={14} />{zh ? "等待使用不代表已入网；执行命令后才开始。" : "Waiting does not mean joined; the join starts after the command runs."}</span>
          </section>
        </aside>
      </div>
      {admissions.length > 0 && <section className="join-method-records">
        <header className="join-method-records-head"><h4>{zh ? "最近入网码" : "Recent join codes"}</h4><span>{zh ? `${admissions.length} 条记录` : `${admissions.length} records`}</span></header>
        <div className="join-code-records">
          <div className="join-code-record is-head"><span>{zh ? "节点名" : "Node"}</span><span>{zh ? "状态" : "State"}</span><span>{zh ? "有效期" : "Valid until"}</span></div>
          {admissions.slice(0, 6).map((admission) => {
            const state = admissionState(admission);
            return <div className="join-code-record" key={`${admission.cluster_id}/${admission.node_name}/${admission.issued_at ?? ""}`}>
              <strong>{admission.node_name}</strong>
              <span><span className={`status ${state.tone}`}><i />{state.label}</span></span>
              <small>{new Date(admission.expires_at).toLocaleString()}</small>
            </div>;
          })}
        </div>
      </section>}
    </div>
  </section>;
}

// Adding a node is a page of its own, laid out as creating a cluster is:
// first the cluster and the way, then what that way needs.
// sshJoin, when given, opens the SSH tab on that join of its cluster (from the admission records).
export function JoinNodePage({ zh, reissued, sshJoin, onBack, onScanSettings }: { zh: boolean; reissued?: IssuedNodeAdmission; sshJoin?: { clusterID: string; name: string }; onBack: () => void; onScanSettings: () => void }) {
  const [clusters, setClusters] = useState<ClusterSummary[]>([]);
  const [siteNetwork, setSiteNetwork] = useState<SiteNetworkView>();
  const [clusterID, setClusterID] = useState(reissued?.admission.cluster_id ?? sshJoin?.clusterID ?? "");
  const [method, setMethod] = useState<Method>(reissued ? "code" : sshJoin ? "ssh" : "machines");
  const [error, setError] = useState("");
  const [selectedSSH, setSelectedSSH] = useState("");
  const [railReissued, setRailReissued] = useState<IssuedNodeAdmission>();
  const [machineSelection, setMachineSelection] = useState<string[]>([]);

  useEffect(() => {
    clustersApi.list<ClusterSummary>().then((list) => {
      const usable = list.filter((cluster) => !cluster.agentNamespaceConflict);
      setClusters(usable);
      setClusterID((current) => current || usable[0]?.id || "");
    }, (reason) => setError((reason as Error).message));
  }, []);
  useEffect(() => {
    setMachineSelection([]);
    setSelectedSSH("");
    setRailReissued(undefined);
  }, [clusterID]);
  useEffect(() => {
    siteNetworkApi.get().then(setSiteNetwork).catch(() => setSiteNetwork(undefined));
  }, []);
  // Discovery is reported per site: a cluster only sees the machines of the site it belongs to.
  const clusterSite = siteOfCluster(siteNetwork, clusterID);

  const methods: { id: Method; icon: ReactNode; title: string; note: string }[] = [
    { id: "machines", icon: <ListChecks size={15} />, title: zh ? "未加入的机器" : "Discovered machines", note: zh ? "从场地网络发现的机器中选择，由 RLark 自动登录并入网。" : "Select discovered machines for RLark to log in and join." },
    { id: "ssh", icon: <Terminal size={15} />, title: zh ? "SSH 入网" : "SSH join", note: zh ? "提供地址和 SSH 凭据，由 RLark 远程完成入网。" : "Provide an address and SSH credentials for RLark to join remotely." },
    { id: "code", icon: <KeyRound size={15} />, title: zh ? "入网码" : "Join code", note: zh ? "生成一次性入网码，在机器上执行一行命令。" : "Generate a one-time code and run one command on the machine." },
  ];

  const siteDiscovery = siteNetwork?.sites.find((site) => site.name === clusterSite)?.discovery;
  const siteHint = clusterSite
    ? `站点 ${clusterSite}${siteDiscovery?.enabled ? ` · 每 ${siteDiscovery.intervalSeconds} 秒扫描` : " · 扫描未开启"}`
    : "未关联站点";
  const clusterName = clusters.find((cluster) => cluster.id === clusterID)?.name ?? clusterID;

  return <div className="page-content resource-page create-cluster-page join-node-page">
    <header className="join-page-head">
      <div>
        <h2>{zh ? "加入节点" : "Add a node"}</h2>
        <p>{zh
          ? "把机器加入集群：勾选扫描发现的机器，确认 SSH 口令后交给 RLark 自动入网；普通电脑/GPU 可以直接用 SSH 或一次性入网码。"
          : "Add machines to a cluster: tick discovered machines, confirm the SSH password and let RLark join them. Ordinary computers/GPUs can join over SSH or with a one-time code."}</p>
      </div>
      <div className="join-page-actions"><button type="button" className="secondary-button" onClick={onBack}><ChevronLeft size={16} />{zh ? "返回场站节点" : "Back to site nodes"}</button></div>
    </header>

    <div className="join-toolbar">
      <label className="join-toolbar-field">{zh ? "目标集群" : "Target cluster"}
        <select value={clusterID} onChange={(event) => setClusterID(event.target.value)}>
          {!clusters.length && <option value="">{zh ? "暂无可用集群" : "No available clusters"}</option>}
          {clusters.map((cluster) => <option key={cluster.id} value={cluster.id}>{cluster.name} ({cluster.agentNamespace})</option>)}
        </select>
      </label>
      <div className="join-method-tabs" role="tablist" aria-label={zh ? "加入方式" : "Join method"}>
        {methods.map((entry) => <button key={entry.id} type="button" role="tab" id={`join-method-${entry.id}`} aria-controls={`join-panel-${entry.id}`} aria-selected={method === entry.id} tabIndex={method === entry.id ? 0 : -1} className={method === entry.id ? "is-active" : ""} onKeyDown={(event) => { const tabs = [...event.currentTarget.parentElement!.querySelectorAll<HTMLButtonElement>("[role=tab]")]; const index = tabs.indexOf(event.currentTarget); const next = event.key === "ArrowRight" ? tabs[(index + 1) % tabs.length] : event.key === "ArrowLeft" ? tabs[(index + tabs.length - 1) % tabs.length] : event.key === "Home" ? tabs[0] : event.key === "End" ? tabs[tabs.length - 1] : undefined; if (next) { event.preventDefault(); next.focus(); next.click(); } }} onClick={() => setMethod(entry.id)}>
          <span className="join-method-icon">{entry.icon}</span>
          <span className="join-method-copy"><strong>{entry.title}</strong></span>
        </button>)}
      </div>
      <div className="join-toolbar-context">
        <span className="join-site-hint"><i className={siteDiscovery?.enabled ? "is-on" : ""} />{siteHint}</span>
        <button type="button" className="secondary-button" onClick={onScanSettings}>{zh ? "扫描设置" : "Scan settings"}</button>
      </div>
    </div>
    {error && <div className="cert-error" role="alert">{error}</div>}

    <div className="join-body"><main className="join-main">
      <section className="join-workspace" role="tabpanel" id={`join-panel-${method}`} aria-labelledby={`join-method-${method}`}>
        {method === "machines" ? <UnjoinedMachines key={`${clusterID}:${clusterSite}`} zh={zh} clusterID={clusterID} clusterName={clusterName} site={clusterSite} ticked={machineSelection} setTicked={setMachineSelection} onViewProgress={() => document.querySelector<HTMLElement>(".join-rail")?.scrollIntoView({ behavior: "smooth", block: "start" })} onUseMethod={setMethod} onOpenSSHJoin={(record) => { setSelectedSSH(record); setMethod("ssh"); }} />
          : method === "ssh" ? <SSHJoin key={clusterID} zh={zh} clusterID={clusterID} initial={clusterID === sshJoin?.clusterID ? sshJoin.name : undefined} selectedOperation={selectedSSH} />
          : <JoinCode key={`${clusterID}:${railReissued?.code ?? ""}`} zh={zh} clusterID={clusterID} reissued={railReissued ?? (clusterID === reissued?.admission.cluster_id ? reissued : undefined)} />}
      </section>
    </main><aside className="join-rail-wrap"><JoinOperationRail zh={zh} clusterID={clusterID} onOpenSSH={(name) => { setSelectedSSH(name); setMethod("ssh"); }} onOpenMachines={() => setMethod("machines")} onReissue={(issued) => { setRailReissued(issued); setMethod("code"); }} /></aside></div>
  </div>;
}
