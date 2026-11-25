import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import {
  Activity,
  Bot,
  ChevronRight,
  CloudCog,
  Link2,
  MoreHorizontal,
  MoreVertical,
  MapPin,
  Network,
  Server,
  Unlink2,
} from "lucide-react";
import type { Copy } from "../i18n";
import type { ClusterSummary, CRDNode, NodeCategory } from "../types";
import { useAutoRefresh } from "../hooks";
import {
  getNodeCategories,
  getNodeDeviceModel,
  getNodeGPUModel,
  hasNodeCategory,
  isBusinessWorkerNode,
} from "../utils/nodes";
import { buildNodeWorkloads } from "../utils/nodeWorkloads";
import { nodeConnectionState } from "../utils/nodeState";
import {
  ColumnFilterButton,
  MetricCard,
  PageToolbar,
  Pagination,
  RefreshOverlay,
  useColumnFilter,
} from "../components/shared";
import { ColumnFilterPopover } from "../components/ColumnFilterPopover";
import { NodeResourceBrowser } from "../components/NodeResourceBrowser";
import {
  clustersApi,
  dataPlaneNodesApi,
  nodesApi,
  podsApi,
  tasksApi,
} from "../backend";
import type { ClusterDataPlaneNodes } from "../backend";

type ClusterAction =
  "cordon" | "uncordon" | "drain" | "undrain" | "delist" | "restore" | "delete";

const clusterActionDescriptions: Record<
  Exclude<ClusterAction, "delete">,
  { zh: string; en: string }
> = {
  cordon: {
    zh: "暂停新任务调度到集群",
    en: "Stop scheduling new tasks to this cluster",
  },
  uncordon: {
    zh: "恢复集群的新任务调度",
    en: "Resume scheduling new tasks to this cluster",
  },
  drain: {
    zh: "停止集群内所有运行中的 RLark 任务",
    en: "Stop all running RLark tasks in this cluster",
  },
  undrain: {
    zh: "取消排空并恢复排空前的调度状态",
    en: "Cancel draining and restore the pre-drain scheduling state",
  },
  delist: {
    zh: "下架后用户将无法看到该集群和节点",
    en: "Delist: stop showing this cluster and its nodes to users",
  },
  restore: {
    zh: "恢复集群和节点对用户可见",
    en: "Make this cluster and its nodes visible to users again",
  },
};

function clusterTypeLabel(type: string, zh: boolean): string {
  if (!type) return "—";
  const map: Record<string, { zh: string; en: string }> = {
    Cloud: { zh: "云集群", en: "Cloud" },
    Embodied: { zh: "具身集群", en: "Embodied" },
    Hybrid: { zh: "混合集群", en: "Hybrid" },
  };
  const entry = map[type];
  if (!entry) return type;
  return zh ? entry.zh : entry.en;
}

function aggregateClusterNodes(
  cluster: ClusterSummary,
  nodes: CRDNode[],
  workerOnly = false,
): ClusterSummary {
  const clusterNodes = cluster.agentNamespaceConflict
    ? []
    : nodes.filter(
        (node) => node.metadata.namespace === cluster.agentNamespace,
      );
  const categories: Record<NodeCategory, number> = {
    cloud: 0,
    edge: 0,
    robot: 0,
    unknown: 0,
  };
  clusterNodes.forEach((node) =>
    getNodeCategories(node).forEach((category) => categories[category]++),
  );
  const inferredType =
    categories.cloud > 0 && categories.edge === 0 && categories.robot === 0
      ? "Cloud"
      : categories.cloud === 0 && (categories.edge > 0 || categories.robot > 0)
        ? "Embodied"
        : "Hybrid";
  const workerNodes = workerOnly
    ? clusterNodes.filter(isBusinessWorkerNode)
    : clusterNodes;
  const totalNodes = workerNodes.length;
  const models = (category: NodeCategory) => [
    ...new Set(
      clusterNodes
        .filter((node) => hasNodeCategory(node, category))
        .map((node) =>
          category === "cloud"
            ? getNodeGPUModel(node)
            : getNodeDeviceModel(node),
        )
        .filter((value): value is string => Boolean(value)),
    ),
  ];
  return {
    ...cluster,
    type: cluster.type || inferredType,
    totalNodes,
    offlineNodes: Math.max(
      0,
      totalNodes - (cluster.readyNodes ?? cluster.onlineNodes),
    ),
    cloudNodes: categories.cloud,
    embodiedNodes: categories.edge,
    robots: categories.robot,
    gpuModels: models("cloud"),
    robotModels: models("robot"),
  };
}

