import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Activity, AlertTriangle, Boxes, CircleCheck, Clock, Layers3, Plus, Radar, RefreshCw, Server, Trash2 } from "lucide-react";
import { ApiError } from "../../api";
import { useAutoRefresh } from "../../hooks";
import { RefreshOverlay } from "../../components/shared";
import { useAction } from "../components/useAction";
import { clustersApi } from "../../backend";
import type { ClusterSummary } from "../../types";
import { isSiteNetworkConflict, validateDiscovery } from "../lib/siteNetwork";
import { machinesApi, siteNetworkApi, siteNodesApi, type SiteNetworkDiscovery, type SiteNetworkSite, type SiteNetworkView } from "../api";
import { isRobotNode, robotPowerState, seenTime, type Machine, type SiteNode } from "../lib/siteNodeDisplay";
import { publishedCameras } from "../lib/cameras";
import "./SiteNetworkSettings.css";

const condition = (data: SiteNetworkView | undefined, name: string) => data?.status?.conditions?.find((item) => item.type === name);
const defaultDiscovery = (site: SiteNetworkSite): SiteNetworkDiscovery => site.discovery ? structuredClone(site.discovery) : {
  enabled: false, ranges: [""], intervalSeconds: 30, from: { clusterID: "", nodeName: "" }, includeUnclassified: false, sshPorts: [],
};
// A site that never had discovery and is left disabled: saving it removes the
// field (null), which the API accepts without requiring ranges or a from node.
const isUnconfiguredDisabled = (site: SiteNetworkSite, draft: SiteNetworkDiscovery) =>
  !draft.enabled && !site.discovery && draft.ranges.every((range) => !range.trim()) && !draft.from.clusterID && !draft.from.nodeName;

type ScanEventTone = "ok" | "warn" | "bad" | "info";
type ScanEvent = { at: string; text: string; tone: ScanEventTone };

