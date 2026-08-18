import { useMemo, useState } from "react";
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  Boxes,
  CheckCircle2,
  CloudCog,
  Database,
  HardDrive,
  Image,
  Network,
  PackagePlus,
  RefreshCw,
  Server,
  Settings,
  ShieldCheck,
} from "lucide-react";
import type { Copy } from "../i18n";
import type { CRDDomain, CRDJob, CRDNode } from "../types";
import { useAutoRefresh } from "../hooks";
import { formatChinaDateTime } from "../utils/time";

type DashboardData = {
  nodes: CRDNode[];
  jobs: CRDJob[];
  domains: CRDDomain[];
  storageClasses: Array<{ name?: string }>;
};

const emptyData: DashboardData = {
  nodes: [],
  jobs: [],
  domains: [],
  storageClasses: [],
};

async function fetchItems<T>(url: string): Promise<T[]> {
  const response = await fetch(url);
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  const data = await response.json();
  if (Array.isArray(data)) return data;
  if (Array.isArray(data.items)) return data.items;
  if (data.data && typeof data.data === "object") {
    return Object.values(data.data) as T[];
  }
  return [];
}

export function AdminDashboard({
  copy: c,
  onNavigate,
}: {
  copy: Copy;
  onNavigate: (id: string, sub?: string) => void;
}) {
  const zh = c.nav.overview === "总览";
  const [data, setData] = useState<DashboardData>(emptyData);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);

  const fetchDashboard = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    try {
      const [nodes, jobs, domains, storageClasses] = await Promise.all([
        fetchItems<CRDNode>("/api/v1/rlinf.io/v1alpha1/nodes"),
        fetchItems<CRDJob>("/api/v1/rlinf.io/v1alpha1/jobs"),
        fetchItems<CRDDomain>("/api/v1/rlinf.io/v1alpha1/domains"),
        fetchItems<{ name?: string }>("/api/v1/storage/storageclass"),
      ]);
      setData({ nodes, jobs, domains, storageClasses });
      setUpdatedAt(new Date());
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchDashboard, 15000);

  const summary = useMemo(() => {
    const clusterNames = new Set(
      data.nodes.map((node) => node.metadata.namespace).filter(Boolean),
    );
    const onlineNodes = data.nodes.filter(
      (node) => node.status?.phase === "Online",
    ).length;
    const attentionNodes = data.nodes.filter(
      (node) =>
        node.status?.phase !== "Online" || node.spec.unschedulable === true,
    );
    const runningJobs = data.jobs.filter(
      (job) => job.status?.phase === "Running",
    ).length;
    const pendingJobs = data.jobs.filter(
      (job) => job.status?.phase === "Pending",
    );
    return {
      clusters: clusterNames.size,
      onlineNodes,
      attentionNodes,
      runningJobs,
      pendingJobs,
    };
  }, [data]);

  const healthPercent = data.nodes.length
    ? Math.round((summary.onlineNodes / data.nodes.length) * 100)
    : 100;
  const recentItems = useMemo(
    () =>
      [
        ...data.jobs.map((job) => ({
          id: `job-${job.metadata.name}`,
          type: zh ? "任务" : "Job",
          name: job.metadata.name,
          state: job.status?.phase ?? "Pending",
          time: job.metadata.creationTimestamp,
          target: "jobs",
        })),
        ...data.nodes.map((node) => ({
          id: `node-${node.metadata.name}`,
          type: zh ? "节点" : "Node",
          name: node.metadata.name,
          state: node.status?.phase ?? "Offline",
          time: node.metadata.creationTimestamp,
          target: "clusters-nodes",
        })),
      ]
        .filter((item) => item.time)
        .sort(
          (left, right) =>
            new Date(right.time!).getTime() - new Date(left.time!).getTime(),
        )
        .slice(0, 5),
    [data.jobs, data.nodes, zh],
  );

  const metrics = [
    {
      label: zh ? "已纳管集群" : "Managed clusters",
      value: summary.clusters,
      note: `${data.domains.length} ${zh ? "个网络域" : "domains"}`,
      icon: Network,
      tone: "violet",
    },
    {
      label: zh ? "在线节点" : "Online nodes",
      value: `${summary.onlineNodes} / ${data.nodes.length}`,
      note: `${healthPercent}% ${zh ? "健康率" : "healthy"}`,
      icon: Server,
      tone: "mint",
    },
    {
      label: zh ? "运行中任务" : "Running jobs",
      value: summary.runningJobs,
      note: `${data.jobs.length} ${zh ? "个任务总计" : "jobs total"}`,
      icon: Activity,
      tone: "blue",
    },
    {
      label: zh ? "存储配置" : "Storage classes",
      value: data.storageClasses.length,
      note: zh ? "对象存储接入" : "object storage connections",
      icon: HardDrive,
      tone: "orange",
    },
  ];

  const actions = [
    {
      label: zh ? "创建集群" : "Create cluster",
      description: zh
        ? "接入新的计算或端侧集群"
        : "Connect a compute or edge cluster",
      icon: PackagePlus,
      target: "create-cluster",
    },
    {
      label: zh ? "管理节点" : "Manage nodes",
      description: zh
        ? "查看节点健康与调度状态"
        : "Review health and scheduling",
      icon: Server,
      target: "clusters-nodes",
    },
    {
      label: zh ? "配置存储" : "Configure storage",
      description: zh
        ? "管理任务数据与对象存储"
        : "Manage job data and object storage",
      icon: Database,
      target: "storageClass",
    },
    {
      label: zh ? "镜像仓库" : "Image registries",
      description: zh
        ? "维护工作负载镜像凭据"
        : "Maintain workload image credentials",
      icon: Image,
      target: "image-registries",
    },
    {
      label: zh ? "组件市场" : "Addons",
      description: zh
        ? "安装和管理平台扩展组件"
        : "Install platform extensions",
      icon: Boxes,
      target: "addons",
    },
    {
      label: zh ? "系统配置" : "System config",
      description: zh ? "调整平台级运行参数" : "Adjust platform settings",
      icon: Settings,
      target: "config",
    },
  ];

  return (
    <div className="page-content resource-page admin-dashboard-page">
      <section className="admin-dashboard-hero">
        <div>
          <span className="eyebrow">
            <ShieldCheck size={14} />
            {zh ? "ADMIN CONTROL CENTER" : "ADMIN CONTROL CENTER"}
          </span>
          <h2>{zh ? "管理工作台" : "Admin dashboard"}</h2>
          <p>
            {zh
              ? "掌握平台运行状态，快速处理资源、任务与基础设施管理工作。"
              : "Monitor platform health and act quickly across resources, jobs, and infrastructure."}
          </p>
        </div>
        <div className="admin-dashboard-sync">
          <span className={error ? "warning" : "healthy"}>
            {error ? <AlertTriangle size={15} /> : <CheckCircle2 size={15} />}
            {error
              ? zh
                ? "部分数据获取失败"
                : "Some data unavailable"
              : zh
                ? "平台运行正常"
                : "Platform operational"}
          </span>
          <small>
            {updatedAt
              ? `${zh ? "更新于" : "Updated"} ${updatedAt.toLocaleTimeString(zh ? "zh-CN" : "en-US", { hour12: false })}`
              : zh
                ? "正在同步数据"
                : "Syncing data"}
          </small>
          <button
            className="secondary-button"
            onClick={() => fetchDashboard()}
            disabled={loading}
          >
            <RefreshCw size={15} className={loading ? "spin" : ""} />
            {c.common.refresh}
          </button>
        </div>
      </section>

      <section
        className="admin-dashboard-metrics"
        aria-label={zh ? "平台概览" : "Platform overview"}
      >
        {metrics.map(({ label, value, note, icon: Icon, tone }) => (
          <button
            type="button"
            className={`admin-dashboard-metric ${tone}`}
            key={label}
            onClick={() =>
              onNavigate(
                label.includes(zh ? "集群" : "cluster")
                  ? "clusters-list"
                  : label.includes(zh ? "节点" : "node")
                    ? "clusters-nodes"
                    : label.includes(zh ? "任务" : "job")
                      ? "jobs"
                      : "storageClass",
              )
            }
          >
            <span>
              <Icon size={19} />
            </span>
            <div>
              <small>{label}</small>
              <strong>{value}</strong>
              <em>{note}</em>
            </div>
            <ArrowRight size={16} />
          </button>
        ))}
      </section>

      <div className="admin-dashboard-main-grid">
        <section className="admin-dashboard-panel admin-dashboard-actions">
          <div className="admin-dashboard-panel-head">
            <div>
              <span>{zh ? "核心操作" : "Core actions"}</span>
              <small>
                {zh ? "常用管理入口" : "Common administration entry points"}
              </small>
            </div>
          </div>
          <div className="admin-action-grid">
            {actions.map(({ label, description, icon: Icon, target }) => (
              <button key={target} onClick={() => onNavigate(target)}>
                <span>
                  <Icon size={18} />
                </span>
                <div>
                  <strong>{label}</strong>
                  <small>{description}</small>
                </div>
                <ArrowRight size={15} />
              </button>
            ))}
          </div>
        </section>

        <section className="admin-dashboard-panel admin-attention-panel">
          <div className="admin-dashboard-panel-head">
            <div>
              <span>{zh ? "待处理事项" : "Needs attention"}</span>
              <small>
                {zh
                  ? "需要管理员关注的状态"
                  : "Items requiring administrator review"}
              </small>
            </div>
            <b>{summary.attentionNodes.length + summary.pendingJobs.length}</b>
          </div>
          <div className="admin-attention-list">
            {summary.attentionNodes.slice(0, 3).map((node) => (
              <button
                key={node.metadata.name}
                onClick={() => onNavigate("clusters-nodes", node.metadata.name)}
              >
                <span className="danger">
                  <AlertTriangle size={15} />
                </span>
                <div>
                  <strong>{node.metadata.name}</strong>
                  <small>
                    {node.spec.unschedulable
                      ? zh
                        ? "节点不可调度"
                        : "Unschedulable"
                      : zh
                        ? "节点离线"
                        : "Node offline"}
                  </small>
                </div>
                <ArrowRight size={14} />
              </button>
            ))}
            {summary.pendingJobs.slice(0, 3).map((job) => (
              <button
                key={job.metadata.name}
                onClick={() => onNavigate("jobs", job.metadata.name)}
              >
                <span className="pending">
                  <Activity size={15} />
                </span>
                <div>
                  <strong>{job.metadata.name}</strong>
                  <small>
                    {zh ? "任务等待调度" : "Job pending scheduling"}
                  </small>
                </div>
                <ArrowRight size={14} />
              </button>
            ))}
            {summary.attentionNodes.length === 0 &&
              summary.pendingJobs.length === 0 && (
                <div className="admin-attention-empty">
                  <CheckCircle2 size={22} />
                  <strong>
                    {zh ? "当前没有待处理事项" : "Nothing needs attention"}
                  </strong>
                  <small>
                    {zh
                      ? "所有节点和任务状态正常"
                      : "Nodes and jobs are healthy"}
                  </small>
                </div>
              )}
          </div>
        </section>
      </div>

      <section className="admin-dashboard-panel admin-recent-panel">
        <div className="admin-dashboard-panel-head">
          <div>
            <span>{zh ? "近期资源动态" : "Recent resource activity"}</span>
            <small>
              {zh
                ? "最近创建或接入的资源"
                : "Recently created or connected resources"}
            </small>
          </div>
        </div>
        <div className="admin-recent-list">
          {recentItems.map((item) => (
            <button
              key={item.id}
              onClick={() => onNavigate(item.target, item.name)}
            >
              <span className="admin-recent-type">{item.type}</span>
              <strong>{item.name}</strong>
              <small>{item.state}</small>
              <time>{formatChinaDateTime(item.time)}</time>
              <ArrowRight size={14} />
            </button>
          ))}
          {recentItems.length === 0 && (
            <div className="admin-recent-empty">
              {zh ? "暂无资源动态" : "No recent activity"}
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
