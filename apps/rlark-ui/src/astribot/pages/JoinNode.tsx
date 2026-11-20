import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  Check,
  ChevronLeft,
  Fingerprint,
  KeyRound,
  ListChecks,
  RefreshCw,
  Server,
  ShieldCheck,
  Terminal,
  TerminalSquare,
} from "lucide-react";
import { ApiError } from "../../api";
import { clustersApi } from "../../backend";
import { machinesApi, nodeAdmissionsApi, nodeJoinsApi } from "../api";
import { RetryClear } from "../components/RetryClear";
import { StageList } from "../components/StageList";
import { useAction } from "../components/useAction";
import type { ClusterSummary } from "../../types";
import { CopyButton } from "../components/CopyButton";
import { tunnelEndpointProblem } from "../lib/clusterProvision";
import { joinStageLabel, nodeJoinView, type NodeJoin } from "../lib/nodeJoin";
import {
  classPasswordLabel,
  machineClassAccess,
  machineLabelChoices,
  machineStateLabel,
  parseLabelLines,
  parseTaintLines,
  robotIDOfNode,
  robotTaint,
  s1uRobotLabels,
  sshJoinFailure,
  unjoinedMachines,
  type AdmittedMachine,
  type IssuedNodeAdmission,
  type MachineList,
  type NodeClassAccess,
} from "../lib/siteNodeDisplay";
import "./JoinNode.css";

const ttlOptions = [
  { seconds: 7200, zh: "2 小时", en: "2 hours" },
  { seconds: 86400, zh: "24 小时", en: "24 hours" },
  { seconds: 604800, zh: "7 天", en: "7 days" },
];

type Method = "machines" | "ssh" | "code";

function StepHead({ step, title, note }: { step: string; title: string; note: string }) {
  return <div className="cluster-enrollment-card-head">
    <span className="cluster-enrollment-step">{step}</span>
    <div><strong>{title}</strong><small>{note}</small></div>
  </div>;
}

