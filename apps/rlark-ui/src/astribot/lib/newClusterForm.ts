import { defaultClusterName, newClusterProblem, provisionStages, tunnelEndpointProblem, type ClusterProvision, type Lang, type NewClusterForm } from "./clusterProvision.js";

export type NewClusterField = keyof NewClusterForm;

export function newClusterFieldProblems(form: NewClusterForm, lang: Lang): Partial<Record<NewClusterField, string>> {
  const problems: Partial<Record<NewClusterField, string>> = {};
  const zh = lang === "zh";
  if (!form.clusterType) problems.clusterType = zh ? "选择集群类型：GPU 集群或机器人集群" : "Choose the cluster type: GPU or robot";
  const site = form.site.trim();
  const cluster = form.cluster.trim() || (form.clusterType ? defaultClusterName(site, form.clusterType) : "");
  if (!/^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$/.test(site)) problems.site = zh ? "场地名只能用小写字母、数字和 -" : "The site is lower-case letters, digits and -";
  if (form.cluster.trim() && (!cluster.startsWith("rlark-") || !/^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$/.test(cluster))) problems.cluster = zh ? "集群名以 rlark- 开头，只用小写字母、数字和 -" : "The cluster starts with rlark- and is lower-case letters, digits and -";
  const address = form.address.trim();
  if (!address || /[\s/@:]/.test(address) && !/^[0-9a-fA-F:]+$/.test(address)) problems.address = zh ? "填机器的 IP 或主机名" : "Give the machine's IP address or host name";
  const port = Number(form.port);
  if (!Number.isInteger(port) || port < 1 || port > 65535) problems.port = zh ? "SSH 端口是 1-65535" : "The SSH port is 1-65535";
  if (!form.user.trim() || !form.password) {
    const credentialsProblem = zh ? "填 SSH 账号和密码" : "Give the SSH account and password";
    if (!form.user.trim()) problems.user = credentialsProblem;
    if (!form.password) problems.password = credentialsProblem;
  }
  const endpointProblem = tunnelEndpointProblem(form.tunnelEndpoint, lang);
  if (endpointProblem) problems.tunnelEndpoint = endpointProblem;
  // Keep the page's field messages aligned with the final request guard for a single invalid field.
  const finalProblem = newClusterProblem(form, lang);
  if (finalProblem && !Object.values(problems).includes(finalProblem) && form.clusterType && site && address && form.user.trim() && form.password) {
    problems.tunnelEndpoint = finalProblem;
  }
  return problems;
}

export function firstNewClusterProblem(problems: Partial<Record<NewClusterField, string>>): NewClusterField | undefined {
  return (["clusterType", "site", "cluster", "tunnelEndpoint", "address", "port", "user", "password"] as NewClusterField[]).find((key) => problems[key]);
}

export function newClusterSummary(form: NewClusterForm, lang: Lang) {
  const problems = newClusterFieldProblems(form, lang);
  return {
    clusterType: form.clusterType || (lang === "zh" ? "未选择" : "Not selected"),
    site: form.site.trim() || "—",
    cluster: form.cluster.trim() || (form.clusterType ? defaultClusterName(form.site.trim() || "<site>", form.clusterType) : "—"),
    machine: form.address.trim() || "—",
    remaining: Object.keys(problems).length,
  };
}

export function provisionProgress(provision: ClusterProvision | undefined) {
  if (!provision) return { completed: 0, total: provisionStages.length, percent: 0, stageNumber: 0, failedStage: "" };
  const stages = provision.status?.stages ?? [];
  const completed = stages.filter((stage) => stage.state === "Done" || stage.state === "Skipped").length;
  const currentIndex = provisionStages.findIndex((stage) => stage.name === provision.status?.stage);
  const stageNumber = currentIndex >= 0 ? currentIndex + 1 : Math.min(provisionStages.length, completed + 1);
  const failedStage = provision.status?.phase === "Failed" ? stages.find((stage) => stage.name === provision.status?.stage)?.message ?? "" : "";
  return { completed, total: provisionStages.length, percent: Math.round((completed / provisionStages.length) * 100), stageNumber, failedStage };
}
