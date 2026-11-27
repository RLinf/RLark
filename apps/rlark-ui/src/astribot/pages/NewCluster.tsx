import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Eye, EyeOff, LoaderCircle, Server, Shield } from "lucide-react";
import type { Lang } from "../../i18n";
import { clusterProvisionsApi, clusterTypes, selfHealApi, type ClusterType } from "../api";
import { RetryClear } from "../components/RetryClear";
import { StageList } from "../components/StageList";
import { useAction } from "../components/useAction";
import { defaultClusterName, newClusterProblem, provisionView, type ClusterProvision, type NewClusterForm } from "../lib/clusterProvision";
import { firstNewClusterProblem, newClusterFieldProblems, newClusterSummary, provisionProgress, type NewClusterField } from "../lib/newClusterForm";
import "../components/provision.css";

const emptyForm: NewClusterForm = { clusterType: "", site: "", cluster: "", address: "", port: "22", user: "root", password: "", tunnelEndpoint: "" };

function provisionPhaseLabel(phase: string | undefined, zh: boolean) {
  if (phase === "Succeeded") return zh ? "已建好" : "Made";
  if (phase === "Failed") return zh ? "失败" : "Failed";
  return zh ? "进行中" : "Running";
}

function provisionPhaseClass(phase: string | undefined) {
  if (phase === "Succeeded") return "status-online";
  if (phase === "Failed") return "status-failed";
  return "status-pending";
}