// What a join answered, laid out as the console's other results (a signed
// cluster's YAML): a header, then blocks with a copy button.
function Result({ title, note, children }: { title: string; note: string; children: ReactNode }) {
  const shown = useRef<HTMLElement>(null);
  useEffect(() => {
    shown.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, []);
  return <section ref={shown} className="cert-result cluster-enrollment-result join-node-result" aria-live="polite">
    <div className="cert-result-header">
      <div><Check size={18} /><strong>{title}</strong></div>
      <small>{note}</small>
    </div>
    {children}
  </section>;
}

function Block({ title, value, zh }: { title: string; value: string; zh: boolean }) {
  return <div className="cert-yaml-block cluster-yaml-card">
    <div className="cert-yaml-head">
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

// The machines the hubs of the sites have found and that are not nodes yet:
// ticking them is all it takes. RLark names each one, logs in to it with the
// accounts of its class and joins it; nobody types anything at the machine.
function UnjoinedMachines({ zh, clusterID }: { zh: boolean; clusterID: string }) {
  const [list, setList] = useState<MachineList>();
  const [ticked, setTicked] = useState<string[]>([]);
  const [labels, setLabels] = useState<Record<string, string>>({});
  const { pending, error, setError, run } = useAction();
  const [answers, setAnswers] = useState<AdmittedMachine[]>([]);

  const load = (signal?: AbortSignal) => machinesApi.list(signal).then(setList, (reason) => {
    if (!signal?.aborted) setError((reason as Error).message);
  });
  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    // A machine that was just plugged in shows up within a minute.
    const timer = window.setInterval(() => void load(controller.signal), 10000);
    return () => {
      controller.abort();
      window.clearInterval(timer);
    };
  }, []);

  const machines = unjoinedMachines(list?.items ?? []);
  const chosen = machines.filter((machine) => machine.admissible && ticked.includes(machine.name));
  const choices = machineLabelChoices(chosen, list?.classes ?? {});
  const admissible = machines.filter((machine) => machine.admissible).map((machine) => machine.name);
  const admitted = answers.filter((answer) => answer.admitted);

  const submit = () => run(async () => {
    const replaced = Object.fromEntries(Object.entries(labels).filter(([key, value]) => key in choices && value.trim() && value.trim() !== choices[key]).map(([key, value]) => [key, value.trim()]));
    const answer = await machinesApi.admit({
      cluster_id: clusterID,
      machines: chosen.map((machine) => machine.name),
      ...(Object.keys(replaced).length ? { labels: replaced } : {}),
    });
    setAnswers(answer.machines);
    setTicked([]);
    await load();
  });

  return <>
    {machineClassAccess(list?.classes ?? {}).map(({ name, access }) => <ClassPasswords key={name} zh={zh} name={name} access={access} onSaved={() => load()} />)}
    <div className="table-panel join-machines-panel">
      <div className="storage-table-heading">
        <div>
          <strong>{zh ? "未加入的机器" : "Machines that have not joined"}</strong>
          <small>{zh ? "各场地的 Hub 每 30 秒扫一遍场地网" : "Every site's hub looks at its network every 30 s"}</small>
        </div>
        <button type="button" className="secondary-button" onClick={() => void load()}><RefreshCw size={14} />{zh ? "刷新" : "Refresh"}</button>
      </div>
      {list && machines.length === 0 ? <div className="join-machines-empty muted">
        {zh ? "没有扫到未加入的机器。新机器插上网线后一分钟内出现在这里；场地的第一台用 SSH 入网或入网码。" : "No machine that has not joined was found. A new machine shows up here within a minute of being plugged in; a site's first machine joins over SSH or with a code."}
      </div> : <div className="join-machines-scroll"><table className="stations-table">
        <thead>
          <tr>
            <th><input type="checkbox" aria-label={zh ? "全选" : "Select all"} checked={admissible.length > 0 && admissible.every((name) => ticked.includes(name))} disabled={admissible.length === 0 || pending}
              onChange={(event) => setTicked(event.target.checked ? admissible : [])} /></th>
            <th>{zh ? "序列号" : "Serial"}</th><th>{zh ? "机型" : "Class"}</th><th>{zh ? "场地 / 地址" : "Site / address"}</th><th>{zh ? "发现于" : "Seen"}</th><th>{zh ? "状态" : "State"}</th>
          </tr>
        </thead>
        <tbody>
          {machines.map((machine) => <tr key={machine.name} className={machine.admissible ? "" : "is-disabled"}>
            <td><input type="checkbox" aria-label={machine.serial || machine.name} checked={ticked.includes(machine.name)} disabled={!machine.admissible || pending}
              onChange={(event) => setTicked((current) => event.target.checked ? [...current, machine.name] : current.filter((name) => name !== machine.name))} /></td>
            <td><code className="inline-code">{machine.serial || "—"}</code></td>
            <td>{machine.node_class}</td>
            <td>{machine.site} · {machine.address}</td>
            <td>{machine.last_seen ? new Date(machine.last_seen).toLocaleTimeString() : "—"}</td>
            <td title={machine.message}>
              <span className={machine.problem ? "status status-failed" : "status status-pending"}><i />{machineStateLabel(machine, zh)}</span>
              {machine.node_name && <small className="muted"> {machine.node_name}</small>}
            </td>
          </tr>)}
        </tbody>
      </table></div>}
    </div>
    {Object.keys(choices).length > 0 && <details className="join-node-advanced">
      <summary>{zh ? "这几台和机型预设不一样的地方" : "Where these machines differ from their class"}</summary>
      <div className="form-grid">
        {Object.entries(choices).map(([key, value]) => <label key={key}>{key}
          <input value={labels[key] ?? value} onChange={(event) => setLabels((current) => ({ ...current, [key]: event.target.value }))} maxLength={63} autoComplete="off" />
        </label>)}
      </div>
    </details>}
    {answers.filter((answer) => !answer.admitted).map((answer) => <div key={answer.name} className="cert-error" role="alert">{answer.name}: {answer.error}</div>)}
    {error && <div className="cert-error" role="alert">{error}</div>}
    <div className="join-node-actions">
      <button type="button" className="primary-button" onClick={() => void submit()} disabled={pending || chosen.length === 0 || !clusterID}>
        {pending ? (zh ? "提交中…" : "Admitting…") : (zh ? `加入所选 ${chosen.length} 台` : `Add the ${chosen.length} selected`)}
      </button>
    </div>
    {admitted.length > 0 && <Result
      title={zh ? `已开始加入 ${admitted.length} 台` : `${admitted.length} machine(s) joining`}
      note={zh ? "进度在场站节点页的“入网记录”里" : "Progress is in the admission records"}
    >
      <div className="join-node-admitted">{admitted.map((answer) => <code key={answer.name} className="inline-code">{answer.name}</code>)}</div>
    </Result>}
  </>;
}

