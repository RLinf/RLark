import {
  Fragment,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from "react";
import {
  Activity,
  Check,
  ChevronRight,
  CloudCog,
  Copy as CopyIcon,
  Cpu,
  HardDrive,
  KeyRound,
  MemoryStick,
  Network,
  Package,
  RefreshCw,
  Server,
  Settings,
  TerminalSquare,
  Download,
} from "lucide-react";
import type { Phase } from "../data";
import type { Copy } from "../i18n";
import type { CRDNode } from "../types";
import { useAutoRefresh } from "../hooks";
import {
  categoryLabels,
  formatResourceQuantity,
  getGPUResourceKey,
  getNodeDeviceModel,
  getNodeCategories,
  getNodeCategory,
  getNodeGPUModel,
  getNodeLocation,
  getNodeResourceSummary,
  isBusinessWorkerNode,
  parseResourceQuantity,
} from "../utils/nodes";
import {
  compareSortValues,
  MetricCard,
  SortButton,
  StatusBadge,
  type SortDirection,
} from "../components/shared";
import { NodeResourceBrowser } from "../components/NodeResourceBrowser";
import { formatChinaDateTime } from "../utils/time";

type NodeWorker = {
  id: string;
  crName: string;
  name: string;
  job: string;
  role: string;
  node: string;
  ip: string;
  phase: string;
  requests: Record<string, string>;
};

function addRequestedResource(
  totals: Record<string, number>,
  key: string,
  raw?: string,
) {
  const value = parseResourceQuantity(key, raw);
  if (value !== null) totals[key] = (totals[key] ?? 0) + value;
}

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
  const [nodeWorkloads, setNodeWorkloads] = useState<
    Record<string, { jobs: string[]; workers: number }>
  >({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const resourceView = initialView ?? "clusters";
  const [selectedClusterNs, setSelectedClusterNs] = useState<string | null>(
    null,
  );

  const fetchNodes = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    try {
      const [nodesResponse, tasksResponse, podsResponse] = await Promise.all([
        fetch("/api/v1/rlinf.io/v1alpha1/nodes"),
        fetch("/api/v1/rlinf.io/v1alpha1/tasks"),
        fetch("/api/v1/rlinf.io/v1alpha1/pods"),
      ]);
      if (!nodesResponse.ok || !tasksResponse.ok || !podsResponse.ok) {
        throw new Error(
          `HTTP ${nodesResponse.status}/${tasksResponse.status}/${podsResponse.status}`,
        );
      }
      const [nodesData, tasksData, podsData] = await Promise.all([
        nodesResponse.json(),
        tasksResponse.json(),
        podsResponse.json(),
      ]);
      setRealNodes(nodesData.items ?? []);
      const taskJobs = new Map<string, string>(
        (tasksData.items ?? []).map(
          (task: {
            metadata?: { name?: string; labels?: Record<string, string> };
          }) => [
            task.metadata?.name ?? "",
            task.metadata?.labels?.["rlinf.io/job"] ?? "",
          ],
        ),
      );
      const workloadMap = new Map<
        string,
        { jobs: Set<string>; workers: number }
      >();
      for (const pod of podsData.items ?? []) {
        if (pod.status?.phase !== "Running" || !pod.status?.node) continue;
        const nodeName = pod.status.node as string;
        const current = workloadMap.get(nodeName) ?? {
          jobs: new Set<string>(),
          workers: 0,
        };
        const jobName = taskJobs.get(pod.spec?.taskName ?? "");
        if (jobName) current.jobs.add(jobName);
        current.workers += 1;
        workloadMap.set(nodeName, current);
      }
      setNodeWorkloads(
        Object.fromEntries(
          [...workloadMap].map(([name, workload]) => [
            name,
            { jobs: [...workload.jobs].sort(), workers: workload.workers },
          ]),
        ),
      );
    } catch (e) {
      setRealNodes([]);
      setNodeWorkloads({});
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchNodes, 10000);

  const workerNodes = useMemo(
    () => realNodes.filter(isBusinessWorkerNode),
    [realNodes],
  );

  const clustersList = useMemo(() => {
    const map = new Map<string, CRDNode[]>();
    for (const n of workerNodes) {
      const ns = n.metadata.namespace ?? "default";
      if (!map.has(ns)) map.set(ns, []);
      map.get(ns)!.push(n);
    }
    return Array.from(map.entries()).sort((a, b) => a[0].localeCompare(b[0]));
  }, [workerNodes]);

  const selectedCluster =
    clustersList.find(([ns]) => ns === selectedClusterNs) ?? clustersList[0];
  const selectedClusterNodes = selectedCluster?.[1] ?? [];

  const onlineClusters = clustersList.filter(([, nsNodes]) =>
    nsNodes.some((n) => n.status?.phase === "Online"),
  ).length;
  const totalNodes = workerNodes.length;

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
    workerNodes.find((n) => n.metadata.name === selectedNodeName) ?? null;

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
            note={`${workerNodes.filter((n) => n.status?.phase === "Online").length} ${c.status.Online}`}
          />
          <MetricCard
            icon={Activity}
            tone="orange"
            label={zh ? "在线率" : "Online Rate"}
            value={`${totalNodes > 0 ? Math.round((workerNodes.filter((n) => n.status?.phase === "Online").length / totalNodes) * 100) : 0}%`}
            note={`${workerNodes.filter((n) => n.status?.phase === "Online").length}/${totalNodes} ${zh ? "在线" : "online"}`}
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
            nodes={workerNodes}
            nodeWorkloads={nodeWorkloads}
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

// formatBytes converts a byte count into a human-readable string (B/KB/MB/GB).
function formatBytes(bytes: number): string {
  if (!bytes || bytes < 0) return "0B";
  const GB = 1024 * 1024 * 1024;
  const MB = 1024 * 1024;
  const KB = 1024;
  if (bytes >= GB) return (bytes / GB).toFixed(1) + "GB";
  if (bytes >= MB) return (bytes / MB).toFixed(1) + "MB";
  if (bytes >= KB) return (bytes / KB).toFixed(1) + "KB";
  return bytes + "B";
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
                    {resource.lines.length ? (
                      resource.lines.map((line) => (
                        <small key={line.key} className="cluster-resource-line">
                          {line.primary}
                        </small>
                      ))
                    ) : (
                      <small>{resource.primary}</small>
                    )}
                  </td>
                  <td>
                    {resource.lines.length ? (
                      resource.lines.map((line) => (
                        <small key={line.key} className="cluster-resource-line">
                          {line.secondary}
                        </small>
                      ))
                    ) : (
                      <small>—</small>
                    )}
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
  onTaskNavigate,
}: {
  node: CRDNode;
  copy: Copy;
  onTaskNavigate?: (name: string) => void;
}) {
  const zh = c.nav.overview === "总览";
  const [nodeWorkers, setNodeWorkers] = useState<NodeWorker[]>([]);
  useAutoRefresh(
    async () => {
      const [tasksResponse, podsResponse] = await Promise.all([
        fetch("/api/v1/rlinf.io/v1alpha1/tasks"),
        fetch("/api/v1/rlinf.io/v1alpha1/pods"),
      ]);
      if (!tasksResponse.ok || !podsResponse.ok) {
        throw new Error(`HTTP ${tasksResponse.status}/${podsResponse.status}`);
      }
      const [tasksBody, podsBody] = await Promise.all([
        tasksResponse.json(),
        podsResponse.json(),
      ]);
      const tasks = new Map<string, Record<string, unknown>>(
        (tasksBody.items ?? []).map((task: Record<string, unknown>) => [
          (task as { metadata?: { name?: string } }).metadata?.name ?? "",
          task,
        ]),
      );
      const workers: NodeWorker[] = (podsBody.items ?? [])
        .filter(
          (pod: { status?: { node?: string } }) =>
            pod.status?.node === node.metadata.name,
        )
        .map(
          (pod: {
            metadata?: { name?: string; namespace?: string };
            spec?: { taskName?: string; podName?: string };
            status?: { phase?: string; ip?: string };
          }) => {
            const task = tasks.get(pod.spec?.taskName ?? "") as
              | {
                  metadata?: { labels?: Record<string, string> };
                  spec?: {
                    role?: string;
                    kubernetes?: {
                      workload?: {
                        template?: {
                          spec?: {
                            containers?: Array<{
                              resources?: {
                                requests?: Record<string, string>;
                              };
                            }>;
                          };
                        };
                      };
                    };
                  };
                }
              | undefined;
            const job = task?.metadata?.labels?.["rlinf.io/job"];
            if (!job) return null;
            return {
              id: `${pod.metadata?.namespace ?? ""}/${pod.metadata?.name ?? ""}`,
              crName: pod.metadata?.name ?? "",
              name: pod.spec?.podName || pod.metadata?.name || "—",
              job,
              role: task?.spec?.role ?? "—",
              node: node.metadata.name,
              ip: pod.status?.ip ?? "—",
              phase: pod.status?.phase ?? "Pending",
              requests:
                task?.spec?.kubernetes?.workload?.template?.spec
                  ?.containers?.[0]?.resources?.requests ?? {},
            };
          },
        )
        .filter(
          (worker: NodeWorker | null): worker is NodeWorker => worker !== null,
        );
      setNodeWorkers(workers.sort((a, b) => a.name.localeCompare(b.name)));
    },
    10000,
    [node.metadata.name],
  );
  const phase = (node.status?.phase ?? "Offline") as Phase;
  const labels = node.metadata.labels ?? {};
  const addresses = node.status?.addresses ?? [];
  const internalAddress =
    addresses.find((address) => address.type === "InternalIP")?.address ??
    addresses[0]?.address ??
    "—";
  const categories = getNodeCategories(node);
  const capacity = node.status?.capacity ?? {};
  const allocatable = node.status?.allocatable ?? {};
  const requestedFallback = useMemo(() => {
    const totals: Record<string, number> = {};
    nodeWorkers.forEach((worker) =>
      Object.entries(worker.requests).forEach(([key, value]) =>
        addRequestedResource(totals, key, value),
      ),
    );
    return Object.fromEntries(
      Object.entries(totals).map(([key, value]) => [key, String(value)]),
    );
  }, [nodeWorkers]);
  const reportedUsed = node.status?.used ?? {};
  const used =
    Object.keys(reportedUsed).length > 0 ? reportedUsed : requestedFallback;
  const pullProgress = node.status?.pullProgress ?? [];
  const getPercent = (key: string) => {
    const rawUsed = used[key];
    if (!rawUsed && (capacity[key] ?? allocatable[key])) return 0;
    if (rawUsed?.endsWith("%"))
      return Math.min(100, Math.max(0, Number.parseFloat(rawUsed)));
    const usedNumber = parseResourceQuantity(key, rawUsed);
    const capacityNumber = parseResourceQuantity(
      key,
      capacity[key] ?? allocatable[key],
    );
    return usedNumber !== null && capacityNumber !== null && capacityNumber > 0
      ? Math.min(
          100,
          Math.max(0, Math.round((usedNumber / capacityNumber) * 100)),
        )
      : null;
  };
  const formatUsedResource = (key: string) => {
    const raw = used[key];
    if (!raw && (capacity[key] ?? allocatable[key])) {
      return formatResourceQuantity(key, "0");
    }
    if (raw?.endsWith("%")) {
      const capacityValue = parseResourceQuantity(
        key,
        capacity[key] ?? allocatable[key],
      );
      const percent = Number.parseFloat(raw);
      if (capacityValue !== null && Number.isFinite(percent)) {
        return formatResourceQuantity(
          key,
          String((capacityValue * percent) / 100),
        );
      }
    }
    return formatResourceQuantity(key, raw);
  };
  const formatAvailableResource = (key: string) => {
    const available = parseResourceQuantity(
      key,
      allocatable[key] ?? capacity[key],
    );
    const requested = parseResourceQuantity(key, used[key]);
    if (available === null) return "—";
    return formatResourceQuantity(
      key,
      String(Math.max(0, available - (requested ?? 0))),
    );
  };
  const gpuResourceKey = getGPUResourceKey(node);
  const resourceKeys = Array.from(
    new Set([
      ...Object.keys(capacity),
      ...Object.keys(allocatable),
      ...Object.keys(used),
    ]),
  );
  const hasGPUResource = resourceKeys.includes(gpuResourceKey);
  const deviceResourceKeys = resourceKeys.filter(
    (key) => key === "rlinf.io/device" || key.startsWith("rlinf.io/device-"),
  );
  const deviceLabel = (key: string) => {
    const model = key.replace(/^rlinf\.io\/device-?/, "");
    if (!model) return zh ? "端侧设备" : "Edge device";
    const reportedModel = labels["rlark.io/model"];
    if (deviceResourceKeys.length === 1 && reportedModel) return reportedModel;
    return model
      .split("-")
      .filter(Boolean)
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(" ");
  };
  const hasDiskPressure = node.status?.diskPressure === true;
  const diskPressureKnown = node.status?.diskPressure !== undefined;
  const resourceItems = [
    { key: "cpu", label: "CPU", icon: Cpu, available: true },
    {
      key: "memory",
      label: zh ? "内存" : "Memory",
      icon: MemoryStick,
      available: true,
    },
    {
      key: "ephemeral-storage",
      label: zh ? "磁盘" : "Storage",
      icon: HardDrive,
      available: Boolean(
        capacity["ephemeral-storage"] ?? allocatable["ephemeral-storage"],
      ),
    },
    {
      key: gpuResourceKey,
      label: "GPU",
      icon: Activity,
      available: hasGPUResource,
    },
    ...deviceResourceKeys.map((key) => ({
      key,
      label: deviceLabel(key),
      icon: Package,
      available: true,
    })),
    ...(deviceResourceKeys.length === 0
      ? [
          {
            key: "rlinf.io/device",
            label: zh ? "端侧设备" : "Edge device",
            icon: Package,
            available: false,
          },
        ]
      : []),
  ];
  const created = formatChinaDateTime(node.metadata.creationTimestamp);
  return (
    <>
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
            <strong>
              {categories
                .map((value) =>
                  zh ? categoryLabels[value].zh : categoryLabels[value].en,
                )
                .join(" / ")}
            </strong>
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
                    {zh
                      ? "资源占用按运行 Worker 申请统计，压力来自节点状态"
                      : "Usage uses Worker requests; pressure uses node status"}
                  </small>
                </div>
              </div>
              <div className="node-capacity-grid">
                {resourceItems.map(({ key, label, icon: Icon, available }) => {
                  if (key === "ephemeral-storage") {
                    return (
                      <div
                        className={`node-capacity-card node-pressure-card ${
                          hasDiskPressure ? "is-warning" : ""
                        }`}
                        key={key}
                      >
                        <div className="node-capacity-title">
                          <span>
                            <Icon size={16} />
                          </span>
                          <strong>{zh ? "磁盘压力" : "Disk pressure"}</strong>
                          <b>
                            {diskPressureKnown
                              ? hasDiskPressure
                                ? zh
                                  ? "存在"
                                  : "Detected"
                                : zh
                                  ? "正常"
                                  : "Normal"
                              : zh
                                ? "未知"
                                : "Unknown"}
                          </b>
                        </div>
                        <div className="node-capacity-track">
                          <i
                            style={{ width: hasDiskPressure ? "100%" : "0%" }}
                          />
                        </div>
                        <div className="node-capacity-amounts">
                          <span>
                            <em>{zh ? "状态" : "Status"}</em>
                            <strong>
                              {diskPressureKnown
                                ? hasDiskPressure
                                  ? zh
                                    ? "节点存在磁盘压力"
                                    : "Node has disk pressure"
                                  : zh
                                    ? "节点无磁盘压力"
                                    : "No disk pressure"
                                : zh
                                  ? "未上报"
                                  : "Not reported"}
                            </strong>
                          </span>
                          <span>
                            <em>{zh ? "可分配容量" : "Allocatable"}</em>
                            <strong>
                              {available
                                ? formatResourceQuantity(
                                    key,
                                    allocatable[key] ?? capacity[key],
                                  )
                                : "—"}
                            </strong>
                          </span>
                          <small>
                            {zh ? "来自节点健康状态" : "From node health"}
                          </small>
                        </div>
                      </div>
                    );
                  }
                  const percent = available ? getPercent(key) : null;
                  return (
                    <div className="node-capacity-card" key={key}>
                      <div className="node-capacity-title">
                        <span>
                          <Icon size={16} />
                        </span>
                        <strong>{label}</strong>
                        <b>
                          {available
                            ? percent === null
                              ? "—"
                              : `${percent}%`
                            : zh
                              ? "无"
                              : "None"}
                        </b>
                      </div>
                      <div className="node-capacity-track">
                        <i style={{ width: `${percent ?? 0}%` }} />
                      </div>
                      <div className="node-capacity-amounts">
                        <span>
                          <em>{zh ? "已请求" : "Requested"}</em>
                          <strong>
                            {available
                              ? formatUsedResource(key)
                              : zh
                                ? "无"
                                : "None"}
                          </strong>
                        </span>
                        <span>
                          <em>{zh ? "总量" : "Total"}</em>
                          <strong>
                            {available
                              ? formatResourceQuantity(
                                  key,
                                  capacity[key] ?? allocatable[key],
                                )
                              : zh
                                ? "无"
                                : "None"}
                          </strong>
                        </span>
                        <small>
                          {zh ? "剩余" : "Available"}{" "}
                          {available
                            ? formatAvailableResource(key)
                            : zh
                              ? "无"
                              : "None"}
                        </small>
                      </div>
                    </div>
                  );
                })}
              </div>
            </section>

            {pullProgress.length > 0 && (
              <section className="node-insight-section">
                <div className="node-insight-section-head">
                  <div>
                    <span>
                      <Download size={13} style={{ verticalAlign: -2 }} />{" "}
                      {zh ? "镜像拉取进度" : "Image Pull Progress"}
                    </span>
                    <small>
                      {zh
                        ? "节点正在拉取的镜像及其进度"
                        : "Images currently being pulled on this node"}
                    </small>
                  </div>
                </div>
                <div className="node-pull-progress-list">
                  {pullProgress.map((p, i) => {
                    const pct =
                      p.total > 0
                        ? Math.min(
                            100,
                            Math.round((p.downloaded / p.total) * 100),
                          )
                        : 0;
                    const isPulling = p.status === "pulling";
                    return (
                      <div className="node-pull-progress-entry" key={i}>
                        <div className="node-pull-progress-header">
                          <code className="node-pull-image" title={p.image}>
                            {p.image}
                          </code>
                          <span className={`node-pull-status chip-${p.status}`}>
                            {p.message || p.status}
                          </span>
                        </div>
                        {isPulling && p.total > 0 && (
                          <div className="node-pull-progress-bar">
                            <i style={{ width: `${pct}%` }} />
                            <b>{pct}%</b>
                          </div>
                        )}
                        {(p.total > 0 || p.speed > 0) && (
                          <div className="node-pull-progress-meta">
                            {p.total > 0 && (
                              <span>
                                {formatBytes(p.downloaded)} /{" "}
                                {formatBytes(p.total)}
                              </span>
                            )}
                            {p.speed > 0 && (
                              <span>{formatBytes(p.speed)}/s</span>
                            )}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </section>
            )}
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
                  <dt>{zh ? "物理位置" : "Location"}</dt>
                  <dd>
                    {getNodeLocation(node) || (zh ? "未标注" : "Unlabeled")}
                  </dd>
                </div>
                <div>
                  <dt>{zh ? "节点分类" : "Category"}</dt>
                  <dd>
                    {categories
                      .map((value) =>
                        zh
                          ? categoryLabels[value].zh
                          : categoryLabels[value].en,
                      )
                      .join(" / ")}
                  </dd>
                </div>
                <div>
                  <dt>{zh ? "GPU 型号" : "GPU model"}</dt>
                  <dd>
                    {hasGPUResource
                      ? getNodeGPUModel(node) || (zh ? "未标注" : "Unlabeled")
                      : zh
                        ? "无 GPU"
                        : "No GPU"}
                  </dd>
                </div>
                <div>
                  <dt>{zh ? "端设备型号" : "Device model"}</dt>
                  <dd>
                    {deviceResourceKeys.length > 0
                      ? getNodeDeviceModel(node) ||
                        deviceResourceKeys.map(deviceLabel).join("、")
                      : zh
                        ? "无端设备"
                        : "No device"}
                  </dd>
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
          </aside>
        </div>
      </div>
      <section className="node-worker-panel">
        <div className="worker-panel-head">
          <div>
            <span className="eyebrow">
              {zh ? "运行实例" : "Runtime instances"}
            </span>
            <h3>{zh ? "Worker 列表" : "Worker list"}</h3>
          </div>
          <span className="worker-total-count">
            {nodeWorkers.length} {zh ? "个 Worker" : "workers"}
          </span>
        </div>
        <NodeWorkerTable
          workers={nodeWorkers}
          copy={c}
          onTaskNavigate={onTaskNavigate}
        />
      </section>
    </>
  );
}

function NodeWorkerTable({
  workers,
  copy: c,
  onTaskNavigate,
}: {
  workers: NodeWorker[];
  copy: Copy;
  onTaskNavigate?: (name: string) => void;
}) {
  const zh = c.nav.overview === "总览";
  const [expanded, setExpanded] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [sort, setSort] = useState<{
    key: "name" | "job" | "role" | "ip" | "requests" | "phase";
    direction: SortDirection;
  }>({ key: "name", direction: "asc" });
  const tableRef = useRef<HTMLDivElement>(null);
  const dragState = useRef({ active: false, x: 0, scrollLeft: 0 });
  const [dragging, setDragging] = useState(false);
  const [sshConfig, setSSHConfig] = useState<{
    sshJumpHost?: string;
    sshJumpPort?: string;
  }>({});
  useEffect(() => {
    fetch("/api/v1/system-config")
      .then((response) => (response.ok ? response.json() : {}))
      .then(setSSHConfig)
      .catch(() => setSSHConfig({}));
  }, []);
  const requestTextFor = useCallback(
    (worker: NodeWorker) =>
      Object.entries(worker.requests)
        .filter(
          ([key, value]) =>
            Number.parseFloat(value) > 0 &&
            (/(^|[./-])gpu($|[./-])/i.test(key) ||
              key === "rlinf.io/device" ||
              key.startsWith("rlinf.io/device-")),
        )
        .map(([key, value]) => {
          if (/(^|[./-])gpu($|[./-])/i.test(key)) return `GPU ${value}`;
          const model = key.replace(/^rlinf\.io\/device-?/, "");
          return `${model || (zh ? "设备" : "Device")} ${value}`;
        })
        .filter(Boolean)
        .join(" · "),
    [zh],
  );
  const sortedWorkers = useMemo(
    () =>
      [...workers].sort((a, b) => {
        const value = (worker: NodeWorker) =>
          sort.key === "requests" ? requestTextFor(worker) : worker[sort.key];
        return compareSortValues(value(a), value(b), sort.direction);
      }),
    [workers, sort, requestTextFor],
  );
  const toggleSort = (key: typeof sort.key) =>
    setSort((current) => ({
      key,
      direction:
        current.key === key && current.direction === "asc" ? "desc" : "asc",
    }));
  const handlePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0 || !tableRef.current) return;
    const target = event.target as HTMLElement;
    if (target.closest("button, a, [role='button']")) return;
    dragState.current = {
      active: true,
      x: event.clientX,
      scrollLeft: tableRef.current.scrollLeft,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
    setDragging(true);
  };
  const handlePointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!dragState.current.active || !tableRef.current) return;
    tableRef.current.scrollLeft =
      dragState.current.scrollLeft - (event.clientX - dragState.current.x);
  };
  const stopDragging = () => {
    dragState.current.active = false;
    setDragging(false);
  };
  if (workers.length === 0) {
    return (
      <div className="node-worker-empty">
        {zh ? "当前没有运行中的 Worker" : "No running workers"}
      </div>
    );
  }
  const sshUser = sessionStorage.getItem("rlark-user-name") || "<ssh-user>";
  return (
    <div
      ref={tableRef}
      className={`node-worker-table-wrap worker-table worker-console-table worker-table-scroll${dragging ? " dragging" : ""}`}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={stopDragging}
      onPointerCancel={stopDragging}
    >
      <table className="node-worker-table">
        <thead>
          <tr>
            {(
              [
                ["name", zh ? "Worker 名称" : "Worker"],
                ["job", "Job"],
                ["role", zh ? "角色" : "Role"],
                ["ip", "IP"],
                ["requests", zh ? "GPU / 端设备" : "GPU / edge devices"],
                ["phase", zh ? "状态" : "Status"],
              ] as const
            ).map(([key, label]) => (
              <th key={key}>
                <SortButton
                  label={label}
                  active={sort.key === key}
                  direction={sort.direction}
                  onClick={() => toggleSort(key)}
                />
              </th>
            ))}
            <th>{zh ? "操作" : "Actions"}</th>
          </tr>
        </thead>
        <tbody>
          {sortedWorkers.map((worker) => {
            const requestText = requestTextFor(worker);
            const sshJump = sshConfig.sshJumpHost
              ? `${sshUser}@${sshConfig.sshJumpHost}${sshConfig.sshJumpPort ? `:${sshConfig.sshJumpPort}` : ""}`
              : "";
            const sshCommand = sshJump
              ? `ssh -J ${sshJump} root@${worker.name}`
              : "";
            const isExpanded = expanded === worker.id;
            return (
              <Fragment key={worker.id}>
                <tr>
                  <td>
                    <strong>{worker.name}</strong>
                  </td>
                  <td>
                    <button
                      className="plain-button node-worker-job-link"
                      disabled={worker.job === "—" || !onTaskNavigate}
                      onClick={() => onTaskNavigate?.(worker.job)}
                    >
                      {worker.job}
                    </button>
                  </td>
                  <td>{worker.role}</td>
                  <td>
                    <code>{worker.ip}</code>
                  </td>
                  <td>{requestText || (zh ? "未使用" : "Not used")}</td>
                  <td>
                    <StatusBadge phase={worker.phase as Phase} copy={c} />
                  </td>
                  <td>
                    <div className="worker-table-actions">
                      <button
                        className="icon-button worker-copy-ssh-icon"
                        disabled={!sshCommand}
                        title={zh ? "复制 SSH 命令" : "Copy SSH command"}
                        onClick={async () => {
                          if (!sshCommand) return;
                          await navigator.clipboard.writeText(sshCommand);
                          setCopied(worker.id);
                          window.setTimeout(() => setCopied(null), 1500);
                        }}
                      >
                        {copied === worker.id ? (
                          <Check size={15} />
                        ) : (
                          <KeyRound size={15} />
                        )}
                      </button>
                      <button
                        className="icon-button worker-terminal-icon"
                        disabled={!worker.crName}
                        title={zh ? "打开 WebTerminal" : "Open WebTerminal"}
                        onClick={() => {
                          if (!worker.crName) return;
                          const params = new URLSearchParams({
                            job: worker.job,
                            worker: worker.crName,
                            status: worker.phase,
                          });
                          const terminalWindow = window.open(
                            `/terminal?${params.toString()}`,
                            "_blank",
                          );
                          if (terminalWindow) terminalWindow.opener = null;
                        }}
                      >
                        <TerminalSquare size={15} />
                      </button>
                      <button
                        className="icon-button"
                        title={zh ? "查看详情" : "View details"}
                        onClick={() =>
                          setExpanded(isExpanded ? null : worker.id)
                        }
                      >
                        <ChevronRight
                          size={15}
                          style={{
                            transform: isExpanded ? "rotate(90deg)" : "none",
                          }}
                        />
                      </button>
                    </div>
                  </td>
                </tr>
                {isExpanded && (
                  <tr className="worker-expanded-row node-worker-expanded">
                    <td colSpan={7}>
                      <div className="worker-detail-drawer">
                        <div className="worker-detail-head">
                          <div>
                            <span className="eyebrow">
                              {zh ? "Worker 详情" : "Worker details"}
                            </span>
                            <strong>{worker.name}</strong>
                          </div>
                          {sshCommand && (
                            <div className="worker-ssh-inline">
                              <KeyRound size={15} />
                              <code title={sshCommand}>{sshCommand}</code>
                              <button
                                className="icon-button"
                                onClick={async () => {
                                  await navigator.clipboard.writeText(
                                    sshCommand,
                                  );
                                  setCopied(worker.id);
                                  window.setTimeout(
                                    () => setCopied(null),
                                    1500,
                                  );
                                }}
                                aria-label={
                                  zh ? "复制 SSH 地址" : "Copy SSH address"
                                }
                              >
                                {copied === worker.id ? (
                                  <Check size={15} />
                                ) : (
                                  <CopyIcon size={15} />
                                )}
                              </button>
                            </div>
                          )}
                        </div>
                        <div className="worker-detail-grid">
                          <div>
                            <span>{zh ? "角色" : "Role"}</span>
                            <strong>{worker.role}</strong>
                          </div>
                          <div>
                            <span>{zh ? "节点" : "Node"}</span>
                            <strong>{worker.node}</strong>
                          </div>
                          <div>
                            <span>{zh ? "申请 CPU" : "CPU request"}</span>
                            <strong>
                              {worker.requests.cpu ||
                                (zh ? "未申请" : "Not requested")}
                            </strong>
                          </div>
                          <div>
                            <span>{zh ? "申请内存" : "Memory request"}</span>
                            <strong>
                              {worker.requests.memory ||
                                (zh ? "未申请" : "Not requested")}
                            </strong>
                          </div>
                          <div>
                            <span>
                              {zh ? "GPU / 端设备" : "GPU / edge devices"}
                            </span>
                            <strong>
                              {requestText || (zh ? "未使用" : "Not used")}
                            </strong>
                          </div>
                        </div>
                        <div className="pod-subtable">
                          <table>
                            <thead>
                              <tr>
                                <th>{zh ? "实例名称" : "Worker name"}</th>
                                <th>Job</th>
                                <th>{zh ? "实例 IP" : "Worker IP"}</th>
                                <th>{zh ? "状态" : "Status"}</th>
                              </tr>
                            </thead>
                            <tbody>
                              <tr>
                                <td>
                                  <code className="inline-code">
                                    {worker.name}
                                  </code>
                                </td>
                                <td>{worker.job}</td>
                                <td>{worker.ip}</td>
                                <td>
                                  <StatusBadge
                                    phase={worker.phase as Phase}
                                    copy={c}
                                  />
                                </td>
                              </tr>
                            </tbody>
                          </table>
                        </div>
                      </div>
                    </td>
                  </tr>
                )}
              </Fragment>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