export function NewClusterPage({ lang }: { lang: Lang }) {
  const zh = lang === "zh";
  const [form, setForm] = useState<NewClusterForm>(emptyForm);
  const [provisions, setProvisions] = useState<ClusterProvision[]>([]);
  const [selected, setSelected] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [cleared, setCleared] = useState<string[]>([]);
  const { pending: busy, error, setError, run } = useAction();
  const refs = useRef<Partial<Record<NewClusterField, HTMLElement | null>>>({});
  const problem = newClusterProblem(form, lang);
  const fieldProblems = useMemo(() => submitted ? newClusterFieldProblems(form, lang) : {}, [submitted, form, lang]);
  const summary = newClusterSummary(form, lang);
  const current = provisions.find((p) => p.metadata?.name === selected) ?? provisions[0];
  const view = current ? provisionView(current, lang) : undefined;
  const progress = provisionProgress(current);

  const refresh = useCallback(async () => {
    try {
      const latest = await clusterProvisionsApi.list();
      setProvisions(latest);
      setLoadError("");
      setSelected((value) => value && latest.some((item) => item.metadata?.name === value) ? value : latest[0]?.metadata?.name ?? "");
      return latest;
    } catch (reason) {
      setLoadError((reason as Error).message);
      return undefined;
    }
  }, []);

  useEffect(() => {
    let timer: number | undefined;
    let active = true;
    const poll = async () => {
      if (document.visibilityState !== "visible") return;
      const latest = await clusterProvisionsApi.list().catch((reason: unknown) => {
        if (active) setLoadError((reason as Error).message);
        return undefined;
      });
      if (!active) return;
      if (latest) {
        setProvisions(latest);
        setLoadError("");
        setSelected((value) => value && latest.some((item) => item.metadata?.name === value) ? value : latest[0]?.metadata?.name ?? "");
      }
      const selectedProvision = latest?.find((item) => item.metadata?.name === selected) ?? latest?.[0];
      if (document.visibilityState === "visible") {
        timer = window.setTimeout(() => void poll(), selectedProvision?.status?.phase === "Running" ? 3000 : 20000);
      }
    };
    const onVisibility = () => {
      if (document.visibilityState === "visible") void poll();
      else if (timer !== undefined) window.clearTimeout(timer);
    };
    void poll();
    document.addEventListener("visibilitychange", onVisibility);
    return () => { active = false; if (timer !== undefined) window.clearTimeout(timer); document.removeEventListener("visibilitychange", onVisibility); };
  }, [selected]);

  const update = (key: NewClusterField, value: string) => setForm((currentForm) => ({ ...currentForm, [key]: value }));
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitted(true);
    if (problem) {
      const first = firstNewClusterProblem(newClusterFieldProblems(form, lang));
      if (first) refs.current[first]?.focus();
      return;
    }
    void run(async () => {
      const made = await clusterProvisionsApi.create({ ...form, clusterType: form.clusterType as ClusterType });
      setForm(emptyForm);
      setSubmitted(false);
      setSelected(made.metadata?.name ?? "");
      await refresh();
      setSelected(made.metadata?.name ?? "");
    });
  };

  const input = (key: NewClusterField, label: string, placeholder: string, options: { type?: string; helper?: string; full?: boolean; value?: string } = {}) => {
    const errorID = `new-cluster-${key}-error`;
    return <label key={key} className={`new-cluster-field${options.full ? " is-full" : ""}`}>
      <span>{label}</span>
      <input ref={(element) => { refs.current[key] = element; }} type={options.type ?? "text"} value={options.value ?? form[key]} placeholder={placeholder} autoComplete={key === "password" ? "new-password" : "off"} onChange={(event) => update(key, event.target.value)} aria-invalid={!!fieldProblems[key]} aria-describedby={[options.helper ? `${errorID}-help` : "", fieldProblems[key] ? errorID : ""].filter(Boolean).join(" ") || undefined} />
      {options.helper && <small id={`${errorID}-help`} className="new-cluster-helper">{options.helper}</small>}
      {fieldProblems[key] && <small className="field-error" id={errorID}>{fieldProblems[key]}</small>}
    </label>;
  };

  const reset = () => { setForm(emptyForm); setSubmitted(false); setShowPassword(false); setError(""); setCleared([]); };
  const timestamp = (provision: ClusterProvision) => provision.metadata?.creationTimestamp ? new Date(provision.metadata.creationTimestamp).toLocaleString(zh ? "zh-CN" : "en-US") : "—";

  return <div className="page-content resource-page create-cluster-page">
    <div className="section-heading create-cluster-heading"><div><span className="eyebrow"><Shield size={13} />{zh ? "新建集群" : "New Cluster"}</span><h2>{zh ? "新建集群" : "New Cluster"}<span className="new-cluster-heading-count">{zh ? " · 2 步" : " · 2 steps"}</span></h2><p>{zh ? "从场地身份开始，再用 SSH 配置第一台机器。" : "Set the site identity, then configure its first machine over SSH."}</p></div></div>
    <div className="new-cluster-layout">
      <form id="new-cluster-form" className="new-cluster-form" onSubmit={submit} noValidate aria-busy={busy}>
        {submitted && summary.remaining > 0 && <div className="new-cluster-validation-summary" role="alert">{zh ? `还需填写 ${summary.remaining} 项` : `${summary.remaining} field${summary.remaining === 1 ? "" : "s"} still need attention`}</div>}
        <section className="cluster-enrollment-card new-cluster-card">
          <div className="cluster-enrollment-card-head"><span className="cluster-enrollment-step">01</span><div><strong>{zh ? "场地与身份" : "Site and identity"}</strong><small>{zh ? "选择集群类型并命名场地与集群。" : "Choose a cluster type and name the site and cluster."}</small></div></div>
          <fieldset className="new-cluster-type-field" aria-describedby={fieldProblems.clusterType ? "new-cluster-type-error" : undefined}>
            <legend>{zh ? "集群类型" : "Cluster type"}</legend>
            <div className="new-cluster-type-cards">{clusterTypes.map((type) => <label key={type.value} className={`new-cluster-type-card${form.clusterType === type.value ? " is-selected" : ""}`}>
              <input ref={type.value === "gpu" ? (element) => { refs.current.clusterType = element; } : undefined} type="radio" name="clusterType" value={type.value} checked={form.clusterType === type.value} onChange={() => update("clusterType", type.value)} aria-invalid={!!fieldProblems.clusterType} />
              <span className="new-cluster-type-icon"><Server size={18} /></span><span><strong>{zh ? type.zh : type.en}</strong><small>{type.value === "gpu" ? (zh ? "面向 GPU 工作负载" : "For GPU workloads") : (zh ? "面向机器人与具身设备" : "For robots and embodied devices")}</small></span>
            </label>)}</div>
            {fieldProblems.clusterType && <small className="field-error" id="new-cluster-type-error">{fieldProblems.clusterType}</small>}
          </fieldset>
          <div className="new-cluster-fields">
            {input("site", zh ? "场地名" : "Site name", "baidu", { helper: zh ? "小写字母、数字和连字符" : "Lower-case letters, digits and hyphens" })}
            {input("cluster", zh ? "集群名" : "Cluster name", "", { value: form.cluster || (form.clusterType ? defaultClusterName(form.site, form.clusterType) : ""), helper: zh ? `默认：${form.clusterType ? defaultClusterName(form.site || "<site>", form.clusterType) : "选择类型后生成"}` : `Default: ${form.clusterType ? defaultClusterName(form.site || "<site>", form.clusterType) : "generated after type selection"}` })}
            {input("tunnelEndpoint", zh ? "隧道端点（选填）" : "Tunnel endpoint (optional)", form.address.trim() ? `${form.address.trim()}:51820` : "1.2.3.4:51820", { full: true, helper: zh ? `留空使用机器 IP 和 UDP 51820。建议值仅作提示，不会自动填写。` : `Leave empty to use the machine IP at UDP 51820. The suggestion is not filled in automatically.` })}
          </div>
        </section>
        <section className="cluster-enrollment-card new-cluster-card">
          <div className="cluster-enrollment-card-head"><span className="cluster-enrollment-step">02</span><div><strong>{zh ? "第一台机器（SSH）" : "First machine (SSH)"}</strong><small>{zh ? "需要 systemd、tar 和 WireGuard 内核模块，且允许隧道 UDP 入站。" : "Requires systemd, tar, WireGuard kernel module, and inbound tunnel UDP access."}</small></div></div>
          <div className="new-cluster-fields">
            {input("address", zh ? "机器 IP 或主机名" : "Machine IP or hostname", "1.2.3.4", { full: true })}
            {input("port", zh ? "SSH 端口" : "SSH port", "22")}
            {input("user", zh ? "SSH 账号" : "SSH account", "root")}
            <label className="new-cluster-field">
              <span>{zh ? "SSH 密码" : "SSH password"}</span>
              <span className="new-cluster-password"><input ref={(element) => { refs.current.password = element; }} type={showPassword ? "text" : "password"} value={form.password} autoComplete="new-password" onChange={(event) => update("password", event.target.value)} aria-invalid={!!fieldProblems.password} aria-describedby={fieldProblems.password ? "new-cluster-password-error new-cluster-password-help" : "new-cluster-password-help"} /><button type="button" className="new-cluster-password-toggle" aria-label={showPassword ? (zh ? "隐藏密码" : "Hide password") : (zh ? "显示密码" : "Show password")} onClick={() => setShowPassword((value) => !value)}>{showPassword ? <EyeOff size={16} /> : <Eye size={16} />}</button></span>
              <small id="new-cluster-password-help" className="new-cluster-helper">{zh ? "口令仅用于本次 SSH 操作，不会保存。" : "Used for this SSH operation only and never saved."}</small>
              {fieldProblems.password && <small className="field-error" id="new-cluster-password-error">{fieldProblems.password}</small>}
            </label>
          </div>
          {error && <div className="cert-error" role="alert">{error}</div>}
          {loadError && <div className="cert-error" role="alert"><span>{loadError}</span><button type="button" className="secondary-button" onClick={() => void refresh()}>{zh ? "重试加载" : "Retry loading"}</button></div>}
          {cleared.length > 0 && <ol className="provision-cleared">{cleared.map((step) => <li key={step}>{step}</li>)}</ol>}
          <div className="new-cluster-form-actions"><button type="button" className="secondary-button" onClick={reset} disabled={busy}>{zh ? "重置" : "Reset"}</button><button id="new-cluster-submit" type="submit" className="primary-button" disabled={busy}>{busy && <LoaderCircle size={15} className="job-action-loading" />}{busy ? (zh ? "提交中…" : "Starting…") : zh ? "新建集群" : "Create cluster"}</button></div>
        </section>
      </form>

      <aside className="new-cluster-aside">
        <section className="cluster-enrollment-card cluster-summary">
          <div className="cluster-summary-head"><div><small>{zh ? "创建摘要" : "Creation summary"}</small><h3>{zh ? "集群信息" : "Cluster details"}</h3></div><span className={`status ${problem ? "status-pending" : "status-online"}`}><i />{problem ? (zh ? "待完善" : "Incomplete") : (zh ? "已就绪" : "Ready")}</span></div>
          {submitted && summary.remaining > 0 && <p className="new-cluster-summary-alert" role="status">{zh ? `还需填写 ${summary.remaining} 项` : `${summary.remaining} fields remain`}</p>}
          <dl><div><dt>{zh ? "类型" : "Type"}</dt><dd>{form.clusterType ? clusterTypes.find((type) => type.value === form.clusterType)?.[lang] : summary.clusterType}</dd></div><div><dt>{zh ? "场地" : "Site"}</dt><dd>{summary.site}</dd></div><div><dt>{zh ? "集群名" : "Cluster"}</dt><dd><code>{summary.cluster}</code></dd></div><div><dt>{zh ? "首台机器" : "First machine"}</dt><dd>{summary.machine}{form.address && <small>:{form.port}</small>}</dd></div><div><dt>{zh ? "隧道端点" : "Tunnel endpoint"}</dt><dd><code>{form.tunnelEndpoint.trim() || (form.address.trim() ? `${form.address.trim()}:51820` : "—")}</code></dd></div></dl>
          <div className="new-cluster-summary-actions"><button type="button" className="secondary-button" onClick={reset} disabled={busy}>{zh ? "重置" : "Reset"}</button><button type="submit" form="new-cluster-form" className="primary-button" disabled={busy}>{busy && <LoaderCircle size={15} className="job-action-loading" />}{zh ? "新建集群" : "Create cluster"}</button></div>
        </section>
        {current && view && <section className="cluster-enrollment-card provision-progress new-cluster-progress" aria-live="polite">
          <div className="cluster-summary-head"><div><small>{zh ? "创建进度" : "Provisioning"}</small><h3>{current.metadata?.name}</h3></div><span className={`status ${provisionPhaseClass(view.phase)}`}><i />{provisionPhaseLabel(view.phase, zh)}</span></div>
          <div className="new-cluster-progress-meta"><strong>{zh ? `阶段 ${progress.stageNumber}/${progress.total}` : `Stage ${progress.stageNumber}/${progress.total}`}</strong><span>{progress.completed}/{progress.total}</span></div><progress className="new-cluster-progress-track" value={progress.percent} max={100} aria-label={zh ? "创建进度" : "Provisioning progress"} />
          {view.phase === "Failed" && <p className="new-cluster-failure">{zh ? `失败阶段：${view.stages.find((stage) => stage.name === current.status?.stage)?.label ?? current.status?.stage ?? "—"}` : `Failed stage: ${view.stages.find((stage) => stage.name === current.status?.stage)?.label ?? current.status?.stage ?? "—"}`}{progress.failedStage && ` · ${progress.failedStage}`}</p>}
          <StageList stages={view.stages} zh={zh} />
          <RetryClear zh={zh} view={view} clearReach={zh ? "要登录机器把它恢复原样" : "to put the machine back"} goneWhy={zh ? "（重装或退还）" : " (reinstalled or returned)"} user={current.spec.machine.user} pending={busy} run={run}
            retry={async (user, password) => { await clusterProvisionsApi.retry(current.metadata!.name!, password, user); await refresh(); }}
            clear={async (user, password, machineGone) => { setCleared(await selfHealApi.clearClusterProvision(current.metadata!.name!, password, user, machineGone)); setSelected(""); await refresh(); }}
            clearNote={view.phase === "Succeeded" ? zh ? "集群删除以后，这次新建留下的场地、名字和节点声明还占着：清掉后才能用同一个场地或集群名重新建。集群还在时不能清。" : "After the cluster is deleted, the declarations from this creation still hold the site and name. Clear them before reusing either. They cannot be cleared while the cluster is listed." : zh ? "不想从这一步重试：撤销这次新建。RLark 撤掉机器上建到一半的控制面和隧道，删掉未完成的集群与凭据并清理场地声明。" : "To abandon this attempt, clear it. RLark removes the partial control plane and tunnel, the unfinished cluster and credentials, and the site declarations."}
            clearLabel={view.phase === "Succeeded" ? (zh ? "清掉这次新建留下的声明" : "Clear creation declarations") : (zh ? "撤销这次新建，清掉重来" : "Clear this creation and start over")} />
        </section>}
      </aside>
    </div>
    {provisions.length > 0 && <section className="signed-clusters-panel new-cluster-history">
      <div className="signed-clusters-heading"><div><h3>{zh ? "新建历史" : "Creation history"}</h3></div><span>{provisions.length}</span></div>
      <div className="new-cluster-history-list">{[...provisions].sort((a, b) => Date.parse(b.metadata?.creationTimestamp ?? "") - Date.parse(a.metadata?.creationTimestamp ?? "")).map((p) => {
        const selectedRow = p.metadata?.name === current?.metadata?.name;
        return <button type="button" key={p.metadata?.name} className={`new-cluster-history-row${selectedRow ? " is-current" : ""}`} onClick={() => setSelected(p.metadata?.name ?? "")} aria-expanded={selectedRow}>
          <span className="new-cluster-history-icon"><Server size={16} /></span><span className="new-cluster-history-main"><strong>{p.metadata?.name}</strong><small>{p.spec.site} · {p.spec.machine.address} · {p.spec.machine.user}</small>{selectedRow && <small>{p.status?.stage ?? "—"}</small>}</span><span className={`status ${provisionPhaseClass(p.status?.phase)}`}><i />{provisionPhaseLabel(p.status?.phase, zh)}</span><time>{timestamp(p)}</time>
        </button>;
      })}</div>
    </section>}
  </div>;
}
