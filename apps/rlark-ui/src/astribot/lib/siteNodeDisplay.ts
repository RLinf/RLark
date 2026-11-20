// GET /api/v1/site-nodes?cluster_id=: what the console reads of one machine's heartbeat, labels and site-network
// membership, joined by the Gateway.
import type { SiteNodeLink } from "./nodeNetwork.js";

export interface SiteNode {
  cluster_id: string;
  node_name: string;
  online: boolean;
  labels?: Record<string, string>;
  network?: { public_key?: string; links?: SiteNodeLink[]; link_summary?: string };
}

// Expired is also a code withdrawn before it was used.
export type NodeAdmissionState = "Pending" | "Redeemed" | "Joined" | "Expired";

// GET /api/v1/node-admissions?cluster_id=: never carries a code.
export interface NodeAdmission {
  cluster_id: string;
  node_name: string;
  state: NodeAdmissionState;
  reason?: string;
  labels?: Record<string, string>;
  expires_at: string;
  issued_by?: string;
  issued_at?: string;
  public_key?: string;
  tunnel_address?: string;
  // A machine joined by SSH from the console: user@host:port, and the host
  // key it showed the first time it was logged in to.
  ssh_login?: string;
  ssh_host_key?: string;
  // What RLark found of the machine's earlier attempt or node and put right by itself.
  self_healed?: string;
  // Redeemed longer ago than a join takes, and no node yet: reason says where it stopped.
  stuck?: boolean;
  // Where the node's SSH join stands (its NodeJoin), when it was joined so.
  join_phase?: string;
  join_stage?: string;
}

// What a refused SSH join says to an administrator, by the Gateway's reason.
export function sshJoinFailure(reason: string | undefined, error: string, zh: boolean): string {
  if (!zh) return error;
  const said: Record<string, string> = {
    Unreachable: "连不上这台机器的 SSH：检查 IP、端口，以及它是否开机、在网上。",
    LoginRejected: "账号或密码不对，机器拒绝了登录。",
    NoSudo: "这个账号没有 sudo 权限，换一个能 sudo 的账号。",
    SudoPasswordRejected: "这个账号的 sudo 不接受登录密码。",
    NodeName: "机器的主机名不能直接当节点名，请在“高级”里填一个节点名。",
  };
  return reason && said[reason] ? `${said[reason]}（${error}）` : error;
}

// The one answer that carries the code and the machine's command.
export interface IssuedNodeAdmission {
  admission: NodeAdmission;
  code: string;
  command: string;
}

// A WireGuard public key is public; the table shows its beginning, which is
// what `wg show <interface> public-key` on the machine starts with.
export function keyFingerprint(key?: string): string {
  return key ? `${key.slice(0, 10)}…` : "";
}

// The machine's kind, from the labels it was admitted with.
export function machineModel(labels?: Record<string, string>): string {
  if (!labels) return "";
  return labels["rlark.io/robot-model"] ?? labels["rlark.io/node-category"] ?? "";
}

export function admissionStateClass(state: NodeAdmissionState): string {
  return ({ Pending: "status-pending", Redeemed: "status-pending", Joined: "status-online", Expired: "status-offline" } as const)[state];
}

export function admissionStateLabel(state: NodeAdmissionState, zh: boolean): string {
  if (!zh) return state;
  return ({ Pending: "待使用", Redeemed: "入网中", Joined: "已入网", Expired: "已过期或作废" } as const)[state];
}

// One "key=value" per line; blank lines are skipped.
export function parseLabelLines(text: string): { labels: Record<string, string>; error?: string } {
  const labels: Record<string, string> = {};
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    const at = line.indexOf("=");
    if (at <= 0) return { labels, error: line };
    labels[line.slice(0, at).trim()] = line.slice(at + 1).trim();
  }
  return { labels };
}

