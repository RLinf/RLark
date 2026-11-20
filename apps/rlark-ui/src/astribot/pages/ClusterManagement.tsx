import { useEffect, useMemo, useState } from "react";
import {
  Activity,
  Bot,
  ChevronRight,
  CloudCog,
  MoreHorizontal,
  MapPin,
  Network,
  Pencil,
  Server,
} from "lucide-react";
import type { Copy } from "../../i18n";
import type { ClusterSummary as RLarkClusterSummary, CRDNode, NodeCategory } from "../../types";
import { useAutoRefresh } from "../../hooks";
import {
  getNodeCategories,
  getNodeDeviceModel,
  getNodeGPUModel,
  hasNodeCategory,
  isBusinessWorkerNode,
} from "../../utils/nodes";
import { controlPlanesOf, type ControlPlaneNode } from "../lib/controlPlanes";
import { kubernetesVersionsOf, kubernetesVersionText, type KubernetesVersions } from "../lib/kubernetesVersion";
import {
  ColumnFilterButton,
  MetricCard,
  PageToolbar,
  Pagination,
  RefreshOverlay,
  useColumnFilter,
} from "../../components/shared";
import { ColumnFilterPopover } from "../../components/ColumnFilterPopover";
import { NodeResourceBrowser } from "../components/NodeResourceBrowser";
import { NodeName } from "../components/NodeName";
import { RobotControls } from "../components/RobotControls";
import { useRobotPower } from "../components/useRobotPower";
import { controlledRobot } from "../lib/siteNodeDisplay";
import { clustersApi, nodesApi } from "../../backend";
import { clusterTypeApi, clusterTypes, displayNameApi, type ClusterType, type WithClusterType } from "../api";
import { displayNameError, displayNameMessage, displayNameRefusal } from "../lib/displayName";
import "./ClusterManagement.css";

// A cluster as this page lists it: the Gateway's summary and what its nodes tell.
type ClusterSummary = RLarkClusterSummary & WithClusterType & {
  // The name it was given; name is this when set, else the ID.
  displayName?: string;
  // The cluster's control-plane nodes; empty for a managed control plane.
  controlPlanes?: ControlPlaneNode[];
  // The Kubernetes versions the cluster's nodes carry, as station-controller last read them.
  kubernetes?: KubernetesVersions;
};

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

// Our cluster type (GPU / robot), set when the cluster is made or by an administrator; "" when never set.
function ourClusterTypeLabel(clusterType: ClusterType | undefined, zh: boolean): string {
  const entry = clusterTypes.find((type) => type.value === clusterType);
  return entry ? (zh ? entry.zh : entry.en) : zh ? "未分类" : "No type";
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
  const onlineNodes = workerNodes.filter(
    (node) => node.status?.phase === "Online",
  ).length;
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
    onlineNodes,
    offlineNodes: totalNodes - onlineNodes,
    cloudNodes: categories.cloud,
    embodiedNodes: categories.edge,
    robots: categories.robot,
    gpuModels: models("cloud"),
    robotModels: models("robot"),
    runningJobs: clusterNodes.filter((node) =>
      Boolean(node.metadata.labels?.["rlark.io/embodied-task-name"]),
    ).length,
    controlPlanes: cluster.agentNamespaceConflict
      ? undefined
      : controlPlanesOf(nodes, cluster.agentNamespace),
    kubernetes: kubernetesVersionsOf(clusterNodes),
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
  const onlineNodes =
    nodeAggregate?.onlineNodes ??
    (validCount(cluster.onlineNodes)
      ? cluster.onlineNodes
      : cluster.phase === "Online"
        ? totalNodes
        : 0);
  const offlineNodes =
    nodeAggregate?.offlineNodes ??
    (validCount(cluster.offlineNodes)
      ? cluster.offlineNodes
      : Math.max(0, totalNodes - onlineNodes));

  return {
    ...cluster,
    type: nodeAggregate?.type ?? cluster.type,
    phase: cluster.phase,
    totalNodes,
    onlineNodes,
    offlineNodes,
    cloudNodes: nodeAggregate?.cloudNodes ?? cluster.cloudNodes ?? 0,
    embodiedNodes: nodeAggregate?.embodiedNodes ?? cluster.embodiedNodes ?? 0,
    robots: nodeAggregate?.robots ?? cluster.robots ?? 0,
    gpuModels: cluster.gpuModels ?? nodeAggregate?.gpuModels ?? [],
    robotModels: cluster.robotModels ?? nodeAggregate?.robotModels ?? [],
    runningJobs: nodeAggregate?.runningJobs ?? cluster.runningJobs ?? 0,
    controlPlanes: nodeAggregate?.controlPlanes,
    kubernetes: nodeAggregate?.kubernetes,
    region: cluster.region ?? "",
    location: cluster.location ?? "",
    description: cluster.description ?? "",
  };
}

