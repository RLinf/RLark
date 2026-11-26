import { machineStateLabel, machineUnseen, seenTime, unseenFor, type Machine, type MachineList, type NodeClassAccess } from "./siteNodeDisplay.js";

// The steps of a ticked machine's join: station-controller logs in to read it and gives it its node name (the
// Machine is Admitted), then the NodeOperation that joins it goes through its stages
// (installerStages, astribot/station/controller/node_install.go).
export const machineJoinSteps: { name: string; zh: string; en: string }[] = [
  { name: "Admit", zh: "登录读取、分配节点名", en: "Read and named" },
  { name: "Preflight", zh: "登录 x86 预检", en: "Pre-check" },
  { name: "Uplink", zh: "Orin 上联", en: "Entry device uplink" },
  { name: "Push", zh: "发送安装包", en: "Installer sent" },
  { name: "Install", zh: "安装并入网", en: "Installed" },
  { name: "VerifyNode", zh: "节点 Ready", en: "Node Ready" },
  { name: "VerifyRLark", zh: "RLark 登记", en: "Known to RLark" },
  { name: "VerifyAddons", zh: "插件就绪", en: "Addons ready" },
  { name: "RotateEntryHostKey", zh: "Orin 换自己的主机密钥", en: "Entry device's own host key" },
];

// A machine that joined is shown for a day after it was asked for; one still to join or whose join stopped, as long
// as the request stands.
const shownFor = 24 * 60 * 60 * 1000;

// The problems of a machine that is not admitted yet which go away by themselves, and how often RLark tries again:
// one the machine does not answer, every 5 minutes; a fault of the site's hub or of the login, after a wait that
// grows to about 16 minutes (station-controller's retry).
const passing: Record<string, [string, string]> = {
  EntryUnreachable: ["RLark 每 5 分钟再试", "RLark tries again every 5 minutes"],
  NodeUnreachable: ["RLark 每 5 分钟再试", "RLark tries again every 5 minutes"],
  HubUnreachable: ["RLark 自动再试，间隔逐渐拉长到约 16 分钟", "RLark tries again, waiting longer each time, up to about 16 minutes"],
  InspectFailed: ["RLark 自动再试，间隔逐渐拉长到约 16 分钟", "RLark tries again, waiting longer each time, up to about 16 minutes"],
};

export type MachineJoinState = "joined" | "running" | "failed";

export interface MachineJoinRow {
  machine: Machine;
  state: MachineJoinState;
  // How many of machineJoinSteps are done.
  done: number;
  // The step under way, or where and why it stopped.
  text: string;
}

export interface MachineJoinProgress {
  rows: MachineJoinRow[];
  total: number;
  joined: number;
  running: number;
  failed: number;
}

function stepsDone(machine: Machine): number {
  if (machine.phase === "Joined") return machineJoinSteps.length;
  if (machine.phase !== "Admitted") return 0;
  // Named: the first step is done, and those of its join up to the last it completed.
  return Math.max(1, machineJoinSteps.findIndex((step) => step.name === machine.join?.stage) + 1);
}

// Why a machine stopped, in words: who cannot reach it and since when, or its problem and what RLark said.
function cause(machine: Machine, now: number, zh: boolean): string {
  const seen = seenTime(machine.last_seen, now);
  switch (machine.problem) {
    case "EntryUnreachable":
      return zh ? `Hub 连不到这台机器，上次看到是 ${seen}` : `The hub cannot reach the machine, last seen ${seen}`;
    case "HubUnreachable":
      return zh ? `场地的 Hub 不应答，上次看到这台机器是 ${seen}` : `The site's hub does not answer; the machine was last seen ${seen}`;
    case "NodeUnreachable":
      return zh ? "Orin 通，后面的 x86 不应答" : "The entry device answers, the computer behind it does not";
  }
  const label = machineStateLabel(machine, zh);
  return machine.message ? `${label}${zh ? "：" : ": "}${machine.message}` : label;
}