function validCount(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function normalizeClusterSummary(
  cluster: ClusterSummary,
  nodeAggregate?: ClusterSummary,
): ClusterSummary {
  const categoryTotal =
    (validCount(cluster.cloudNodes) ? cluster.cloudNodes : 0) +
    (validCount(cluster.embodiedNodes) ? cluster.embodiedNodes : 0) +
    (validCount(cluster.robots) ? cluster.robots : 0);
  const totalNodes =
    nodeAggregate?.totalNodes ??
    (validCount(cluster.totalNodes) ? cluster.totalNodes : categoryTotal);
  const onlineNodes = validCount(cluster.onlineNodes)
    ? cluster.onlineNodes
    : cluster.phase === "Online"
      ? totalNodes
      : 0;
  const offlineNodes = validCount(cluster.offlineNodes)
    ? cluster.offlineNodes
    : Math.max(0, totalNodes - onlineNodes);

  return {
    ...cluster,
    type: nodeAggregate?.type ?? cluster.type,
    phase: cluster.phase,
    totalNodes,
    onlineNodes,
    offlineNodes,
    readyNodes: cluster.readyNodes ?? onlineNodes,
    degradedNodes: cluster.degradedNodes ?? 0,
    disconnectedNodes: cluster.disconnectedNodes ?? 0,
    unschedulableNodes: cluster.unschedulableNodes ?? 0,
    dominantFailureReason: cluster.dominantFailureReason,
    cloudNodes: nodeAggregate?.cloudNodes ?? cluster.cloudNodes ?? 0,
    embodiedNodes: nodeAggregate?.embodiedNodes ?? cluster.embodiedNodes ?? 0,
    robots: nodeAggregate?.robots ?? cluster.robots ?? 0,
    gpuModels: cluster.gpuModels ?? nodeAggregate?.gpuModels ?? [],
    robotModels: cluster.robotModels ?? nodeAggregate?.robotModels ?? [],
    // Running Jobs is a server-owned field: the console must not re-derive it
    // from Node labels. Action gating (drain/delist/delete) relies on it.
    runningJobs: validCount(cluster.runningJobs) ? cluster.runningJobs : 0,
    region: cluster.region ?? "",
    location: cluster.location ?? "",
    description: cluster.description ?? "",
  };
}

// 状态列与筛选项共用同一套标签，与后端 Cluster phase 一一对应。
const CLUSTER_STATUS_LABELS: Record<string, { zh: string; en: string }> = {
  Online: { zh: "在线", en: "Online" },
  Offline: { zh: "离线", en: "Offline" },
  Downgraded: { zh: "服务降级", en: "Downgraded" },
  Delisted: { zh: "已下架", en: "Delisted" },
  Cordoned: { zh: "停用调度", en: "Cordoned" },
  Draining: { zh: "排空中", en: "Draining" },
};

function ClusterStatus({ phase, zh }: { phase: string; zh: boolean }) {
  const label = CLUSTER_STATUS_LABELS[phase] ?? { zh: phase, en: phase };
  return (
    <span className={`cluster-health-badge ${phase.toLowerCase()}`}>
      <i /> {zh ? label.zh : label.en}
    </span>
  );
}

export function ClusterManagementPage({
  copy: c,
  selectedClusterID,
  onSelectCluster,
  onSelectNode,
  workerOnly = false,
  adminMode = false,
}: {
  copy: Copy;
  selectedClusterID?: string;
  onSelectCluster: (id?: string) => void;
  onSelectNode: (name: string) => void;
  workerOnly?: boolean;
  adminMode?: boolean;
}) {
  const zh = c.nav.overview === "总览";
  const [clusters, setClusters] = useState<ClusterSummary[]>([]);
  const [detailNodes, setDetailNodes] = useState<CRDNode[]>([]);
  const [nodeWorkloads, setNodeWorkloads] = useState<
    Record<string, { jobs: string[]; workers: number }>
  >({});
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [query, setQuery] = useState("");
  // 状态列多选筛选；空数组 = 全部
  const [phaseFilterValues, setPhaseFilterValues] = useState<string[]>([]);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [actionClusterID, setActionClusterID] = useState<string>();
  const [confirmation, setConfirmation] = useState<{
    cluster: ClusterSummary;
    action: ClusterAction;
  }>();
  const [actionPending, setActionPending] = useState(false);
  const [error, setError] = useState("");
  const [cordoningNode, setCordoningNode] = useState<string | null>(null);
  const [drainingNode, setDrainingNode] = useState<string | null>(null);
  const [removingNode, setRemovingNode] = useState<string | null>(null);
  const { openKey, anchorRect, openFor, close } = useColumnFilter();
  const clusterActionsRef = useRef<HTMLSpanElement | null>(null);

  // 点击操作菜单以外的区域时，收起已展开的集群操作菜单
  useEffect(() => {
    if (!actionClusterID) return;
    const handlePointerDown = (event: PointerEvent) => {
      if (!clusterActionsRef.current?.contains(event.target as Node)) {
        setActionClusterID(undefined);
      }
    };
    document.addEventListener("pointerdown", handlePointerDown);
    return () => document.removeEventListener("pointerdown", handlePointerDown);
  }, [actionClusterID]);

  const fetchClusters = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    let resolvedNodes: CRDNode[] = [];
    try {
      resolvedNodes = await nodesApi.list({ labelSelector: undefined });
    } catch (e) {
      resolvedNodes = [];
      setError(e instanceof Error ? e.message : String(e));
    }
    // The Pod and Task mirrors only enrich the display; their failure must not
    // hide the Nodes or clusters.
    try {
      const [tasks, pods] = await Promise.all([
        tasksApi.list(),
        podsApi.list(),
      ]);
      setNodeWorkloads(buildNodeWorkloads(tasks, pods));
    } catch (e) {
      setNodeWorkloads({});
      setError(e instanceof Error ? e.message : String(e));
    }

    let resolvedClusters: ClusterSummary[] = [];
    try {
      const rawClusters = await clustersApi.list<ClusterSummary>();
      resolvedClusters = rawClusters.map((cluster) =>
        normalizeClusterSummary(
          cluster,
          aggregateClusterNodes(cluster, resolvedNodes, workerOnly),
        ),
      );
    } catch {
      resolvedClusters = [];
    }
    if (!adminMode) {
      resolvedClusters = resolvedClusters.filter(
        (cluster) => cluster.phase !== "Delisted",
      );
    }

    if (selectedClusterID) {
      try {
        const detail = await clustersApi.get<
          ClusterSummary & { nodes?: CRDNode[] }
        >(selectedClusterID);
        if (!detail) throw new Error("cluster not found");
        const fullNodes = detail.agentNamespaceConflict
          ? []
          : resolvedNodes.filter(
              (node) => node.metadata.namespace === detail.agentNamespace,
            );
        const fullNodeByKey = new Map(
          fullNodes.map((node) => [
            `${node.metadata.namespace ?? ""}/${node.metadata.name}`,
            node,
          ]),
        );
        const enrichedDetailNodes = (detail.nodes ?? []).map((node) => {
          const fullNode = fullNodeByKey.get(
            `${node.metadata.namespace ?? ""}/${node.metadata.name}`,
          );
          return fullNode
            ? {
                ...node,
                metadata: {
                  ...node.metadata,
                  labels: fullNode.metadata.labels,
                  annotations: fullNode.metadata.annotations,
                },
              }
            : node;
        });
        const detailNodeSource =
          fullNodes.length > 0 ? fullNodes : enrichedDetailNodes;
        setDetailNodes(
          workerOnly
            ? detailNodeSource.filter(isBusinessWorkerNode)
            : detailNodeSource,
        );
        if (!resolvedClusters.some((cluster) => cluster.id === detail.id)) {
          resolvedClusters = [
            normalizeClusterSummary(
              detail,
              aggregateClusterNodes(detail, resolvedNodes, workerOnly),
            ),
            ...resolvedClusters,
          ];
        }
      } catch {
        setDetailNodes([]);
      }
    } else {
      setDetailNodes([]);
    }
    setClusters(resolvedClusters);
    setLoading(false);
  };

  useAutoRefresh(fetchClusters, 10000, [selectedClusterID, adminMode]);

  const handleClusterAction = (
    cluster: ClusterSummary,
    action: ClusterAction,
  ) => {
    setError("");
    setActionClusterID(undefined);
    setConfirmation({ cluster, action });
  };

  const confirmClusterAction = async () => {
    if (!confirmation) return;
    const { cluster, action } = confirmation;
    setActionPending(true);
    setError("");
    try {
      if (action === "delete") {
        await clustersApi.remove(cluster.id);
        if (selectedClusterID === cluster.id) onSelectCluster(undefined);
      } else {
        await clustersApi.lifecycle(cluster.id, action);
      }
      await fetchClusters(false);
      setConfirmation(undefined);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setActionPending(false);
    }
  };

  const handleRefresh = async () => {
    if (refreshing) return;
    setRefreshing(true);
    try {
      await fetchClusters(false);
    } finally {
      setRefreshing(false);
    }
  };

  const toggleNodeCordon = async (node: CRDNode) => {
    const willCordon = !node.spec.unschedulable;
    if (
      !confirm(
        willCordon
          ? zh
            ? `确认对节点"${node.metadata.name}"执行暂停调度吗？执行后将停止把新的 RLark 任务调度到该节点，节点上的其他工作负载不受影响，已运行任务也不会被终止。`
            : `Pause scheduling on node "${node.metadata.name}"? New RLark tasks will stop being scheduled to it. Other workloads on the node are unaffected and running tasks remain active.`
          : zh
            ? `确认对节点"${node.metadata.name}"执行恢复调度吗？仅恢复向该节点调度新任务；被排空停止的任务需先取消驱逐才会恢复。`
            : `Resume scheduling on node "${node.metadata.name}"? This only allows new tasks to be scheduled to it; tasks stopped by a drain resume only after you cancel the drain.`,
      )
    )
      return;
    setCordoningNode(node.metadata.name);
    setError("");
    const namespace = node.metadata.namespace ?? "";
    try {
      if (willCordon) {
        // 手动暂停调度: 仅 cordon (unschedulable=true)
        await nodesApi.patch(
          node.metadata.name,
          { spec: { unschedulable: true } },
          namespace,
        );
      } else {
        // 恢复调度: uncordon 会置 unschedulable=false 并取消 Drain 操作；但被排空
        // 停止的任务保持 suspended（node-drain-stopped 标记保留），需运维显式
        // 启动其 Job 才会恢复运行。
        await nodesApi.uncordon(node.metadata.name, namespace);
      }
      await fetchClusters(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setCordoningNode(null);
    }
  };

  const drainNode = async (node: CRDNode) => {
    if (
      !confirm(
        zh
          ? `确认对节点"${node.metadata.name}"执行排空吗？将停止该节点接收新任务，并尝试将现有任务重调度到其他节点。`
          : `Drain node "${node.metadata.name}"? It will stop accepting new tasks and attempt to reschedule running tasks to other nodes.`,
      )
    )
      return;
    setDrainingNode(node.metadata.name);
    setError("");
    try {
      await nodesApi.drain(node.metadata.name, node.metadata.namespace ?? "");
      await fetchClusters(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setDrainingNode(null);
    }
  };

  const undrainNode = async (node: CRDNode) => {
    if (
      !confirm(
        zh
          ? `确认取消节点"${node.metadata.name}"的排空请求吗？`
          : `Cancel the drain request for node "${node.metadata.name}"?`,
      )
    )
      return;
    setDrainingNode(node.metadata.name);
    setError("");
    try {
      await nodesApi.undrain(node.metadata.name, node.metadata.namespace ?? "");
      await fetchClusters(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setDrainingNode(null);
    }
  };

  const unmanageNode = async (node: CRDNode) => {
    // 取消纳管前置检查:节点必须已 cordon (unschedulable=true)。
    // 例外:Agent 离线/未连接的节点无法接收新任务、也无处执行暂停调度，
    // 因此允许直接取消纳管（与后端 ensureNodeCordoned 的判定保持一致）。
    if (
      !node.spec.unschedulable &&
      nodeConnectionState(node).kind === "connected"
    ) {
      alert(
        zh
          ? `节点 "${node.metadata.name}" 尚未暂停调度。请先对该节点执行暂停调度，再取消纳管，避免取消纳管过程中有新任务被调度上来。`
          : `Scheduling has not been paused on node "${node.metadata.name}" yet. Pause scheduling on it first to stop new task placement before unmanaging, so no new workloads are placed on it during the transition.`,
      );
      return;
    }
    if (
      !confirm(
        zh
          ? `确认取消纳管节点 "${node.metadata.name}"？数据面 K8s Node 的 rlark.io/enabled 标签将被移除，agent 不再为其上报状态。此操作不会删除 K8s 节点或其中的运行任务。\n\n注意：取消纳管后节点可能需要 30~60 秒才会从列表中消失，具体取决于 agent 心跳间隔和控制器同步延迟。`
          : `Unmanage node "${node.metadata.name}"? The rlark.io/enabled label on the data-plane K8s Node will be removed and the agent will stop reporting status. This will not delete the K8s node or running tasks.\n\nNote: the node may take 30~60 seconds to disappear from the list after unmanagement, depending on the agent heartbeat interval and controller sync delay.`,
      )
    )
      return;
    setRemovingNode(node.metadata.name);
    setError("");
    try {
      const agentID = (node.metadata.namespace ?? "").replace(/^rlark-/, "");
      await dataPlaneNodesApi.unmanage(agentID, node.metadata.name);
      await fetchClusters(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setRemovingNode(null);
    }
  };

  const filteredClusters = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return clusters.filter((cluster) => {
      const searchable =
        `${cluster.name} ${cluster.id} ${cluster.type} ${cluster.region} ${cluster.location}`.toLowerCase();
      const phaseHit =
        phaseFilterValues.length === 0 ||
        phaseFilterValues.includes(cluster.phase);
      return (!normalized || searchable.includes(normalized)) && phaseHit;
    });
  }, [clusters, phaseFilterValues, query]);

  const totalPages = Math.max(1, Math.ceil(filteredClusters.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pagedClusters = filteredClusters.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );
  useEffect(() => setPage(1), [pageSize, phaseFilterValues, query]);

  const selectedCluster = selectedClusterID
    ? clusters.find((cluster) => cluster.id === selectedClusterID)
    : undefined;

  if (selectedClusterID && selectedCluster) {
    const visibleDetailNodes = detailNodes;
    const onlineRate = selectedCluster.totalNodes
      ? Math.round(
          (selectedCluster.onlineNodes / selectedCluster.totalNodes) * 100,
        )
      : 0;
    return (
      <div className="page-content resource-page cluster-management-page cluster-detail-page">
        <div className="section-heading compact">
          <div>
            <button
              className="plain-button back-button"
              onClick={() => onSelectCluster()}
            >
              ← {zh ? "返回集群列表" : "Back to clusters"}
            </button>
            <span className="eyebrow">
              {zh ? "集群详情" : "Cluster detail"}
            </span>
          </div>
        </div>

        <section className="panel cluster-detail-hero">
          <div className="cluster-detail-identity">
            <span>
              <CloudCog size={24} />
            </span>
            <div>
              <small>{selectedCluster.type || (zh ? "集群" : "Cluster")}</small>
              <h2>{selectedCluster.name}</h2>
              <p>
                <MapPin size={13} />
                {[selectedCluster.region, selectedCluster.location]
                  .filter(Boolean)
                  .join(" · ") ||
                  (zh ? "未配置区域信息" : "No region metadata")}
              </p>
            </div>
          </div>
          <ClusterStatus phase={selectedCluster.phase} zh={zh} />
        </section>

        <section className="cluster-detail-metrics">
          <MetricCard
            icon={Server}
            tone="blue"
            label={zh ? "节点总数" : "Nodes"}
            value={`${selectedCluster.totalNodes}`}
            note={`${selectedCluster.readyNodes ?? selectedCluster.onlineNodes} ${zh ? "可调度就绪" : "schedulable ready"}`}
          />
          <MetricCard
            icon={Activity}
            tone="mint"
            label={zh ? "可用率" : "Availability"}
            value={`${onlineRate}%`}
            note={`${selectedCluster.readyNodes ?? selectedCluster.onlineNodes}/${selectedCluster.totalNodes}`}
          />
          <MetricCard
            icon={Network}
            tone="orange"
            label={zh ? "异常节点" : "Impaired nodes"}
            value={`${(selectedCluster.degradedNodes ?? 0) + (selectedCluster.disconnectedNodes ?? 0)}`}
            note={`${selectedCluster.degradedNodes ?? 0} ${zh ? "降级" : "degraded"} · ${selectedCluster.disconnectedNodes ?? 0} ${zh ? "失联" : "disconnected"} · ${selectedCluster.unschedulableNodes ?? 0} ${zh ? "不可调度" : "unschedulable"}`}
          />
          <MetricCard
            icon={Bot}
            tone="purple"
            label={zh ? "运行任务" : "Running jobs"}
            value={`${selectedCluster.runningJobs}`}
            note={zh ? "任务状态统计" : "From task status"}
          />
        </section>

        <section className="cluster-detail-nodes">
          <div className="section-heading compact">
            <div>
              <span className="eyebrow">
                {zh ? "集群节点" : "Cluster nodes"}
              </span>
              <h2>{zh ? "节点资源" : "Node resources"}</h2>
            </div>
          </div>
          <NodeResourceBrowser
            nodes={visibleDetailNodes}
            copy={c}
            onRefresh={handleRefresh}
            refreshing={refreshing}
            onSelectNode={onSelectNode}
            nodeWorkloads={nodeWorkloads}
            onToggleScheduling={adminMode ? toggleNodeCordon : undefined}
            updatingNode={
              adminMode ? (cordoningNode ?? drainingNode ?? removingNode) : null
            }
            onUnmanageNode={adminMode ? unmanageNode : undefined}
            onDrainNode={adminMode ? drainNode : undefined}
            onUndrainNode={adminMode ? undrainNode : undefined}
            compact
          />
        </section>

        {adminMode && <DataPlaneNodesPanel cluster={selectedCluster} zh={zh} />}
      </div>
    );
  }

  return (
    <div className="page-content resource-page cluster-management-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">
            {zh ? "资源纳管" : "Resource management"}
          </span>
          <h2>{zh ? "集群管理" : "Cluster Management"}</h2>
          <p>
            {zh
              ? "查看集群运行状态和节点规模，点击集群进入资源详情。"
              : "Review cluster health and capacity, then open a cluster for details."}
          </p>
        </div>
      </div>
      <PageToolbar
        placeholder={
          zh
            ? "搜索集群名称、类型或区域..."
            : "Search cluster, type or region..."
        }
        value={query}
        onChange={setQuery}
        count={filteredClusters.length}
        copy={c}
        onRefresh={handleRefresh}
        refreshing={refreshing}
      />
      {error && (
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}
      <section
        className={`panel cluster-management-table-panel refreshable-region${refreshing ? " is-refreshing" : ""}`}
        aria-busy={refreshing}
      >
        <div className="cluster-management-table-head">
          <span>{zh ? "集群名称" : "Cluster"}</span>
          <span>{zh ? "类型" : "Type"}</span>
          <span>{zh ? "节点数" : "Nodes"}</span>
          <span>{zh ? "在线" : "Online"}</span>
          <span>{zh ? "离线" : "Offline"}</span>
          <span>{zh ? "在线率" : "Rate"}</span>
          <ColumnFilterButton
            label={zh ? "状态" : "Status"}
            selectedCount={phaseFilterValues.length}
            onClick={openFor("phase")}
          />
          <span>{adminMode ? (zh ? "操作" : "Actions") : ""}</span>
        </div>
        <div className="cluster-management-table-body">
          {loading ? (
            <div className="cluster-management-empty">
              {zh ? "正在加载集群..." : "Loading clusters..."}
            </div>
          ) : pagedClusters.length === 0 ? (
            <div className="cluster-management-empty">
              {zh ? "没有符合条件的集群" : "No matching clusters"}
            </div>
          ) : (
            pagedClusters.map((cluster) => {
              const drainDisabled =
                actionPending || cluster.phase !== "Cordoned";
              const noRunningJobs = cluster.runningJobs === 0;
              const delistOrDeleteDisabled =
                drainDisabled || cluster.runningJobs !== 0;
              const rate = cluster.totalNodes
                ? Math.round((cluster.onlineNodes / cluster.totalNodes) * 100)
                : 0;
              return (
                <div
                  key={cluster.id}
                  className="cluster-management-row"
                  role="button"
                  tabIndex={0}
                  onClick={() => {
                    setActionClusterID(undefined);
                    onSelectCluster(cluster.id);
                  }}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ") {
                      event.preventDefault();
                      setActionClusterID(undefined);
                      onSelectCluster(cluster.id);
                    }
                  }}
                >
                  <span className="cluster-management-name">
                    <i>
                      <CloudCog size={16} />
                    </i>
                    <span>
                      <strong>{cluster.name}</strong>
                    </span>
                  </span>
                  <span className="cluster-type-chip">
                    {clusterTypeLabel(cluster.type, zh)}
                    {cluster.agentNamespaceConflict &&
                      (zh ? " · 命名空间冲突" : " · Namespace conflict")}
                  </span>
                  <strong>{cluster.totalNodes}</strong>
                  <span>{cluster.onlineNodes}</span>
                  <span>{cluster.offlineNodes}</span>
                  <span className="cluster-list-rate">
                    <i>
                      <b style={{ width: `${rate}%` }} />
                    </i>
                    <small>{rate}%</small>
                  </span>
                  <ClusterStatus phase={cluster.phase} zh={zh} />
                  {adminMode ? (
                    <span
                      className="cluster-actions"
                      ref={
                        actionClusterID === cluster.id
                          ? clusterActionsRef
                          : undefined
                      }
                      onClick={(event) => event.stopPropagation()}
                      onKeyDown={(event) => event.stopPropagation()}
                    >
                      <button
                        type="button"
                        className="icon-button"
                        title={zh ? "集群操作" : "Cluster actions"}
                        aria-label={zh ? "集群操作" : "Cluster actions"}
                        onClick={() =>
                          setActionClusterID((current) =>
                            current === cluster.id ? undefined : cluster.id,
                          )
                        }
                      >
                        <MoreHorizontal size={16} />
                      </button>
                      {actionClusterID === cluster.id && (
                        <span className="cluster-action-menu">
                          {cluster.phase === "Delisted" ? (
                            <>
                              <button
                                type="button"
                                disabled={actionPending}
                                title={
                                  zh
                                    ? "上架：集群对用户恢复可见状态"
                                    : "Restore: make the cluster visible to users"
                                }
                                onClick={() =>
                                  handleClusterAction(cluster, "restore")
                                }
                              >
                                {zh ? "上架" : "Restore"}
                              </button>
                              <button
                                type="button"
                                disabled={actionPending}
                                className="danger"
                                title={
                                  zh
                                    ? "删除：永久清理集群及其数据面资源"
                                    : "Delete: permanently remove the cluster and its data-plane resources"
                                }
                                onClick={() =>
                                  handleClusterAction(cluster, "delete")
                                }
                              >
                                {zh ? "删除" : "Delete"}
                              </button>
                            </>
                          ) : (
                            <>
                              {cluster.phase === "Cordoned" ? (
                                <button
                                  type="button"
                                  disabled={actionPending}
                                  title={
                                    zh
                                      ? "恢复调度：恢复集群接收新任务调度"
                                      : "Resume Schedule: resume scheduling new tasks to the cluster"
                                  }
                                  onClick={() =>
                                    handleClusterAction(cluster, "uncordon")
                                  }
                                >
                                  {zh ? "恢复调度" : "Resume Schedule"}
                                </button>
                              ) : (
                                <button
                                  type="button"
                                  disabled={actionPending}
                                  title={
                                    zh
                                      ? "暂停调度：停止集群接收新任务调度"
                                      : "Pause Schedule: stop scheduling new tasks to the cluster"
                                  }
                                  onClick={() =>
                                    handleClusterAction(cluster, "cordon")
                                  }
                                >
                                  {zh ? "暂停调度" : "Pause Schedule"}
                                </button>
                              )}
                              {cluster.phase === "Draining" ? (
                                <button
                                  type="button"
                                  disabled={actionPending}
                                  title={
                                    zh
                                      ? "取消驱逐任务并恢复排空前的调度状态"
                                      : "Cancel draining and restore the pre-drain scheduling state"
                                  }
                                  onClick={() =>
                                    handleClusterAction(cluster, "undrain")
                                  }
                                >
                                  {zh ? "取消驱逐" : "Cancel drain"}
                                </button>
                              ) : (
                                <button
                                  type="button"
                                  disabled={drainDisabled || noRunningJobs}
                                  title={
                                    cluster.phase === "Cordoned" &&
                                    noRunningJobs
                                      ? zh
                                        ? "无运行中任务"
                                        : "No running tasks"
                                      : zh
                                        ? "驱逐任务：停止集群内所有运行中的 RLark 任务"
                                        : "Drain: stop all running RLark tasks in the cluster"
                                  }
                                  onClick={() =>
                                    handleClusterAction(cluster, "drain")
                                  }
                                >
                                  {zh ? "驱逐任务" : "Drain"}
                                </button>
                              )}
                              <button
                                type="button"
                                disabled={delistOrDeleteDisabled}
                                title={
                                  zh
                                    ? "下架：集群不再对用户可见"
                                    : "Delist: stop showing the cluster to users"
                                }
                                onClick={() =>
                                  handleClusterAction(cluster, "delist")
                                }
                              >
                                {zh ? "下架" : "Delist"}
                              </button>
                              <button
                                type="button"
                                disabled={delistOrDeleteDisabled}
                                className="danger"
                                title={
                                  zh
                                    ? "删除：永久清理集群及其数据面资源"
                                    : "Delete: permanently remove the cluster and its data-plane resources"
                                }
                                onClick={() =>
                                  handleClusterAction(cluster, "delete")
                                }
                              >
                                {zh ? "删除" : "Delete"}
                              </button>
                            </>
                          )}
                        </span>
                      )}
                    </span>
                  ) : (
                    <ChevronRight size={15} />
                  )}
                </div>
              );
            })
          )}
        </div>
        <RefreshOverlay
          visible={refreshing}
          label={zh ? "正在刷新集群列表" : "Refreshing cluster list"}
        />
      </section>
      {confirmation && (
        <ClusterActionConfirmDialog
          cluster={confirmation.cluster}
          action={confirmation.action}
          zh={zh}
          pending={actionPending}
          error={error}
          onClose={() => setConfirmation(undefined)}
          onConfirm={confirmClusterAction}
        />
      )}
      {openKey === "phase" && (
        <ColumnFilterPopover
          label={zh ? "状态" : "Status"}
          options={[
            "Online",
            "Offline",
            "Downgraded",
            "Delisted",
            "Cordoned",
            "Draining",
          ].map((value) => {
            const label = CLUSTER_STATUS_LABELS[value];
            return { value, label: zh ? label.zh : label.en };
          })}
          selected={phaseFilterValues}
          onChange={setPhaseFilterValues}
          anchorRect={anchorRect}
          onClose={close}
          zh={zh}
        />
      )}
      <Pagination
        page={currentPage}
        pageSize={pageSize}
        total={filteredClusters.length}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
        zh={zh}
      />
    </div>
  );
}

function ClusterActionConfirmDialog({
  cluster,
  action,
  zh,
  pending,
  error,
  onClose,
  onConfirm,
}: {
  cluster: ClusterSummary;
  action: ClusterAction;
  zh: boolean;
  pending: boolean;
  error: string;
  onClose: () => void;
  onConfirm: () => Promise<void>;
}) {
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !pending) onClose();
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [onClose, pending]);

  const confirmedAction = action === "delete" ? undefined : action;
  const content =
    action === "delete"
      ? {
          eyebrow: zh ? "危险操作" : "Danger zone",
          title: zh ? "确认删除集群？" : "Delete this cluster?",
          description: zh
            ? "删除将清理集群及其数据面资源，且无法恢复。"
            : "Delete will remove the cluster and its data-plane resources permanently.",
          confirm: zh ? "确认删除" : "Delete cluster",
          pending: zh ? "删除中…" : "Deleting…",
          danger: true,
        }
      : {
          eyebrow: zh ? "集群操作" : "Cluster action",
          title: zh ? "确认执行集群操作？" : "Confirm cluster action?",
          description: zh
            ? clusterActionDescriptions[confirmedAction!].zh
            : clusterActionDescriptions[confirmedAction!].en,
          confirm: zh ? "确认执行" : "Confirm",
          pending: zh ? "处理中…" : "Processing…",
          danger: false,
        };

  return (
    <div
      className="modal-backdrop job-lifecycle-backdrop"
      onMouseDown={(event) =>
        event.target === event.currentTarget && !pending && onClose()
      }
    >
      <section
        className="modal job-lifecycle-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="cluster-action-title"
      >
        <div className="job-lifecycle-head cluster-action-confirm-head">
          <div>
            <span className="eyebrow">{content.eyebrow}</span>
            <h2 id="cluster-action-title">{content.title}</h2>
          </div>
          <button
            className="icon-button"
            onClick={onClose}
            disabled={pending}
            aria-label={zh ? "关闭" : "Close"}
          >
            ×
          </button>
        </div>
        <div className="job-lifecycle-body">
          <p>{content.description}</p>
          <div className="delete-job-target">
            <span>{zh ? "目标集群" : "Target cluster"}</span>
            <strong>{cluster.name}</strong>
          </div>
          {error && (
            <div className="delete-job-error" role="alert">
              {error}
            </div>
          )}
        </div>
        <div className="job-lifecycle-actions">
          <button
            className="secondary-button"
            onClick={onClose}
            disabled={pending}
          >
            {zh ? "取消" : "Cancel"}
          </button>
          <button
            className={content.danger ? "danger-button" : "primary-button"}
            onClick={() => void onConfirm()}
            disabled={pending}
          >
            {pending ? content.pending : content.confirm}
          </button>
        </div>
      </section>
    </div>
  );
}

