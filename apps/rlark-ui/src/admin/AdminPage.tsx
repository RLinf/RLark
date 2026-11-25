import { useEffect, useMemo, useState } from "react";
import {
  Activity,
  AlertTriangle,
  Ban,
  CloudCog,
  MapPin,
  Network,
  Pencil,
  Plus,
  RefreshCw,
  Save,
  Server,
  Settings,
  X,
} from "lucide-react";
import { type Phase } from "../data";
import { type Copy } from "../i18n";
import { type CRDNode, type NodeCategory } from "../types";
import {
  NODE_CATEGORY_LABEL,
  categoryLabels,
  getNodeAllLabels,
  getNodeCategories,
  getNodeDeviceModel,
  getNodeGPUModel,
  getNodeLocation,
} from "../utils/nodes";
import { buildNodeWorkloads } from "../utils/nodeWorkloads";
import {
  NODE_CATEGORY_KEYS,
  getRemovedLabelKeys,
} from "../utils/nodeBatchMetadata";
import { useAutoRefresh } from "../hooks";
import { MetricCard, RefreshOverlay, StatusBadge } from "../components/shared";
import { NodeResourceBrowser } from "../components/NodeResourceBrowser";
import { ClusterDetailReal, NodeDetailReal } from "../pages/Clusters";

// Administrator-managed Node metadata lives on the management Node CR as
// annotations. It is not derived from the data-plane Kubernetes Node, so these
// fields are edited and saved through the management API rather than the label
// relay.
const NODE_FREE_TEXT_ANNOTATION_KEYS = [
  "rlark.io/city",
  "rlark.io/gpu-model",
  "rlark.io/device-model",
] as const;

const NODE_MANAGED_ANNOTATION_KEYS = [
  ...NODE_FREE_TEXT_ANNOTATION_KEYS,
  "rlark.io/node-category-cloud",
  "rlark.io/node-category-edge",
  "rlark.io/node-category-robot",
] as const;

const NODE_MANAGED_KEYS = new Set<string>([
  ...NODE_MANAGED_ANNOTATION_KEYS,
  ...NODE_CATEGORY_KEYS,
]);

type BatchMetadataField = "location" | "categories" | "gpu" | "device";

// Node names are only unique within a namespace, and the admin list spans
// namespaces, so drafts and saves must be keyed by namespace/name.
const nodeKey = (node: CRDNode) =>
  `${node.metadata.namespace ?? ""}/${node.metadata.name}`;

