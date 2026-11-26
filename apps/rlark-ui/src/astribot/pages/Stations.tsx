import { Fragment, useEffect, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { AlertTriangle, Boxes, Check, KeyRound, ListFilter } from "lucide-react";
import { clustersApi, nodesApi } from "../../backend";
import { nodeAdmissionsApi, selfHealApi, siteNodesApi, stationsApi, type StationItem } from "../api";
import { stationDeletable, stationsOfPhase } from "../lib/nodeRemoval";
import { PageToolbar, RefreshOverlay } from "../../components/shared";
import { useAutoRefresh } from "../../hooks";
import type { Copy } from "../../i18n";
import type { ClusterSummary, CRDNode } from "../../types";
import { RobotControls } from "../components/RobotControls";
import { useRobotPower } from "../components/useRobotPower";
import { nodeLabel } from "../lib/displayName";
import { clusterForStation, stationPhase } from "../lib/stationDisplay";
import { isControlPlaneLabels } from "../../utils/nodeVisibility";
import { isRobotNode, robotPowerAfter, robotPowerDesired, type IssuedNodeAdmission, type NodeAdmission, type RobotPowerAction, type SiteNode } from "../lib/siteNodeDisplay";
import { cameraCells, publishedCameras, type CameraCell, type CameraView } from "../lib/cameras";
import { CameraPreviewDialog } from "./CameraPreviewDialog";
import { StationCard, type StationCardVariant } from "./StationCard";
import { StationDetailPanel } from "./StationDetailPanel";
import { AdmissionRecords, NodeOperations } from "./StationRecords";
import "./Stations.css";
import "./StationsCards.css";
import "../components/controlPlaneBadge.css";

function keyOf(item: StationItem) {
  return `${item.metadata.namespace}/${item.metadata.name}`;
}

// The resolved column count of the card grid. The grid decides its tracks from the page width; this reads them back
// so a row's cards can be grouped and the open detail can sit under that row, spanning every column.
function useGridColumns(ref: RefObject<HTMLDivElement | null>) {
  const [columns, setColumns] = useState(1);
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const update = () => {
      const template = getComputedStyle(element).gridTemplateColumns;
      const count = template && template !== "none" ? template.split(" ").filter(Boolean).length : 1;
      setColumns((current) => (current === count ? current : Math.max(1, count)));
    };
    update();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, [ref]);
  return columns;
}

function CardGrid({ items, className, openKey, renderCard, renderPanel }: {
  items: StationItem[];
  className: string;
  openKey: string;
  renderCard: (item: StationItem) => ReactNode;
  renderPanel: (item: StationItem) => ReactNode;
}) {
  const grid = useRef<HTMLDivElement>(null);
  const columns = useGridColumns(grid);
  const rows: StationItem[][] = [];
  for (let start = 0; start < items.length; start += columns) rows.push(items.slice(start, start + columns));
  return <div ref={grid} className={`stations-card-grid ${className}`}>
    {rows.map((row, index) => {
      const opened = row.find((item) => keyOf(item) === openKey);
      return <Fragment key={`${className}-${index}-${row[0] ? keyOf(row[0]) : ""}`}>
        {row.map(renderCard)}
        {opened && renderPanel(opened)}
      </Fragment>;
    })}
  </div>;
}