// A machine RLark logs in to with the account typed here, and joins in stages: the Gateway records them on the
// join (NodeJoin), so the list below shows every join of the cluster that runs or has lately run, as it goes, from
// any browser and after the page is left. The password is sent with the request that starts or retries a join, and
// forgotten.
function SSHJoin({ zh, clusterID, initial }: { zh: boolean; clusterID: string; initial?: string }) {
  const lang = zh ? "zh" : "en";
  const [form, setForm] = useState({ address: "", user: "", password: "", port: "22", nodeName: "", tunnelEndpoint: "" });
  const { pending, error, run } = useAction();
  // The last poll's failure, shown only until a poll succeeds again (a Gateway that restarts).
  const [pollError, setPollError] = useState("");
  const [joins, setJoins] = useState<NodeJoin[]>([]);
  const [selected, setSelected] = useState(initial ?? "");
  // Whether the shown join is the operator's to choose: from the first list on, or once a join is named.
  const chosen = useRef(Boolean(initial));
  const [cleared, setCleared] = useState<string[]>([]);
  const update = (patch: Partial<typeof form>) => setForm((current) => ({ ...current, ...patch }));
  const port = Number(form.port);
  const endpointProblem = tunnelEndpointProblem(form.tunnelEndpoint, lang);
  const ready = clusterID && form.address.trim() && form.user.trim() && form.password && Number.isInteger(port) && port >= 1 && port <= 65535 && !endpointProblem;

  const refresh = async () => {
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
  };
  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 3000);
    return () => window.clearInterval(timer);
  }, [clusterID]);

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
        update({ password: "" });
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

  return <>
    <form onSubmit={(event) => { event.preventDefault(); void submit(); }}>
      <div className="form-grid join-ssh-grid">
        <label>{zh ? "IP" : "IP"} *<input value={form.address} onChange={(event) => update({ address: event.target.value })} maxLength={253} autoComplete="off" placeholder="10.11.107.154" disabled={pending} /></label>
        <label>{zh ? "账号" : "Account"} *<input value={form.user} onChange={(event) => update({ user: event.target.value })} maxLength={64} autoComplete="off" disabled={pending} /></label>
        <label>{zh ? "密码" : "Password"} *<input type="password" value={form.password} onChange={(event) => update({ password: event.target.value })} maxLength={4096} autoComplete="new-password" disabled={pending} /></label>
      </div>
      <details className="join-node-advanced">
        <summary>{zh ? "高级" : "Advanced"}</summary>
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
      <div className="cluster-enrollment-notes">
        <span><KeyRound size={16} />{zh ? "密码只用于这次入网，不保存" : "The password is used by this join only and kept nowhere"}</span>
        <span><ShieldCheck size={16} />{zh ? "首次连接记下主机指纹" : "The host key is recorded on first connect"}</span>
        <span><Terminal size={16} />{zh ? "分步入网，离开这页也照常进行" : "Joins in stages, and goes on when this page is left"}</span>
      </div>
      {error && <div className="cert-error" role="alert">{error}</div>}
      {pollError && <div className="cert-error" role="status">{zh ? `读不到入网记录：${pollError}` : `The joins cannot be read: ${pollError}`}</div>}
      {cleared.length > 0 && <ol className="provision-cleared">{cleared.map((step) => <li key={step}>{step}</li>)}</ol>}
      <div className="join-node-actions">
        <button type="submit" className="primary-button" disabled={pending || !ready}>{pending ? (zh ? "提交中…" : "Starting…") : (zh ? "一键入网" : "Join")}</button>
      </div>
    </form>
    {joins.length > 0 && <div className="join-ssh-records">
      <strong>{zh ? "SSH 入网记录" : "SSH joins"}</strong>
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
            <RetryClear zh={zh} view={view} clearReach={zh ? "要登录机器撤掉隧道" : "to take the tunnel away"} user={j.spec.machine.user} pending={pending} run={run}
              retry={async (user, password) => {
                await nodeJoinsApi.retry(selected, password, user);
                await refresh();
              }}
              clear={async (user, password, machineGone) => {
                setCleared(await nodeJoinsApi.clear(selected, password, user, machineGone));
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
    </div>}
  </>;
}

// A one-time code for a machine RLark cannot log in to: one line on the
// machine joins it. A robot's labels and taint follow from its node name;
// they are under Advanced, for the rare machine that differs.
function JoinCode({ zh, clusterID, reissued }: { zh: boolean; clusterID: string; reissued?: IssuedNodeAdmission }) {
  const [form, setForm] = useState({
    nodeName: "", kind: "robot" as "robot" | "other", site: "", ttl: 7200,
    robotID: "", labels: undefined as string | undefined, taints: undefined as string | undefined, address: "", uplink: "",
  });
  const { pending, error, run } = useAction();
  const [issued, setIssued] = useState(reissued);
  const update = (patch: Partial<typeof form>) => setForm((current) => ({ ...current, ...patch }));

  const robot = form.kind === "robot";
  const robotID = form.robotID.trim() || robotIDOfNode(form.nodeName);
  const labelText = form.labels ?? (robot ? s1uRobotLabels(robotID) : "");
  const taintText = form.taints ?? (robot ? robotTaint : "");
  const labels = parseLabelLines(labelText);
  const taints = parseTaintLines(taintText);
  const ready = clusterID && form.nodeName.trim() && !labels.error && !taints.error;

  const submit = () => !pending && ready && run(async () => setIssued(await nodeAdmissionsApi.issue({
    cluster_id: clusterID,
    node_name: form.nodeName.trim(),
    ...(form.site.trim() ? { site: form.site.trim() } : {}),
    ...(form.address.trim() ? { address: form.address.trim() } : {}),
    ...(form.uplink.trim() ? { uplink: form.uplink.trim() } : {}),
    ...(robot ? { node_class: "s1u" } : {}),
    ...(Object.keys(labels.labels).length ? { labels: labels.labels } : {}),
    ...(taints.taints.length ? { taints: taints.taints } : {}),
    ttl_seconds: form.ttl,
  })));

  return <form onSubmit={(event) => { event.preventDefault(); void submit(); }}>
    <div className="form-grid">
      <label>{zh ? "节点名" : "Node name"} *<input value={form.nodeName} onChange={(event) => update({ nodeName: event.target.value })} maxLength={253} autoComplete="off" placeholder="s1u-09-x86" /><small className="muted">{zh ? "Kubernetes 节点名（调度用，加入后不可改，之后只能改显示名）" : "The Kubernetes Node name (used for scheduling; it cannot change after the join, only the display name can)"}</small></label>
      <label>{zh ? "机器类型" : "Machine kind"}
        <select value={form.kind} onChange={(event) => update({ kind: event.target.value as "robot" | "other", labels: undefined, taints: undefined })}>
          <option value="robot">{zh ? "S1U 机器人" : "S1U robot"}</option>
          <option value="other">{zh ? "其他机器" : "Other machine"}</option>
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
      <summary>{zh ? "高级" : "Advanced"}</summary>
      <div className="form-grid">
        {robot && <label>{zh ? "机器人编号" : "Robot ID"}<input value={form.robotID} onChange={(event) => update({ robotID: event.target.value })} maxLength={63} autoComplete="off" placeholder={robotIDOfNode(form.nodeName) || "s1u-09"} /></label>}
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
    <div className="cluster-enrollment-notes">
      <span><KeyRound size={16} />{zh ? "管理面只保存入网码的哈希" : "Only the code's hash is kept"}</span>
      <span><RefreshCw size={16} />{zh ? "同名节点再发码会替换原来的入网记录" : "A new code for a node replaces its admission"}</span>
    </div>
    {error && <div className="cert-error" role="alert">{error}</div>}
    <div className="join-node-actions">
      <button type="submit" className="primary-button" disabled={pending || !ready}>{pending ? (zh ? "发码中…" : "Issuing…") : (zh ? "发入网码" : "Issue code")}</button>
    </div>
    {issued && <IssuedJoinCode key={issued.code} issued={issued} zh={zh} />}
  </form>;
}

// Adding a node is a page of its own, laid out as creating a cluster is:
// first the cluster and the way, then what that way needs.
// sshJoin, when given, opens the SSH tab on that join of its cluster (from the admission records).
export function JoinNodePage({ zh, reissued, sshJoin, onBack }: { zh: boolean; reissued?: IssuedNodeAdmission; sshJoin?: { clusterID: string; name: string }; onBack: () => void }) {
  const [clusters, setClusters] = useState<ClusterSummary[]>([]);
  const [clusterID, setClusterID] = useState(reissued?.admission.cluster_id ?? sshJoin?.clusterID ?? "");
  const [method, setMethod] = useState<Method>(reissued ? "code" : sshJoin ? "ssh" : "machines");
  const [error, setError] = useState("");

  useEffect(() => {
    clustersApi.list<ClusterSummary>().then((list) => {
      const usable = list.filter((cluster) => !cluster.agentNamespaceConflict);
      setClusters(usable);
      setClusterID((current) => current || usable[0]?.id || "");
    }, (reason) => setError((reason as Error).message));
  }, []);

  const methods: { id: Method; icon: ReactNode; title: string; note: string }[] = [
    { id: "machines", icon: <ListChecks size={15} />, title: zh ? "未加入的机器" : "Machines found", note: zh ? "扫到的机器人打勾即可" : "Tick robots the site found" },
    { id: "ssh", icon: <Terminal size={15} />, title: zh ? "SSH 入网" : "Join over SSH", note: zh ? "填 IP、账号、密码，一键入网" : "IP, account, password, one click" },
    { id: "code", icon: <KeyRound size={15} />, title: zh ? "入网码" : "Join code", note: zh ? "在机器上执行一行命令" : "One line on the machine" },
  ];
  const chosen = methods.find((entry) => entry.id === method)!;

  return <div className="page-content resource-page create-cluster-page join-node-page">
    <div className="section-heading create-cluster-heading join-node-heading">
      <div>
        <span className="eyebrow"><Fingerprint size={13} />{zh ? "场站节点" : "Site Nodes"}</span>
        <h2>{zh ? "加入节点" : "Add a node"}</h2>
        <p>{zh
          ? "把机器加入集群：场地扫到的机器人打勾即可；新机器用 SSH 入网（填 IP、账号、密码）或入网码。入网后机器建好隧道，出现在场站节点列表里。"
          : "Add machines to a cluster: tick the robots a site has found; a new machine joins over SSH (IP, account, password) or with a code. Once joined, it brings up its tunnel and is listed under Site Nodes."}</p>
      </div>
      <button type="button" className="secondary-button" onClick={onBack}><ChevronLeft size={17} />{zh ? "返回" : "Back"}</button>
    </div>

    <div className="cluster-enrollment-flow storage-create-form">
      <section className="cluster-enrollment-card">
        <StepHead step="01" title={zh ? "集群和方式" : "Cluster and way"} note={zh ? "节点加入哪个集群，用哪种方式加入" : "The cluster the node joins, and how"} />
        <div className="form-grid">
          <label>{zh ? "集群" : "Cluster"}
            <select value={clusterID} onChange={(event) => setClusterID(event.target.value)}>
              {clusters.map((cluster) => <option key={cluster.id} value={cluster.id}>{cluster.name} ({cluster.agentNamespace})</option>)}
            </select>
          </label>
        </div>
        <div className="role-template selectable join-node-methods" role="radiogroup" aria-label={zh ? "加入方式" : "How to add"}>
          {methods.map((entry) => <button key={entry.id} type="button" role="radio" aria-checked={method === entry.id} className={method === entry.id ? "active" : ""} onClick={() => setMethod(entry.id)}>
            {entry.icon}{entry.title}<small>{entry.note}</small>
          </button>)}
        </div>
        {error && <div className="cert-error" role="alert">{error}</div>}
      </section>

      <section className="cluster-enrollment-card">
        <StepHead step="02" title={chosen.title} note={method === "machines"
          ? (zh ? "勾选后 RLark 自己登录、起名、入网，机器旁不用敲任何命令" : "RLark logs in, names and joins the ticked machines")
          : method === "ssh"
            ? (zh ? "RLark 登录这台机器，分步入网：预检、需要时先建隧道、发码并用 sudo 执行入网命令、等节点就绪" : "RLark logs in and joins the machine in stages: pre-check, its tunnel when it needs one, the code's line with sudo, the node ready")
            : (zh ? "给一台新机器发一次性入网码，在机器上执行一行命令" : "A one-time code; one line on the machine joins it")} />
        {method === "machines" ? <UnjoinedMachines key={clusterID} zh={zh} clusterID={clusterID} />
          : method === "ssh" ? <SSHJoin key={clusterID} zh={zh} clusterID={clusterID} initial={clusterID === sshJoin?.clusterID ? sshJoin.name : undefined} />
          : <JoinCode key={clusterID} zh={zh} clusterID={clusterID} reissued={clusterID === reissued?.admission.cluster_id ? reissued : undefined} />}
      </section>
    </div>
  </div>;
}