// Where a cluster's control plane runs: its node and address, or "managed"
// when no node of it is one (a Kingsoft KCE cluster's).
function ClusterControlPlane({ cluster, zh }: { cluster: ClusterSummary; zh: boolean }) {
  if (!cluster.controlPlanes) return <span className="muted">—</span>;
  if (cluster.controlPlanes.length === 0)
    return <span className="cluster-control-plane muted">{zh ? "托管" : "Managed"}</span>;
  return (
    <span className="cluster-control-plane">
      {cluster.controlPlanes.map((node) => (
        <span key={node.name}>
          <strong>
            <NodeName node={{ metadata: { name: node.name }, spec: { displayName: node.displayName } }} />
          </strong>
          {node.address && <small>{node.address}</small>}
        </span>
      ))}
    </span>
  );
}

function ClusterStatus({ phase, zh }: { phase: string; zh: boolean }) {
  const labels: Record<string, { zh: string; en: string }> = {
    Online: { zh: "在线", en: "Online" },
    Offline: { zh: "离线", en: "Offline" },
    Downgraded: { zh: "服务降级", en: "Downgraded" },
    Delisted: { zh: "已下架", en: "Delisted" },
    Cordoned: { zh: "停用调度", en: "Cordoned" },
    Draining: { zh: "排空中", en: "Draining" },
  };
  const label = labels[phase] ?? { zh: phase, en: phase };
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
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [query, setQuery] = useState("");
  // 状态列多选筛选；空数组 = 全部
  const [phaseFilterValues, setPhaseFilterValues] = useState<string[]>([]);
  // 类型列多选筛选（gpu / robot / none）；空数组 = 全部
  const [typeFilterValues, setTypeFilterValues] = useState<string[]>([]);
  // The robots chosen in a robot cluster's node list, by namespace/name.
  const [chosenRobotKeys, setChosenRobotKeys] = useState<Set<string>>(new Set());
  const [typePending, setTypePending] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [actionClusterID, setActionClusterID] = useState<string>();
  const [confirmation, setConfirmation] = useState<{
    cluster: ClusterSummary;
    action: ClusterAction;
  }>();
  const [actionPending, setActionPending] = useState(false);
  const [error, setError] = useState("");
  // The rename dialog's draft; undefined while it is closed.
  const [renameDraft, setRenameDraft] = useState<string>();
  const [renameError, setRenameError] = useState("");
  const [renamePending, setRenamePending] = useState(false);
  const { openKey, anchorRect, openFor, close } = useColumnFilter();
  const robotPower = useRobotPower(zh, () => fetchClusters(false));

  const fetchClusters = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    let resolvedNodes: CRDNode[] = [];
    try {
      resolvedNodes = await nodesApi.list({
        labelSelector: undefined,
      });
    } catch {
      resolvedNodes = [];
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

  const filteredClusters = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return clusters.filter((cluster) => {
      const searchable =
        `${cluster.name} ${cluster.id} ${cluster.type} ${ourClusterTypeLabel(cluster.clusterType, zh)} ${cluster.region} ${cluster.location}`.toLowerCase();
      const phaseHit =
        phaseFilterValues.length === 0 ||
        phaseFilterValues.includes(cluster.phase);
      const typeHit =
        typeFilterValues.length === 0 ||
        typeFilterValues.includes(cluster.clusterType ?? "none");
      return (!normalized || searchable.includes(normalized)) && phaseHit && typeHit;
    });
  }, [clusters, phaseFilterValues, typeFilterValues, query, zh]);

  const totalPages = Math.max(1, Math.ceil(filteredClusters.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pagedClusters = filteredClusters.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );
  useEffect(() => setPage(1), [pageSize, phaseFilterValues, typeFilterValues, query]);
  useEffect(() => setChosenRobotKeys(new Set()), [selectedClusterID]);

  const selectedCluster = selectedClusterID
    ? clusters.find((cluster) => cluster.id === selectedClusterID)
    : undefined;

  if (selectedClusterID && selectedCluster) {
    const visibleDetailNodes = detailNodes;
    const saveClusterName = async (draft: string) => {
      const invalid = displayNameError(
        draft,
        selectedCluster.id,
        clusters.map((cluster) => ({
          id: cluster.id,
          name: cluster.id,
          displayName: cluster.displayName,
        })),
      );
      if (invalid) {
        setRenameError(displayNameMessage(invalid, "cluster", zh));
        return;
      }
      setRenamePending(true);
      setRenameError("");
      try {
        await displayNameApi.cluster(selectedCluster.id, draft.trim());
        await fetchClusters(false);
        setRenameDraft(undefined);
      } catch (e) {
        const refusal = displayNameRefusal(e);
        setRenameError(refusal ? displayNameMessage(refusal, "cluster", zh) : e instanceof Error ? e.message : String(e));
      } finally {
        setRenamePending(false);
      }
    };
    const saveClusterType = async (clusterType: ClusterType) => {
      setTypePending(true);
      setError("");
      try {
        await clusterTypeApi.set(selectedCluster.id, clusterType);
        await fetchClusters(false);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        setTypePending(false);
      }
    };
    // A robot cluster's robots, and those chosen in its node list.
    const robots =
      selectedCluster.clusterType === "robot"
        ? visibleDetailNodes
            .filter((node) => hasNodeCategory(node, "robot"))
            .map(controlledRobot)
        : [];
    const chosenRobots = robots.filter((robot) => chosenRobotKeys.has(robot.key));
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
              <small>
                {ourClusterTypeLabel(selectedCluster.clusterType, zh)}
                {selectedCluster.type && ` · ${clusterTypeLabel(selectedCluster.type, zh)}`}
              </small>
              <h2>{selectedCluster.name}</h2>
              {selectedCluster.name !== selectedCluster.id && (
                <code className="real-name">{selectedCluster.id}</code>
              )}
              <p>
                <MapPin size={13} />
                {[selectedCluster.region, selectedCluster.location]
                  .filter(Boolean)
                  .join(" · ") ||
                  (zh ? "未配置区域信息" : "No region metadata")}
              </p>
            </div>
          </div>
          <div className="cluster-detail-hero-actions">
            {adminMode && (
              <select
                aria-label={zh ? "集群类型" : "Cluster type"}
                value={selectedCluster.clusterType ?? ""}
                disabled={typePending}
                onChange={(e) => void saveClusterType(e.target.value as ClusterType)}
              >
                <option value="" disabled>
                  {zh ? "设置集群类型" : "Set the cluster type"}
                </option>
                {clusterTypes.map((type) => (
                  <option key={type.value} value={type.value}>
                    {zh ? type.zh : type.en}
                  </option>
                ))}
              </select>
            )}
            {adminMode && (
              <button
                type="button"
                className="secondary-button"
                onClick={() => {
                  setRenameDraft(selectedCluster.displayName ?? "");
                  setRenameError("");
                }}
              >
                <Pencil size={14} />
                {zh ? "重命名" : "Rename"}
              </button>
            )}
            <ClusterStatus phase={selectedCluster.phase} zh={zh} />
          </div>
        </section>
        {error && (
          <div className="cert-error" role="alert" style={{ marginBottom: 12 }}>
            {error}
          </div>
        )}
        {renameDraft !== undefined && (
          // A form: the backdrop does not dismiss it.
          <div className="modal-backdrop cluster-rename-backdrop">
            <div
              className="modal"
              style={{ maxWidth: 480 }}
              role="dialog"
              aria-modal="true"
              aria-labelledby="cluster-rename-title"
            >
              <div className="modal-head">
                <div>
                  <span className="eyebrow">
                    {zh ? "集群显示名" : "Cluster display name"}
                  </span>
                  <h2 id="cluster-rename-title">
                    {zh ? "重命名集群" : "Rename cluster"}
                  </h2>
                </div>
                <button
                  className="icon-button"
                  disabled={renamePending}
                  onClick={() => setRenameDraft(undefined)}
                  aria-label={zh ? "关闭" : "Close"}
                >
                  ×
                </button>
              </div>
              <div className="modal-body">
                <div className="form-section">
                  <div className="form-section-head">
                    <small>{zh ? "集群 ID（不变）" : "Cluster ID (never changes)"}</small>
                  </div>
                  <code className="real-name">{selectedCluster.id}</code>
                </div>
                <div className="form-section">
                  <div className="form-section-head">
                    <small>{zh ? "显示名" : "Display name"}</small>
                  </div>
                  <input
                    value={renameDraft}
                    maxLength={64}
                    autoFocus
                    disabled={renamePending}
                    placeholder={selectedCluster.id}
                    onChange={(e) => setRenameDraft(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") void saveClusterName(renameDraft);
                    }}
                  />
                  <small className="muted" style={{ display: "block", marginTop: 4 }}>
                    {zh
                      ? "留空则显示 ID。最多 64 个字符，支持中英文、数字以及-_."
                      : "Leave empty to show the ID. At most 64 Chinese/English letters, digits, -, _ and ."}
                  </small>
                </div>
                {renameError && (
                  <div className="cert-error" role="alert" style={{ marginBottom: 12 }}>
                    {renameError}
                  </div>
                )}
                <div className="step-actions">
                  <button
                    className="secondary-button"
                    disabled={renamePending}
                    onClick={() => setRenameDraft(undefined)}
                  >
                    {zh ? "取消" : "Cancel"}
                  </button>
                  <button
                    className="primary-button"
                    disabled={renamePending}
                    onClick={() => void saveClusterName(renameDraft)}
                  >
                    {renamePending
                      ? zh
                        ? "保存中…"
                        : "Saving…"
                      : zh
                        ? "保存"
                        : "Save"}
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        <section className="cluster-detail-metrics">
          <MetricCard
            icon={Server}
            tone="blue"
            label={zh ? "节点总数" : "Nodes"}
            value={`${selectedCluster.totalNodes}`}
            note={`${selectedCluster.onlineNodes} ${zh ? "在线" : "online"}`}
          />
          <MetricCard
            icon={Activity}
            tone="mint"
            label={zh ? "在线率" : "Online rate"}
            value={`${onlineRate}%`}
            note={`${selectedCluster.onlineNodes}/${selectedCluster.totalNodes}`}
          />
          <MetricCard
            icon={Network}
            tone="orange"
            label={zh ? "离线节点" : "Offline nodes"}
            value={`${selectedCluster.offlineNodes}`}
            note={
              selectedCluster.offlineNodes
                ? zh
                  ? "需要关注"
                  : "Needs attention"
                : zh
                  ? "运行正常"
                  : "Healthy"
            }
          />
          <MetricCard
            icon={Bot}
            tone="purple"
            label={zh ? "运行任务" : "Running jobs"}
            value={`${selectedCluster.runningJobs}`}
            note={zh ? "节点标签统计" : "From node labels"}
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
          {robots.length > 0 && (
            <RobotControls robots={robots} chosen={chosenRobots} power={robotPower} zh={zh} />
          )}
          <NodeResourceBrowser
            nodes={visibleDetailNodes}
            clusters={clusters}
            copy={c}
            onRefresh={handleRefresh}
            refreshing={refreshing}
            onSelectNode={onSelectNode}
            {...(robots.length > 0 && {
              selectedNodeKeys: chosenRobotKeys,
              onSelectionChange: setChosenRobotKeys,
              onSelectFiltered: (keys: string[]) => setChosenRobotKeys(new Set(keys)),
              onClearSelection: () => setChosenRobotKeys(new Set()),
            })}
          />
        </section>
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
          <ColumnFilterButton
            label={zh ? "类型" : "Type"}
            selectedCount={typeFilterValues.length}
            onClick={openFor("type")}
          />
          <span>{zh ? "控制面" : "Control plane"}</span>
          <span>Kubernetes</span>
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
                      {cluster.name !== cluster.id && (
                        <code className="real-name">{cluster.id}</code>
                      )}
                      {cluster.region && <small>{cluster.region}</small>}
                    </span>
                  </span>
                  <span
                    className="cluster-type-chip"
                    title={clusterTypeLabel(cluster.type, zh)}
                  >
                    {ourClusterTypeLabel(cluster.clusterType, zh)}
                    {cluster.agentNamespaceConflict &&
                      (zh ? " · 命名空间冲突" : " · Namespace conflict")}
                  </span>
                  <ClusterControlPlane cluster={cluster} zh={zh} />
                  <span className="cluster-kubernetes-version">
                    {kubernetesVersionText(cluster.kubernetes, zh)}
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
                          ) : (
                            <>
                              {cluster.phase === "Cordoned" ? (
                                <button
                                  type="button"
                                  disabled={actionPending}
                                  title={
                                    zh
                                      ? "故障恢复：恢复集群的新任务调度"
                                      : "Uncordon: resume scheduling new tasks to the cluster"
                                  }
                                  onClick={() =>
                                    handleClusterAction(cluster, "uncordon")
                                  }
                                >
                                  {zh ? "故障恢复" : "Uncordon"}
                                </button>
                              ) : (
                                <button
                                  type="button"
                                  disabled={actionPending}
                                  title={
                                    zh
                                      ? "故障维修：暂停新任务调度到集群"
                                      : "Cordon: stop scheduling new tasks to the cluster"
                                  }
                                  onClick={() =>
                                    handleClusterAction(cluster, "cordon")
                                  }
                                >
                                  {zh ? "故障维修" : "Cordon"}
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
                                  disabled={actionPending}
                                  title={
                                    zh
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
                                disabled={actionPending}
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
      {openKey === "type" && (
        <ColumnFilterPopover
          label={zh ? "类型" : "Type"}
          options={[
            ...clusterTypes.map((type) => ({
              value: type.value,
              label: zh ? type.zh : type.en,
            })),
            { value: "none", label: zh ? "未分类" : "No type" },
          ]}
          selected={typeFilterValues}
          onChange={setTypeFilterValues}
          anchorRect={anchorRect}
          onClose={close}
          zh={zh}
        />
      )}
      {openKey === "phase" && (
        <ColumnFilterPopover
          label={zh ? "状态" : "Status"}
          options={[
            { value: "Online", label: zh ? "在线" : "Online" },
            { value: "Downgraded", label: zh ? "服务降级" : "Downgraded" },
            { value: "Delisted", label: zh ? "已下架" : "Delisted" },
            { value: "Cordoned", label: zh ? "停用调度" : "Cordoned" },
            { value: "Draining", label: zh ? "排空中" : "Draining" },
            { value: "Offline", label: zh ? "离线" : "Offline" },
          ]}
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
