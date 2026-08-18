import { useMemo, useState } from "react";
import {
  Activity,
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
  categoryLabels,
  getNodeCategories,
  getNodeDeviceModel,
  getNodeGPUModel,
  getNodeLocation,
} from "../utils/nodes";
import { useAutoRefresh } from "../hooks";
import { MetricCard, StatusBadge } from "../components/shared";
import { NodeResourceBrowser } from "../components/NodeResourceBrowser";
import { ClusterDetailReal, NodeDetailReal } from "../pages/Clusters";

const NODE_FREE_TEXT_KEYS = [
  "rlark.io/city",
  "rlark.io/gpu-model",
  "rlark.io/device-model",
] as const;
type BatchMetadataField = "location" | "categories" | "gpu" | "device";

export function ClustersOverviewAdminPage({ copy: c }: { copy: Copy }) {
  const zh = c.nav.overview === "总览";
  const [nodes, setNodes] = useState<CRDNode[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [selectedClusterNs, setSelectedClusterNs] = useState<string | null>(
    null,
  );

  const fetchNodes = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    try {
      const resp = await fetch("/api/v1/rlinf.io/v1alpha1/nodes");
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      setNodes(data.items ?? []);
    } catch (e) {
      setNodes([]);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchNodes, 10000);

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
          <button className="secondary-button" onClick={() => fetchNodes()}>
            <RefreshCw size={16} />
            {c.common.refresh}
          </button>
        </div>
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      </div>
    );
  }

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
        <button className="secondary-button" onClick={() => fetchNodes()}>
          <RefreshCw size={16} />
          {c.common.refresh}
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
              <span>Distribution</span>
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
  const isEditing = ctx.editingNode === node.metadata.name;
  const labels = node.metadata.labels ?? {};
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
                  : "Uncordon"
                : zh
                  ? "停止调度"
                  : "Cordon"}
          </button>
          {!isEditing && (
            <button
              className="primary-button"
              onClick={() => ctx.onStartEdit(node)}
            >
              <Pencil size={15} />
              {zh ? "管理标签" : "Manage labels"}
            </button>
          )}
        </div>
      </div>

      <NodeDetailReal node={node} copy={c} />

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
              {Object.keys(labels).length} {zh ? "项" : "items"}
            </b>
          )}
        </div>
        {isEditing ? (
          <div className="label-editor admin-label-editor">
            <div className="label-edit-row city-label-row">
              <code>rlark.io/city</code>
              <input
                value={ctx.labelDraft["rlark.io/city"] ?? ""}
                onChange={(event) =>
                  ctx.onUpdateLabel("rlark.io/city", event.target.value)
                }
                placeholder={
                  zh ? "所在城市，例如：上海市" : "City, e.g. Shanghai"
                }
              />
              <MapPin size={15} />
            </div>
            {Object.entries(ctx.labelDraft)
              .filter(([key]) => key !== "rlark.io/city")
              .map(([key, value]) => (
                <div className="label-edit-row" key={key}>
                  <code>{key}</code>
                  <input
                    value={value}
                    onChange={(event) =>
                      ctx.onUpdateLabel(key, event.target.value)
                    }
                    placeholder="value"
                  />
                  <button
                    className="icon-button danger"
                    onClick={() => ctx.onRemoveLabel(key)}
                    aria-label={`${zh ? "删除标签" : "Remove label"} ${key}`}
                  >
                    <X size={14} />
                  </button>
                </div>
              ))}
            <div className="label-add-row">
              <input
                value={ctx.newLabelKey}
                onChange={(event) => ctx.onNewLabelKey(event.target.value)}
                placeholder={
                  zh
                    ? "标签键，例如 rlark.io/zone"
                    : "Label key, e.g. rlark.io/zone"
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
            {Object.entries(labels).length === 0 ? (
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
              Object.entries(labels).map(([key, value]) => (
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
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editingNode, setEditingNode] = useState<string | null>(null);
  const [labelDraft, setLabelDraft] = useState<Record<string, string>>({});
  const [newLabelKey, setNewLabelKey] = useState("");
  const [newLabelValue, setNewLabelValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [cordoningNode, setCordoningNode] = useState<string | null>(null);
  const [selectedNodeKeys, setSelectedNodeKeys] = useState<Set<string>>(
    new Set(),
  );
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
  const [batchScheduling, setBatchScheduling] = useState(false);

  const fetchNodes = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    try {
      const resp = await fetch("/api/v1/rlinf.io/v1alpha1/nodes");
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      setNodes(data.items ?? []);
    } catch (e) {
      setNodes([]);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchNodes, 10000);

  const startEdit = (node: CRDNode) => {
    setEditingNode(node.metadata.name);
    setLabelDraft({
      ...(node.metadata.labels ?? {}),
      ...Object.fromEntries(
        NODE_FREE_TEXT_KEYS.map((key) => [
          key,
          key === "rlark.io/city"
            ? getNodeLocation(node)
            : (node.metadata.annotations?.[key] ??
              node.metadata.labels?.[key] ??
              ""),
        ]),
      ),
    });
    setNewLabelKey("");
    setNewLabelValue("");
  };

  const cancelEdit = () => {
    setEditingNode(null);
    setLabelDraft({});
    setNewLabelKey("");
    setNewLabelValue("");
  };

  const saveLabels = async (nodeName: string, namespace: string) => {
    setSaving(true);
    setError("");
    try {
      const labels = { ...labelDraft };
      const annotations = {
        ...(nodes.find((node) => node.metadata.name === nodeName)?.metadata
          .annotations ?? {}),
      };
      NODE_FREE_TEXT_KEYS.forEach((key) => {
        annotations[key] = labels[key]?.trim() ?? "";
        delete labels[key];
      });
      const labelPatch: Record<string, string | null> = { ...labels };
      NODE_FREE_TEXT_KEYS.forEach((key) => {
        labelPatch[key] = null;
      });
      const patch = { metadata: { labels: labelPatch, annotations } };
      const resp = await fetch(
        `/api/v1/rlinf.io/v1alpha1/nodes/${nodeName}?namespace=${encodeURIComponent(namespace)}`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/merge-patch+json" },
          body: JSON.stringify(patch),
        },
      );
      if (!resp.ok)
        throw new Error(`HTTP ${resp.status}: ${await resp.text()}`);
      setNodes((prev) =>
        prev.map((n) =>
          n.metadata.name === nodeName
            ? { ...n, metadata: { ...n.metadata, labels, annotations } }
            : n,
        ),
      );
      cancelEdit();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const addLabel = () => {
    if (!newLabelKey.trim()) return;
    setLabelDraft((prev) => ({ ...prev, [newLabelKey.trim()]: newLabelValue }));
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

  const toggleCordon = async (node: CRDNode) => {
    const willCordon = !node.spec.unschedulable;
    if (
      !confirm(
        willCordon
          ? zh
            ? `确认封锁节点“${node.metadata.name}”吗？封锁后将不再接收新任务，已运行任务不会被终止。`
            : `Cordon node "${node.metadata.name}"? It will stop accepting new jobs, while running jobs remain active.`
          : zh
            ? `确认解封节点“${node.metadata.name}”并恢复任务调度吗？`
            : `Uncordon node "${node.metadata.name}" and resume scheduling?`,
      )
    )
      return;
    setCordoningNode(node.metadata.name);
    setError("");
    try {
      const patch = { spec: { unschedulable: !node.spec.unschedulable } };
      const resp = await fetch(
        `/api/v1/rlinf.io/v1alpha1/nodes/${node.metadata.name}?namespace=${encodeURIComponent(node.metadata.namespace ?? "")}`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/merge-patch+json" },
          body: JSON.stringify(patch),
        },
      );
      if (!resp.ok)
        throw new Error(`HTTP ${resp.status}: ${await resp.text()}`);
      setNodes((prev) =>
        prev.map((n) =>
          n.metadata.name === node.metadata.name
            ? {
                ...n,
                spec: { ...n.spec, unschedulable: !node.spec.unschedulable },
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

  const saveBatchMetadata = async () => {
    const selectedNodes = nodes.filter((node) =>
      selectedNodeKeys.has(
        `${node.metadata.namespace ?? ""}/${node.metadata.name}`,
      ),
    );
    if (selectedNodes.length === 0) return;
    if (batchFields.size === 0) {
      setError(zh ? "请至少填写一个批量设置字段" : "Enter at least one field");
      return;
    }
    setBatchSaving(true);
    setError("");
    try {
      await Promise.all(
        selectedNodes.map(async (node) => {
          const labels = { ...(node.metadata.labels ?? {}) };
          const annotations = { ...(node.metadata.annotations ?? {}) };
          const removedLabelKeys = new Set<string>();
          if (batchFields.has("location")) {
            const value = batchLocation.trim();
            if (value) annotations["rlark.io/city"] = value;
            else delete annotations["rlark.io/city"];
            delete labels["rlark.io/city"];
            removedLabelKeys.add("rlark.io/city");
          }
          const categories = batchFields.has("categories")
            ? batchCategories
            : getNodeCategories(node);
          const appliesGPU = categories.includes("cloud");
          const appliesDevice =
            categories.includes("edge") || categories.includes("robot");
          if (batchFields.has("categories")) {
            delete labels["rlark.io/node-category"];
            removedLabelKeys.add("rlark.io/node-category");
            (["cloud", "edge", "robot"] as const).forEach((category) => {
              const key = `rlark.io/node-category-${category}`;
              if (batchCategories.includes(category)) labels[key] = "true";
              else delete labels[key];
            });
          }
          if (batchFields.has("gpu") && appliesGPU) {
            const value = batchGPUModel.trim();
            if (value) annotations["rlark.io/gpu-model"] = value;
            else delete annotations["rlark.io/gpu-model"];
            delete labels["rlark.io/gpu-model"];
            removedLabelKeys.add("rlark.io/gpu-model");
          }
          if (batchFields.has("device") && appliesDevice) {
            const value = batchDeviceModel.trim();
            if (value) annotations["rlark.io/device-model"] = value;
            else delete annotations["rlark.io/device-model"];
            delete labels["rlark.io/device-model"];
            removedLabelKeys.add("rlark.io/device-model");
          }
          const labelPatch: Record<string, string | null> = { ...labels };
          removedLabelKeys.forEach((key) => {
            labelPatch[key] = null;
          });
          const annotationPatch: Record<string, string | null> = {
            ...annotations,
          };
          if (batchFields.has("location") && !batchLocation.trim())
            annotationPatch["rlark.io/city"] = null;
          if (batchFields.has("gpu") && appliesGPU && !batchGPUModel.trim())
            annotationPatch["rlark.io/gpu-model"] = null;
          if (
            batchFields.has("device") &&
            appliesDevice &&
            !batchDeviceModel.trim()
          )
            annotationPatch["rlark.io/device-model"] = null;
          const resp = await fetch(
            `/api/v1/rlinf.io/v1alpha1/nodes/${encodeURIComponent(node.metadata.name)}?namespace=${encodeURIComponent(node.metadata.namespace ?? "")}`,
            {
              method: "PATCH",
              headers: { "Content-Type": "application/merge-patch+json" },
              body: JSON.stringify({
                metadata: { labels: labelPatch, annotations: annotationPatch },
              }),
            },
          );
          if (!resp.ok) {
            throw new Error(
              `${node.metadata.name}: HTTP ${resp.status} ${await resp.text()}`,
            );
          }
          return { node, labels, annotations };
        }),
      ).then((updated) => {
        const updates = new Map(
          updated.map(({ node, labels, annotations }) => [
            `${node.metadata.namespace ?? ""}/${node.metadata.name}`,
            { labels, annotations },
          ]),
        );
        setNodes((current) =>
          current.map((node) => {
            const metadata = updates.get(
              `${node.metadata.namespace ?? ""}/${node.metadata.name}`,
            );
            return metadata
              ? {
                  ...node,
                  metadata: { ...node.metadata, ...metadata },
                }
              : node;
          }),
        );
      });
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
      if (values.length > 0 && values.every((value) => value === values[0]))
        return values[0];
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
        ? "封锁"
        : "cordon"
      : zh
        ? "解封"
        : "uncordon";
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
          const resp = await fetch(
            `/api/v1/rlinf.io/v1alpha1/nodes/${encodeURIComponent(node.metadata.name)}?namespace=${encodeURIComponent(node.metadata.namespace ?? "")}`,
            {
              method: "PATCH",
              headers: { "Content-Type": "application/merge-patch+json" },
              body: JSON.stringify({ spec: { unschedulable } }),
            },
          );
          if (!resp.ok) {
            throw new Error(
              `${node.metadata.name}: HTTP ${resp.status} ${await resp.text()}`,
            );
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
    onNewLabelKey: setNewLabelKey,
    onNewLabelValue: setNewLabelValue,
  };

  const selectedNodeObj =
    nodes.find((n) => n.metadata.name === selectedNode) ?? null;

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
    <div className="page-content resource-page">
      <div className="section-heading">
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
        <div className="admin-node-list-actions">
          <button
            className="secondary-button danger"
            disabled={selectedNodeKeys.size === 0 || batchScheduling}
            onClick={() => updateSelectedScheduling(true)}
          >
            <Ban size={15} />
            {zh ? "批量封锁" : "Cordon"}
          </button>
          <button
            className="secondary-button"
            disabled={selectedNodeKeys.size === 0 || batchScheduling}
            onClick={() => updateSelectedScheduling(false)}
          >
            {zh ? "批量解封" : "Uncordon"}
          </button>
          <button
            className="primary-button"
            disabled={selectedNodeKeys.size === 0}
            onClick={openBatchEditor}
          >
            <Pencil size={15} />
            {zh
              ? `批量设置 (${selectedNodeKeys.size})`
              : `Batch edit (${selectedNodeKeys.size})`}
          </button>
          <button className="secondary-button" onClick={() => fetchNodes()}>
            <RefreshCw size={16} />
            {c.common.refresh}
          </button>
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
                  ? "先启用需要修改的字段；未启用字段保持原值。清空已启用字段可删除原值。"
                  : "Enable only fields to change. Disabled fields stay unchanged; clear an enabled field to remove it."}
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
                            .map((category) =>
                              category === "robot" && zh
                                ? "具身节点"
                                : categoryLabels[category][zh ? "zh" : "en"],
                            )
                            .join("、")
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
                    aria-label={zh ? "节点分类（可多选）" : "Node categories"}
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
                              ? "具身节点"
                              : categoryLabels[category].zh
                            : categoryLabels[category].en}
                        </button>
                      ),
                    )}
                  </div>
                  <small className="admin-node-batch-hint">
                    {zh
                      ? "可多选。例如 GPU 服务器选择“云算力”，机器人本体可同时选择“端算力”和“具身节点”。"
                      : "Multiple selections are allowed. For example, choose Cloud for GPU servers; a robot may be both Edge and Embodied."}
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
          onRefresh={() => fetchNodes()}
          onSelectNode={(name) => onNavigate(name)}
          onToggleScheduling={toggleCordon}
          updatingNode={cordoningNode}
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
