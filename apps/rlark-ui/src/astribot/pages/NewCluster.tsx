import { useEffect, useState } from "react";
import { Server, Shield } from "lucide-react";
import type { Lang } from "../../i18n";
import { clusterProvisionsApi, selfHealApi } from "../api";
import { StageList } from "../components/StageList";
import { useAction } from "../components/useAction";
import {
  defaultClusterName,
  newClusterProblem,
  provisionView,
  type ClusterProvision,
  type NewClusterForm,
} from "../lib/clusterProvision";

const emptyForm: NewClusterForm = { site: "", cluster: "", address: "", port: "22", user: "root", password: "", tunnelEndpoint: "" };

// NewClusterPage makes a new site's cluster from its first machine: RLark logs in to it, makes it the site's
// first hub and the cluster's control plane, and shows each stage as it goes, where it stopped and why.
export function NewClusterPage({ lang }: { lang: Lang }) {
  const zh = lang === "zh";
  const [form, setForm] = useState<NewClusterForm>(emptyForm);
  const [provisions, setProvisions] = useState<ClusterProvision[]>([]);
  const [selected, setSelected] = useState("");
  const [retryPassword, setRetryPassword] = useState("");
  const [retryUser, setRetryUser] = useState("");
  const { pending: busy, error, setError, run } = useAction();
  const [machineGone, setMachineGone] = useState(false);
  const [cleared, setCleared] = useState<string[]>([]);

  const refresh = async () => {
    try {
      setProvisions(await clusterProvisionsApi.list());
    } catch (e) {
      setError((e as Error).message);
    }
  };

  useEffect(() => {
    refresh();
    const timer = window.setInterval(refresh, 3000);
    return () => window.clearInterval(timer);
  }, []);

  const problem = newClusterProblem(form, lang);
  const current = provisions.find((p) => p.metadata?.name === selected) ?? provisions[0];
  const view = current ? provisionView(current, lang) : undefined;
  const act = (work: () => Promise<unknown>) => run(async () => {
    await work();
    await refresh();
  });
  const create = () => act(async () => {
    const made = await clusterProvisionsApi.create(form);
    setForm(emptyForm);
    setSelected(made.metadata?.name ?? "");
  });
  const retry = () => act(async () => {
    await clusterProvisionsApi.retry(current!.metadata!.name!, retryPassword, retryUser || current!.spec.machine.user);
    setRetryPassword("");
    setRetryUser("");
  });
  // A making that stopped (or what a deleted cluster's left) is taken away: the machine is put back, and the site,
  // the cluster's name and the machine can be used again.
  const clear = () => act(async () => {
    setCleared(await selfHealApi.clearClusterProvision(current!.metadata!.name!, machineGone ? "" : retryPassword, retryUser || current!.spec.machine.user, machineGone));
    setRetryPassword("");
    setRetryUser("");
    setMachineGone(false);
    setSelected("");
  });

  const field = (key: keyof NewClusterForm, labelZh: string, labelEn: string, placeholder: string, type = "text") => (
    <label>
      <span>{zh ? labelZh : labelEn}</span>
      <input
        type={type}
        value={form[key]}
        placeholder={placeholder}
        autoComplete={type === "password" ? "new-password" : "off"}
        onChange={(e) => setForm({ ...form, [key]: e.target.value })}
      />
    </label>
  );

  return (
    <div className="page-content resource-page create-cluster-page">
      <div className="section-heading create-cluster-heading">
        <div>
          <span className="eyebrow">
            <Shield size={13} />
            {zh ? "新建集群" : "New Cluster"}
          </span>
          <h2>{zh ? "新建集群" : "New Cluster"}</h2>
          <p>
            {zh
              ? "给出新场地的名字和第一台机器的 SSH。RLark 登录这台机器，把它做成新场地的第一台 Hub 和新集群的控制面，装好集群 agent、Station 和插件，最后跑一个验收 Job。口令只用于这一次，不保存。之后的机器用入网码或 SSH 入网加入这个集群。"
              : "Give the new site's name and its first machine's SSH. RLark logs in to the machine, makes it the site's first hub and the cluster's control plane, installs the cluster agent, Station and the add-ons, and runs a check Job. The password is used for this once and kept nowhere. Later machines join the cluster by a join code or over SSH."}
          </p>
        </div>
      </div>

      <div className="cluster-enrollment-flow">
        <section className="cluster-enrollment-card">
          <div className="cluster-enrollment-card-head">
            <span className="cluster-enrollment-step">01</span>
            <div>
              <strong>{zh ? "场地和第一台机器" : "Site and first machine"}</strong>
              <small>{zh
                ? "机器要有 systemd、tar 和 WireGuard 内核模块，入站放开隧道的 UDP 端口（默认 51820）。几台机器共用一个公网 IP 时，填云上转发给这台的 公网IP:UDP端口"
                : "The machine needs systemd, tar and the WireGuard module, with its tunnel's UDP port open inbound (51820 by default). For machines behind one public IP, give the public IP and the UDP port the cloud forwards to this one"}</small>
            </div>
          </div>
          <div className="cert-form new-cluster-form">
            {field("site", "场地名", "Site", "baidu")}
            {field("cluster", "集群名", "Cluster", form.site.trim() ? defaultClusterName(form.site) : "rlark-<site>-gpu")}
            {field("address", "机器 IP", "Machine IP", "1.2.3.4")}
            {field("port", "SSH 端口", "SSH port", "22")}
            {field("user", "SSH 账号", "SSH account", "root")}
            {field("password", "SSH 密码", "SSH password", "", "password")}
            {field("tunnelEndpoint", "隧道公网端点（UDP，选填）", "Tunnel endpoint (UDP, optional)", form.address.trim() ? `${form.address.trim()}:51820` : "1.2.3.4:51820")}
            <button className="primary-button" onClick={create} disabled={busy || problem !== ""} title={problem}>
              {busy ? (zh ? "提交中…" : "Starting…") : zh ? "新建集群" : "Make the cluster"}
            </button>
          </div>
          {error && <div className="cert-error">{error}</div>}
          {cleared.length > 0 && (
            <ol className="provision-cleared">
              {cleared.map((step) => <li key={step}>{step}</li>)}
            </ol>
          )}
        </section>

        {current && view && (
          <section className="cluster-enrollment-card provision-progress">
            <div className="cluster-enrollment-card-head">
              <span className="cluster-enrollment-step">02</span>
              <div>
                <strong>
                  {current.metadata?.name} · {current.spec.site} · {current.spec.machine.address}
                </strong>
                <small>
                  {view.phase === "Succeeded"
                    ? zh ? "已建好" : "Made"
                    : view.phase === "Failed"
                      ? zh ? "停在下面标红的一步" : "Stopped at the stage in red"
                      : zh ? "进行中" : "In progress"}
                </small>
              </div>
            </div>
            <StageList stages={view.stages} zh={zh} />
            {view.canRetry && (
              <div className="cert-form provision-retry">
                {view.retryNeedsPassword && (
                  <label>
                    <span>{zh ? "SSH 账号" : "SSH account"}</span>
                    <input value={retryUser || current.spec.machine.user} onChange={(e) => setRetryUser(e.target.value)} />
                  </label>
                )}
                {view.retryNeedsPassword && (
                  <label>
                    <span>{zh ? "SSH 密码（这一步要重新登录机器）" : "SSH password (this stage logs in again)"}</span>
                    <input type="password" autoComplete="new-password" value={retryPassword} onChange={(e) => setRetryPassword(e.target.value)} />
                  </label>
                )}
                <button className="primary-button" onClick={retry} disabled={busy || (view.retryNeedsPassword && !retryPassword)}>
                  {zh ? "从这一步重试" : "Retry from this stage"}
                </button>
              </div>
            )}
            {view.canClear && (
              <div className="provision-clear">
                <small>
                  {view.phase === "Succeeded"
                    ? zh ? "集群删除以后，这次新建留下的场地、名字和节点声明还占着：清掉后才能用同一个场地或集群名重新建。集群还在时不能清。" : "After the cluster is deleted, what its making declared still holds the site and the name: clear it to make them again. It is not cleared while the cluster is listed."
                    : zh ? "不想从这一步重试（机器或名字填错、要换机器）：撤销这次新建。RLark 撤掉机器上建到一半的控制面和隧道，删掉没建完就登记了的集群和凭据，去掉场地、名字和节点声明；之后可以重新建。" : "Not retrying (a wrong machine or name): take this making away. RLark undoes the half-made control plane and tunnel on the machine, deletes the Cluster and credentials it listed, and takes the site, name and node out of the SiteNetwork; then it can be made again."}
                </small>
                <div className="cert-form">
                  {view.clearReachesMachine && !view.retryNeedsPassword && !machineGone && (
                    <label>
                      <span>{zh ? "SSH 密码（要登录机器把它恢复原样）" : "SSH password (to put the machine back)"}</span>
                      <input type="password" autoComplete="new-password" value={retryPassword} onChange={(e) => setRetryPassword(e.target.value)} />
                    </label>
                  )}
                  {view.clearReachesMachine && (
                    <label className="provision-clear-gone">
                      <input type="checkbox" checked={machineGone} onChange={(e) => setMachineGone(e.target.checked)} />
                      <span>{zh ? "机器已经不在了（重装或退还）：只清 RLark 里的记录" : "The machine is gone (reinstalled or returned): clear RLark's records only"}</span>
                    </label>
                  )}
                  <button className="secondary-button danger" onClick={clear} disabled={busy || (view.clearReachesMachine && !machineGone && !retryPassword)}>
                    {view.phase === "Succeeded" ? (zh ? "清掉这次新建留下的声明" : "Clear what the making left") : (zh ? "撤销这次新建，清掉重来" : "Take this making away and start over")}
                  </button>
                </div>
              </div>
            )}
          </section>
        )}
      </div>

      {provisions.length > 1 && (
        <section className="signed-clusters-panel">
          <div className="signed-clusters-heading">
            <div>
              <h3>{zh ? "新建过的集群" : "Clusters made"}</h3>
            </div>
            <span>{provisions.length}</span>
          </div>
          <div className="cert-list">
            {provisions.map((p) => (
              <div key={p.metadata?.name} className="cert-list-item">
                <div
                  className={"cert-list-row" + (p === current ? " expanded" : "")}
                  onClick={() => setSelected(p.metadata?.name ?? "")}
                >
                  <span className="cert-list-icon">
                    <Server size={15} />
                  </span>
                  <span className="cert-list-name">{p.metadata?.name}</span>
                  <small className="cert-list-date">
                    {p.status?.phase} · {p.status?.stage}
                  </small>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