// One "key[=value]:Effect" per line.
export function parseTaintLines(text: string): { taints: string[]; error?: string } {
  const taints: string[] = [];
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    if (!/^[^:=\s]+(=[^:\s]*)?:(NoSchedule|PreferNoSchedule|NoExecute)$/.test(line)) return { taints, error: line };
    taints.push(line);
  }
  return { taints };
}

// A robot's x86 is named after the robot: node s1u-09-x86 is robot s1u-09.
export const robotIDOfNode = (nodeName: string) => nodeName.trim().replace(/-x86$/, "");

// What a robot of the S1U build is admitted with.
export const s1uRobotLabels = (robotID: string) => [
  "rlark.io/node-category=robot",
  "rlark.io/robot-model=s1u",
  "rlark.io/robot-body=s1u",
  `rlark.io/robot-id=${robotID}`,
].join("\n");
export const robotTaint = "rlark.io/node-category=robot:NoSchedule";

// A machine RLark knows of (Gateway, /api/v1/station/machines): seen on its
// site's network by a hub, a node or not.
export interface Machine {
  name: string;
  site: string;
  node_class: string;
  phase: "Discovered" | "Admitted" | "Joined";
  problem?: string;
  message?: string;
  serial?: string;
  // When RLark last logged in to its entry device (the Orin) and reached the computer behind it (the x86), whose
  // host key it read.
  inspected_at?: string;
  node_host_key_fingerprint?: string;
  address?: string;
  // The hardware address of the entry device it was last seen through: what tells one's robots from others on the
  // same network.
  mac?: string;
  last_seen?: string;
  cluster_id?: string;
  node_name?: string;
  // It can be ticked: it was read and has no problem, or its join stopped.
  admissible: boolean;
  // An administrator's request for it is being acted on, until the join it starts succeeds or stops.
  joining: boolean;
  // The administrator's decision that it is to be a node: who, when, of which cluster.
  requested_by?: string;
  requested_at?: string;
  admit_cluster_id?: string;
  // Where the NodeOperation of the present request stands: its phase and the last stage it completed.
  join?: { phase?: string; stage?: string };
}

// One password every machine of a NodeClass has: the value of a secret kept
// in RLark. The Gateway says whether it has been entered, never what it is.
export interface NodeClassPassword {
  secret: string;
  key: string;
  configured: boolean;
  set_by?: string;
}

export interface NodeClassAccess {
  entry: NodeClassPassword;
  node: NodeClassPassword;
}

export interface MachineList {
  items: Machine[];
  // The labels of each NodeClass, and which of its passwords have been entered, by its name.
  classes: Record<string, { labels?: Record<string, string>; access?: NodeClassAccess }>;
}

export interface AdmittedMachine {
  name: string;
  admitted: boolean;
  error?: string;
}

const machineProblems: Record<string, [string, string]> = {
  CredentialRejected: ["口令不符", "Password refused"],
  HostKeyChanged: ["主机密钥变了", "Host key changed"],
  NodeUnreachable: ["Orin 通，x86 不通", "Computer behind the entry device does not answer"],
  EntryUnreachable: ["连不上", "Does not answer"],
  HubUnreachable: ["场地 Hub 不应答", "The site's hub does not answer"],
  InspectFailed: ["读取失败", "Could not be read"],
  DuplicateSerial: ["序列号重复", "Duplicate serial"],
  AlreadyNode: ["已是别处的节点", "A node elsewhere already"],
  ClassIncomplete: ["机型声明不全", "Node class incomplete"],
  AccessUnconfigured: ["该机型的 x86 口令未配置", "The class's x86 password has not been entered"],
  JoinBlocked: ["入网停住了", "Join stopped"],
};

// What the list says of a machine in one word or two: its problem, or where
// it is on its way to being a node.
export function machineStateLabel(machine: Pick<Machine, "phase" | "problem">, zh: boolean): string {
  if (machine.problem) {
    const known = machineProblems[machine.problem];
    return known ? known[zh ? 0 : 1] : machine.problem;
  }
  if (machine.phase === "Admitted") return zh ? "入网中" : "Joining";
  if (machine.phase === "Joined") return zh ? "已入网" : "Joined";
  return zh ? "未加入" : "Not joined";
}