export function ClustersOverviewAdminPage({ copy: c }: { copy: Copy }) {
  const zh = c.nav.overview === "总览";
  const [nodes, setNodes] = useState<CRDNode[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [selectedClusterNs, setSelectedClusterNs] = useState<string | null>(
    null,
  );

  const fetchNodes = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    try {
      setNodes(await nodesApi.list());
    } catch (e) {
      setNodes([]);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchNodes, 10000);

  const handleRefresh = async () => {
    if (refreshing) return;
    setRefreshing(true);
    try {
      await fetchNodes(false);
    } finally {
      setRefreshing(false);
    }
  };

  const clustersList = useMemo(() => {
    const map = new Map<string, CRDNode[]>();
    for (const n of nodes) {
      const ns = n.metadata.namespace ?? "default";
      if (!map.has(ns)) map.set(ns, []);
      map.get(ns)!.push(n);
    }
    return Array.from(map.entries()).sort((a, b) => a[0].localeCompare(b[0]));
  }, [nodes]);

  const selectedCluster =
    clustersList.find(([ns]) => ns === selectedClusterNs) ?? clustersList[0];
  const selectedClusterNodes = selectedCluster?.[1] ?? [];

  const onlineClusters = clustersList.filter(([, nsNodes]) =>
    nsNodes.some((n) => n.status?.phase === "Online"),
  ).length;
  const totalNodes = nodes.length;

  const categoryCounts = useMemo(() => {
    const counts: Record<NodeCategory, number> = {
      cloud: 0,
      edge: 0,
      robot: 0,
      unknown: 0,
    };
    for (const n of nodes) {
      getNodeCategories(n).forEach((category) => counts[category]++);
    }
    return counts;
  }, [nodes]);

  if (loading) {
    return (
      <div className="page-content resource-page cluster-page">
        <div className="section-heading">
          <div>
            <span className="eyebrow">
              <Network size={13} />
              {zh ? "资源纳管" : "Managed resources"}
            </span>
            <h2>{zh ? "集群概览" : "Clusters Overview"}</h2>
            <p>
              {zh
                ? "按命名空间（集群）分组查看所有已纳管节点。"
                : "View all managed nodes grouped by namespace (cluster)."}
            </p>
          </div>
        </div>
        <p className="muted">{zh ? "加载中..." : "Loading..."}</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="page-content resource-page cluster-page">
        <div className="section-heading">
          <div>
            <span className="eyebrow">
              <Network size={13} />
              {zh ? "资源纳管" : "Managed resources"}
            </span>
            <h2>{zh ? "集群概览" : "Clusters Overview"}</h2>
            <p>
              {zh
                ? "按命名空间（集群）分组查看所有已纳管节点。"
                : "View all managed nodes grouped by namespace (cluster)."}
            </p>
          </div>
          <button
            className="secondary-button"
            onClick={handleRefresh}
            disabled={refreshing}
            aria-busy={refreshing}
          >
            <RefreshCw
              size={16}
              className={refreshing ? "job-action-loading" : ""}
            />
            {refreshing
              ? zh
                ? "刷新中..."
                : "Refreshing..."
              : c.common.refresh}
          </button>
        </div>
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      </div>
    );
  }

  return (
    <div
      className={`page-content resource-page cluster-page refreshable-region page-refresh-region${refreshing ? " is-refreshing" : ""}`}
      aria-busy={refreshing}
    >
      <div className="section-heading">
        <div>
          <span className="eyebrow">
            <Network size={13} />
            {zh ? "资源纳管" : "Managed resources"}
          </span>
          <h2>{zh ? "集群概览" : "Clusters Overview"}</h2>
          <p>
            {zh
              ? "按命名空间（集群）分组查看所有已纳管节点。"
              : "View all managed nodes grouped by namespace (cluster)."}
          </p>
        </div>
        <button
          className="secondary-button"
          onClick={handleRefresh}
          disabled={refreshing}
          aria-busy={refreshing}
        >
          <RefreshCw
            size={16}
            className={refreshing ? "job-action-loading" : ""}
          />
          {refreshing ? (zh ? "刷新中..." : "Refreshing...") : c.common.refresh}
        </button>
      </div>
      <section className="cluster-overview-grid">
        <MetricCard
          icon={Network}
          tone="blue"
          label={zh ? "集群总数" : "Clusters"}
          value={`${clustersList.length}`}
          note={`${onlineClusters} ${c.status.Online}`}
        />
        <MetricCard
          icon={Server}
          tone="mint"
          label={zh ? "节点总数" : "Nodes"}
          value={`${totalNodes}`}
          note={`${nodes.filter((n) => n.status?.phase === "Online").length} ${c.status.Online}`}
        />
        <MetricCard
          icon={CloudCog}
          tone="violet"
          label={zh ? "节点分类" : "Categories"}
          value={`${categoryCounts.cloud} / ${categoryCounts.edge} / ${categoryCounts.robot}`}
          note={zh ? "云 / 端 / 真机" : "Cloud / Edge / Robot"}
        />
        <MetricCard
          icon={Activity}
          tone="orange"
          label={zh ? "未知类型" : "Unknown"}
          value={`${categoryCounts.unknown}`}
          note={zh ? "未设置分类标签" : "No category label"}
        />
      </section>
      <section className="cluster-topology-grid">
        <div className="panel cluster-map-card">
          <div className="panel-title">
            <div>
              <span>{zh ? "分布" : "Distribution"}</span>
              <h3>{zh ? "集群分布" : "Cluster Distribution"}</h3>
            </div>
          </div>
          <div className="cluster-map">
            <div className="map-grid" />
            {clustersList.map(([ns, nsNodes], index) => (
              <button
                key={ns}
                className={`map-pin pin-${index} ${selectedCluster?.[0] === ns ? "active" : ""}`}
                onClick={() => setSelectedClusterNs(ns)}
              >
                <span>
                  <CloudCog size={15} />
                </span>
                <strong>{ns}</strong>
                <small>
                  {nsNodes.length} {zh ? "节点" : "nodes"}
                </small>
              </button>
            ))}
          </div>
        </div>
        <ClusterDetailReal
          namespace={selectedCluster?.[0] ?? ""}
          nodes={selectedClusterNodes}
          copy={c}
        />
      </section>
      <section className="panel cluster-list-panel">
        <div className="panel-title">
          <div>
            <span>{zh ? "集群列表" : "Cluster List"}</span>
            <h3>{zh ? "集群整体情况" : "Cluster Overview"}</h3>
          </div>
        </div>
        <div className="cluster-card-grid">
          {clustersList.map(([ns, nsNodes]) => {
            const onlineCount = nsNodes.filter(
              (n) => n.status?.phase === "Online",
            ).length;
            const phase: Phase = onlineCount > 0 ? "Online" : "Offline";
            return (
              <button
                key={ns}
                className={
                  selectedCluster?.[0] === ns
                    ? "cluster-card selected"
                    : "cluster-card"
                }
                onClick={() => setSelectedClusterNs(ns)}
              >
                <div className="cluster-card-head">
                  <span className="cloud">
                    <CloudCog size={18} />
                  </span>
                  <StatusBadge phase={phase} copy={c} />
                </div>
                <strong>{ns}</strong>
                <small>
                  {nsNodes.length} {zh ? "节点" : "nodes"}
                </small>
                <div className="cluster-loads">
                  <i>
                    <b
                      style={{
                        width: `${nsNodes.length > 0 ? (onlineCount / nsNodes.length) * 100 : 0}%`,
                      }}
                    />
                  </i>
                </div>
                <div className="cluster-card-foot">
                  <span>
                    {onlineCount} / {nsNodes.length} {zh ? "在线" : "online"}
                  </span>
                </div>
              </button>
            );
          })}
        </div>
      </section>
      <RefreshOverlay
        visible={refreshing}
        label={zh ? "正在刷新集群概览" : "Refreshing cluster overview"}
      />
    </div>
  );
}

export function NodeCategoryColumn({
  cat,
  catNodes,
  selectedNode,
  onSelectNode,
  ...ctx
}: {
  cat: NodeCategory;
  catNodes: CRDNode[];
  selectedNode: string | null;
  onSelectNode: (name: string) => void;
  zh: boolean;
  c: Copy;
  editingNode: string | null;
  labelDraft: Record<string, string>;
  newLabelKey: string;
  newLabelValue: string;
  saving: boolean;
  cordoning: boolean;
  onStartEdit: (node: CRDNode) => void;
  onToggleCordon: (node: CRDNode) => void;
  onCancelEdit: () => void;
  onSaveLabels: (nodeName: string, namespace: string) => void;
  onAddLabel: () => void;
  onRemoveLabel: (key: string) => void;
  onUpdateLabel: (key: string, value: string) => void;
  onNewLabelKey: (v: string) => void;
  onNewLabelValue: (v: string) => void;
}) {
  const { zh, c } = ctx;
  const info = categoryLabels[cat];
  const Icon = info.icon;
  const onlineCount = catNodes.filter(
    (n) => n.status?.phase === "Online",
  ).length;

  return (
    <div className={"panel node-category-column cat-" + cat}>
      <div className="node-category-header">
        <span className="node-category-icon">
          <Icon size={18} />
        </span>
        <div>
          <strong>{zh ? info.zh : info.en}</strong>
          <small>
            {catNodes.length} {zh ? "节点" : "nodes"} · {onlineCount}{" "}
            {zh ? "在线" : "online"}
          </small>
        </div>
      </div>
      {catNodes.length === 0 ? (
        <div className="node-category-empty">
          <small className="muted">{zh ? "暂无节点" : "No nodes"}</small>
        </div>
      ) : (
        <div className="node-category-list">
          {catNodes.map((node) => {
            const phase = (node.status?.phase ?? "Offline") as Phase;
            const isSelected = selectedNode === node.metadata.name;
            return (
              <div
                className={"node-row" + (isSelected ? " selected" : "")}
                key={node.metadata.name}
                onClick={() => onSelectNode(node.metadata.name)}
              >
                <span className={"node-status-ring " + phase.toLowerCase()} />
                <span className="node-row-name">{node.metadata.name}</span>
                <StatusBadge phase={phase} copy={c} />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function NodeDetailPanel({
  node,
  ...ctx
}: {
  node: CRDNode;
  zh: boolean;
  c: Copy;
  editingNode: string | null;
  labelDraft: Record<string, string>;
  annotationDraft: Record<string, string>;
  newLabelKey: string;
  newLabelValue: string;
  saving: boolean;
  cordoning: boolean;
  onStartEdit: (node: CRDNode) => void;
  onToggleCordon: (node: CRDNode) => void;
  onCancelEdit: () => void;
  onSaveLabels: (nodeName: string, namespace: string) => void;
  onAddLabel: () => void;
  onRemoveLabel: (key: string) => void;
  onUpdateLabel: (key: string, value: string) => void;
  onUpdateAnnotation: (key: string, value: string) => void;
  onNewLabelKey: (v: string) => void;
  onNewLabelValue: (v: string) => void;
}) {
  const { zh, c } = ctx;
  const isEditing = ctx.editingNode === nodeKey(node);
  // Display labels come from status.observedNodeLabels (Agent-observed truth on
  // the K8s Node). Editing drafts use the same source so the user sees exactly
  // what will be changed.
  const allLabels = getNodeAllLabels(node);
  // Administrator metadata is stored on the management Node CR as annotations,
  // so show it alongside the observed labels without treating it as a label.
  const managedAnnotations = Object.fromEntries(
    NODE_MANAGED_ANNOTATION_KEYS.flatMap((key) => {
      const value = node.metadata.annotations?.[key];
      return value ? [[key, value] as [string, string]] : [];
    }),
  );
  const visibleLabels = { ...allLabels, ...managedAnnotations };
  const adminInsight = (
    <div className="admin-node-insight">
      <div className="admin-node-actionbar">
        <div>
          <span className="eyebrow">{zh ? "节点运维" : "Node operations"}</span>
          <strong>{zh ? "调度与标签管理" : "Scheduling and labels"}</strong>
          <small>
            {node.spec.unschedulable
              ? zh
                ? "该节点当前已停止接收新任务"
                : "This node is not accepting new workloads"
              : zh
                ? "该节点当前允许任务调度"
                : "This node is accepting workloads"}
          </small>
        </div>
        <div className="row-actions">
          {!isEditing && (
            <button
              className="primary-button"
              onClick={() => ctx.onStartEdit(node)}
            >
              <Pencil size={15} />
              {zh ? "管理标签" : "Manage labels"}
            </button>
          )}
          <button
            className={
              node.spec.unschedulable
                ? "primary-button"
                : "secondary-button admin-cordon-button"
            }
            disabled={ctx.cordoning}
            onClick={() => ctx.onToggleCordon(node)}
          >
            <Ban size={14} />
            {ctx.cordoning
              ? zh
                ? "处理中…"
                : "Updating…"
              : node.spec.unschedulable
                ? zh
                  ? "恢复调度"
                  : "Resume Schedule"
                : zh
                  ? "暂停调度"
                  : "Pause Schedule"}
          </button>
        </div>
      </div>

      <section className="admin-node-labels node-insight-section">
        <div className="node-insight-section-head">
          <div>
            <span>{zh ? "调度标签" : "Scheduling labels"}</span>
            <small>
              {zh
                ? "标签会直接影响任务的节点选择与调度"
                : "Labels affect node selection and workload scheduling"}
            </small>
          </div>
          {!isEditing && (
            <b>
              {Object.keys(visibleLabels).length} {zh ? "项" : "items"}
            </b>
          )}
        </div>
        {isEditing ? (
          <div className="label-editor admin-label-editor">
            <div className="admin-node-managed-fields">
              <div className="admin-node-managed-field admin-node-managed-categories">
                <label>{zh ? "节点分类" : "Node categories"}</label>
                <div className="admin-node-category-chips">
                  {(["cloud", "edge", "robot"] as NodeCategory[]).map(
                    (category) => {
                      const key = `rlark.io/node-category-${category}`;
                      const selected = ctx.annotationDraft[key] === "true";
                      return (
                        <button
                          type="button"
                          key={category}
                          className={selected ? "selected" : ""}
                          aria-pressed={selected}
                          onClick={() =>
                            ctx.onUpdateAnnotation(key, selected ? "" : "true")
                          }
                        >
                          {zh
                            ? category === "robot"
                              ? "端真机"
                              : categoryLabels[category].zh
                            : categoryLabels[category].en}
                        </button>
                      );
                    },
                  )}
                </div>
              </div>
              {NODE_FREE_TEXT_ANNOTATION_KEYS.map((key) => (
                <div className="admin-node-managed-field" key={key}>
                  <label>
                    {key === "rlark.io/city"
                      ? zh
                        ? "物理位置"
                        : "Location"
                      : key === "rlark.io/gpu-model"
                        ? zh
                          ? "GPU 型号"
                          : "GPU model"
                        : zh
                          ? "端侧设备型号"
                          : "Device model"}
                  </label>
                  <input
                    value={ctx.annotationDraft[key] ?? ""}
                    onChange={(event) =>
                      ctx.onUpdateAnnotation(key, event.target.value)
                    }
                    placeholder={
                      key === "rlark.io/city"
                        ? zh
                          ? "例如：上海市"
                          : "e.g. Shanghai"
                        : zh
                          ? "留空可移除"
                          : "Leave blank to remove"
                    }
                  />
                </div>
              ))}
            </div>
            <div className="admin-node-extension-head">
              <div>
                <strong>
                  {zh ? "扩展调度标签" : "Extended scheduling labels"}
                </strong>
                <small>
                  {zh
                    ? "用于自定义亲和性、拓扑与业务调度约束"
                    : "Custom affinity, topology, and workload constraints"}
                </small>
              </div>
            </div>
            {Object.entries(ctx.labelDraft)
              .filter(([key]) => !NODE_MANAGED_KEYS.has(key))
              .map(([key, value]) => {
                const warning = key.includes("kubernetes.io")
                  ? {
                      level: "danger",
                      text: zh
                        ? "修改该标签可能影响 Kubernetes 基础功能"
                        : "Modifying this label may affect core Kubernetes functionality",
                    }
                  : key.includes("rlark.io")
                    ? {
                        level: "warn",
                        text: zh
                          ? "修改该标签可能影响 RLark 平台任务"
                          : "Modifying this label may affect RLark platform tasks",
                      }
                    : null;
                return (
                  <div className="label-edit-row" key={key}>
                    <div className="label-key-cell">
                      <code>{key}</code>
                      {warning && (
                        <span
                          className={`label-key-tooltip label-key-tooltip-${warning.level}`}
                          data-tooltip={warning.text}
                          tabIndex={0}
                          role="img"
                          aria-label={warning.text}
                        >
                          <AlertTriangle size={13} />
                        </span>
                      )}
                    </div>
                    <input
                      value={value}
                      onChange={(event) =>
                        ctx.onUpdateLabel(key, event.target.value)
                      }
                      placeholder="value"
                    />
                    <button
                      type="button"
                      className="icon-button danger"
                      onClick={() => ctx.onRemoveLabel(key)}
                      aria-label={`${zh ? "删除标签" : "Remove label"} ${key}`}
                    >
                      <X size={14} />
                    </button>
                  </div>
                );
              })}
            <div className="label-add-row">
              <input
                value={ctx.newLabelKey}
                onChange={(event) => ctx.onNewLabelKey(event.target.value)}
                placeholder={
                  zh ? "标签键，例如 team/zone" : "Label key, e.g. team/zone"
                }
                onKeyDown={(event) => event.key === "Enter" && ctx.onAddLabel()}
              />
              <input
                value={ctx.newLabelValue}
                onChange={(event) => ctx.onNewLabelValue(event.target.value)}
                placeholder={zh ? "标签值" : "Label value"}
                onKeyDown={(event) => event.key === "Enter" && ctx.onAddLabel()}
              />
              <button className="secondary-button" onClick={ctx.onAddLabel}>
                <Plus size={14} />
                {zh ? "添加" : "Add"}
              </button>
            </div>
            <div className="admin-label-actions">
              <button
                className="primary-button"
                disabled={ctx.saving}
                onClick={() =>
                  ctx.onSaveLabels(
                    node.metadata.name,
                    node.metadata.namespace ?? "",
                  )
                }
              >
                <Save size={15} />
                {ctx.saving
                  ? zh
                    ? "保存中…"
                    : "Saving…"
                  : zh
                    ? "保存标签"
                    : "Save labels"}
              </button>
              <button className="secondary-button" onClick={ctx.onCancelEdit}>
                {zh ? "取消" : "Cancel"}
              </button>
            </div>
          </div>
        ) : (
          <div className="label-list">
            {Object.entries(visibleLabels).length === 0 ? (
              <div className="admin-label-empty">
                <small>
                  {zh
                    ? "该节点暂无调度标签"
                    : "This node has no scheduling labels"}
                </small>
                <button
                  className="secondary-button"
                  onClick={() => ctx.onStartEdit(node)}
                >
                  <Plus size={14} />
                  {zh ? "添加标签" : "Add label"}
                </button>
              </div>
            ) : (
              Object.entries(visibleLabels).map(([key, value]) => (
                <span
                  key={key}
                  className="label-chip"
                  title={`${key}=${value}`}
                >
                  <code>{key}</code>
                  <i>{value}</i>
                </span>
              ))
            )}
          </div>
        )}
      </section>

      <NodeDetailReal node={node} copy={c} showConditions />
    </div>
  );

  return adminInsight;
  /* Legacy compact detail markup retained temporarily for diff continuity.
  const cat = getNodeCategory(node);
  const catInfo = categoryLabels[cat];
  const CatIcon = catInfo.icon;

  const resKeys = (obj?: Record<string, string>) =>
    obj ? Object.keys(obj) : [];
  const capKeys = resKeys(node.status?.capacity);
  const allocKeys = resKeys(node.status?.allocatable);
  const usedKeys = resKeys(node.status?.used);
  const allResKeys = Array.from(
    new Set([...capKeys, ...allocKeys, ...usedKeys]),
  );

  return (
    <div className="panel node-detail-panel">
      <div className="node-detail-header">
        <span className={"node-status-ring " + phase.toLowerCase()}>
          <CatIcon size={18} />
        </span>
        <div>
          <h3>{node.metadata.name}</h3>
          <small>
            <StatusBadge phase={phase} copy={c} />
            {" · "}
            {node.spec.agentType ?? "—"}
            {" · "}
            {node.status?.nodeInfo?.architecture ?? "—"}
            {" · "}
            {node.status?.nodeInfo?.operatingSystem ?? "—"}
          </small>
        </div>
        <div className="row-actions" style={{ gap: 4 }}>
          <button
            className={
              node.spec.unschedulable ? "primary-button" : "secondary-button"
            }
            disabled={ctx.cordoning}
            onClick={() => ctx.onToggleCordon(node)}
            title={
              node.spec.unschedulable
                ? zh
                  ? "取消调度限制"
                  : "Uncordon"
                : zh
                  ? "调度限制"
                  : "Cordon"
            }
          >
            <Ban size={14} />
            {node.spec.unschedulable
              ? zh
                ? "Uncordon"
                : "Uncordon"
              : zh
                ? "Cordon"
                : "Cordon"}
          </button>
          <button
            className="icon-button"
            onClick={() => ctx.onStartEdit(node)}
            title={zh ? "编辑标签" : "Edit Labels"}
          >
            <Pencil size={15} />
          </button>
        </div>
      </div>

      <div className="node-detail-body">
        {node.status?.addresses && node.status.addresses.length > 0 && (
          <div className="node-detail-section">
            <span className="node-detail-label">
              {zh ? "地址" : "Addresses"}
            </span>
            <div className="node-detail-value">
              {node.status.addresses.map((addr, i) => (
                <span key={i} className="label-chip">
                  <code>{addr.type}</code>
                  <i>{addr.address}</i>
                </span>
              ))}
            </div>
          </div>
        )}

        <div className="node-detail-section">
          <span className="node-detail-label">
            {zh ? "系统信息" : "System Info"}
          </span>
          <div className="node-detail-grid">
            <div>
              <span className="muted">{zh ? "架构" : "Arch"}</span>
              <strong>{node.status?.nodeInfo?.architecture ?? "—"}</strong>
            </div>
            <div>
              <span className="muted">{zh ? "内核" : "Kernel"}</span>
              <strong>{node.status?.nodeInfo?.kernelVersion ?? "—"}</strong>
            </div>
            <div>
              <span className="muted">{zh ? "系统" : "OS"}</span>
              <strong>{node.status?.nodeInfo?.operatingSystem ?? "—"}</strong>
            </div>
            <div>
              <span className="muted">{zh ? "Agent" : "Agent"}</span>
              <strong>{node.status?.nodeInfo?.agentVersion ?? "—"}</strong>
            </div>
          </div>
        </div>

        {allResKeys.length > 0 && (
          <div className="node-detail-section">
            <span className="node-detail-label">
              {zh ? "资源" : "Resources"}
            </span>
            <div className="node-detail-scroll">
            <table className="node-resource-table">
              <thead>
                <tr>
                  <th>{zh ? "资源类型" : "Resource Type"}</th>
                  <th>{zh ? "总量" : "Capacity"}</th>
                  <th>{zh ? "可分配" : "Allocatable"}</th>
                  <th>{zh ? "已用" : "Used"}</th>
                </tr>
              </thead>
              <tbody>
                {allResKeys.map((k) => (
                  <tr key={k}>
                    <td>
                      <code>{k}</code>
                    </td>
                    <td>{node.status?.capacity?.[k] ?? "—"}</td>
                    <td>{node.status?.allocatable?.[k] ?? "—"}</td>
                    <td>{node.status?.used?.[k] ?? "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            </div>
          </div>
        )}

        <div className="node-detail-section">
          <span className="node-detail-label">{zh ? "标签" : "Labels"}</span>
          <div className="node-detail-scroll">
          {isEditing ? (
            <div className="label-editor">
              {Object.entries(ctx.labelDraft).map(([k, v]) => (
                <div className="label-edit-row" key={k}>
                  <code>{k}</code>
                  <input
                    value={v}
                    onChange={(e) => ctx.onUpdateLabel(k, e.target.value)}
                    placeholder="value"
                  />
                  <button
                    className="icon-button danger"
                    onClick={() => ctx.onRemoveLabel(k)}
                  >
                    <X size={14} />
                  </button>
                </div>
              ))}
              <div className="label-add-row">
                <input
                  value={ctx.newLabelKey}
                  onChange={(e) => ctx.onNewLabelKey(e.target.value)}
                  placeholder={
                    zh
                      ? "标签键（请以 rlark.io/ 开头）"
                      : "label key (prefix with rlark.io/)"
                  }
                  onKeyDown={(e) => e.key === "Enter" && ctx.onAddLabel()}
                />
                <input
                  value={ctx.newLabelValue}
                  onChange={(e) => ctx.onNewLabelValue(e.target.value)}
                  placeholder={zh ? "标签值" : "label value"}
                  onKeyDown={(e) => e.key === "Enter" && ctx.onAddLabel()}
                />
                <button className="secondary-button" onClick={ctx.onAddLabel}>
                  <Plus size={14} />
                  {zh ? "添加" : "Add"}
                </button>
              </div>
              <div className="row-actions">
                <button
                  className="primary-button"
                  disabled={ctx.saving}
                  onClick={() =>
                    ctx.onSaveLabels(
                      node.metadata.name,
                      node.metadata.namespace ?? "",
                    )
                  }
                >
                  <Save size={15} />
                  {ctx.saving
                    ? zh
                      ? "保存中..."
                      : "Saving..."
                    : zh
                      ? "保存"
                      : "Save"}
                </button>
                <button className="secondary-button" onClick={ctx.onCancelEdit}>
                  {zh ? "取消" : "Cancel"}
                </button>
              </div>
            </div>
          ) : (
            <div className="label-list">
              {Object.entries(labels).length === 0 ? (
                <small className="muted">{zh ? "无标签" : "No labels"}</small>
              ) : (
                Object.entries(labels).map(([k, v]) => (
                  <span key={k} className="label-chip" title={`${k}=${v}`}>
                    <code>{k}</code>
                    <i>{v}</i>
                  </span>
                ))
              )}
            </div>
          )}
          </div>
        </div>
      </div>
    </div>
  );
  */
}

export function AdminPage({
  copy: c,
  selectedNode,
  onNavigate,
}: {
  copy: Copy;
  selectedNode: string;
  onNavigate: (sub?: string) => void;
}) {
  const zh = c.nav.overview === "总览";
  const [nodes, setNodes] = useState<CRDNode[]>([]);
  const [nodeWorkloads, setNodeWorkloads] = useState<
    Record<string, { jobs: string[]; workers: number }>
  >({});
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [editingNode, setEditingNode] = useState<string | null>(null);
  const [labelDraft, setLabelDraft] = useState<Record<string, string>>({});
  const [annotationDraft, setAnnotationDraft] = useState<
    Record<string, string>
  >({});
  const [newLabelKey, setNewLabelKey] = useState("");
  const [newLabelValue, setNewLabelValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [cordoningNode, setCordoningNode] = useState<string | null>(null);
  const [selectedNodeKeys, setSelectedNodeKeys] = useState<Set<string>>(
    new Set(),
  );
  const [batchScheduling, setBatchScheduling] = useState(false);
  const [batchOpen, setBatchOpen] = useState(false);
  const [batchLocation, setBatchLocation] = useState("");
  const [batchGPUModel, setBatchGPUModel] = useState("");
  const [batchDeviceModel, setBatchDeviceModel] = useState("");
  const [batchCategories, setBatchCategories] = useState<NodeCategory[]>([]);
  const [batchFields, setBatchFields] = useState<Set<BatchMetadataField>>(
    new Set(),
  );
  const [batchMixedFields, setBatchMixedFields] = useState<
    Set<BatchMetadataField>
  >(new Set());
  const [batchSaving, setBatchSaving] = useState(false);
  const [removingNode, setRemovingNode] = useState<string | null>(null);
  const [drainingNode, setDrainingNode] = useState<string | null>(null);

  const fetchNodes = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    try {
      setNodes(await nodesApi.list());
    } catch (e) {
      setNodes([]);
      setError(e instanceof Error ? e.message : String(e));
    }
    // The Pod and Task mirrors only enrich the display; their failure must not
    // hide the Nodes themselves.
    try {
      const [tasks, pods] = await Promise.all([
        tasksApi.list(),
        podsApi.list(),
      ]);
      setNodeWorkloads(buildNodeWorkloads(tasks, pods));
    } catch (e) {
      setNodeWorkloads({});
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchNodes, 10000);

  const handleRefresh = async () => {
    if (refreshing) return;
    setRefreshing(true);
    try {
      await fetchNodes(false);
    } finally {
      setRefreshing(false);
    }
  };

  const startEdit = (node: CRDNode) => {
    // 草稿基于 observedNodeLabels，确保用户看到的是 K8s Node 上实际存在的标签。
    const observedLabels = getNodeAllLabels(node);
    const annotations = node.metadata.annotations ?? {};
    const annotationValues: Record<string, string> = {};
    // Managed fields live in annotations only; keep a legacy label out of the
    // label draft so saving never writes these keys back to the data plane.
    const labelValues = { ...observedLabels };
    for (const key of NODE_MANAGED_ANNOTATION_KEYS) {
      // The annotation is canonical; fall back to a legacy label so an existing
      // value stays visible until it is saved back as an annotation.
      const value = annotations[key] ?? observedLabels[key] ?? "";
      if (value) annotationValues[key] = value;
      delete labelValues[key];
    }
    // Surface the legacy comma-separated category value as boolean annotations;
    // the legacy label itself is left untouched on the data plane.
    const legacyCategories =
      annotations[NODE_CATEGORY_LABEL] ??
      observedLabels[NODE_CATEGORY_LABEL] ??
      "";
    for (const rawCategory of legacyCategories.split(",")) {
      const category = rawCategory.trim();
      if (category === "cloud" || category === "edge" || category === "robot") {
        annotationValues[`rlark.io/node-category-${category}`] = "true";
      }
    }
    delete labelValues[NODE_CATEGORY_LABEL];
    setEditingNode(nodeKey(node));
    setLabelDraft(labelValues);
    setAnnotationDraft(annotationValues);
    setNewLabelKey("");
    setNewLabelValue("");
  };

  const cancelEdit = () => {
    setEditingNode(null);
    setLabelDraft({});
    setAnnotationDraft({});
    setNewLabelKey("");
    setNewLabelValue("");
  };

  const saveLabels = async (nodeName: string, namespace: string) => {
    setSaving(true);
    setError("");
    try {
      // 删除键必须基于与草稿相同的来源（observedNodeLabels），否则会把
      // metadata.labels 里的系统标签误判为需要删除。
      const originalNode = nodes.find(
        (node) =>
          node.metadata.name === nodeName &&
          (node.metadata.namespace ?? "") === namespace,
      );
      const originalLabels = originalNode ? getNodeAllLabels(originalNode) : {};
      const originalAnnotations = originalNode?.metadata.annotations ?? {};

      // 管理元数据写入管理面 Node CR 的 Annotation。它与数据面无关，因此先写，
      // 避免数据面 relay 失败时丢失管理员配置。
      const annotationPatch: Record<string, string | null> = {};
      for (const key of NODE_MANAGED_ANNOTATION_KEYS) {
        const value = (annotationDraft[key] ?? "").trim();
        if (value) {
          if (originalAnnotations[key] !== value) annotationPatch[key] = value;
        } else if (originalAnnotations[key] !== undefined) {
          annotationPatch[key] = null;
        }
      }
      // 旧版逗号分隔分类已迁移为布尔 Annotation，保存时移除旧注解。
      if (originalAnnotations[NODE_CATEGORY_LABEL] !== undefined) {
        annotationPatch[NODE_CATEGORY_LABEL] = null;
      }
      if (Object.keys(annotationPatch).length > 0) {
        await nodesApi.patch(
          nodeName,
          { metadata: { annotations: annotationPatch } },
          namespace,
        );
      }

      // 调度标签写入数据面 K8s Node；只发送真正的变更，空补丁会被 Gateway 拒绝。
      // 管理字段只写 CR Annotation：即使数据面存在同名的历史 Label，也不在这里
      // 写入或删除，避免管理操作改动数据面标签。
      const labelPatch: Record<string, string | null> = {};
      for (const [key, value] of Object.entries(labelDraft)) {
        if (originalLabels[key] !== value) labelPatch[key] = value;
      }
      getRemovedLabelKeys(originalLabels, labelDraft)
        .filter((key) => !NODE_MANAGED_KEYS.has(key))
        .forEach((key) => {
          labelPatch[key] = null;
        });
      if (Object.keys(labelPatch).length > 0) {
        await nodesApi.patchLabels(nodeName, labelPatch, namespace);
      }

      // 乐观本地更新 — Gateway patch 成功后刷新本地 observedNodeLabels。
      try {
        setNodes(await nodesApi.list());
      } catch {
        // Non-fatal; the next auto-refresh will pick it up.
      }
      cancelEdit();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const addLabel = () => {
    const key = newLabelKey.trim();
    if (!key) return;
    if (NODE_MANAGED_KEYS.has(key)) {
      setError(
        zh
          ? `${key} 是管理元数据，请使用上方对应字段编辑。`
          : `${key} is administrator metadata; edit it with the fields above.`,
      );
      return;
    }
    setLabelDraft((prev) => ({ ...prev, [key]: newLabelValue }));
    setNewLabelKey("");
    setNewLabelValue("");
  };

  const removeLabel = (key: string) => {
    setLabelDraft((prev) => {
      const next = { ...prev };
      delete next[key];
      return next;
    });
  };

  const updateLabel = (key: string, value: string) => {
    setLabelDraft((prev) => ({ ...prev, [key]: value }));
  };

  const updateAnnotation = (key: string, value: string) => {
    setAnnotationDraft((prev) => ({ ...prev, [key]: value }));
  };

  const toggleCordon = async (node: CRDNode) => {
    const willCordon = !node.spec.unschedulable;
    if (
      !confirm(
        willCordon
          ? zh
            ? `确认对节点“${node.metadata.name}”执行暂停调度吗？执行后将停止把新的 RLark 任务调度到该节点，节点上的其他工作负载不受影响，已运行任务也不会被终止。`
            : `Pause scheduling on node "${node.metadata.name}"? New RLark tasks will stop being scheduled to it. Other workloads on the node are unaffected and running tasks remain active.`
          : zh
            ? `确认对节点“${node.metadata.name}”执行恢复调度吗？仅恢复向该节点调度新任务；被排空停止的任务需先取消驱逐才会恢复。`
            : `Resume scheduling on node "${node.metadata.name}"? This only allows new tasks to be scheduled to it; tasks stopped by a drain resume only after you cancel the drain.`,
      )
    )
      return;
    setCordoningNode(node.metadata.name);
    setError("");
    try {
      if (willCordon) {
        await nodesApi.patch(
          node.metadata.name,
          { spec: { unschedulable: true } },
          node.metadata.namespace ?? "",
        );
      } else {
        // 恢复调度: uncordon 会置 unschedulable=false 并取消 Drain 操作；但被排空
        // 停止的任务保持 suspended（node-drain-stopped 标记保留），需运维显式
        // 启动其 Job 才会恢复运行。
        await nodesApi.uncordon(
          node.metadata.name,
          node.metadata.namespace ?? "",
        );
      }
      setNodes((prev) =>
        prev.map((n) =>
          n.metadata.name === node.metadata.name
            ? {
                ...n,
                spec: {
                  ...n.spec,
                  unschedulable: !node.spec.unschedulable,
                },
              }
            : n,
        ),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setCordoningNode(null);
    }
  };

  const unmanageNode = async (node: CRDNode) => {
    if (
      !confirm(
        zh
          ? `确认取消纳管节点"${node.metadata.name}"？该节点的 RLark Node 资源将被删除，agent 不再为其上报状态。此操作不会删除 K8s 节点或其中的运行任务。`
          : `Unmanage node "${node.metadata.name}"? The RLark Node resource will be removed and the agent will stop reporting status. This will not delete the K8s node or running tasks.`,
      )
    )
      return;
    setRemovingNode(node.metadata.name);
    setError("");
    try {
      await nodesApi.remove(node.metadata.name, node.metadata.namespace);
      setNodes((prev) =>
        prev.filter((n) => n.metadata.name !== node.metadata.name),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setRemovingNode(null);
    }
  };

  const drainNode = async (node: CRDNode) => {
    if (
      !confirm(
        zh
          ? `确认对节点"${node.metadata.name}"执行排空？将停止该节点接收新任务，并尝试将现有任务重调度到其他节点。该操作会同时暂停节点调度（Cordon）。`
          : `Drain node "${node.metadata.name}"? This will stop the node from accepting new tasks and attempt to reschedule running tasks. It will also cordon the node.`,
      )
    )
      return;
    setDrainingNode(node.metadata.name);
    setError("");
    try {
      const resp = await nodesApi.drain(
        node.metadata.name,
        node.metadata.namespace ?? "",
      );
      const updated = resp.data;
      if (updated) {
        setNodes((prev) =>
          prev.map((n) =>
            n.metadata.name === node.metadata.name ? updated : n,
          ),
        );
      } else {
        // 保守更新本地缓存，让 UI 立即响应
        setNodes((prev) =>
          prev.map((n) =>
            n.metadata.name === node.metadata.name
              ? {
                  ...n,
                  spec: {
                    ...n.spec,
                    unschedulable: true,
                    operations: [
                      ...(n.spec.operations ?? []),
                      {
                        id: `drain-local-${Date.now()}`,
                        type: "Drain" as const,
                      },
                    ],
                  },
                }
              : n,
          ),
        );
      }
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
          ? `确认取消节点"${node.metadata.name}"的排空请求？`
          : `Cancel the drain request for node "${node.metadata.name}"?`,
      )
    )
      return;
    setDrainingNode(node.metadata.name);
    setError("");
    try {
      const resp = await nodesApi.undrain(
        node.metadata.name,
        node.metadata.namespace ?? "",
      );
      const updated = resp.data;
      if (updated) {
        setNodes((prev) =>
          prev.map((n) =>
            n.metadata.name === node.metadata.name ? updated : n,
          ),
        );
      } else {
        setNodes((prev) =>
          prev.map((n) =>
            n.metadata.name === node.metadata.name
              ? {
                  ...n,
                  spec: {
                    ...n.spec,
                    operations: (n.spec.operations ?? []).filter(
                      (op) => op.type !== "Drain",
                    ),
                  },
                }
              : n,
          ),
        );
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setDrainingNode(null);
    }
  };

  const saveBatchMetadata = async () => {
    const selectedNodes = nodes.filter((node) =>
      selectedNodeKeys.has(
        `${node.metadata.namespace ?? ""}/${node.metadata.name}`,
      ),
    );
    if (selectedNodes.length === 0) return;
    if (batchFields.size === 0) {
      setError(zh ? "请至少启用一个批量设置字段" : "Enable at least one field");
      return;
    }
    setBatchSaving(true);
    setError("");
    try {
      await Promise.all(
        selectedNodes.map(async (node) => {
          const original = node.metadata.annotations ?? {};
          const annotations: Record<string, string | null> = {};
          if (batchFields.has("location")) {
            annotations["rlark.io/city"] = batchLocation.trim() || null;
          }
          if (batchFields.has("categories")) {
            const categories = new Set(batchCategories);
            for (const category of [
              "cloud",
              "edge",
              "robot",
            ] as NodeCategory[]) {
              annotations[`rlark.io/node-category-${category}`] =
                categories.has(category) ? "true" : null;
            }
            // 旧版逗号分隔分类注解统一迁移为布尔注解。
            annotations[NODE_CATEGORY_LABEL] = null;
          }
          if (batchFields.has("gpu")) {
            annotations["rlark.io/gpu-model"] = batchGPUModel.trim() || null;
          }
          if (batchFields.has("device")) {
            annotations["rlark.io/device-model"] =
              batchDeviceModel.trim() || null;
          }
          // 未启用字段保持原值；仅当原有值存在时才发送删除标记。管理字段只写
          // 管理面 Node CR 的 Annotation，不触碰数据面标签。
          const patch: Record<string, string | null> = {};
          for (const [key, value] of Object.entries(annotations)) {
            if (value !== null || original[key] !== undefined) {
              patch[key] = value;
            }
          }
          if (Object.keys(patch).length === 0) return;
          try {
            await nodesApi.patch(
              node.metadata.name,
              { metadata: { annotations: patch } },
              node.metadata.namespace ?? "",
            );
          } catch (error) {
            throw new Error(`${node.metadata.name}: ${String(error)}`);
          }
        }),
      );
      await fetchNodes(false);
      setSelectedNodeKeys(new Set());
      setBatchOpen(false);
      setBatchLocation("");
      setBatchGPUModel("");
      setBatchDeviceModel("");
      setBatchCategories([]);
      setBatchFields(new Set());
      setBatchMixedFields(new Set());
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBatchSaving(false);
    }
  };

  const openBatchEditor = () => {
    const selected = nodes.filter((node) =>
      selectedNodeKeys.has(
        `${node.metadata.namespace ?? ""}/${node.metadata.name}`,
      ),
    );
    const mixed = new Set<BatchMetadataField>();
    const commonValue = (field: BatchMetadataField, values: string[]) => {
      if (values.length > 0 && values.every((value) => value === values[0])) {
        return values[0];
      }
      mixed.add(field);
      return "";
    };
    setBatchLocation(commonValue("location", selected.map(getNodeLocation)));
    setBatchGPUModel(commonValue("gpu", selected.map(getNodeGPUModel)));
    setBatchDeviceModel(
      commonValue("device", selected.map(getNodeDeviceModel)),
    );
    const categorySets = selected.map((node) =>
      getNodeCategories(node).filter((category) => category !== "unknown"),
    );
    const sameCategories =
      categorySets.length > 0 &&
      categorySets.every(
        (categories) => categories.join(",") === categorySets[0].join(","),
      );
    if (!sameCategories) mixed.add("categories");
    setBatchCategories(sameCategories ? categorySets[0] : []);
    setBatchMixedFields(mixed);
    setBatchFields(new Set());
    setBatchOpen(true);
  };

  const toggleBatchField = (field: BatchMetadataField) => {
    setBatchFields((current) => {
      const next = new Set(current);
      if (next.has(field)) next.delete(field);
      else next.add(field);
      return next;
    });
  };

  const updateSelectedScheduling = async (unschedulable: boolean) => {
    const selectedNodes = nodes.filter((node) =>
      selectedNodeKeys.has(
        `${node.metadata.namespace ?? ""}/${node.metadata.name}`,
      ),
    );
    if (selectedNodes.length === 0) return;
    const action = unschedulable
      ? zh
        ? "暂停调度"
        : "pause scheduling"
      : zh
        ? "恢复调度"
        : "resume scheduling";
    if (
      !confirm(
        zh
          ? `确认批量${action} ${selectedNodes.length} 个节点吗？`
          : `Confirm ${action} for ${selectedNodes.length} nodes?`,
      )
    )
      return;
    setBatchScheduling(true);
    setError("");
    try {
      await Promise.all(
        selectedNodes.map(async (node) => {
          try {
            await nodesApi.patch(
              node.metadata.name,
              { spec: { unschedulable } },
              node.metadata.namespace ?? "",
            );
          } catch (error) {
            throw new Error(`${node.metadata.name}: ${String(error)}`);
          }
        }),
      );
      setNodes((current) =>
        current.map((node) =>
          selectedNodeKeys.has(
            `${node.metadata.namespace ?? ""}/${node.metadata.name}`,
          )
            ? { ...node, spec: { ...node.spec, unschedulable } }
            : node,
        ),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBatchScheduling(false);
    }
  };

  const sharedProps = {
    zh,
    c,
    editingNode,
    labelDraft,
    annotationDraft,
    newLabelKey,
    newLabelValue,
    saving,
    cordoning: cordoningNode !== null,
    onStartEdit: startEdit,
    onToggleCordon: toggleCordon,
    onCancelEdit: cancelEdit,
    onSaveLabels: saveLabels,
    onAddLabel: addLabel,
    onRemoveLabel: removeLabel,
    onUpdateLabel: updateLabel,
    onUpdateAnnotation: updateAnnotation,
    onNewLabelKey: setNewLabelKey,
    onNewLabelValue: setNewLabelValue,
  };

  const selectedNodeObj =
    nodes.find((n) => n.metadata.name === selectedNode) ?? null;

  // Reset the detail-page container's scrollTop only when the selected node
  // changes. The node detail view is rendered inside an
  // `overflow: hidden auto` container; browser scroll-restore (and any prior
  // interaction that left the container scrolled) would otherwise hide the
  // top action bar (Cordon, Manage labels, Back button) below the viewport.
  // Do not depend on selectedNodeObj: the periodic refresh replaces the node
  // objects, and that must not reset the user's scroll position.
  useEffect(() => {
    if (!selectedNode) return;
    const container = document.querySelector(".node-detail-page");
    if (container) container.scrollTop = 0;
  }, [selectedNode]);

  const nodeOverview = {
    online: nodes.filter((node) => node.status?.phase === "Online").length,
    schedulable: nodes.filter((node) => !node.spec.unschedulable).length,
    clusters: new Set(nodes.map((node) => node.metadata.namespace ?? "default"))
      .size,
  };

  if (selectedNodeObj) {
    return (
      <div className="page-content resource-page node-detail-page">
        <div className="section-heading">
          <div>
            <button
              className="plain-button back-button"
              onClick={() => onNavigate()}
            >
              ← {zh ? "返回节点列表" : "Back"}
            </button>
            <span className="eyebrow">
              <Settings size={13} />
              {zh ? "节点详情" : "Node Detail"}
            </span>
          </div>
        </div>
        <NodeDetailPanel node={selectedNodeObj} {...sharedProps} />
      </div>
    );
  }

  return (
    <div className="page-content resource-page admin-node-management-page">
      <div className="section-heading admin-node-page-heading">
        <div>
          <span className="eyebrow">
            <Settings size={13} />
            {zh ? "运维管理" : "Admin"}
          </span>
          <h2>{zh ? "节点管理" : "Node Management"}</h2>
          <p>
            {zh
              ? "统一查看各类节点，进入详情后管理调度状态与节点标签。"
              : "Browse every node type in one list, then manage scheduling and labels in details."}
          </p>
        </div>
        <div
          className="admin-node-list-actions"
          aria-label={zh ? "节点批量操作" : "Node batch actions"}
        >
          <button
            className="secondary-button danger"
            disabled={selectedNodeKeys.size === 0 || batchScheduling}
            onClick={() => updateSelectedScheduling(true)}
          >
            <Ban size={15} />
            {zh ? "批量暂停调度" : "Pause Schedule"}
          </button>
          <button
            className="secondary-button"
            disabled={selectedNodeKeys.size === 0 || batchScheduling}
            onClick={() => updateSelectedScheduling(false)}
          >
            {zh ? "批量恢复调度" : "Resume Schedule"}
          </button>
          <button
            className="primary-button"
            disabled={selectedNodeKeys.size === 0 || batchSaving}
            onClick={openBatchEditor}
          >
            <Pencil size={15} />
            {zh
              ? `批量设置 (${selectedNodeKeys.size})`
              : `Batch edit (${selectedNodeKeys.size})`}
          </button>
          <button
            className="secondary-button"
            onClick={handleRefresh}
            disabled={refreshing}
            aria-busy={refreshing}
          >
            <RefreshCw
              size={16}
              className={refreshing ? "job-action-loading" : ""}
            />
            {refreshing
              ? zh
                ? "刷新中..."
                : "Refreshing..."
              : c.common.refresh}
          </button>
        </div>
      </div>

      <div
        className="admin-node-overview"
        aria-label={zh ? "节点概况" : "Node overview"}
      >
        <div>
          <span className="admin-node-overview-icon total">
            <Server size={16} />
          </span>
          <small>{zh ? "全部节点" : "Total nodes"}</small>
          <strong>{nodes.length}</strong>
        </div>
        <div>
          <span className="admin-node-overview-icon online">
            <Activity size={16} />
          </span>
          <small>{zh ? "在线节点" : "Online"}</small>
          <strong>{nodeOverview.online}</strong>
        </div>
        <div>
          <span className="admin-node-overview-icon ready">
            <CloudCog size={16} />
          </span>
          <small>{zh ? "允许调度" : "Schedulable"}</small>
          <strong>{nodeOverview.schedulable}</strong>
        </div>
        <div>
          <span className="admin-node-overview-icon clusters">
            <Network size={16} />
          </span>
          <small>{zh ? "所属集群" : "Clusters"}</small>
          <strong>{nodeOverview.clusters}</strong>
        </div>
      </div>

      {error && (
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}

      {batchOpen && (
        <section className="panel admin-node-batch-panel">
          <div className="admin-node-batch-head">
            <div>
              <span className="eyebrow">
                {zh ? "批量标注" : "Batch metadata"}
              </span>
              <strong>
                {zh
                  ? `已选择 ${selectedNodeKeys.size} 个节点`
                  : `${selectedNodeKeys.size} nodes selected`}
              </strong>
              <small>
                {zh
                  ? "先启用需要修改的字段；未启用字段保持原值。清空已启用字段可删除原值，保存后写入节点 CR 的 Annotation。"
                  : "Enable only the fields to change. Disabled fields stay unchanged; clear an enabled field to remove it. Values are saved as Node CR annotations."}
              </small>
            </div>
            <button
              className="icon-button"
              onClick={() => setBatchOpen(false)}
              aria-label={zh ? "关闭" : "Close"}
            >
              <X size={17} />
            </button>
          </div>
          <div className="admin-node-batch-fields">
            <div
              className={`admin-node-batch-field${batchFields.has("location") ? " active" : ""}`}
            >
              <button
                type="button"
                className="admin-node-batch-field-toggle"
                aria-pressed={batchFields.has("location")}
                onClick={() => toggleBatchField("location")}
              >
                <span className="admin-node-batch-field-icon">
                  <MapPin size={16} />
                </span>
                <span>
                  <strong>{zh ? "地理位置" : "Location"}</strong>
                  <small>
                    {batchMixedFields.has("location")
                      ? zh
                        ? "当前存在多个值"
                        : "Multiple current values"
                      : batchLocation || (zh ? "当前未设置" : "Not set")}
                  </small>
                </span>
                <i>
                  {batchFields.has("location")
                    ? zh
                      ? "将修改"
                      : "Changing"
                    : zh
                      ? "保持原值"
                      : "Keep"}
                </i>
              </button>
              <input
                hidden={!batchFields.has("location")}
                value={batchLocation}
                onChange={(event) => setBatchLocation(event.target.value)}
                placeholder={
                  batchMixedFields.has("location")
                    ? zh
                      ? "多个不同值"
                      : "Multiple values"
                    : zh
                      ? "例如：杭州市"
                      : "e.g. Hangzhou"
                }
              />
              {batchFields.has("location") && (
                <div className="admin-node-batch-examples">
                  <span>{zh ? "示例" : "Examples"}</span>
                  {["杭州市", "北京市", "上海市"].map((value) => (
                    <button
                      type="button"
                      key={value}
                      onClick={() => setBatchLocation(value)}
                    >
                      {value}
                    </button>
                  ))}
                </div>
              )}
            </div>
            <div
              className={`admin-node-batch-field${batchFields.has("categories") ? " active" : ""}`}
            >
              <button
                type="button"
                className="admin-node-batch-field-toggle"
                aria-pressed={batchFields.has("categories")}
                onClick={() => toggleBatchField("categories")}
              >
                <span className="admin-node-batch-field-icon">
                  <Network size={16} />
                </span>
                <span>
                  <strong>{zh ? "节点分类" : "Node categories"}</strong>
                  <small>
                    {batchMixedFields.has("categories")
                      ? zh
                        ? "当前存在多个值"
                        : "Multiple current values"
                      : batchCategories.length
                        ? batchCategories
                            .map(
                              (category) =>
                                categoryLabels[category][zh ? "zh" : "en"],
                            )
                            .join(zh ? "、" : ", ")
                        : zh
                          ? "当前未设置"
                          : "Not set"}
                  </small>
                </span>
                <i>
                  {batchFields.has("categories")
                    ? zh
                      ? "将修改"
                      : "Changing"
                    : zh
                      ? "保持原值"
                      : "Keep"}
                </i>
              </button>
              {batchFields.has("categories") && (
                <>
                  <div
                    className="admin-node-category-chips"
                    role="group"
                    aria-label={zh ? "节点分类" : "Node category"}
                  >
                    {(["cloud", "edge", "robot"] as NodeCategory[]).map(
                      (category) => (
                        <button
                          type="button"
                          key={category}
                          className={
                            batchCategories.includes(category) ? "selected" : ""
                          }
                          aria-pressed={batchCategories.includes(category)}
                          onClick={() =>
                            setBatchCategories((current) =>
                              current.includes(category)
                                ? current.filter((item) => item !== category)
                                : [...current, category],
                            )
                          }
                        >
                          {zh
                            ? category === "robot"
                              ? "端真机"
                              : categoryLabels[category].zh
                            : categoryLabels[category].en}
                        </button>
                      ),
                    )}
                  </div>
                  <small className="admin-node-batch-hint">
                    {zh
                      ? "可多选。例如 GPU 服务器选择「云算力」，机器人本体选择「端真机」。"
                      : "Multiple selection. For example, choose Cloud for GPU servers and Robot for robot devices."}
                  </small>
                </>
              )}
            </div>
            <div
              className={`admin-node-batch-field${batchFields.has("gpu") ? " active" : ""}`}
            >
              <button
                type="button"
                className="admin-node-batch-field-toggle"
                aria-pressed={batchFields.has("gpu")}
                onClick={() => toggleBatchField("gpu")}
              >
                <span className="admin-node-batch-field-icon">
                  <CloudCog size={16} />
                </span>
                <span>
                  <strong>{zh ? "GPU 型号" : "GPU model"}</strong>
                  <small>
                    {batchMixedFields.has("gpu")
                      ? zh
                        ? "当前存在多个值"
                        : "Multiple current values"
                      : batchGPUModel || (zh ? "当前未设置" : "Not set")}
                  </small>
                </span>
                <i>
                  {batchFields.has("gpu")
                    ? zh
                      ? "将修改"
                      : "Changing"
                    : zh
                      ? "保持原值"
                      : "Keep"}
                </i>
              </button>
              <input
                hidden={!batchFields.has("gpu")}
                value={batchGPUModel}
                onChange={(event) => setBatchGPUModel(event.target.value)}
                placeholder={
                  batchMixedFields.has("gpu")
                    ? zh
                      ? "多个不同值"
                      : "Multiple values"
                    : "NVIDIA H800"
                }
              />
              {batchFields.has("gpu") && (
                <div className="admin-node-batch-examples">
                  <span>{zh ? "示例" : "Examples"}</span>
                  {["NVIDIA H800", "NVIDIA A100", "NVIDIA RTX 4090"].map(
                    (value) => (
                      <button
                        type="button"
                        key={value}
                        onClick={() => setBatchGPUModel(value)}
                      >
                        {value}
                      </button>
                    ),
                  )}
                </div>
              )}
            </div>
            <div
              className={`admin-node-batch-field${batchFields.has("device") ? " active" : ""}`}
            >
              <button
                type="button"
                className="admin-node-batch-field-toggle"
                aria-pressed={batchFields.has("device")}
                onClick={() => toggleBatchField("device")}
              >
                <span className="admin-node-batch-field-icon">
                  <Server size={16} />
                </span>
                <span>
                  <strong>
                    {zh ? "具身设备型号" : "Embodied device model"}
                  </strong>
                  <small>
                    {batchMixedFields.has("device")
                      ? zh
                        ? "当前存在多个值"
                        : "Multiple current values"
                      : batchDeviceModel || (zh ? "当前未设置" : "Not set")}
                  </small>
                </span>
                <i>
                  {batchFields.has("device")
                    ? zh
                      ? "将修改"
                      : "Changing"
                    : zh
                      ? "保持原值"
                      : "Keep"}
                </i>
              </button>
              <input
                hidden={!batchFields.has("device")}
                value={batchDeviceModel}
                onChange={(event) => setBatchDeviceModel(event.target.value)}
                placeholder={
                  batchMixedFields.has("device")
                    ? zh
                      ? "多个不同值"
                      : "Multiple values"
                    : "Unitree G1"
                }
              />
              {batchFields.has("device") && (
                <div className="admin-node-batch-examples">
                  <span>{zh ? "示例" : "Examples"}</span>
                  {["Unitree G1", "Unitree Go2", "Robodog OT-T12"].map(
                    (value) => (
                      <button
                        type="button"
                        key={value}
                        onClick={() => setBatchDeviceModel(value)}
                      >
                        {value}
                      </button>
                    ),
                  )}
                </div>
              )}
            </div>
          </div>
          <div className="admin-node-batch-actions">
            <span>
              {zh
                ? `将修改 ${batchFields.size} 项属性 · ${selectedNodeKeys.size} 个节点`
                : `${batchFields.size} properties · ${selectedNodeKeys.size} nodes`}
            </span>
            <button
              className="secondary-button"
              onClick={() => setBatchOpen(false)}
            >
              {zh ? "取消" : "Cancel"}
            </button>
            <button
              className="primary-button"
              disabled={batchSaving || batchFields.size === 0}
              onClick={saveBatchMetadata}
            >
              <Save size={15} />
              {batchSaving
                ? zh
                  ? "保存中…"
                  : "Saving…"
                : zh
                  ? "应用设置"
                  : "Apply"}
            </button>
          </div>
        </section>
      )}

      {loading ? (
        <p className="muted">{zh ? "加载中..." : "Loading..."}</p>
      ) : (
        <NodeResourceBrowser
          nodes={nodes}
          copy={c}
          onRefresh={handleRefresh}
          refreshing={refreshing}
          onSelectNode={(name) => onNavigate(name)}
          nodeWorkloads={nodeWorkloads}
          onToggleScheduling={toggleCordon}
          updatingNode={cordoningNode ?? removingNode ?? drainingNode}
          onUnmanageNode={unmanageNode}
          onDrainNode={drainNode}
          onUndrainNode={undrainNode}
          selectedNodeKeys={selectedNodeKeys}
          onSelectionChange={setSelectedNodeKeys}
          onSelectFiltered={(keys) =>
            setSelectedNodeKeys((current) => new Set([...current, ...keys]))
          }
          onClearSelection={() => setSelectedNodeKeys(new Set())}
        />
      )}
    </div>
  );
}
import { nodesApi, podsApi, tasksApi } from "../backend";
