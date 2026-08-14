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
import { categoryLabels, getNodeCategory } from "../utils/nodes";
import { useAutoRefresh } from "../hooks";
import { MetricCard, StatusBadge } from "../components/shared";
import { NodeResourceBrowser } from "../components/NodeResourceBrowser";
import { ClusterDetailReal, NodeDetailReal } from "../pages/Clusters";

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
      counts[getNodeCategory(n)]++;
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
  const phase = (node.status?.phase ?? "Offline") as Phase;
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

      <NodeDetailReal node={node} copy={c} hideLabels />

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
  const [cordoning, setCordoning] = useState(false);

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
    setLabelDraft({ ...(node.metadata.labels ?? {}) });
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
      const patch = { metadata: { labels: labelDraft } };
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
            ? { ...n, metadata: { ...n.metadata, labels: { ...labelDraft } } }
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
    setCordoning(true);
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
      setCordoning(false);
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
    cordoning,
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
        <button className="secondary-button" onClick={() => fetchNodes()}>
          <RefreshCw size={16} />
          {c.common.refresh}
        </button>
      </div>

      {error && (
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}

      {loading ? (
        <p className="muted">{zh ? "加载中..." : "Loading..."}</p>
      ) : (
        <NodeResourceBrowser
          nodes={nodes}
          copy={c}
          onRefresh={() => fetchNodes()}
          onSelectNode={(name) => onNavigate(name)}
        />
      )}
    </div>
  );
}