// The machines the "add node" list of a cluster shows: every one that is not a node yet and is not being joined to
// this cluster (those are in its join progress), the ones that can be ticked first, and among them the newest
// sighting first.
export function unjoinedMachines(machines: Machine[], clusterID: string): Machine[] {
  return machines.filter((machine) => machine.phase !== "Joined" && !(machine.joining && machine.admit_cluster_id === clusterID)).sort((a, b) =>
    Number(b.admissible) - Number(a.admissible) || (b.last_seen ?? "").localeCompare(a.last_seen ?? ""));
}

export type ReachState = "ok" | "fail" | "checking" | "unknown";

// Whether RLark got through to one computer of a machine, and why not.
export interface Reach {
  state: ReachState;
  reason: string;
}

// The problems that say RLark did not get in to the entry device: it does not answer, from its site's hub either;
// the hub does not; the device refused the class's account or shows another key.
const entryFaults = ["EntryUnreachable", "HubUnreachable", "CredentialRejected", "HostKeyChanged"];

// Whether RLark got through to a machine's Orin (its serial was read) and, through it, to the x86 behind it (its
// host key was read), as the last login found. Nothing is known of the x86 while the Orin is not reached.
export function machineReach(machine: Machine, zh: boolean): { orin: Reach; x86: Reach } {
  const problem = machine.problem ?? "";
  const label = machineStateLabel(machine, zh);
  const checking: Reach = {
    state: "checking",
    reason: problem === "InspectFailed" ? `${label}${zh ? "，RLark 自动再试" : "; RLark tries again"}` : zh ? "RLark 还没登录读取" : "Not read by RLark yet",
  };
  let orin: Reach;
  if (entryFaults.includes(problem)) orin = { state: "fail", reason: label };
  else if (problem === "InspectFailed" || !machine.inspected_at || !machine.serial) orin = checking;
  else orin = { state: "ok", reason: zh ? `读到序列号 ${machine.serial}` : `Serial ${machine.serial} read` };
  let x86: Reach;
  if (orin.state === "fail") x86 = { state: "unknown", reason: zh ? "Orin 不通，到不了 x86" : "Not reached: the entry device is not" };
  else if (problem === "NodeUnreachable") x86 = { state: "fail", reason: label };
  else if (orin.state === "checking") x86 = checking;
  else if (machine.node_host_key_fingerprint) x86 = { state: "ok", reason: zh ? `读到主机密钥 ${machine.node_host_key_fingerprint}` : `Host key ${machine.node_host_key_fingerprint} read` };
  else x86 = { state: "unknown", reason: problem ? label : (zh ? "没读到主机密钥" : "No host key read") };
  return { orin, x86 };
}

export function reachLabel(state: ReachState, zh: boolean): string {
  return { ok: zh ? "通" : "OK", fail: zh ? "不通" : "Unreachable", checking: zh ? "检查中" : "Checking", unknown: "—" }[state];
}

// Why a machine of the "add node" list cannot be ticked; empty when it can.
export function machineNotTickable(machine: Machine, zh: boolean): string {
  if (machine.admissible) return "";
  if (machine.joining) {
    return zh ? `正在加入集群 ${machine.admit_cluster_id}（${machine.requested_by} 提交）` : `Being joined to cluster ${machine.admit_cluster_id} (asked by ${machine.requested_by})`;
  }
  const { orin, x86 } = machineReach(machine, zh);
  if (orin.state === "fail") return `Orin ${reachLabel("fail", zh)}${zh ? "：" : ": "}${orin.reason}`;
  if (x86.state === "fail") return `x86 ${reachLabel("fail", zh)}${zh ? "：" : ": "}${x86.reason}`;
  if (orin.state === "checking") return zh ? "等 RLark 登录读取完" : "Waiting for RLark to read it";
  return machineStateLabel(machine, zh);
}

