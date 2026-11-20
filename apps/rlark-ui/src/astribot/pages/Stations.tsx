import { useEffect, useRef, useState } from "react";
import {
  AlertTriangle,
  CircleCheck,
  CircleHelp,
  CircleX,
  Fingerprint,
  KeyRound,
  ListFilter,
  MonitorSmartphone,
  Power,
  ShieldCheck,
  Unplug,
} from "lucide-react";
import { clustersApi, nodesApi } from "../../backend";
import { nodeAdmissionsApi, selfHealApi, siteNodesApi, stationsApi, type StationItem } from "../api";
import { stationDeletable, stationsOfPhase } from "../lib/nodeRemoval";
import { PageToolbar, RefreshOverlay } from "../../components/shared";
import { useAutoRefresh } from "../../hooks";
import type { Copy } from "../../i18n";
import type { ClusterSummary, CRDNode } from "../../types";
import { formatChinaDateTime } from "../../utils/time.js";
import { NodeName } from "../components/NodeName";
import { nodeLabel } from "../lib/displayName";
import {
  clusterForStation,
  formatRelativeTime,
  shortUID,
  stationNetworkState,
  stationPhase,
  stationPhaseLabel,
  stationStatusClass,
  stationUptime,
  type StationNetworkState,
} from "../lib/stationDisplay";
import {
  admissionStateClass,
  admissionStateLabel,
  isRobotNode,
  keyFingerprint,
  machineModel,
  robotPowerAfter,
  robotPowerDesired,
  robotPowerNodes,
  type IssuedNodeAdmission,
  type NodeAdmission,
  type RobotPowerDesired,
  type SiteNode,
} from "../lib/siteNodeDisplay";
import { isControlPlaneLabels } from "../../utils/nodeVisibility";
import { StationDrawer } from "./StationDrawer";
import { AdmissionRecords, NodeOperations } from "./StationRecords";
import "./Stations.css";
import "../components/controlPlaneBadge.css";

function NetworkBadge({ state, zh }: { state: StationNetworkState; zh: boolean }) {
  const Icon = state === "verified" ? CircleCheck : state === "failed" ? CircleX : CircleHelp;
  const label = state === "verified" ? (zh ? "已验证" : "Verified") : state === "failed" ? (zh ? "探测失败" : "Probe failed") : (zh ? "未知" : "Unknown");
  return <span className={`station-network station-network-${state}`}><Icon size={14} aria-hidden="true" />{label}</span>;
}