export function StationsPage({ copy: c, onJoin, onModels }: { copy: Copy; onJoin: (reissued?: IssuedNodeAdmission, sshJoin?: { clusterID: string; name: string }) => void; onModels?: () => void }) {
  const zh = c.nav.overview === "总览";
  const [items, setItems] = useState<StationItem[]>([]);
  const [clusters, setClusters] = useState<ClusterSummary[]>([]);
  // The Nodes by "<namespace>/<name>", for the display names of the site nodes.
  const [nodesByKey, setNodesByKey] = useState<Record<string, CRDNode>>({});
  const [clusterID, setClusterID] = useState("");
  const [phase, setPhase] = useState("");
  const [search, setSearch] = useState("");
  const [continueToken, setContinueToken] = useState("");
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  // Per "<clusterID>/<nodeName>": the Gateway's site-node view; and the admissions of the clusters shown.
  const [siteNodes, setSiteNodes] = useState<Record<string, SiteNode>>({});
  const [admissions, setAdmissions] = useState<NodeAdmission[]>([]);
  const [admissionError, setAdmissionError] = useState("");
  const [siteDataState, setSiteDataState] = useState<"loading" | "ready" | "unavailable">("loading");
  const [showNodeOperations, setShowNodeOperations] = useState(false);
  const [deletePending, setDeletePending] = useState("");
  const [admissionPending, setAdmissionPending] = useState<{ key: string; kind: "reissue" | "revoke" }>();
  const [actionError, setActionError] = useState<{ key: string; message: string }>();
  const [chosen, setChosen] = useState<Set<string>>(new Set());
  // One station is open at a time; its detail spans the row below its card. Every station starts closed.
  const [openKey, setOpenKey] = useState("");
  const [zoom, setZoom] = useState<{ item: StationItem; view: CameraView; cell?: CameraCell; trigger: HTMLElement }>();
  const [now, setNow] = useState(() => Date.now());
  const activeRequest = useRef<AbortController | null>(null);

  const load = async (append = false) => {
    if (append) setLoadingMore(true);
    else setLoading(true);
    setError("");
    activeRequest.current?.abort();
    const controller = new AbortController();
    activeRequest.current = controller;
    try {
      const [response, latestClusters, nodes] = await Promise.all([
        stationsApi.list({ cluster_id: clusterID || undefined, limit: "200", continue: append ? continueToken || undefined : undefined }, controller.signal),
        clustersApi.list<ClusterSummary>(),
        nodesApi.list(),
      ]);
      if (controller.signal.aborted) return;
      setClusters(latestClusters);
      setNodesByKey(Object.fromEntries(nodes.map((node) => [`${node.metadata.namespace}/${node.metadata.name}`, node])));
      if (!append) void loadSiteData(latestClusters, controller.signal);
      setItems((current) => (append ? [...current, ...response.items] : response.items));
      setContinueToken(response.continue ?? "");
    } catch (reason) {
      if (controller.signal.aborted) return;
      setError((reason as Error).message);
      if (!append) setItems([]);
    } finally {
      if (activeRequest.current === controller) {
        activeRequest.current = null;
        setLoading(false);
        setLoadingMore(false);
        setRefreshing(false);
      }
    }
  };

  // One call per cluster for its site nodes and one for its admissions; a cluster whose Station API does not
  // answer just shows no network facts.
  const loadSiteData = async (latestClusters: ClusterSummary[], signal: AbortSignal) => {
    setSiteDataState("loading");
    const targets = latestClusters.filter((cluster) => (!clusterID || cluster.id === clusterID) && !cluster.agentNamespaceConflict);
    const results = await Promise.all(targets.map((cluster) => Promise.allSettled([siteNodesApi.list(cluster.id, signal), nodeAdmissionsApi.list(cluster.id, signal)])));
    if (signal.aborted) return;
    const nextNodes: Record<string, SiteNode> = {};
    const nextAdmissions: NodeAdmission[] = [];
    let failure = "";
    for (const [nodes, admitted] of results) {
      if (nodes.status === "fulfilled") for (const node of nodes.value.items) nextNodes[`${node.cluster_id}/${node.node_name}`] = node;
      else failure = (nodes.reason as Error).message;
      if (admitted.status === "fulfilled") nextAdmissions.push(...admitted.value.items);
      else failure = (admitted.reason as Error).message;
    }
    setSiteNodes(nextNodes);
    setAdmissions(nextAdmissions);
    setAdmissionError(failure);
    setSiteDataState(failure ? "unavailable" : "ready");
  };

  useEffect(() => {
    void load(false);
    return () => activeRequest.current?.abort();
    // Filter changes replace the current snapshot and reset pagination.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clusterID]);
  useAutoRefresh(() => load(false), 30000);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 15000);
    return () => window.clearInterval(timer);
  }, []);
  useEffect(() => setNow(Date.now()), [items]);

  // A namespace's Node by its name, or its name alone when the Node is gone.
  const nodeAt = (namespace: string, name: string) => nodesByKey[`${namespace}/${name}`] ?? { metadata: { name } };
  const nodeOf = (item: StationItem) => nodeAt(item.metadata.namespace, item.metadata.name);
  const siteNodeOf = (item: StationItem) => siteNodes[`${clusterForStation(clusters, item.metadata.namespace)?.id}/${item.metadata.name}`];
  const admissionOf = (item: StationItem) => admissions.find((entry) => entry.cluster_id === clusterForStation(clusters, item.metadata.namespace)?.id && entry.node_name === item.metadata.name);
  const labelsOf = (item: StationItem) => siteNodeOf(item)?.labels ?? admissionOf(item)?.labels;
  const isRobotItem = (item: StationItem) => isRobotNode(labelsOf(item)) && stationPhase(item) !== "Retired";

  // A cluster with an active robot is shown whole in the robot-cluster section; the other clusters stay below.
  const query = search.trim().toLocaleLowerCase();
  const visible = stationsOfPhase(items, phase).filter((item) => !query || [
    clusterForStation(clusters, item.metadata.namespace)?.name ?? "",
    item.metadata.name,
    item.metadata.namespace,
    nodeLabel(nodeOf(item)),
    item.spec.nodeUID,
    item.status?.observation?.systemUUID ?? "",
  ].some((value) => value.toLocaleLowerCase().includes(query)));
  const groups = (() => {
    const map = new Map<string, { id: string; name: string; namespace: string; cluster?: ClusterSummary; items: StationItem[] }>();
    for (const item of visible) {
      const cluster = clusterForStation(clusters, item.metadata.namespace);
      const id = cluster?.id ?? item.metadata.namespace;
      const entry = map.get(id) ?? { id, name: cluster?.name ?? (zh ? "集群未确定" : "Cluster unresolved"), namespace: item.metadata.namespace, cluster, items: [] };
      entry.items.push(item);
      map.set(id, entry);
    }
    return [...map.values()].sort((a, b) => a.name.localeCompare(b.name));
  })();
  // A robot cluster stays a robot cluster even when the filter leaves only its control plane on screen.
  const robotClusterIDs = new Set<string>();
  for (const item of items) {
    if (!isRobotItem(item)) continue;
    const cluster = clusterForStation(clusters, item.metadata.namespace);
    robotClusterIDs.add(cluster?.id ?? item.metadata.namespace);
  }
  const robotGroups = groups.filter((group) => robotClusterIDs.has(group.id));
  const inventoryGroups = groups.filter((group) => !robotClusterIDs.has(group.id));

  const focusExpander = (item: StationItem) => window.requestAnimationFrame(() => document.getElementById(`station-expand-${item.metadata.namespace}-${item.metadata.name}`)?.focus());
  const toggleOpen = (item: StationItem) => {
    const key = keyOf(item);
    if (openKey === key) {
      setOpenKey("");
      focusExpander(item);
    } else {
      setOpenKey(key);
    }
  };
  const closePanel = (item: StationItem) => {
    setOpenKey("");
    focusExpander(item);
  };

  const openZoom = (item: StationItem, view: CameraView, trigger: HTMLElement) => {
    const cells = cameraCells(publishedCameras(siteNodeOf(item)?.cameras), zh, siteDataState !== "unavailable");
    setZoom({ item, view, cell: cells.find((cell) => cell.view === view), trigger });
  };
  const closeZoom = () => {
    const trigger = zoom?.trigger;
    setZoom(undefined);
    window.requestAnimationFrame(() => trigger?.focus());
  };

  const removeRecord = async (item: StationItem) => {
    const itemCluster = clusterForStation(clusters, item.metadata.namespace);
    const key = keyOf(item);
    const hasNode = `${item.metadata.namespace}/${item.metadata.name}` in nodesByKey;
    if (deletePending || !itemCluster || !stationDeletable(item, hasNode) || !window.confirm(zh
      ? `删除场站节点记录 ${item.metadata.name}（节点 UID ${item.spec.nodeUID.slice(0, 8)}）？删除后不能恢复；这个名字的节点如果还在上报，会按新机器重新登记。`
      : `Delete the site node record ${item.metadata.name} (Node UID ${item.spec.nodeUID.slice(0, 8)})? It cannot be brought back; a node of the name that still reports registers again as a new machine.`)) return;
    setDeletePending(key);
    setError("");
    try {
      await selfHealApi.deleteStation(item.metadata.name, itemCluster.id);
      await load(false);
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setDeletePending("");
    }
  };

  const actAdmission = async (item: StationItem, kind: "reissue" | "revoke") => {
    const itemCluster = clusterForStation(clusters, item.metadata.namespace);
    const key = keyOf(item);
    if (!itemCluster || admissionPending || deletePending === key) return;
    if (kind === "revoke" && !window.confirm(zh ? `作废 ${item.metadata.name} 的入网码？` : `Revoke the join code of ${item.metadata.name}?`)) return;
    setAdmissionPending({ key, kind });
    setActionError(undefined);
    try {
      if (kind === "reissue") onJoin(await nodeAdmissionsApi.reissue(item.metadata.name, itemCluster.id));
      else await nodeAdmissionsApi.revoke(item.metadata.name, itemCluster.id);
      await load(false);
    } catch (reason) {
      setActionError({ key, message: (reason as Error).message });
    } finally {
      setAdmissionPending(undefined);
    }
  };

  const power = useRobotPower(zh, () => load(false));

  const count = (value: string) => stationsOfPhase(items, value).length;
  const phaseTabs = [
    { value: "", label: zh ? "全部" : "All" },
    { value: "Bound", label: zh ? "在线" : "Online" },
    { value: "Offline", label: zh ? "离线" : "Offline" },
    { value: "Conflict", label: zh ? "冲突" : "Conflicts" },
    { value: "Retired", label: zh ? "已移除" : "Retired" },
  ];

  const powerPress = (item: StationItem, action: RobotPowerAction, label: string) =>
    void power.set([{ node: item.metadata.name, namespace: item.metadata.namespace, desired: robotPowerAfter(action, robotPowerDesired(labelsOf(item))) }], label);

  const card = (item: StationItem, variant: StationCardVariant) => {
    const key = keyOf(item);
    const labels = labelsOf(item);
    return <StationCard
      key={key}
      station={item}
      cluster={clusterForStation(clusters, item.metadata.namespace)}
      node={nodeOf(item)}
      siteNode={siteNodeOf(item)}
      admission={admissionOf(item)}
      labels={labels}
      siteDataState={siteDataState}
      now={now}
      zh={zh}
      expanded={openKey === key}
      chosen={chosen.has(key)}
      power={{ pending: power.pending, press: (action, label) => powerPress(item, action, label) }}
      variant={variant}
      onToggle={() => toggleOpen(item)}
      onChoose={(checked) => setChosen((current) => { const next = new Set(current); if (checked) next.add(key); else next.delete(key); return next; })}
      onZoom={(view, trigger) => openZoom(item, view, trigger)}
    />;
  };

  const panel = (item: StationItem) => {
    const key = keyOf(item);
    return <StationDetailPanel
      key={`panel-${key}`}
      station={item}
      cluster={clusterForStation(clusters, item.metadata.namespace)}
      siteNode={siteNodeOf(item)}
      admission={admissionOf(item)}
      hasNode={`${item.metadata.namespace}/${item.metadata.name}` in nodesByKey}
      labels={labelsOf(item)}
      powerNote={nodesByKey[`${item.metadata.namespace}/${item.metadata.name}`]?.metadata.annotations?.["rlark.io/power"]}
      siteDataState={siteDataState}
      refreshError={actionError?.key === key ? actionError.message : ""}
      now={now}
      zh={zh}
      power={{ pending: power.pending, press: (action, label) => powerPress(item, action, label) }}
      deleting={deletePending === key}
      admissionPending={admissionPending?.key === key ? admissionPending.kind : ""}
      onDismiss={() => closePanel(item)}
      onDelete={() => void removeRecord(item)}
      onReissue={() => void actAdmission(item, "reissue")}
      onRevoke={() => void actAdmission(item, "revoke")}
      onRefresh={() => { setActionError(undefined); void load(false); }}
      onZoom={(view, trigger) => openZoom(item, view, trigger)}
    />;
  };

  return (
    <div className="page-content resource-page stations-page">
      <div className="stations-phase-tabs" role="group" aria-label={zh ? "按状态筛选" : "Filter by status"}>
        {phaseTabs.map((tab) => <button key={tab.value} type="button" aria-pressed={phase === tab.value} className={phase === tab.value ? "is-active" : ""} onClick={() => setPhase(tab.value)}>
          {tab.label}<span>{count(tab.value)}</span>
        </button>)}
      </div>

      <div className="stations-toolbar">
        <PageToolbar
          placeholder={zh ? "搜索节点名或 UID" : "Search Node or UID"}
          value={search}
          onChange={setSearch}
          count={visible.length}
          copy={c}
          onRefresh={async () => {
            if (refreshing) return;
            setRefreshing(true);
            await load(false);
          }}
          refreshing={refreshing}
        />
        <label className="toolbar-filter stations-cluster-filter">
          <ListFilter size={16} />
          <select aria-label={zh ? "成员集群" : "Member cluster"} value={clusterID} onChange={(event) => setClusterID(event.target.value)}>
            <option value="">{zh ? "所有集群" : "All clusters"}</option>
            {clusters.map((cluster) => <option key={cluster.id} value={cluster.id}>{cluster.name} ({cluster.agentNamespace})</option>)}
          </select>
        </label>
        <button type="button" className="secondary-button station-operation-history" onClick={() => setShowNodeOperations((value) => !value)} disabled={!clusterID} aria-expanded={showNodeOperations}>
          <ListFilter size={15} />{zh ? "节点操作" : "Node operations"}
        </button>
        <button type="button" className="secondary-button" onClick={onModels}>{zh ? "机器人型号" : "Robot models"}<Boxes size={14} /></button>
        <button type="button" className="primary-button station-join-code" onClick={() => onJoin()} disabled={!clusters.length}>
          <KeyRound size={15} />{zh ? "加入节点" : "Add node"}
        </button>
      </div>

      {showNodeOperations && clusterID && <NodeOperations key={clusterID} clusterID={clusterID} zh={zh} nodeAt={nodeAt} />}

      {error && <div className="cert-error" role="alert" style={{ marginBottom: 12 }}>{error}</div>}

      <section className={`stations-groups refreshable-region${refreshing ? " is-refreshing" : ""}`} aria-busy={refreshing} aria-label={zh ? "场站节点" : "Site nodes"}>
        {loading && items.length === 0 ? (
          <div className="stations-state muted">{zh ? "正在加载…" : "Loading…"}</div>
        ) : visible.length === 0 ? (
          <div className="stations-state muted">{zh ? "没有匹配的场站节点" : "No matching site nodes"}</div>
        ) : (
          <>
            {robotGroups.map((group) => {
              const controls = group.items.filter((item) => isControlPlaneLabels(labelsOf(item)));
              const robots = group.items.filter((item) => isRobotItem(item));
              const others = group.items.filter((item) => !isControlPlaneLabels(labelsOf(item)) && !isRobotItem(item));
              const entries = robots.map((item) => ({ key: keyOf(item), node: item.metadata.name, namespace: item.metadata.namespace, labels: labelsOf(item) }));
              const groupChosen = entries.filter((entry) => chosen.has(entry.key));
              const allChosen = entries.length > 0 && groupChosen.length === entries.length;
              return <section className="stations-group stations-robot-group" key={group.id} aria-labelledby={`stations-robot-group-${group.id}`}>
                <header className="stations-group-head">
                  <div><h4 id={`stations-robot-group-${group.id}`}>{group.name}</h4><small>{zh ? "机器人集群" : "Robot cluster"} · {group.namespace}</small></div>
                  <span className="stations-group-count">{robots.length} {zh ? "机器人" : "robots"}</span>
                </header>
                {controls.map((item) => <Fragment key={keyOf(item)}>
                  {card(item, "control")}
                  {openKey === keyOf(item) && <div className="stations-detail-block">{panel(item)}</div>}
                </Fragment>)}
                {robots.length > 0 && <div className="stations-batch-line">
                  <label className="stations-check-label"><input type="checkbox" checked={allChosen} ref={(element) => { if (element) element.indeterminate = groupChosen.length > 0 && !allChosen; }} onChange={(event) => setChosen((current) => { const next = new Set(current); for (const entry of entries) { if (event.target.checked) next.add(entry.key); else next.delete(entry.key); } return next; })} />{zh ? "全选当前集群机器人" : "Select all robots in this cluster"}</label>
                  {groupChosen.length > 0 && <RobotControls robots={entries} chosen={groupChosen} power={power} zh={zh} showMessage={false} />}
                </div>}
                {robots.length > 0 && <CardGrid items={robots} className="is-robot-grid" openKey={openKey} renderCard={(item) => card(item, "robot")} renderPanel={panel} />}
                {others.length > 0 && <CardGrid items={others} className="is-stock-grid" openKey={openKey} renderCard={(item) => card(item, "node")} renderPanel={panel} />}
              </section>;
            })}
            {inventoryGroups.length > 0 ? inventoryGroups.map((group) => {
              const online = group.items.filter((item) => stationPhase(item) === "Bound").length;
              const conflicts = group.items.filter((item) => stationPhase(item) === "Conflict").length;
              return <section className="stations-cluster" key={group.id} aria-label={group.name}>
                <header className="stations-cluster-head">
                  <div><h4>{group.name}</h4><small>{zh ? "普通节点" : "Node inventory"} · {group.namespace}</small></div>
                  <div className="stations-cluster-stats"><span><i className="is-online" />{zh ? "在线" : "Online"} {online}/{group.items.length}</span><span><i className="is-total" />{zh ? "节点" : "Nodes"} {group.items.length}</span>{conflicts > 0 && <span className="is-conflict"><i />{zh ? "冲突" : "Conflicts"} {conflicts}</span>}</div>
                </header>
                <CardGrid items={group.items} className="is-stock-grid" openKey={openKey} renderCard={(item) => card(item, "node")} renderPanel={panel} />
              </section>;
            }) : <p className="stations-state muted">{robotGroups.length ? (zh ? "没有其他场站节点" : "No other site nodes") : (zh ? "没有匹配的场站节点" : "No matching site nodes")}</p>}
          </>
        )}
        <RefreshOverlay visible={refreshing} label={zh ? "刷新中" : "Refreshing"} />
      </section>
      {continueToken && <button className="secondary-button stations-more" type="button" onClick={() => void load(true)} disabled={loadingMore}>{loadingMore ? (zh ? "正在加载…" : "Loading…") : (zh ? "加载更多" : "Load more")}</button>}

      <section className="stations-admission-section" aria-labelledby="stations-admissions-title">
        <header className="stations-admission-section-heading"><div><h3 id="stations-admissions-title">{zh ? "入网记录" : "Admission records"}</h3><small>{zh ? "发码和使用状态 · 入网码本身不会保存" : "Issued and redemption status · codes are never stored"}</small></div><span className="stations-table-count">{admissions.length} {zh ? "条记录" : "records"}</span></header>
        <AdmissionRecords admissions={admissions} clusters={clusters} loadError={admissionError} zh={zh} onJoin={onJoin} onChanged={() => load(false)} />
      </section>
      {zoom && <CameraPreviewDialog node={zoom.item.metadata.name} namespace={zoom.item.metadata.namespace} view={zoom.view} cell={zoom.cell} zh={zh} onDismiss={closeZoom} />}
      {(power.pending || power.message) && <div className={`stations-power-toast${power.failed ? " is-error" : ""}`} role={power.failed ? "alert" : "status"}>
        <span className={`stations-power-toast-icon${power.pending ? " is-pending" : ""}`} aria-hidden="true">{power.pending ? <span className="stations-power-spinner" /> : power.failed ? <AlertTriangle size={16} /> : <Check size={16} />}</span>
        <div className="stations-power-toast-copy">
          <strong>{power.pending ? (zh ? `正在下发：${power.activeLabel || "命令"}` : `Sending: ${power.activeLabel || "command"}`) : power.failed ? (zh ? "操作失败" : "Action failed") : (zh ? "操作已下发" : "Action sent")}</strong>
          <p>{power.pending ? (zh ? "请稍候，命令正在发送…" : "Sending the command…") : power.message}</p>
        </div>
        {!power.pending && <button type="button" className="stations-power-toast-close" onClick={power.clear} aria-label={zh ? "关闭" : "Dismiss"}>×</button>}
      </div>}
    </div>
  );
}