// The classes whose passwords the "add node" list shows: those that declare
// how their machines are logged in to, the ones still waiting for a password first.
export function machineClassAccess(classes: MachineList["classes"]): Array<{ name: string; access: NodeClassAccess }> {
  return Object.entries(classes).flatMap(([name, declared]) => declared.access ? [{ name, access: declared.access }] : [])
    .sort((a, b) => Number(a.access.node.configured) - Number(b.access.node.configured) || a.name.localeCompare(b.name));
}

// What the list says of one password of a class: entered or not, and where from.
export function classPasswordLabel(password: NodeClassPassword, zh: boolean): string {
  if (!password.configured) return zh ? "未配置" : "Not entered";
  const by = password.set_by ? (zh ? `，${password.set_by} 录入` : `, entered by ${password.set_by}`) : "";
  return zh ? `已配置（来自 ${password.secret}${by}）` : `Entered (in ${password.secret}${by})`;
}

// The labels an administrator may replace for the ticked machines: those
// their one class declares. Machines of several classes are admitted with
// each class's own.
export function machineLabelChoices(machines: Machine[], classes: MachineList["classes"]): Record<string, string> {
  const names = new Set(machines.map((machine) => machine.node_class));
  return names.size === 1 ? classes[[...names][0]]?.labels ?? {} : {};
}

// A robot's desired power is kept on its Node: the label rlark.io/robot-power (off | powered | ready, with the time it
// was set; without it a robot is wanted ready), which this page sets through the Gateway (PUT …/nodes/{name}/robot-power,
// which records who did) and the robot's model Addon (s1u-robot) converges on all the time.
// What the robot is now is the label rlark.io/power-state the Addon reports.
export type RobotPowerDesired = "off" | "powered" | "ready";
export type RobotPowerState = RobotPowerDesired | "estop" | "fault" | "unknown";
export type RobotPowerAction = "power_on" | "driver_on" | "driver_off" | "power_off";

export function robotPowerState(labels?: Record<string, string>): RobotPowerState | "" {
  return (labels?.["rlark.io/power-state"] as RobotPowerState | undefined) ?? "";
}

// The desired state in force, as the Addon reports it (rlark.io/power-desired: the label's state, or ready since a Run
// used the robot); a robot whose Addon has not reported yet is wanted ready.
export function robotPowerDesired(labels?: Record<string, string>): RobotPowerDesired {
  const value = labels?.["rlark.io/power-desired"];
  return value === "off" || value === "powered" ? value : "ready";
}

// A node is a robot when it carries a robot model: its desired power can be set whether or not its model Addon
// reports the power yet (one that has not started).
export function isRobotNode(labels?: Record<string, string>): boolean {
  return !!labels?.["rlark.io/robot-model"];
}

// The nodes a batch acts on: those of the chosen keys that are robots.
export function robotPowerNodes<T extends { key: string; labels?: Record<string, string> }>(nodes: T[], chosen: Set<string>): T[] {
  return nodes.filter((node) => chosen.has(node.key) && isRobotNode(node.labels));
}

export function robotPowerStateLabel(state: RobotPowerState, zh: boolean): string {
  return zh
    ? ({ off: "未上电", powered: "已上电，驱动未启动", ready: "已上电，驱动在跑", estop: "急停已按下", fault: "硬件报错", unknown: "读不到（厂商监控没有应答）" } as const)[state]
    : ({ off: "Off", powered: "Powered, driver stopped", ready: "Powered, driver running", estop: "Emergency stop pressed", fault: "Hardware error", unknown: "Unknown (the vendor's monitor does not answer)" } as const)[state];
}

export function robotPowerStateClass(state: RobotPowerState): string {
  return ({ off: "status-offline", powered: "status-pending", ready: "status-online", estop: "status-error", fault: "status-error", unknown: "status-pending" } as const)[state];
}