// Edit discovery settings for one site at a time while retaining independent drafts.
export function SiteNetworkSettingsPage({ zh, navigate }: { zh: boolean; navigate: (id: string, sub?: string) => void }) {
  const [data, setData] = useState<SiteNetworkView>();
  const [drafts, setDrafts] = useState<Record<string, SiteNetworkDiscovery>>({});
  const [dirty, setDirty] = useState<Record<string, boolean>>({});
  const [selectedSite, setSelectedSite] = useState("");
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [notice, setNotice] = useState<Record<string, string>>({});
  const [noticeTone, setNoticeTone] = useState<Record<string, "success" | "warning">>({});
  const [readOnly, setReadOnly] = useState(false);
  const [machines, setMachines] = useState<Machine[]>([]);
  const [siteNodes, setSiteNodes] = useState<SiteNode[]>([]);
  const [clusterDisplayNames, setClusterDisplayNames] = useState<Record<string, string>>({});
  const [insightsLoaded, setInsightsLoaded] = useState(false);
  const [savedEvents, setSavedEvents] = useState<ScanEvent[]>([]);
  const { pending, error: actionError, setError, run } = useAction();
  const dirtyRef = useRef(dirty);
  const tabRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const insightsRequest = useRef(0);
  useEffect(() => { dirtyRef.current = dirty; }, [dirty]);

  const loadInsights = useCallback(async (view: SiteNetworkView) => {
    const request = ++insightsRequest.current;
    const clusterIDs = [...new Set(view.sites.flatMap((site) => site.clusters))];
    const [machineResult, nodeResults, clusterSummaries] = await Promise.all([
      machinesApi.list().catch(() => ({ items: [] as Machine[], classes: {} })),
      Promise.all(clusterIDs.map((cluster) => siteNodesApi.list(cluster).catch(() => ({ items: [] as SiteNode[] })))),
      clustersApi.list<ClusterSummary>().catch(() => [] as ClusterSummary[]),
    ]);
    if (request !== insightsRequest.current) return;
    setMachines(machineResult.items);
    setSiteNodes(nodeResults.flatMap((result) => result.items));
    setClusterDisplayNames(Object.fromEntries(clusterSummaries.map((cluster) => [cluster.id, cluster.name] as const).filter(([, name]) => Boolean(name))));
    setInsightsLoaded(true);
  }, []);

  const load = useCallback(async (quiet = false) => {
    if (quiet) setRefreshing(true); else setLoading(true);
    try {
      const result = await siteNetworkApi.get();
      setData(result);
      setReadOnly(false);
      setLoadError("");
      setDrafts((current) => {
        const next = { ...current };
        for (const site of result.sites) if (!dirtyRef.current[site.name]) next[site.name] = defaultDiscovery(site);
        return next;
      });
      void loadInsights(result);
    } catch (reason) {
      if (reason instanceof ApiError && reason.status === 403) setReadOnly(true);
      setLoadError(reason instanceof ApiError && reason.status === 404
        ? (zh ? "站点网络尚未初始化。" : "The site network has not been initialized.")
        : (reason as Error).message || (zh ? "站点网络读取失败。" : "Could not load the site network."));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [loadInsights, zh]);

  useEffect(() => { void load(); }, [load]);
  useAutoRefresh(() => load(true), 30000);

  useEffect(() => {
    const sites = data?.sites ?? [];
    if (sites.length === 0) {
      setSelectedSite("");
      return;
    }
    if (!sites.some((site) => site.name === selectedSite)) setSelectedSite(sites[0].name);
  }, [data, selectedSite]);

  const hubs = useMemo(() => data?.members?.filter((member) => member.role === "Hub") ?? [], [data]);
  const selectedSiteData = data?.sites.find((site) => site.name === selectedSite);
  const siteHubs = useMemo(() => data?.members.filter((member) => member.role === "Hub" && member.site === selectedSite) ?? [], [data, selectedSite]);
  const draft = selectedSiteData ? drafts[selectedSite] ?? defaultDiscovery(selectedSiteData) : undefined;
  const validation = selectedSiteData && draft && dirty[selectedSite]
    ? (isUnconfiguredDisabled(selectedSiteData, draft) ? "" : validateDiscovery(draft, zh))
    : "";
  const selectedFrom = draft?.from.clusterID && draft.from.nodeName
    ? hubs.find((hub) => hub.clusterID === draft.from.clusterID && hub.nodeName === draft.from.nodeName)
    : undefined;
  const generationPending = (data?.status?.observedGeneration ?? 0) < (data?.generation ?? 0);
  const unreachable = data?.status?.unreachable ?? [];
  const now = Date.now();

  const siteLoad = useMemo(() => (data?.sites ?? []).map((site) => ({
    name: site.name,
    count: machines.filter((machine) => machine.site === site.name).length,
    hubs: data?.members.filter((member) => member.role === "Hub" && member.site === site.name).length ?? 0,
  })), [data, machines]);
  const maxLoad = Math.max(1, ...siteLoad.map((item) => item.count));
  const scanningSites = data?.sites.filter((site) => site.discovery?.enabled && data.members.some((member) => member.role === "Hub" && member.site === site.name)).length ?? 0;
  const seenValues = machines.map((machine) => machine.last_seen).filter((value): value is string => Boolean(value)).sort();
  const latestSeen = seenValues[seenValues.length - 1];
  const robots = useMemo(() => siteNodes.filter((node) => isRobotNode(node.labels)), [siteNodes]);
  const robotsOnline = robots.filter((node) => node.online).length;
  const power = useMemo(() => {
    const counts = { ready: 0, fault: 0, other: 0 };
    for (const robot of robots) {
      const state = robotPowerState(robot.labels);
      if (state === "ready") counts.ready += 1;
      else if (state === "fault" || state === "estop") counts.fault += 1;
      else counts.other += 1;
    }
    return counts;
  }, [robots]);
  const cameraStats = useMemo(() => {
    let ok = 0;
    let total = 0;
    for (const robot of robots) {
      let report;
      try { report = publishedCameras(robot.cameras); } catch { report = undefined; }
      if (!report) continue;
      for (const camera of Object.values(report.cameras)) {
        if (!camera) continue;
        total += 1;
        if (camera.state === "ok") ok += 1;
      }
    }
    return { ok, total };
  }, [robots]);
  const siteOnline = useMemo(() => (data?.sites ?? []).map((site) => {
    const nodes = siteNodes.filter((node) => site.clusters.includes(node.cluster_id));
    return { name: site.name, online: nodes.filter((node) => node.online).length, total: nodes.length };
  }), [data, siteNodes]);
  const joining = machines.filter((machine) => machine.joining).length;
  const eventItems = useMemo<ScanEvent[]>(() => {
    const discovered = machines
      .filter((machine) => machine.last_seen)
      .sort((a, b) => (b.last_seen ?? "").localeCompare(a.last_seen ?? ""))
      .slice(0, 6)
      .map((machine) => ({
        at: seenTime(machine.last_seen, now),
        text: `${machine.site || (zh ? "站点" : "Site")} ${zh ? "发现" : "found"} ${machine.serial || machine.address || machine.name}`,
        tone: (machine.joining ? "info" : machine.problem ? "warn" : "ok") as ScanEventTone,
      }));
    return [...savedEvents, ...discovered].slice(0, 6);
  }, [machines, savedEvents, now, zh]);

  const updateDraft = (site: string, update: (current: SiteNetworkDiscovery) => SiteNetworkDiscovery) => {
    const currentSite = data?.sites.find((item) => item.name === site);
    if (!currentSite) return;
    setDrafts((current) => ({ ...current, [site]: update(current[site] ?? defaultDiscovery(currentSite)) }));
    setDirty((current) => ({ ...current, [site]: true }));
    setNotice((current) => ({ ...current, [site]: "" }));
  };

  const save = (site: SiteNetworkSite) => {
    const siteDraft = drafts[site.name] ?? defaultDiscovery(site);
    const unconfiguredDisabled = isUnconfiguredDisabled(site, siteDraft);
    const siteValidation = unconfiguredDisabled ? "" : validateDiscovery(siteDraft, zh);
    if (siteValidation) { setError(siteValidation); return; }
    if (readOnly) return;
    void run(async () => {
      setNotice((current) => ({ ...current, [site.name]: "" }));
      try {
        await siteNetworkApi.updateDiscovery(site.name, {
          resourceVersion: data!.resourceVersion,
          discovery: unconfiguredDisabled ? null : { ...siteDraft, ranges: siteDraft.ranges.map((item) => item.trim()).filter(Boolean) },
        });
        setDirty((current) => ({ ...current, [site.name]: false }));
        setNoticeTone((current) => ({ ...current, [site.name]: "success" }));
        setNotice((current) => ({ ...current, [site.name]: zh ? "设置已保存。" : "Settings saved." }));
        const at = new Date().toLocaleTimeString(zh ? "zh-CN" : "en-GB", { hour12: false });
        setSavedEvents((current) => [{ at, text: zh ? `${site.name} 扫描设置已保存` : `${site.name} discovery settings saved`, tone: "ok" as ScanEventTone }, ...current].slice(0, 4));
        await load(true);
      } catch (reason) {
        if (isSiteNetworkConflict(reason)) {
          setNoticeTone((current) => ({ ...current, [site.name]: "warning" }));
          setNotice((current) => ({ ...current, [site.name]: zh ? "设置刚被其他管理员修改，已重新加载最新配置。请核对后再保存。" : "Another administrator changed these settings. The latest configuration was reloaded. Review it before saving." }));
          setDirty((current) => ({ ...current, [site.name]: false }));
          try {
            const latest = await siteNetworkApi.get();
            setData(latest);
            const latestSite = latest.sites.find((item) => item.name === site.name);
            if (latestSite) setDrafts((current) => ({ ...current, [site.name]: defaultDiscovery(latestSite) }));
          } catch (reloadError) {
            setLoadError((reloadError as Error).message);
          }
          return;
        }
        if (reason instanceof ApiError && reason.status === 403) setReadOnly(true);
        throw reason;
      }
    });
  };

  const rawConditionDetail = (name: string) => {
    const value = condition(data, name);
    if (!value) return `${name} · ${zh ? "未知" : "Unknown"}`;
    return `${name} · ${value.status}${value.reason ? ` · ${value.reason}` : ""}${value.message ? ` — ${value.message}` : ""}`;
  };
  const conditionIsTrue = (name: string) => condition(data, name)?.status === "True";
  const selectSite = (site: string, focus = false) => {
    setSelectedSite(site);
    if (focus) requestAnimationFrame(() => tabRefs.current[site]?.focus());
  };

  const ready = conditionIsTrue("Ready");
  const connected = conditionIsTrue("FullyConnected");
  const stale = conditionIsTrue("DiscoveryStale");
  const readyTone = ready ? "is-ok" : condition(data, "Ready")?.status === "False" ? "is-bad" : "is-idle";
  const connectedTone = connected ? "is-ok" : condition(data, "FullyConnected")?.status === "False" ? "is-warn" : "is-idle";
  const staleTone = stale ? "is-warn" : condition(data, "DiscoveryStale")?.status === "False" ? "is-ok" : "is-idle";
  const observedGeneration = data?.status?.observedGeneration;
  const siteState = (enabled: boolean, hubs: number) => enabled ? (hubs ? "is-ok" : "is-warn") : "is-idle";

  return <main className="page-content resource-page site-network-page">
    <header className="site-network-page-head">
      <div className="site-network-head-copy">
        <h2>{zh ? "站点扫描设置" : "Discovery Settings"}</h2>
        <p>{zh ? "查看全局网络状态，并逐个站点调整扫描范围与频率。修改会保留在各站点草稿里，保存后约 30 秒生效。" : "Review network health and tune each site's scan scope and cadence. Edits stay as per-site drafts and take effect about 30 seconds after saving."}</p>
      </div>
      <div className="site-network-health" aria-label={zh ? "站点网络健康" : "Site network health"}>
        <div className={`site-network-health-item ${readyTone}`} title={rawConditionDetail("Ready")}>
          <i /><span><small>{zh ? "网络就绪" : "Ready"}</small><strong>{ready ? (zh ? "就绪" : "Ready") : condition(data, "Ready")?.status === "False" ? (zh ? "未就绪" : "Not ready") : (zh ? "未知" : "Unknown")}</strong></span>
        </div>
        <div className={`site-network-health-item ${connectedTone}`} title={rawConditionDetail("FullyConnected")}>
          <i /><span><small>{zh ? "全部互通" : "Connected"}</small><strong>{connected ? (zh ? "全部可达" : "All reachable") : condition(data, "FullyConnected")?.status === "False" ? (zh ? "存在不可达" : "Unreachable paths") : (zh ? "未知" : "Unknown")}</strong></span>
        </div>
        <div className={`site-network-health-item ${staleTone}`} title={rawConditionDetail("DiscoveryStale")}>
          <i /><span><small>{zh ? "扫描新鲜度" : "Discovery"}</small><strong>{stale ? (zh ? "数据已过期" : "Stale") : condition(data, "DiscoveryStale")?.status === "False" ? (zh ? "正常" : "Current") : (zh ? "未知" : "Unknown")}</strong></span>
        </div>
        <div className={`site-network-health-item ${generationPending ? "is-warn" : "is-ok"}`} title={`${zh ? "配置版本" : "Generation"} · ${observedGeneration ?? "—"} / ${data?.generation ?? "—"}`}>
          <i /><span><small>{zh ? "配置版本" : "Generation"}</small><strong>{observedGeneration ?? "—"} / {data?.generation ?? "—"}</strong></span>
        </div>
      </div>
      <div className="site-network-head-actions">
        <button type="button" className="secondary-button" onClick={() => navigate("stations", "models")}><Boxes size={14} />{zh ? "机器人型号" : "Robot models"}</button>
        <button type="button" className="secondary-button" onClick={() => navigate("stations", "join")}>{zh ? "返回入网" : "Back to joining"}</button>
        <button type="button" className="secondary-button" onClick={() => void load(true)} disabled={refreshing}><RefreshCw size={14} />{zh ? "刷新" : "Refresh"}</button>
      </div>
    </header>

    {data && (unreachable.length > 0 || generationPending) && <div className="site-network-inline-status" role="status">
      <AlertTriangle size={15} aria-hidden="true" />
      <span>{zh ? "网络仍在检查或配置尚未收敛。" : "Network checks or configuration reconciliation are still in progress."} {unreachable.length > 0 && (zh ? `不可达路径：${unreachable.join("、")}。` : `Unreachable paths: ${unreachable.join(", ")}.`)} {generationPending && (zh ? `当前配置版本 ${observedGeneration ?? 0} / ${data.generation}。` : `Configuration generation ${observedGeneration ?? 0} / ${data.generation}.`)}</span>
    </div>}

    {readOnly && <div className="site-network-inline-status" role="alert">{zh ? "当前账号没有管理员权限，无法读取或保存扫描设置。" : "This account cannot read or save discovery settings."}</div>}
    {loadError && <div className="site-network-error" role="alert">{loadError}{!readOnly && <button type="button" className="secondary-button" onClick={() => void load()}>{zh ? "重试" : "Retry"}</button>}</div>}
    {actionError && <div className="site-network-error" role="alert">{actionError}</div>}

    {loading && !data ? <p role="status">{zh ? "正在读取站点网络…" : "Loading site network…"}</p> : data && data.sites.length === 0 ? <section className="site-network-empty"><strong>{zh ? "没有站点" : "No sites"}</strong></section> : data && <>
      <div className="refreshable-region" aria-busy={pending || refreshing}>
        <div className="site-network-board">
          <div className="site-network-rail">
            <div className="site-network-rail-head"><span>{zh ? "站点" : "Sites"}</span><span>{data.sites.length}</span></div>
            <div className="site-network-sites" role="tablist" aria-label={zh ? "站点" : "Sites"}>
              {data.sites.map((siteRecord) => {
                const site = {
                  ...siteRecord,
                  clusters: siteRecord.clusters.map((cluster) => clusterDisplayNames[cluster]?.trim() || cluster),
                };
                const count = data.members.filter((member) => member.role === "Hub" && member.site === site.name).length;
                const clusterNames = site.clusters.join(" · ");
                const enabled = Boolean(site.discovery?.enabled);
                const active = site.name === selectedSite;
                return <button
                  type="button"
                  role="tab"
                  id={`site-network-tab-${site.name}`}
                  aria-selected={active}
                  tabIndex={active ? 0 : -1}
                  ref={(element) => { tabRefs.current[site.name] = element; }}
                  className={`site-network-site-row${active ? " is-active" : ""}`}
                  key={site.name}
                  onClick={() => selectSite(site.name)}
                  onKeyDown={(event) => {
                    const index = data.sites.findIndex((item) => item.name === site.name);
                    const nextIndex = event.key === "ArrowRight" ? (index + 1) % data.sites.length
                      : event.key === "ArrowLeft" ? (index + data.sites.length - 1) % data.sites.length
                        : event.key === "Home" ? 0 : event.key === "End" ? data.sites.length - 1 : -1;
                    if (nextIndex >= 0) {
                      event.preventDefault();
                      selectSite(data.sites[nextIndex].name, true);
                    }
                  }}
                >
                  <i className={`site-network-site-dot ${siteState(enabled, count)}`} aria-hidden="true" />
                  <span className="site-network-site-copy"><strong>{site.name}</strong><small title={site.clusters.length > 1 ? clusterNames : undefined}>{count} {zh ? "Hub" : count === 1 ? "Hub" : "Hubs"} · {clusterNames || (zh ? "未关联集群" : "No clusters")}</small></span>
                  <span className={`site-network-site-state ${siteState(enabled, count)}`}>{enabled ? (count ? (zh ? "扫描中" : "Scanning") : (zh ? "无 Hub" : "No Hub")) : (zh ? "已关闭" : "Off")}</span>
                </button>;
              })}
            </div>
          </div>

          {selectedSiteData && draft && <section className="site-network-workspace" role="tabpanel" aria-labelledby={`site-network-tab-${selectedSiteData.name}`}>
            <header className="site-network-editor-head">
              <div>
                <h3>{selectedSiteData.name}<span className={`status ${draft.enabled ? (siteHubs.length ? "status-running" : "status-offline") : "status-stopped"}`}><i />{draft.enabled ? siteHubs.length ? (zh ? "正在扫描" : "Scanning") : (zh ? "已开启，无 Hub 不会扫描" : "Enabled, no Hub to scan") : (zh ? "已关闭" : "Disabled")}</span></h3>
                <p>{selectedSiteData.clusters.join(" · ") || (zh ? "未关联集群" : "No clusters")} · {siteHubs.length} {zh ? "个 Hub" : siteHubs.length === 1 ? "Hub" : "Hubs"}</p>
              </div>
              <div className="site-network-editor-actions">
                <span className="site-network-editor-note">{zh ? "保存后约 30 秒生效" : "Takes effect in about 30 seconds"}</span>
                {dirty[selectedSite] && <span className="site-network-unsaved">{zh ? "有未保存的更改" : "Unsaved changes"}</span>}
                <button type="button" className="secondary-button" disabled={!dirty[selectedSite] || pending} onClick={() => { setDrafts((current) => ({ ...current, [selectedSite]: defaultDiscovery(selectedSiteData) })); setDirty((current) => ({ ...current, [selectedSite]: false })); setNotice((current) => ({ ...current, [selectedSite]: "" })); setError(""); }}>{zh ? "取消" : "Cancel"}</button>
                <button type="button" className="primary-button" disabled={!dirty[selectedSite] || pending || !!validation || readOnly} onClick={() => save(selectedSiteData)}>{pending ? (zh ? "保存中…" : "Saving…") : (zh ? "保存设置" : "Save settings")}</button>
              </div>
            </header>

            <div className="site-network-editor-grid">
              <section className="site-network-editor-col">
                <label className="site-network-switch">
                  <input type="checkbox" checked={draft.enabled} disabled={readOnly || pending} onChange={(event) => updateDraft(selectedSite, (current) => ({ ...current, enabled: event.target.checked }))} />
                  <span className="site-network-switch-track" aria-hidden="true" />
                  <span><strong>{zh ? "启用此站点扫描" : "Enable discovery for this site"}</strong><small>{draft.enabled ? siteHubs.length ? (zh ? "正在扫描" : "Scanning") : (zh ? "已开启，但无 Hub 不会扫描。" : "Enabled, but no Hub is available to scan.") : siteHubs.length ? (zh ? "已关闭" : "Disabled") : (zh ? "已关闭，且无 Hub 不会扫描" : "Disabled, and there is no Hub to scan")}</small></span>
                </label>
                <label className="site-network-field">{zh ? "扫描间隔" : "Scan interval"}
                  <span className="site-network-interval"><input type="number" min={10} max={3600} value={draft.intervalSeconds} disabled={readOnly || pending} onChange={(event) => updateDraft(selectedSite, (current) => ({ ...current, intervalSeconds: Number(event.target.value) }))} /><span>{zh ? "秒" : "s"}</span></span>
                  <small>{zh ? "10–3600 秒；Hub 按这个间隔上报发现的机器。" : "10–3600 seconds; hubs report discovered machines at this cadence."}</small>
                </label>
                <label className="site-network-switch">
                  <input type="checkbox" checked={Boolean(draft.includeUnclassified)} disabled={readOnly || pending} onChange={(event) => updateDraft(selectedSite, (current) => ({ ...current, includeUnclassified: event.target.checked }))} />
                  <span className="site-network-switch-track" aria-hidden="true" />
                  <span><strong>{zh ? "发现未分类 SSH 候选" : "Discover unclassified SSH candidates"}</strong><small>{zh ? "不登录、不检查机器内容；入网时再选择类型。" : "No login or host inspection. Choose the machine type at join time."}</small></span>
                </label>
                {draft.includeUnclassified && <label className="site-network-field">{zh ? "额外 SSH 端口" : "Additional SSH ports"}
                  <input type="text" inputMode="numeric" value={(draft.sshPorts ?? []).join(", ")} disabled={readOnly || pending} placeholder="22, 2222" onChange={(event) => updateDraft(selectedSite, (current) => ({ ...current, sshPorts: event.target.value.trim() ? event.target.value.split(",").map((item) => { const port = Number(item.trim()); return Number.isInteger(port) ? port : 0; }) : [] }))} />
                  <small>{zh ? "逗号分隔，最多 4 个端口；留空时扫描 22。" : "Comma-separated, up to 4 ports. Empty uses port 22."}</small>
                </label>}
              </section>

              <section className="site-network-editor-col">
                <label className="site-network-field">{zh ? "管理面拨号 Hub" : "Management-plane dial-out Hub"}
                  <select value={draft.from.clusterID && draft.from.nodeName ? `${draft.from.clusterID}/${draft.from.nodeName}` : ""} disabled={readOnly || pending} onChange={(event) => { const hub = hubs.find((item) => `${item.clusterID}/${item.nodeName}` === event.target.value); updateDraft(selectedSite, (current) => ({ ...current, from: { clusterID: hub?.clusterID ?? "", nodeName: hub?.nodeName ?? "" } })); }}>
                    <option value="">{zh ? "选择网络中的 Hub" : "Select a network Hub"}</option>
                    {hubs.map((hub) => <option key={`${hub.clusterID}/${hub.nodeName}`} value={`${hub.clusterID}/${hub.nodeName}`}>{hub.clusterID}/{hub.nodeName} · {hub.site} · {hub.address}</option>)}
                  </select>
                  <small>{zh ? "station-controller 从这里拨号连接 Hub 中继；它不是负责扫描的 Hub。" : "station-controller dials through this Hub. It is not the scanner."}</small>
                </label>
                <div className="site-network-readonly"><small>{zh ? "当前拨号源" : "Current dial-out source"}</small><span>{selectedFrom ? `${selectedFrom.clusterID}/${selectedFrom.nodeName} · ${selectedFrom.site} · ${selectedFrom.address}` : (zh ? "尚未选择" : "Not selected")}</span></div>
              </section>

              <section className="site-network-editor-col">
                <div className="site-network-field-head"><strong>{zh ? "额外扫描网段" : "Additional scan ranges"}</strong><small>{zh ? "Hub 直连私网会自动扫描，这里填写额外网段。" : "Hub-connected private networks are scanned automatically. Add extra ranges here."}</small></div>
                <div className="site-network-ranges">
                  {draft.ranges.map((range, index) => <div className="site-network-range-row" key={index}>
                    <input aria-label={zh ? `网段 ${index + 1}` : `Range ${index + 1}`} value={range} disabled={readOnly || pending} placeholder="192.168.31.0/24" onChange={(event) => updateDraft(selectedSite, (current) => ({ ...current, ranges: current.ranges.map((item, at) => at === index ? event.target.value : item) }))} />
                    <button type="button" className="icon-button" aria-label={zh ? "删除网段" : "Remove range"} disabled={readOnly || pending || draft.ranges.length <= 1} onClick={() => updateDraft(selectedSite, (current) => ({ ...current, ranges: current.ranges.filter((_, at) => at !== index) }))}><Trash2 size={14} /></button>
                  </div>)}
                  <div className="site-network-range-actions">
                    <button type="button" className="secondary-button" disabled={readOnly || pending || draft.ranges.length >= 8} onClick={() => updateDraft(selectedSite, (current) => ({ ...current, ranges: [...current.ranges, ""] }))}><Plus size={14} />{zh ? "添加网段" : "Add range"}</button>
                    <span>{zh ? "1–8 条 · /22 到 /32" : "1–8 ranges · /22 to /32"}</span>
                  </div>
                  {validation && <small className="site-network-inline-error" role="alert">{validation}</small>}
                </div>
                <div className="site-network-nonrobot">{zh ? "扫描只覆盖声明了发现规则的入口设备（当前为 S1U 入口）。普通 x86/GPU 服务器请用 SSH 入网或入网码。" : "Discovery covers only entry devices with a discovery declaration (currently S1U). Ordinary x86/GPU servers join over SSH or with a code."}</div>
              </section>
            </div>

            {notice[selectedSite] && <p className={noticeTone[selectedSite] === "warning" ? "site-network-warning" : "site-network-success"} role="status">{notice[selectedSite]}</p>}
          </section>}

          <aside className="site-network-details">
            <section className="site-network-detail-section">
              <h4><Layers3 size={14} />{zh ? "站点信息" : "Site details"}</h4>
              <div className="site-network-chip-list">{selectedSiteData?.clusters.map((cluster) => <span className="site-network-cluster-chip" key={cluster}>{cluster}</span>)}</div>
              <div className="site-network-detail-list">{siteHubs.length ? siteHubs.map((hub) => <div className="site-network-detail-item" key={`${hub.clusterID}/${hub.nodeName}`}><strong>{hub.nodeName}</strong><small>{hub.clusterID} · {hub.address}</small></div>) : <p className="muted">{zh ? "无 Hub，不会扫描。" : "No Hub, so this site will not scan."}</p>}</div>
            </section>
            <section className="site-network-detail-section">
              <h4><Radar size={14} />{zh ? "可发现类型" : "Discoverable types"}</h4>
              {data.nodeClasses.length ? <div className="site-network-detail-list">{data.nodeClasses.map((item) => <div className="site-network-detail-item" key={item.name}><strong>{item.name === "s1u" ? "S1U" : item.name}</strong><small>{zh ? "入口端口" : "Entry port"} {item.entryPort ?? "—"} · {zh ? "访问" : "Access"} {item.hasAccess ? (zh ? "已开启" : "on") : (zh ? "关闭" : "off")} · {zh ? "发现" : "Discovery"} {item.hasDiscovery ? (zh ? "已开启" : "on") : (zh ? "关闭" : "off")}</small></div>)}</div> : <p className="site-network-warning">{zh ? "没有可发现的类型，开启后也不会产生 Machine。" : "No discoverable NodeClass is configured. Enabling discovery will not produce Machines."}</p>}
              <div className="site-network-nonrobot">{zh ? "普通 x86/GPU 没有声明 discovery，不会出现在扫描列表。它们应使用 SSH 入网或入网码。" : "Ordinary x86/GPU servers have no discovery declaration, so they never appear in the scan list. Use SSH or a join code for them."}</div>
            </section>
            <details className="site-network-detail-section">
              <summary><span><CircleCheck size={14} />{zh ? `工厂主机密钥（${data.nodeClasses.reduce((count, item) => count + (item.hostKeyFingerprints?.length ?? 0), 0)}）` : `Factory host keys (${data.nodeClasses.reduce((count, item) => count + (item.hostKeyFingerprints?.length ?? 0), 0)})`}</span><span aria-hidden="true">⌄</span></summary>
              {data.nodeClasses.some((item) => item.hostKeyFingerprints?.length) ? <div className="site-network-detail-list">{data.nodeClasses.map((item) => (item.hostKeyFingerprints ?? []).length > 0 && <div className="site-network-detail-item" key={item.name}><strong>{item.name === "s1u" ? "S1U" : item.name}</strong>{(item.hostKeyFingerprints ?? []).map((fingerprint) => <code className="site-network-fingerprint" key={`${item.name}/${fingerprint}`}>{fingerprint}</code>)}</div>)}</div> : <p className="muted">{zh ? "没有配置工厂主机密钥。" : "No factory host keys are configured."}</p>}
            </details>
          </aside>
        </div>
        <RefreshOverlay visible={refreshing} label={zh ? "正在刷新站点网络…" : "Refreshing site network…"} />
      </div>

      <details className="site-network-rules-panel">
        <summary><span>{zh ? "扫描范围与限制" : "Scanning scope and limits"}</span><span>{zh ? "5 条 · 所有站点共用" : "5 rules · shared by all sites"}</span></summary>
        <ul className="site-network-rules">
          <li>{zh ? "只发现 NodeClass.spec.discovery.hostKeyFingerprints 匹配的入口设备，不是所有 PC，也不是公司设备清单。" : "Only entry devices matching NodeClass.spec.discovery.hostKeyFingerprints are discovered. This is not a scan of every PC or a company device inventory."}</li>
          <li>{zh ? "Hub 直连的私网也会扫描；大于 /22 的网段只扫描 Hub 所在的 /22。" : "Hub-connected private networks are scanned too. For networks wider than /22, only the Hub's /22 is scanned."}</li>
          <li>{zh ? "多个站点网段重叠时可能出现重复 Machine。" : "Overlapping site ranges may produce duplicate Machine records."}</li>
          <li>{zh ? "设置约 30 秒生效；已有 Machine 不会自动删除。" : "Changes take effect in about 30 seconds. Existing Machines are not deleted automatically."}</li>
          <li>{zh ? "没有已入网 Hub 的站点不会扫描。" : "A site without an enrolled Hub will not scan."}</li>
        </ul>
      </details>

      <section className="site-network-bottom">
        <article className="site-network-card">
          <header className="site-network-card-head">
            <div><h3><Activity size={15} />{zh ? "最近扫描事件" : "Recent scan activity"}</h3><p>{zh ? "扫描发现与设置变化" : "Discoveries and settings changes"}</p></div>
            <button type="button" className="icon-button" onClick={() => void load(true)} aria-label={zh ? "刷新扫描事件" : "Refresh scan activity"}><RefreshCw size={14} /></button>
          </header>
          <ul className="site-network-event-list">
            {!insightsLoaded || eventItems.length === 0 ? <li className="site-network-empty">{insightsLoaded ? (zh ? "暂无扫描事件" : "No scan activity yet") : (zh ? "正在读取…" : "Loading…")}</li> : eventItems.map((event, index) => <li key={`${event.at}-${index}`}>
              <time><Clock size={11} />{event.at}</time>
              <i className={`site-network-event-dot is-${event.tone}`} aria-hidden="true" />
              <span>{event.text}</span>
            </li>)}
          </ul>
        </article>

        <article className="site-network-card">
          <header className="site-network-card-head"><div><h3><Radar size={15} />{zh ? "站点发现负载" : "Site discovery load"}</h3><p>{zh ? "各站点本轮发现的机器" : "Machines found by each site"}</p></div></header>
          <div className="site-network-load-list">
            {siteLoad.map((site) => <button type="button" key={site.name} className={`site-network-load-row${site.name === selectedSite ? " is-current" : ""}`} onClick={() => selectSite(site.name)}>
              <span className="site-network-load-head"><span><i className={`site-network-site-dot ${siteState(Boolean(data.sites.find((item) => item.name === site.name)?.discovery?.enabled), site.hubs)}`} />{site.name}<small>{data.sites.find((item) => item.name === site.name)?.clusters[0] ?? ""}</small></span><span>{insightsLoaded ? `${site.count} 台` : "—"} <small>· {site.hubs} Hub</small></span></span>
              <span className="site-network-load-bar"><i style={{ width: `${insightsLoaded ? Math.round(site.count / maxLoad * 100) : 0}%` }} /></span>
            </button>)}
          </div>
          <div className="site-network-load-foot">{scanningSites} {zh ? "个站点在扫描" : "sites scanning"}{latestSeen ? ` · ${zh ? "最近发现" : "last seen"} ${seenTime(latestSeen, now)}` : ""}</div>
        </article>

        <article className="site-network-card">
          <header className="site-network-card-head">
            <div><h3><Server size={15} />{zh ? "Astribot 机器人" : "Astribot robots"}</h3><p>{zh ? "在线、电源、相机与站点" : "Online, power, cameras and sites"}</p></div>
            <span className={`status ${insightsLoaded ? "status-online" : "status-stopped"}`}><i />{insightsLoaded ? (zh ? "实时" : "Live") : (zh ? "读取中" : "Loading")}</span>
          </header>
          <div className="site-network-robot-hero"><strong>{insightsLoaded ? robotsOnline : "—"}</strong><span>{zh ? "台在线" : "online"}</span><span className="status status-stopped">{zh ? `共 ${robots.length} 台` : `${robots.length} total`}</span></div>
          <dl className="site-network-robot-facts">
            <div><dt>{zh ? "机器人在线" : "Robots online"}</dt><dd>{insightsLoaded ? `${robotsOnline}/${robots.length}` : "—"}</dd></div>
            <div><dt>{zh ? "电源" : "Power"}</dt><dd>{insightsLoaded && robots.length ? `${power.ready} ${zh ? "正常" : "ok"} · ${power.fault} ${zh ? "故障" : "fault"} · ${power.other} ${zh ? "其他" : "other"}` : "—"}</dd></div>
            <div><dt>{zh ? "相机" : "Cameras"}</dt><dd>{insightsLoaded && cameraStats.total ? `${cameraStats.ok}/${cameraStats.total} ${zh ? "正常" : "ok"}` : "—"}</dd></div>
            <div><dt>{zh ? "正在入网" : "Joining"}</dt><dd>{insightsLoaded ? joining : "—"}</dd></div>
            <div><dt>{zh ? "站点在线" : "Sites online"}</dt><dd>{insightsLoaded && siteOnline.length ? siteOnline.map((item) => `${item.name} ${item.online}/${item.total}`).join(" · ") : "—"}</dd></div>
          </dl>
          <button type="button" className="secondary-button" onClick={() => navigate("stations")}>{zh ? "查看场站节点" : "View site nodes"}</button>
        </article>
      </section>
    </>}
  </main>;
}
