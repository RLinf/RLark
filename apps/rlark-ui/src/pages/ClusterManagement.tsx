import { useEffect, useMemo, useState } from "react";
import {
  Activity,
  Bot,
  ChevronRight,
  CloudCog,
  MapPin,
  Network,
  Server,
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

// 集群类型中文化：数据层保留英文枚举，渲染时映射
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

function clusterIDForNode(node: CRDNode) {
  return (
    node.metadata.labels?.["rlark.io/cluster-id"] ??
    node.metadata.namespace ??
    "default"
  );
}

function aggregateClusters(
  nodes: CRDNode[],
  clusterTypes = new Map<string, string>(),
  workerOnly = false,
): ClusterSummary[] {
  const groups = new Map<string, CRDNode[]>();
  nodes.forEach((node) => {
    const id = clusterIDForNode(node);
    groups.set(id, [...(groups.get(id) ?? []), node]);
  });

  return [...groups.entries()]
    .map(([id, clusterNodes]) => {
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
          : categories.cloud === 0 &&
              (categories.edge > 0 || categories.robot > 0)
            ? "Embodied"
            : "Hybrid";
      const type = clusterTypes.get(id) ?? inferredType;
      const workerNodes = workerOnly
        ? clusterNodes.filter(isBusinessWorkerNode)
        : clusterNodes;
      const onlineNodes = workerNodes.filter(
        (node) => node.status?.phase === "Online",
      ).length;
      const totalNodes = workerNodes.length;
      const phase =
        onlineNodes === 0
          ? "Offline"
          : onlineNodes === totalNodes
            ? "Online"
            : "Degraded";
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
        id,
        name: id,
        type,
        region: "",
        location: "",
        phase,
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
        description: "",
      } satisfies ClusterSummary;
    })
    .sort((a, b) => a.name.localeCompare(b.name));
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
    phase: nodeAggregate?.phase ?? cluster.phase,
    totalNodes,
    onlineNodes,
    offlineNodes,
    cloudNodes: nodeAggregate?.cloudNodes ?? cluster.cloudNodes ?? 0,
    embodiedNodes: nodeAggregate?.embodiedNodes ?? cluster.embodiedNodes ?? 0,
    robots: nodeAggregate?.robots ?? cluster.robots ?? 0,
    gpuModels: cluster.gpuModels ?? nodeAggregate?.gpuModels ?? [],
    robotModels: cluster.robotModels ?? nodeAggregate?.robotModels ?? [],
    runningJobs: nodeAggregate?.runningJobs ?? cluster.runningJobs ?? 0,
    region: cluster.region ?? "",
    location: cluster.location ?? "",
    description: cluster.description ?? "",
  };
}

function ClusterStatus({ phase, zh }: { phase: string; zh: boolean }) {
  const label =
    phase === "Online"
      ? zh
        ? "在线"
        : "Online"
      : phase === "Degraded"
        ? zh
          ? "部分离线"
          : "Degraded"
        : zh
          ? "离线"
          : "Offline";
  return (
    <span className={`cluster-health-badge ${phase.toLowerCase()}`}>
      <i /> {label}
    </span>
  );
}

export function ClusterManagementPage({
  copy: c,
  selectedClusterID,
  onSelectCluster,
  onSelectNode,
  workerOnly = false,
}: {
  copy: Copy;
  selectedClusterID?: string;
  onSelectCluster: (id?: string) => void;
  onSelectNode: (name: string) => void;
  workerOnly?: boolean;
}) {
  const zh = c.nav.overview === "总览";
  const [clusters, setClusters] = useState<ClusterSummary[]>([]);
  const [detailNodes, setDetailNodes] = useState<CRDNode[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [query, setQuery] = useState("");
  // 状态列多选筛选；空数组 = 全部
  const [phaseFilterValues, setPhaseFilterValues] = useState<string[]>([]);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const { openKey, anchorRect, openFor, close } = useColumnFilter();

  const fetchClusters = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    let resolvedNodes: CRDNode[] = [];
    try {
      resolvedNodes = await nodesApi.list({
        labelSelector: selectedClusterID
          ? `rlark.io/cluster-id=${selectedClusterID}`
          : undefined,
      });
    } catch {
      resolvedNodes = [];
    }

    let resolvedClusters: ClusterSummary[] = [];
    try {
      const rawClusters = await clustersApi.list<ClusterSummary>();
      const clusterTypes = new Map(
        rawClusters.map((cluster) => [
          cluster.id || cluster.name,
          cluster.type,
        ]),
      );
      const nodeAggregates = new Map(
        aggregateClusters(resolvedNodes, clusterTypes, workerOnly).map(
          (cluster) => [cluster.id, cluster],
        ),
      );
      resolvedClusters = rawClusters.map((cluster) =>
        normalizeClusterSummary(
          cluster,
          nodeAggregates.get(cluster.id || cluster.name),
        ),
      );
    } catch {
      resolvedClusters = [];
    }
    if (resolvedClusters.length === 0) {
      resolvedClusters = aggregateClusters(
        resolvedNodes,
        new Map(),
        workerOnly,
      );
    }

    if (selectedClusterID) {
      try {
        const detail = await clustersApi.get<
          ClusterSummary & { nodes?: CRDNode[] }
        >(selectedClusterID);
        if (!detail) throw new Error("cluster not found");
        const fullNodes = resolvedNodes.filter(
          (node) => clusterIDForNode(node) === selectedClusterID,
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
        setDetailNodes(fullNodes.length > 0 ? fullNodes : enrichedDetailNodes);
        if (!resolvedClusters.some((cluster) => cluster.id === detail.id)) {
          resolvedClusters = [detail, ...resolvedClusters];
        }
      } catch {
        setDetailNodes(
          resolvedNodes.filter(
            (node) => clusterIDForNode(node) === selectedClusterID,
          ),
        );
      }
    } else {
      setDetailNodes([]);
    }
    setClusters(resolvedClusters);
    setLoading(false);
  };

  useAutoRefresh(fetchClusters, 10000, [selectedClusterID]);

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
    const visibleDetailNodes = workerOnly
      ? detailNodes.filter(isBusinessWorkerNode)
      : detailNodes;
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
          <NodeResourceBrowser
            nodes={visibleDetailNodes}
            copy={c}
            onRefresh={handleRefresh}
            refreshing={refreshing}
            onSelectNode={onSelectNode}
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
          <span />
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
                <button
                  key={cluster.id}
                  type="button"
                  className="cluster-management-row"
                  onClick={() => onSelectCluster(cluster.id)}
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
                  <ChevronRight size={15} />
                </button>
              );
            })
          )}
        </div>
        <RefreshOverlay
          visible={refreshing}
          label={zh ? "正在刷新集群列表" : "Refreshing cluster list"}
        />
      </section>
      {openKey === "phase" && (
        <ColumnFilterPopover
          label={zh ? "状态" : "Status"}
          options={[
            { value: "Online", label: zh ? "在线" : "Online" },
            { value: "Degraded", label: zh ? "部分离线" : "Degraded" },
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
import { clustersApi, nodesApi } from "../backend";
