import type { IClusterProvision } from "../../generated/rlark/rlinf.io/v1alpha1/ClusterProvision.js";

export type ClusterProvision = IClusterProvision;

export interface NewClusterForm {
  site: string;
  cluster: string;
  address: string;
  port: string;
  user: string;
  password: string;
  // The ip:port other sites dial for the machine's tunnel; empty, its IP at UDP 51820.
  tunnelEndpoint: string;
}

export type Lang = "zh" | "en";

// The stages the Gateway makes a cluster in, in order (astribot_cluster_provision.go).
export const provisionStages: { name: string; zh: string; en: string }[] = [
  { name: "precheck", zh: "预检", en: "Pre-check" },
  { name: "bundle", zh: "安装包和隧道密钥", en: "Installer and tunnel key" },
  { name: "declare", zh: "声明场地", en: "Declare the site" },
  { name: "control-plane", zh: "控制面", en: "Control plane" },
  { name: "agent", zh: "集群 agent", en: "Cluster agent" },
  { name: "station", zh: "Station 成员", en: "Station member" },
  { name: "addons", zh: "插件和存储", en: "Add-ons and storage" },
  { name: "accept", zh: "验收 Job", en: "Check Job" },
];

const label = /^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$/;

// A tunnel endpoint is empty (the machine's IP at UDP 51820) or an IPv4 ip:port: for machines behind one public
// address, that address and the UDP port the cloud forwards to the machine.
export function tunnelEndpointProblem(endpoint: string, lang: Lang): string {
  const value = endpoint.trim();
  const match = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3}):(\d{1,5})$/.exec(value);
  if (value === "" || match && match.slice(1, 5).every((octet) => Number(octet) <= 255) && Number(match[5]) >= 1 && Number(match[5]) <= 65535) {
    return "";
  }
  return lang === "zh" ? "隧道公网端点写成 公网IP:UDP端口，例如 180.76.174.250:8100" : "The tunnel endpoint is public-ip:udp-port, e.g. 180.76.174.250:8100";
}

export function defaultClusterName(site: string): string {
  return `rlark-${site.trim()}-gpu`;
}

// What is wrong with the form, or "" when it can be sent.
export function newClusterProblem(form: NewClusterForm, lang: Lang): string {
  const zh = lang === "zh";
  const site = form.site.trim();
  const cluster = form.cluster.trim() || defaultClusterName(site);
  const address = form.address.trim();
  if (!label.test(site)) {
    return zh ? "场地名只能用小写字母、数字和 -" : "The site is lower-case letters, digits and -";
  }
  if (!cluster.startsWith("rlark-") || !label.test(cluster)) {
    return zh ? "集群名以 rlark- 开头，只用小写字母、数字和 -" : "The cluster starts with rlark- and is lower-case letters, digits and -";
  }
  if (!address || /[\s/@:]/.test(address) && !/^[0-9a-fA-F:]+$/.test(address)) {
    return zh ? "填机器的 IP 或主机名" : "Give the machine's IP address or host name";
  }
  const port = Number(form.port);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    return zh ? "SSH 端口是 1-65535" : "The SSH port is 1-65535";
  }
  if (!form.user.trim() || !form.password) {
    return zh ? "填 SSH 账号和密码" : "Give the SSH account and password";
  }
  return tunnelEndpointProblem(form.tunnelEndpoint, lang);
}

// A retry from this stage logs in to the machine again: every stage up to Station's.
export function stageNeedsPassword(stage: string): boolean {
  const index = provisionStages.findIndex((s) => s.name === stage);
  return index >= 0 && index <= provisionStages.findIndex((s) => s.name === "station");
}

export type StageState = "Pending" | "Running" | "Done" | "Skipped" | "Failed";

export interface StageView {
  name: string;
  label: string;
  state: StageState;
  readback: string;
  message: string;
}

interface RecordedStage {
  name: string;
  state: StageState;
  readback?: string;
  message?: string;
}

// The stages of defs in their order, each as the status records it (Pending when it does not yet).
export function stagesView(defs: { name: string; zh: string; en: string }[], recorded: RecordedStage[] | undefined, lang: Lang): StageView[] {
  const byName = new Map((recorded ?? []).map((s) => [s.name, s]));
  return defs.map((stage) => {
    const s = byName.get(stage.name);
    return { name: stage.name, label: stage[lang], state: s?.state ?? "Pending", readback: s?.readback ?? "", message: s?.message ?? "" };
  });
}

export interface ProvisionView {
  phase: string;
  stages: StageView[];
  canRetry: boolean;
  retryNeedsPassword: boolean;
  // A making that does not run can be taken away: one that stopped, or what the making of a deleted cluster left.
  canClear: boolean;
  // It reached the machine (the installer was sent): clearing puts the machine back too, with its password.
  clearReachesMachine: boolean;
}

export function provisionView(p: ClusterProvision, lang: Lang): ProvisionView {
  const stages = stagesView(provisionStages, p.status?.stages, lang);
  const phase = p.status?.phase ?? "Running";
  return {
    phase,
    stages,
    canRetry: phase === "Failed",
    retryNeedsPassword: phase === "Failed" && stageNeedsPassword(p.status?.stage ?? ""),
    canClear: phase !== "Running",
    clearReachesMachine: stages.find((s) => s.name === "bundle")?.state !== "Pending",
  };
}