function row(machine: Machine, now: number, zh: boolean): MachineJoinRow {
  const done = stepsDone(machine);
  if (machine.phase === "Joined") {
    return { machine, state: "joined", done, text: zh ? `已加入，节点 ${machine.node_name ?? ""}` : `Joined as ${machine.node_name ?? ""}` };
  }
  const step = machineJoinSteps[Math.min(done, machineJoinSteps.length - 1)];
  const at = `${Math.min(done + 1, machineJoinSteps.length)}/${machineJoinSteps.length}`;
  if (machine.problem) {
    const why = cause(machine, now, zh);
    if (machine.phase === "Admitted") {
      return { machine, state: "failed", done, text: zh ? `停在第 ${at} 步「${step.zh}」：${why}。勾选它再点加入，重新入网` : `Stopped at step ${at}, ${step.en}: ${why}. Tick it and add it again to join it anew` };
    }
    const again = passing[machine.problem];
    return { machine, state: "failed", done, text: again ? `${why}${zh ? "；" : "; "}${again[zh ? 0 : 1]}` : why };
  }
  const unseen = machineUnseen(machine, now) ? (zh ? `（${unseenFor(machine.last_seen, now, zh)}）` : ` (${unseenFor(machine.last_seen, now, zh)})`) : "";
  return { machine, state: "running", done, text: (zh ? `第 ${at} 步：${step.zh}` : `Step ${at}: ${step.en}`) + unseen };
}

const order: Record<MachineJoinState, number> = { failed: 0, running: 1, joined: 2 };

// A request whose join succeeded while the machine is Discovered again made a node that was removed since: it is
// spent, and the machine waits to be ticked anew.
const spent = (machine: Machine) => machine.phase === "Discovered" && machine.join?.phase === "Succeeded";

// The machines admitted to a cluster that the "add node" page follows: each with the step it has reached, the ones
// that stopped first, and how many have joined, run and stopped.
export function machineJoinProgress(machines: Machine[], clusterID: string, now: number, zh: boolean): MachineJoinProgress {
  const rows = machines
    .filter((machine) => machine.admit_cluster_id === clusterID && !spent(machine) &&
      (machine.phase !== "Joined" || now - Date.parse(machine.requested_at ?? "") < shownFor))
    .map((machine) => row(machine, now, zh))
    .sort((a, b) => order[a.state] - order[b.state] || (a.machine.node_name ?? a.machine.serial ?? a.machine.name).localeCompare(b.machine.node_name ?? b.machine.serial ?? b.machine.name));
  const count = (state: MachineJoinState) => rows.filter((item) => item.state === state).length;
  return { rows, total: rows.length, joined: count("joined"), running: count("running"), failed: count("failed") };
}

export function machineJoinSummary(progress: MachineJoinProgress, zh: boolean): string {
  return zh
    ? `已加入 ${progress.joined} / 共 ${progress.total} 台，进行中 ${progress.running} 台，失败 ${progress.failed} 台`
    : `${progress.joined} of ${progress.total} joined, ${progress.running} under way, ${progress.failed} stopped`;
}

// The passwords a join needs are the class's, one set for every machine of it. The dialog groups the selected
// machines so one password is typed per class, never per machine.
export interface JoinCredentialGroup {
  name: string;
  machines: Machine[];
  access?: NodeClassAccess;
}

export interface JoinCredentialInput {
  password: string;
  sudo: string;
}

export function joinCredentialGroups(machines: Machine[], classes: MachineList["classes"]): JoinCredentialGroup[] {
  const grouped = new Map<string, Machine[]>();
  for (const machine of machines) {
    const list = grouped.get(machine.node_class) ?? [];
    list.push(machine);
    grouped.set(machine.node_class, list);
  }
  return [...grouped.entries()]
    .map(([name, list]) => ({ name, machines: list, access: classes[name]?.access }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

// Why the dialog cannot submit yet: a class whose entry device password is missing, or whose x86 password is neither
// stored nor typed. Empty when every group can be joined.
export function joinCredentialProblem(groups: JoinCredentialGroup[], credentials: Record<string, JoinCredentialInput>, zh: boolean): string {
  if (groups.length === 0) return zh ? "没有选中的机器。" : "No machines are selected.";
  for (const group of groups) {
    if (!group.access) return zh ? `机型 ${group.name} 没有声明登录方式。` : `NodeClass ${group.name} declares no access.`;
    if (!group.access.entry.configured) return zh ? `机型 ${group.name} 的 Orin 入口口令还没有配置，请先在密钥里配置。` : `The entry-device password of NodeClass ${group.name} is not configured. Configure it in Secrets first.`;
    if (!group.access.node.configured && !(credentials[group.name]?.password ?? "").trim()) {
      return zh ? `请填写机型 ${group.name} 的 x86 登录口令。` : `Enter the x86 password for NodeClass ${group.name}.`;
    }
  }
  return "";
}