/**
 * NodeActionMenu — compact "⋯" dropdown for a data-plane node row.
 * Follows the same click-outside / Escape / scroll-to-close pattern used by
 * Jobs.tsx JobActionMenu so interaction stays consistent across the app.
 */
function NodeActionMenu({
  nodeName,
  managed,
  pending,
  zh,
  onManage,
  onUnmanage,
}: {
  nodeName: string;
  managed: boolean;
  pending: boolean;
  zh: boolean;
  onManage: () => void;
  onUnmanage: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [menuStyle, setMenuStyle] = useState<CSSProperties>({});
  const rootRef = useRef<HTMLDivElement>(null);
  const btnRef = useRef<HTMLButtonElement>(null);

  useLayoutEffect(() => {
    if (!open || !rootRef.current) return;
    const btn = btnRef.current;
    if (!btn) return;
    const rect = btn.getBoundingClientRect();
    const ddEl = rootRef.current.querySelector(
      ".action-dropdown",
    ) as HTMLElement | null;
    const ddHeight = ddEl?.offsetHeight ?? 120;
    const ddWidth = ddEl?.offsetWidth ?? 140;
    const spaceBelow = window.innerHeight - rect.bottom;
    const dropUp = spaceBelow < ddHeight + 8;
    const menuLeft = Math.max(
      8,
      Math.min(rect.right - ddWidth, window.innerWidth - ddWidth - 8),
    );
    setMenuStyle(
      dropUp
        ? {
            position: "fixed",
            left: menuLeft,
            bottom: window.innerHeight - rect.top + 4,
            right: "auto",
            top: "auto",
          }
        : {
            position: "fixed",
            left: menuLeft,
            top: rect.bottom + 4,
            right: "auto",
            bottom: "auto",
          },
    );
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handlePointer = (e: PointerEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    const handleClose = () => setOpen(false);
    document.addEventListener("pointerdown", handlePointer);
    document.addEventListener("keydown", handleKey);
    window.addEventListener("scroll", handleClose, true);
    window.addEventListener("resize", handleClose);
    return () => {
      document.removeEventListener("pointerdown", handlePointer);
      document.removeEventListener("keydown", handleKey);
      window.removeEventListener("scroll", handleClose, true);
      window.removeEventListener("resize", handleClose);
    };
  }, [open]);

  const itemIcon = managed ? <Unlink2 size={14} /> : <Link2 size={14} />;
  const itemLabel = managed
    ? zh
      ? "注销纳管"
      : "Unmanage"
    : zh
      ? "纳管节点"
      : "Manage node";
  const itemTitle = managed
    ? zh
      ? "移除 rlark.io/enabled 标签"
      : "Remove rlark.io/enabled label"
    : zh
      ? "添加 rlark.io/enabled=true 标签"
      : "Add rlark.io/enabled=true label";

  const handleToggle = () => setOpen((v) => !v);

  return (
    <div
      ref={rootRef}
      style={{ position: "relative" }}
      className="admin-data-plane-node-action-menu"
    >
      <span
        className="action-tooltip"
        data-tooltip={zh ? "更多操作" : "More actions"}
      >
        <button
          ref={btnRef}
          type="button"
          className="icon-button"
          onClick={handleToggle}
          aria-label={
            zh ? `节点 ${nodeName} 的操作菜单` : `Actions for node ${nodeName}`
          }
          aria-expanded={open}
          disabled={pending}
        >
          {pending ? (
            <span className="admin-data-plane-node-spinner" aria-hidden />
          ) : (
            <MoreVertical size={16} />
          )}
        </button>
      </span>
      {open && (
        <div className="action-dropdown" style={menuStyle}>
          <button
            type="button"
            className={`action-dropdown-item${managed ? " danger" : ""}`}
            title={itemTitle}
            onClick={() => {
              setOpen(false);
              if (managed) onUnmanage();
              else onManage();
            }}
          >
            {itemIcon}
            {itemLabel}
          </button>
        </div>
      )}
    </div>
  );
}

/**
 * DataPlaneNodesPanel — lists only unmanaged K8s nodes in the selected
 * data-plane cluster so administrators can quickly see which raw nodes
 * still need RLark enablement.
 */
function DataPlaneNodesPanel({
  cluster,
  zh,
}: {
  cluster: ClusterSummary;
  zh: boolean;
}) {
  const [data, setData] = useState<ClusterDataPlaneNodes | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [managingNode, setManagingNode] = useState<{
    agentId: string;
    nodeName: string;
  } | null>(null);

  const fetchNodes = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    try {
      const clusters = await dataPlaneNodesApi.listAll();
      setData(
        clusters.find((item) => item.namespace === cluster.agentNamespace) ??
          clusters.find((item) => item.cluster === cluster.name) ??
          null,
      );
    } catch {
      setData(null);
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchNodes, 30000, [cluster.id]);

  const agentId = (data?.namespace ?? cluster.agentNamespace ?? "").replace(
    /^rlark-/,
    "",
  );

  const manageNode = async (nodeName: string) => {
    if (
      !confirm(
        zh
          ? `确认纳管节点 "${nodeName}"？纳管后 agent 将为其创建 RLark Node 资源并开始上报状态。`
          : `Manage node "${nodeName}"? The agent will create an RLark Node resource and start reporting status.`,
      )
    )
      return;
    setManagingNode({ agentId, nodeName });
    setError("");
    try {
      await dataPlaneNodesApi.manage(agentId, nodeName);
      await fetchNodes(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setManagingNode(null);
    }
  };

  const allNodes = data?.nodes ?? [];
  const unmanagedNodes = allNodes.filter((node) => !node.managed);

  const sortedUnmanaged = useMemo(
    () =>
      unmanagedNodes
        .slice()
        .sort((a, b) => a.name.localeCompare(b.name, zh ? "zh-CN" : "en")),
    [unmanagedNodes, zh],
  );

  return (
    <section
      className="panel node-resource-table-panel admin-data-plane-nodes-panel"
      aria-busy={loading}
    >
      <div className="node-resource-table-summary">
        <div>
          <strong>{zh ? "未纳管节点" : "Unmanaged nodes"}</strong>
          <small>
            {zh
              ? "以下 K8s Node 未携带 rlark.io/enabled=true 标签，添加后 agent 会自动创建 RLark Node 资源"
              : "The K8s Nodes below lack the rlark.io/enabled=true label; once labeled, the agent will create RLark Node resources automatically"}
          </small>
        </div>
        <span>
          {loading
            ? zh
              ? "加载中..."
              : "Loading..."
            : !data
              ? zh
                ? "暂无数据"
                : "No data"
              : data.error
                ? zh
                  ? `获取失败: ${data.error}`
                  : `Failed: ${data.error}`
                : zh
                  ? `${sortedUnmanaged.length} 未纳管 · ${allNodes.length - sortedUnmanaged.length} 已纳管`
                  : `${sortedUnmanaged.length} unmanaged · ${allNodes.length - sortedUnmanaged.length} managed`}
        </span>
      </div>
      {error && (
        <div className="cert-error" style={{ margin: "0 16px 12px" }}>
          {error}
        </div>
      )}
      {!loading && data && !data.error && (
        <div className="node-resource-table admin-data-plane-unmanaged-table">
          <div className="node-resource-table-head">
            <span>{zh ? "节点名称" : "Node"}</span>
            <span>{zh ? "IP" : "IP"}</span>
            <span>{zh ? "状态" : "Status"}</span>
            <span>{zh ? "操作" : "Actions"}</span>
          </div>
          <div className="node-resource-table-body">
            {sortedUnmanaged.length === 0 ? (
              <div className="node-resource-empty">
                <Server size={22} />
                <strong>
                  {zh
                    ? "该集群所有节点均已纳管"
                    : "All nodes in this cluster are already managed"}
                </strong>
                <small>
                  {zh
                    ? "无需添加新的 rlark.io/enabled=true 标签"
                    : "No additional rlark.io/enabled=true labels needed"}
                </small>
              </div>
            ) : (
              sortedUnmanaged.map((node) => {
                const isManaging =
                  managingNode?.agentId === agentId &&
                  managingNode?.nodeName === node.name;
                const internalIP = node.addresses?.find(
                  (address) => address.type === "InternalIP",
                )?.address;
                return (
                  <div
                    key={node.name}
                    className="node-resource-row admin-data-plane-unmanaged-row"
                  >
                    <span className="admin-data-plane-node-name">
                      <span
                        className="admin-data-plane-node-dot unmanaged"
                        title={zh ? "未纳管" : "Unmanaged"}
                      />
                      <strong>{node.name}</strong>
                    </span>
                    <span className="admin-data-plane-node-ip">
                      {internalIP ?? "—"}
                    </span>
                    <span
                      className={`admin-data-plane-node-ready ${
                        node.ready ? "ready" : "notready"
                      }`}
                    >
                      {node.ready
                        ? zh
                          ? "就绪"
                          : "Ready"
                        : zh
                          ? "未就绪"
                          : "NotReady"}
                    </span>
                    <span className="admin-data-plane-node-actions">
                      <NodeActionMenu
                        nodeName={node.name}
                        managed={false}
                        pending={isManaging}
                        zh={zh}
                        onManage={() => manageNode(node.name)}
                        onUnmanage={() => {}}
                      />
                    </span>
                  </div>
                );
              })
            )}
          </div>
        </div>
      )}
    </section>
  );
}