// What each of the page's four buttons makes the desired state: powering on leaves a driver that is wanted running
// wanted, stopping the driver leaves the motors on.
export function robotPowerAfter(action: RobotPowerAction, desired: RobotPowerDesired): RobotPowerDesired {
  return ({ power_on: desired === "off" ? "powered" : desired, driver_on: "ready", driver_off: desired === "off" ? "off" : "powered", power_off: "off" } as const)[action];
}

// Where the robot is with a maintenance its Addon does by itself while nobody uses it (a restart its node profile
// needs or a person asked for, the Orin's switch of a payload): waiting | queued | lowering | acting | failed; ""
// where there is none. What it is and why it waits is the Node's annotation rlark.io/maintenance.
export function robotMaintenanceState(labels?: Record<string, string>): string {
  const state = labels?.["rlark.io/maintenance-state"] ?? "";
  return state === "none" ? "" : state;
}

export function robotMaintenanceLabel(state: string, zh: boolean): string {
  return zh
    ? ({ waiting: "要做，在等（有人在用、没轮到或急停）", queued: "排队中", lowering: "正在降下来（停驱动、下电）", acting: "进行中", failed: "失败，已恢复原期望" } as Record<string, string>)[state] ?? state
    : ({ waiting: "Due, waiting (in use, not its turn, or stopped)", queued: "Queued", lowering: "Coming down (driver, motors)", acting: "Under way", failed: "Failed, back at its desired state" } as Record<string, string>)[state] ?? state;
}

// ok | failed | pending (none yet); "" where the node's Addon does not report one.
export function robotSelfCheckState(labels?: Record<string, string>): string {
  return labels?.["rlark.io/self-check-state"] ?? "";
}

export function robotSelfCheckLabel(state: string, zh: boolean): string {
  if (!zh) return state;
  return ({ ok: "通过", failed: "没过", pending: "还没有结果" } as Record<string, string>)[state] ?? state;
}

// The machine a robot node was joined from, when no hub has seen its entry for ten minutes: removing the node then
// leaves nothing that can join it again from here (its tunnel goes with it), only someone at the machine.
export function unseenMachineOf(machines: Machine[], clusterID: string, nodeName: string, now: number): Machine | undefined {
  const machine = machines.find((item) => item.cluster_id === clusterID && item.node_name === nodeName);
  return machine && !machineUnseen(machine, now) ? undefined : machine;
}

// No hub has seen the machine's entry device for ten minutes (a hub looks at its site's network every 30 s).
export function machineUnseen(machine: Pick<Machine, "last_seen">, now: number): boolean {
  return !(now - Date.parse(machine.last_seen ?? "") < 10 * 60 * 1000);
}

const two = (value: number) => String(value).padStart(2, "0");

// When a hub last saw a machine, in local time: the time alone today, with its date before.
export function seenTime(lastSeen: string | undefined, now: number): string {
  if (!lastSeen) return "—";
  const seen = new Date(lastSeen);
  const today = new Date(now);
  const time = `${two(seen.getHours())}:${two(seen.getMinutes())}`;
  if (seen.toDateString() === today.toDateString()) return `${time}:${two(seen.getSeconds())}`;
  const date = `${two(seen.getMonth() + 1)}-${two(seen.getDate())} ${time}`;
  return seen.getFullYear() === today.getFullYear() ? date : `${seen.getFullYear()}-${date}`;
}

// How long no hub has seen the machine, in its largest unit.
export function unseenFor(lastSeen: string | undefined, now: number, zh: boolean): string {
  const minutes = Math.floor((now - Date.parse(lastSeen ?? "")) / 60000);
  if (!Number.isFinite(minutes)) return zh ? "没有被看到过" : "never seen";
  const [count, zhUnit, enUnit] = minutes < 120 ? [minutes, "分钟", "min"] : minutes < 48 * 60 ? [Math.floor(minutes / 60), "小时", "h"] : [Math.floor(minutes / 1440), "天", "d"];
  return zh ? `Hub 已 ${count} ${zhUnit}没看到` : `not seen by a hub for ${count} ${enUnit}`;
}