// onJoin opens the page that adds a node, with the code a reissue answered, or on an SSH join of a node.
export function StationsPage({ copy: c, onJoin }: { copy: Copy; onJoin: (reissued?: IssuedNodeAdmission, sshJoin?: { clusterID: string; name: string }) => void }) {
  const zh = c.nav.overview === "总览";
  const [items, setItems] = useState<StationItem[]>([]);
  const [clusters, setClusters] = useState<ClusterSummary[]>([]);
  // The Nodes by "<namespace>/<name>", for the display names of the site nodes.
  const [nodesByKey, setNodesByKey] = useState<Record<string, CRDNode>>({});
  const [clusterID, setClusterID] = useState("");
  const [phase, setPhase] = useState("");
  const [search, setSearch] = useState("");
  const [selectedID, setSelectedID] = useState("");
  const [continueToken, setContinueToken] = useState("");
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  // Per "<clusterID>/<nodeName>": the Gateway's site-node view; and the admissions of the clusters shown.
  const [siteNodes, setSiteNodes] = useState<Record<string, SiteNode>>({});
  const [admissions, setAdmissions] = useState<NodeAdmission[]>([]);
  const [admissionError, setAdmissionError] = useState("");
  const [showNodeOperations, setShowNodeOperations] = useState(false);
  const [deletePending, setDeletePending] = useState("");
  const [powerPending, setPowerPending] = useState(false);
  const [powerMessage, setPowerMessage] = useState("");
  const [chosen, setChosen] = useState<Set<string>>(new Set());
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
      if (!append && !response.items.some((item) => keyOf(item) === selectedID)) setSelectedID("");
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
    const targets = latestClusters.filter((cluster) => (!clusterID || cluster.id === clusterID) && !cluster.agentNamespaceConflict);
    const results = await Promise.all(targets.map((cluster) => Promise.allSettled([siteNodesApi.list(cluster.id, signal), nodeAdmissionsApi.list(cluster.id, signal)])));
    if (signal.aborted) return;
    const nextNodes: Record<string, SiteNode> = {};
    const nextAdmissions: NodeAdmission[] = [];
    let failure = "";
    for (const [nodes, admitted] of results) {
      if (nodes.status === "fulfilled") for (const node of nodes.value.items) nextNodes[`${node.cluster_id}/${node.node_name}`] = node;
      if (admitted.status === "fulfilled") nextAdmissions.push(...admitted.value.items);
      else failure = (admitted.reason as Error).message;
    }
    setSiteNodes(nextNodes);
    setAdmissions(nextAdmissions);
    setAdmissionError(failure);
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
  const keyOf = (item: StationItem) => `${item.metadata.namespace}/${item.metadata.name}`;
  const siteNodeOf = (item: StationItem) => siteNodes[`${clusterForStation(clusters, item.metadata.namespace)?.id}/${item.metadata.name}`];

  // The record of a removed node, of a node in Conflict or of a name no node reports under and RLark has no Node of
  // is deleted when an administrator asks; a node that comes back registers again with its next report.
  const removeRecord = async (item: StationItem, itemClusterID: string) => {
    if (deletePending || !window.confirm(zh
      ? `删除场站节点记录 ${item.metadata.name}（节点 UID ${item.spec.nodeUID.slice(0, 8)}）？删除后不能恢复；这个名字的节点如果还在上报，会按新机器重新登记。`
      : `Delete the site node record ${item.metadata.name} (Node UID ${item.spec.nodeUID.slice(0, 8)})? It cannot be brought back; a node of the name that still reports registers again as a new machine.`)) return;
    setDeletePending(keyOf(item));
    setError("");
    try {
      await selfHealApi.deleteStation(item.metadata.name, itemClusterID);
      await load(false);
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setDeletePending("");
    }
  };

  const query = search.trim().toLocaleLowerCase();
  const visible = stationsOfPhase(items, phase).filter((item) => !query || [
    clusterForStation(clusters, item.metadata.namespace)?.name ?? "",
    item.metadata.name,
    item.metadata.namespace,
    nodeLabel(nodeOf(item)),
    item.spec.nodeUID,
    item.status?.observation?.systemUUID ?? "",
  ].some((value) => value.toLocaleLowerCase().includes(query)));
  const selected = items.find((item) => keyOf(item) === selectedID);
  const selectedLabels = selected && siteNodeOf(selected)?.labels;

  // The desired state of robots' Nodes, which the Addon's power service converges on: one request per robot, each
  // recorded by the Gateway with who made it.
  const setPower = async (nodes: { node: string; namespace: string; desired: RobotPowerDesired }[], label: string) => {
    if (!nodes.length) return;
    const names = nodes.map((entry) => entry.node).join(zh ? "、" : ", ");
    if (!window.confirm(zh ? `${label}：${names}\n\n请确认这些机器人周围安全、现场有人守着急停。` : `${label}: ${names}\n\nMake sure the robots' surroundings are safe and someone is at their emergency stops.`)) return;
    setPowerPending(true);
    setPowerMessage("");
    const failed: string[] = [];
    await Promise.all(nodes.map((entry) => stationsApi.setRobotPower(entry.node, entry.namespace, entry.desired)
      .catch((reason) => failed.push(`${entry.node}: ${(reason as Error).message}`))));
    setPowerMessage(failed.length
      ? failed.join("; ")
      : (zh ? `期望状态已改（${label}，${nodes.length} 台），机器人几秒内跟上；没跟上的原因见节点注解 rlark.io/power 的 waiting。` : `Desired state set (${label}, ${nodes.length}); the robots follow within seconds. Why one does not is in its Node's annotation rlark.io/power (waiting).`));
    setPowerPending(false);
    await load(false);
  };
  // The robots of the list a batch may act on, and those chosen.
  const robots = visible.map((item) => ({ key: keyOf(item), node: item.metadata.name, namespace: item.metadata.namespace, labels: siteNodeOf(item)?.labels }));
  const powerRobots = robotPowerNodes(robots, new Set(robots.map((robot) => robot.key)));
  const chosenRobots = robotPowerNodes(robots, chosen);
  const pressBatch = (desired: RobotPowerDesired, label: string) =>
    setPower(chosenRobots.map((robot) => ({ node: robot.node, namespace: robot.namespace, desired })), label);

  const count = (value: string) => stationsOfPhase(items, value).length;
  const phaseOptions = [
    { value: "", label: zh ? "所有状态（不含已移除）" : "All phases (not removed)" },
    ...(["Bound", "Offline", "Conflict", "Retired"] as const).map((value) => ({ value, label: stationPhaseLabel(value, zh) })),
  ];

  return (
    <div className="page-content resource-page stations-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow"><MonitorSmartphone size={13} />{zh ? "场站节点" : "Site Nodes"}</span>
          <h2>{zh ? "场站节点" : "Site Nodes"}</h2>
          <p>{zh ? "场站节点是一台节点在场站里的记录：身份、心跳和入网，按节点名一台一条，不是 Fleet Run 里的 Station（Run 的 Station 引用这里的节点）。数据来自 station-agent，每 30 秒刷新。" : "A site node is the record of one node in the site: identity, heartbeat and network membership, one per node name. It is not a Station of a Fleet Run, which refers to a site node. Data comes from station-agent and refreshes every 30 seconds."}</p>
        </div>
      </div>

      <section className="storage-overview-grid stations-overview-grid" aria-label={zh ? "场站节点概况" : "Site node overview"}>
        <div className="storage-overview-card purple">
          <span><Fingerprint size={18} /></span>
          <div><small>{zh ? "场站节点总数" : "Site nodes"}</small><strong>{items.length}</strong><em>{zh ? "已加载的记录" : "loaded records"}</em></div>
        </div>
        <div className="storage-overview-card green">
          <span><ShieldCheck size={18} /></span>
          <div><small>{zh ? "在线" : "Bound"}</small><strong>{count("Bound")}</strong><em>{zh ? "心跳正常" : "heartbeat fresh"}</em></div>
        </div>
        <div className="storage-overview-card orange">
          <span><Unplug size={18} /></span>
          <div><small>{zh ? "离线" : "Offline"}</small><strong>{count("Offline")}</strong><em>{zh ? "心跳超时" : "heartbeat stale"}</em></div>
        </div>
        <div className="storage-overview-card red">
          <span><AlertTriangle size={18} /></span>
          <div><small>{zh ? "冲突" : "Conflict"}</small><strong>{count("Conflict")}</strong><em>{zh ? "节点 UID 变了，删除记录后重新登记" : "Node UID changed; delete the record"}</em></div>
        </div>
      </section>

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
          filterValue={phase}
          onFilterChange={setPhase}
          filterOptions={phaseOptions}
        />
        <label className="toolbar-filter stations-cluster-filter">
          <ListFilter size={16} />
          <select aria-label={zh ? "成员集群" : "Member cluster"} value={clusterID} onChange={(event) => setClusterID(event.target.value)}>
            <option value="">{zh ? "所有集群" : "All clusters"}</option>
            {clusters.map((cluster) => <option key={cluster.id} value={cluster.id}>{cluster.name} ({cluster.agentNamespace})</option>)}
          </select>
        </label>
        <button type="button" className="primary-button station-join-code" onClick={() => onJoin()} disabled={!clusters.length}>
          <KeyRound size={15} />{zh ? "加入节点" : "Add node"}
        </button>
        <button type="button" className="icon-button station-operation-history" onClick={() => setShowNodeOperations((value) => !value)} disabled={!clusterID} aria-label={zh ? "查看节点操作" : "View node operations"} title={zh ? "节点操作历史" : "Node operation history"}>
          <ListFilter size={16} />
        </button>
      </div>

      {showNodeOperations && clusterID && <NodeOperations key={clusterID} clusterID={clusterID} zh={zh} nodeAt={nodeAt} />}

      {error && <div className="cert-error" role="alert" style={{ marginBottom: 12 }}>{error}</div>}

      <div className={`table-panel refreshable-region${refreshing ? " is-refreshing" : ""}`} aria-busy={refreshing} aria-label={zh ? "场站节点列表" : "Site node list"}>
        {loading && items.length === 0 ? (
          <div className="stations-state muted">{zh ? "正在加载…" : "Loading…"}</div>
        ) : visible.length === 0 ? (
          <div className="stations-state muted">{zh ? "没有匹配的场站节点" : "No matching site nodes"}</div>
        ) : (
          <>{powerRobots.length > 0 && <div className="stations-power-batch" role="group" aria-label={zh ? "机器人电源（批量）" : "Robot power (batch)"}>
            <Power size={14} />
            <span>{zh ? `机器人电源：已选 ${chosenRobots.length} / ${powerRobots.length} 台` : `Robot power: ${chosenRobots.length} of ${powerRobots.length} chosen`}</span>
            <button type="button" className="secondary-button" disabled={powerPending || chosenRobots.length === 0} onClick={() => void pressBatch("ready", zh ? "上电并启动驱动" : "Power on and start driver")}>{zh ? "上电并启动驱动" : "Power on and start driver"}</button>
            <button type="button" className="secondary-button" disabled={powerPending || chosenRobots.length === 0} onClick={() => void pressBatch("off", zh ? "关闭驱动并下电" : "Stop driver and power off")}>{zh ? "关闭驱动并下电" : "Stop driver and power off"}</button>
            {!selected && powerMessage && <small role="status">{powerMessage}</small>}
          </div>}
          <table className="stations-table">
            <thead><tr>
              <th>{powerRobots.length > 0 && <input type="checkbox" aria-label={zh ? "全选机器人" : "Select every robot"} checked={chosenRobots.length === powerRobots.length} onChange={(event) => setChosen(event.target.checked ? new Set(powerRobots.map((robot) => robot.key)) : new Set())} />}</th>
              <th>{zh ? "节点" : "Node"}</th>
              <th>{zh ? "集群" : "Cluster"}</th>
              <th>{zh ? "状态" : "Phase"}</th>
              <th>{zh ? "网络" : "Network"}</th>
              <th>{zh ? "入网" : "Admission"}</th>
              <th title={zh ? "与机器上 wg show <接口> public-key 的开头一致" : "The beginning of wg show <interface> public-key on the machine"}>{zh ? "WireGuard 公钥" : "WireGuard key"}</th>
              <th>{zh ? "机型" : "Model"}</th>
              <th>{zh ? "重启" : "Reboots"}</th>
              <th>{zh ? "运行时间" : "Uptime"}</th>
              <th>{zh ? "最近上报" : "Last seen"}</th>
            </tr></thead>
            <tbody>
              {visible.map((item) => {
                const key = keyOf(item);
                const itemPhase = stationPhase(item);
                const retired = itemPhase === "Retired";
                const deletable = stationDeletable(item, `${item.metadata.namespace}/${item.metadata.name}` in nodesByKey);
                const itemCluster = clusterForStation(clusters, item.metadata.namespace);
                const siteNode = siteNodeOf(item);
                const admission = admissions.find((entry) => entry.cluster_id === itemCluster?.id && entry.node_name === item.metadata.name);
                const publicKey = siteNode?.network?.public_key ?? admission?.public_key;
                return (
                  <tr key={key} className={`clickable-row${selectedID === key ? " selected" : ""}`} onClick={() => setSelectedID(key)}>
                    <td onClick={(event) => event.stopPropagation()}>
                      {isRobotNode(siteNode?.labels) && <input type="checkbox" aria-label={zh ? `选中 ${nodeLabel(nodeOf(item))}` : `Select ${nodeLabel(nodeOf(item))}`} checked={chosen.has(key)} onChange={(event) => setChosen((before) => { const next = new Set(before); if (event.target.checked) next.add(key); else next.delete(key); return next; })} />}
                    </td>
                    <td className="stations-name-cell">
                      <button type="button" className="stations-name" onClick={() => setSelectedID(key)}>
                        <strong><NodeName node={nodeOf(item)} /></strong>
                      </button>
                      {isControlPlaneLabels(siteNode?.labels) && <span className="control-plane-badge" title={zh ? "这台是所在集群的控制面" : "This node is its cluster's control plane"}>{zh ? "控制面" : "Control plane"}</span>}
                      <code className="inline-code" title={item.spec.nodeUID}>{shortUID(item.spec.nodeUID)}</code>
                    </td>
                    <td>{itemCluster?.name ?? <span className="muted">{zh ? "集群未确定" : "Cluster unresolved"}</span>}</td>
                    <td onClick={(event) => deletable && event.stopPropagation()}>
                      <span className={`status ${stationStatusClass(itemPhase)}`}><i />{stationPhaseLabel(itemPhase, zh)}</span>
                      {deletable && itemCluster && <button type="button" className="table-action-button danger station-record-delete" onClick={() => void removeRecord(item, itemCluster.id)} disabled={!!deletePending}>
                        {deletePending === key ? (zh ? "删除中…" : "Deleting…") : (zh ? "删除记录" : "Delete record")}
                      </button>}
                    </td>
                    <td>{retired ? <span className="muted">—</span> : <NetworkBadge state={stationNetworkState(item)} zh={zh} />}</td>
                    <td>{retired ? <span className="muted">—</span> : admission ? <span className={`status ${admissionStateClass(admission.state)}`} title={admission.reason}><i />{admissionStateLabel(admission.state, zh)}</span> : <span className="muted">{zh ? "未用入网码" : "no code"}</span>}</td>
                    <td>{publicKey ? <code className="inline-code" title={publicKey}>{keyFingerprint(publicKey)}</code> : <span className="muted">—</span>}</td>
                    <td>{machineModel(siteNode?.labels ?? admission?.labels) || <span className="muted">—</span>}</td>
                    <td>{item.status?.node?.reboots ?? <span className="muted">—</span>}</td>
                    <td><small>{itemPhase === "Bound" ? stationUptime(item.status?.node?.bootTime, now, zh) : "—"}</small></td>
                    <td><small title={formatChinaDateTime(item.status?.node?.lastSeenTime)}>{formatRelativeTime(item.status?.node?.lastSeenTime, now, zh)}</small></td>
                  </tr>
                );
              })}
            </tbody>
          </table></>
        )}
        <RefreshOverlay visible={refreshing} label={zh ? "刷新中" : "Refreshing"} />
      </div>
      {continueToken && <button className="secondary-button stations-more" type="button" onClick={() => void load(true)} disabled={loadingMore}>{loadingMore ? (zh ? "正在加载…" : "Loading…") : (zh ? "加载更多" : "Load more")}</button>}

      <AdmissionRecords admissions={admissions} clusters={clusters} loadError={admissionError} zh={zh} onJoin={onJoin} onChanged={() => load(false)} />

      {selected && <StationDrawer
        key={selectedID}
        station={selected}
        cluster={clusterForStation(clusters, selected.metadata.namespace)}
        node={nodeOf(selected)}
        labels={selectedLabels}
        now={now}
        zh={zh}
        power={{
          pending: powerPending,
          message: powerMessage,
          press: (action, label) => void setPower([{ node: selected.metadata.name, namespace: selected.metadata.namespace, desired: robotPowerAfter(action, robotPowerDesired(selectedLabels)) }], label),
        }}
        onClose={() => setSelectedID("")}
      />}
    </div>
  );
}
