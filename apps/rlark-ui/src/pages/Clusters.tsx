import { useMemo, useState } from "react";
import {
  Activity,
  ArrowUpRight,
  CloudCog,
  Cpu,
  HardDrive,
  MemoryStick,
  Network,
  Package,
  RefreshCw,
  Server,
  Settings,
} from "lucide-react";
import type { Phase } from "../data";
import type { Copy } from "../i18n";
import type { CRDNode } from "../types";
import { useAutoRefresh } from "../hooks";
import {
  categoryLabels,
  getNodeCategory,
  getNodeLocation,
  getNodeResourceSummary,
} from "../utils/nodes";
import { MetricCard, StatusBadge } from "../components/shared";
import { NodeResourceBrowser } from "../components/NodeResourceBrowser";
import { formatChinaDateTime } from "../utils/time";

export function ClustersPage({
  copy: c,
  initialView,
  selectedNodeName,
  onNavigate,
  onTaskNavigate,
}: {
  copy: Copy;
  initialView?: "clusters" | "nodes";
  selectedNodeName?: string;
  onNavigate?: (name?: string) => void;
  onTaskNavigate?: (name: string) => void;
}) {
  const zh = c.nav.overview === "总览";
  const searchParams = new URLSearchParams(window.location.search);
  const requestedCategory = searchParams.get("category");
  const initialCategory =
    requestedCategory === "cloud" ||
    requestedCategory === "edge" ||
    requestedCategory === "robot" ||
    requestedCategory === "unknown"
      ? requestedCategory
      : "all";
  const initialQuery = searchParams.get("city") ?? searchParams.get("q") ?? "";
  const [realNodes, setRealNodes] = useState<CRDNode[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [resourceView, setResourceView] = useState<"clusters" | "nodes">(
    initialView ?? "clusters",
  );
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
      setRealNodes(data.items ?? []);
    } catch (e) {
      setRealNodes([]);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchNodes, 10000);

  const clustersList = useMemo(() => {
    const map = new Map<string, CRDNode[]>();
    for (const n of realNodes) {
      const ns = n.metadata.namespace ?? "default";
      if (!map.has(ns)) map.set(ns, []);
      map.get(ns)!.push(n);
    }
    return Array.from(map.entries()).sort((a, b) => a[0].localeCompare(b[0]));
  }, [realNodes]);

  const selectedCluster =
    clustersList.find(([ns]) => ns === selectedClusterNs) ?? clustersList[0];
  const selectedClusterNodes = selectedCluster?.[1] ?? [];

  const onlineClusters = clustersList.filter(([, nsNodes]) =>
    nsNodes.some((n) => n.status?.phase === "Online"),
  ).length;
  const totalNodes = realNodes.length;

  if (loading) {
    return (
      <div className="page-content resource-page cluster-page">
        <div className="section-heading">
          <div>
            <span className="eyebrow">{c.clusters.eyebrow}</span>
            <h2>{c.clusters.title}</h2>
            <p>{c.clusters.desc}</p>
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
            <span className="eyebrow">{c.clusters.eyebrow}</span>
            <h2>{c.clusters.title}</h2>
            <p>{c.clusters.desc}</p>
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

  const selectedNodeObj =
    realNodes.find((n) => n.metadata.name === selectedNodeName) ?? null;

  if (selectedNodeName && selectedNodeObj) {
    return (
      <div className="page-content resource-page node-detail-page">
        <div className="section-heading">
          <div>
            <button
              className="plain-button back-button"
              onClick={() => onNavigate?.()}
            >
              ← {zh ? "返回节点列表" : "Back"}
            </button>
            <span className="eyebrow">
              <Settings size={13} />
              {zh ? "节点详情" : "Node Detail"}
            </span>
          </div>
        </div>
        <NodeDetailReal
          node={selectedNodeObj}
          copy={c}
          onTaskNavigate={onTaskNavigate}
        />
      </div>
    );
  }

  return (
    <div
      className={`page-content resource-page cluster-page${
        resourceView === "clusters" ? " cluster-overview-page" : ""
      }`}
    >
      <div className="section-heading">
        <div>
          <span className="eyebrow">{c.clusters.eyebrow}</span>
          <h2>
            {resourceView === "nodes"
              ? zh
                ? "节点管理"
                : "Node Management"
              : c.clusters.title}
          </h2>
          <p>
            {resourceView === "nodes"
              ? zh
                ? "统一查看和管理各集群中的算力节点与具身设备。"
                : "View and manage compute nodes and embodied devices across clusters."
              : c.clusters.desc}
          </p>
        </div>
        {resourceView === "clusters" && (
          <button className="secondary-button" onClick={() => fetchNodes()}>
            <RefreshCw size={16} />
            {c.common.refresh}
          </button>
        )}
      </div>
      {resourceView === "clusters" && (
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
            note={`${realNodes.filter((n) => n.status?.phase === "Online").length} ${c.status.Online}`}
          />
          <MetricCard
            icon={Activity}
            tone="orange"
            label={zh ? "在线率" : "Online Rate"}
            value={`${totalNodes > 0 ? Math.round((realNodes.filter((n) => n.status?.phase === "Online").length / totalNodes) * 100) : 0}%`}
            note={`${realNodes.filter((n) => n.status?.phase === "Online").length}/${totalNodes} ${zh ? "在线" : "online"}`}
          />
        </section>
      )}
      {resourceView === "clusters" && (
        <>
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
        </>
      )}
      {resourceView === "nodes" && (
        <section className="nodes-resource-section">
          <NodeResourceBrowser
            nodes={realNodes}
            copy={c}
            initialCategory={initialCategory}
            initialQuery={initialQuery}
            onRefresh={() => fetchNodes()}
            onSelectNode={(name) => onNavigate?.(name)}
          />
        </section>
      )}
    </div>
  );
}

export function ClusterDetailReal({
  namespace,
  nodes: clusterNodes,
  copy: c,
}: {
  namespace: string;
  nodes: CRDNode[];
  copy: Copy;
}) {
  const zh = c.nav.overview === "总览";
  const onlineCount = clusterNodes.filter(
    (n) => n.status?.phase === "Online",
  ).length;
  const phase: Phase = onlineCount > 0 ? "Online" : "Offline";
  const sortedNodes = [...clusterNodes].sort((a, b) =>
    a.metadata.name.localeCompare(b.metadata.name, zh ? "zh-CN" : "en", {
      numeric: true,
    }),
  );
  return (
    <div className="panel selected-cluster-panel">
      <div className="cluster-detail-header">
        <div className="cluster-detail-title">
          <CloudCog size={18} />
          <strong>{namespace}</strong>
          <StatusBadge phase={phase} copy={c} />
        </div>
        <div className="cluster-detail-meta">
          <span>
            {clusterNodes.length} {zh ? "节点" : "nodes"}
          </span>
          <i className="dot" />
          <span>
            {onlineCount} {zh ? "在线" : "online"}
          </span>
          <i className="dot" />
          <span>
            {Array.from(
              new Set(clusterNodes.map((n) => n.spec.agentType ?? "—")),
            ).join(", ") || "—"}
          </span>
        </div>
      </div>
      <div className="cluster-node-table-wrap">
        <table className="cluster-node-table">
          <thead>
            <tr>
              <th>{zh ? "节点名称" : "Name"}</th>
              <th>{zh ? "类型" : "Type"}</th>
              <th>{zh ? "状态" : "Phase"}</th>
              <th>{zh ? "物理位置" : "Location"}</th>
              <th>{zh ? "节点 IP" : "Node IP"}</th>
              <th>{zh ? "资源与空闲" : "Resources"}</th>
              <th>{zh ? "型号" : "Model"}</th>
            </tr>
          </thead>
          <tbody>
            {sortedNodes.map((node) => {
              const category = getNodeCategory(node);
              const categoryInfo = categoryLabels[category];
              const resource = getNodeResourceSummary(node, zh);
              const address =
                node.status?.addresses?.find(
                  (item) => item.type === "InternalIP",
                )?.address ??
                node.status?.addresses?.[0]?.address ??
                "—";
              return (
                <tr key={node.metadata.name}>
                  <td>
                    <strong>{node.metadata.name}</strong>
                  </td>
                  <td>
                    <small>{zh ? categoryInfo.zh : categoryInfo.en}</small>
                  </td>
                  <td>
                    <StatusBadge
                      phase={(node.status?.phase ?? "Offline") as Phase}
                      copy={c}
                    />
                  </td>
                  <td>
                    <small>{getNodeLocation(node) || "—"}</small>
                  </td>
                  <td>
                    <code>{address}</code>
                  </td>
                  <td>
                    <small>{resource.primary}</small>
                  </td>
                  <td>
                    <small>{resource.secondary}</small>
                  </td>
                </tr>
              );
            })}
            {clusterNodes.length === 0 && (
              <tr>
                <td
                  colSpan={7}
                  style={{ textAlign: "center", padding: "16px" }}
                >
                  <small className="muted">
                    {zh ? "暂无节点" : "No nodes"}
                  </small>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export function NodeDetailReal({
  node,
  copy: c,
  hideLabels = false,
  onTaskNavigate,
}: {
  node: CRDNode;
  copy: Copy;
  hideLabels?: boolean;
  onTaskNavigate?: (name: string) => void;
}) {
  const zh = c.nav.overview === "总览";
  const phase = (node.status?.phase ?? "Offline") as Phase;
  const labels = node.metadata.labels ?? {};
  const labelEntries = Object.entries(labels).filter(
    ([k]) => k.startsWith("kubernetes.io/") || k.startsWith("rlark.io/"),
  );
  const addresses = node.status?.addresses ?? [];
  const internalAddress =
    addresses.find((address) => address.type === "InternalIP")?.address ??
    addresses[0]?.address ??
    "—";
  const category = getNodeCategory(node);
  const categoryInfo = categoryLabels[category];
  const taskName =
    labels["rlark.io/embodied-task-name"] ?? labels["rlark.io/task-name"] ?? "";
  const hasTask =
    labels["rlark.io/embodied-task"] === "true" || Boolean(taskName);
  const canOpenTask = Boolean(taskName && onTaskNavigate);
  const capacity = node.status?.capacity ?? {};
  const allocatable = node.status?.allocatable ?? {};
  const used = node.status?.used ?? {};
  const getPercent = (key: string) => {
    const rawUsed = used[key];
    if (rawUsed?.endsWith("%"))
      return Math.min(100, Math.max(0, Number.parseFloat(rawUsed)));
    const usedNumber = Number.parseFloat(rawUsed ?? "");
    const capacityNumber = Number.parseFloat(capacity[key] ?? "");
    return Number.isFinite(usedNumber) &&
      Number.isFinite(capacityNumber) &&
      capacityNumber > 0
      ? Math.min(
          100,
          Math.max(0, Math.round((usedNumber / capacityNumber) * 100)),
        )
      : null;
  };
  const resourceItems = [
    { key: "cpu", label: "CPU", icon: Cpu },
    { key: "memory", label: zh ? "内存" : "Memory", icon: MemoryStick },
    { key: "nvidia.com/gpu", label: "GPU", icon: Activity },
  ];
  const created = formatChinaDateTime(node.metadata.creationTimestamp);
  return (
    <div className="node-resource-detail node-insight-detail">
      <header className="node-insight-hero">
        <div className="node-insight-identity">
          <span className={`node-insight-icon ${phase.toLowerCase()}`}>
            <Server size={22} />
          </span>
          <div>
            <span className="eyebrow">
              {zh ? "节点运行概况" : "Node overview"}
            </span>
            <h3>{node.metadata.name}</h3>
            <p>
              <Network size={13} /> {node.metadata.namespace ?? "default"}
              <span />
              {internalAddress}
            </p>
          </div>
        </div>
        <div className="node-insight-state">
          <StatusBadge phase={phase} copy={c} />
          <span
            className={
              node.spec.unschedulable
                ? "schedule-chip blocked"
                : "schedule-chip"
            }
          >
            {node.spec.unschedulable
              ? zh
                ? "已停止调度"
                : "Unschedulable"
              : zh
                ? "可调度"
                : "Schedulable"}
          </span>
        </div>
      </header>

      {node.status?.reason && (
        <div className="node-health-message">
          <Activity size={15} />
          <span>{node.status.reason}</span>
        </div>
      )}

      <div className="node-insight-facts">
        <div>
          <small>{zh ? "节点类型" : "Node type"}</small>
          <strong>{zh ? categoryInfo.zh : categoryInfo.en}</strong>
        </div>
        <div>
          <small>{zh ? "接入形态" : "Agent type"}</small>
          <strong>{node.spec.agentType ?? "—"}</strong>
        </div>
        <div>
          <small>{zh ? "系统 / 架构" : "System / Arch"}</small>
          <strong>
            {node.status?.nodeInfo?.operatingSystem ?? "—"} ·{" "}
            {node.status?.nodeInfo?.architecture ?? "—"}
          </strong>
        </div>
        <div>
          <small>{zh ? "Agent 版本" : "Agent version"}</small>
          <strong>{node.status?.nodeInfo?.agentVersion ?? "—"}</strong>
        </div>
      </div>

      <div className="node-insight-layout">
        <div className="node-insight-main">
          <section className="node-insight-section">
            <div className="node-insight-section-head">
              <div>
                <span>{zh ? "健康与容量" : "Health & capacity"}</span>
                <small>
                  {zh ? "节点当前资源状态" : "Current node resources"}
                </small>
              </div>
            </div>
            <div className="node-capacity-grid">
              {resourceItems.map(({ key, label, icon: Icon }) => {
                const percent = getPercent(key);
                return (
                  <div className="node-capacity-card" key={key}>
                    <div className="node-capacity-title">
                      <span>
                        <Icon size={16} />
                      </span>
                      <strong>{label}</strong>
                      <b>{percent === null ? "—" : `${percent}%`}</b>
                    </div>
                    <div className="node-capacity-track">
                      <i style={{ width: `${percent ?? 0}%` }} />
                    </div>
                    <small>
                      {zh ? "已用" : "Used"} {used[key] ?? "—"}
                      <span> / </span>
                      {zh ? "可分配" : "Allocatable"}{" "}
                      {allocatable[key] ?? capacity[key] ?? "—"}
                    </small>
                  </div>
                );
              })}
            </div>
          </section>

          <section className="node-insight-section">
            <div className="node-insight-section-head">
              <div>
                <span>{zh ? "具身任务" : "Embodied task"}</span>
                <small>
                  {zh
                    ? "当前与节点关联的任务"
                    : "Task currently associated with this node"}
                </small>
              </div>
            </div>
            <button
              type="button"
              className={`node-task-callout${hasTask ? " active" : ""}${canOpenTask ? " interactive" : ""}`}
              disabled={!canOpenTask}
              onClick={() => taskName && onTaskNavigate?.(taskName)}
              aria-label={
                canOpenTask
                  ? `${zh ? "查看任务" : "View task"} ${taskName}`
                  : undefined
              }
            >
              <span>
                <Package size={19} />
              </span>
              <div>
                <strong>
                  {hasTask
                    ? taskName ||
                      (zh ? "运行中的具身任务" : "Active embodied task")
                    : zh
                      ? "当前没有关联任务"
                      : "No associated task"}
                </strong>
                <small>
                  {hasTask
                    ? zh
                      ? "节点标签报告该任务正在关联运行"
                      : "Reported by node task labels"
                    : phase === "Online" && !node.spec.unschedulable
                      ? zh
                        ? "节点当前可用于新的任务调度"
                        : "The node is available for new workloads"
                      : zh
                        ? "节点当前不可用于任务调度"
                        : "The node is not available for scheduling"}
                </small>
              </div>
              <b>
                {hasTask ? (zh ? "运行中" : "Running") : zh ? "空闲" : "Idle"}
              </b>
              {canOpenTask && (
                <ArrowUpRight size={16} className="node-task-link-icon" />
              )}
            </button>
          </section>
        </div>

        <aside className="node-insight-side">
          <section className="node-insight-section">
            <div className="node-insight-section-head">
              <div>
                <span>{zh ? "基础信息" : "Details"}</span>
              </div>
            </div>
            <dl className="node-info-list">
              <div>
                <dt>{zh ? "所属集群" : "Cluster"}</dt>
                <dd>{node.metadata.namespace ?? "default"}</dd>
              </div>
              <div>
                <dt>{zh ? "内部地址" : "Internal IP"}</dt>
                <dd>
                  <code>{internalAddress}</code>
                </dd>
              </div>
              <div>
                <dt>{zh ? "节点型号" : "Model"}</dt>
                <dd>{labels["rlark.io/model"] ?? "—"}</dd>
              </div>
              <div>
                <dt>{zh ? "内核版本" : "Kernel"}</dt>
                <dd>{node.status?.nodeInfo?.kernelVersion ?? "—"}</dd>
              </div>
              <div>
                <dt>{zh ? "创建时间" : "Created"}</dt>
                <dd>{created}</dd>
              </div>
            </dl>
          </section>

          {!hideLabels && (
            <section className="node-insight-section node-label-section">
              <div className="node-insight-section-head">
                <div>
                  <span>{zh ? "节点标签" : "Labels"}</span>
                  <small>
                    {labelEntries.length} {zh ? "项" : "items"}
                  </small>
                </div>
              </div>
              <div className="label-list">
                {labelEntries.length === 0 ? (
                  <small className="muted">{zh ? "无标签" : "No labels"}</small>
                ) : (
                  labelEntries.map(([key, value]) => (
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
            </section>
          )}
        </aside>
      </div>
    </div>
  );
}
