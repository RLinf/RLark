import { useMemo, useState } from "react";
import {
  ArrowRight,
  Bot,
  ChevronRight,
  CloudCog,
  RefreshCw,
  Server,
  Workflow,
} from "lucide-react";
import { activity, type Cluster, type Job, type Phase } from "../data";
import type { Copy } from "../i18n";
import type { CRDNode, Page, ResourceRow } from "../types";
import { useAutoRefresh } from "../hooks";
import { crdToJob } from "../utils/crd";
import {
  getNodeCategories,
  getNodeDeviceModel,
  getNodeGPUModel,
  hasNodeCategory,
} from "../utils/nodes";
import {
  MetricCard,
  RefreshOverlay,
  ResourceDistribution,
  StatusBadge,
} from "../components/shared";
import { OverviewChinaMap } from "../components/OverviewChinaMap";

export function Overview({
  navigate,
  copy: c,
  isMockMode,
}: {
  navigate: (
    page: Page,
    name?: string,
    options?: { query?: Record<string, string | undefined> },
  ) => void;
  copy: Copy;
  isMockMode: boolean;
}) {
  const [realClusters, setRealClusters] = useState<Cluster[]>([]);
  const [realNodes, setRealNodes] = useState<CRDNode[]>([]);
  const [realJobs, setRealJobs] = useState<Job[]>([]);
  const [refreshing, setRefreshing] = useState(false);
  const isZh = c.nav.overview === "总览";

  const { refresh } = useAutoRefresh(async () => {
    const [clusters, nodes, jobs] = await Promise.all([
      clustersApi.list<Cluster>().catch(() => []),
      nodesApi.list().catch(() => []),
      jobsApi.list().catch(() => []),
    ]);
    setRealClusters(clusters);
    setRealNodes(nodes);
    setRealJobs(jobs.map(crdToJob));
  }, 15000);

  const handleRefresh = async () => {
    if (refreshing) return;
    setRefreshing(true);
    try {
      await refresh();
    } finally {
      setRefreshing(false);
    }
  };

  const displayNodes = realNodes;
  const embodiedClusters = realClusters.filter((x) => x.type === "Embodied");
  const runningJobs = realJobs.filter((x) => x.phase === "Running").length;
  const gpuModelList = Array.from(
    new Set(
      isMockMode
        ? displayNodes
            .filter((node) => hasNodeCategory(node, "cloud"))
            .map(getNodeGPUModel)
            .filter(Boolean)
        : realClusters.flatMap((x) => x.gpuModels),
    ),
  );
  const robotModelList = Array.from(
    new Set(
      isMockMode
        ? displayNodes
            .filter((node) => hasNodeCategory(node, "robot"))
            .map(getNodeDeviceModel)
            .filter(Boolean)
        : realClusters.flatMap((x) => x.robotModels),
    ),
  );

  const categoryCounts = useMemo(() => {
    const counts = { cloud: 0, edge: 0, robot: 0 };
    for (const n of displayNodes) {
      for (const cat of getNodeCategories(n)) {
        if (cat === "cloud") counts.cloud++;
        else if (cat === "edge") counts.edge++;
        else if (cat === "robot") counts.robot++;
      }
    }
    return counts;
  }, [displayNodes]);

  const embodiedNodeCount = isMockMode
    ? categoryCounts.edge + categoryCounts.robot
    : realClusters.reduce(
        (sum, cluster) => sum + cluster.embodiedNodes + cluster.robots,
        0,
      );
  const embodiedModelCount = new Set(
    displayNodes
      .filter(
        (node) =>
          hasNodeCategory(node, "edge") || hasNodeCategory(node, "robot"),
      )
      .map(getNodeDeviceModel)
      .filter(Boolean),
  ).size;
  const embodiedClusterCount = isMockMode
    ? new Set(
        displayNodes
          .filter(
            (node) =>
              hasNodeCategory(node, "edge") || hasNodeCategory(node, "robot"),
          )
          .map((node) => node.metadata.namespace),
      ).size
    : embodiedClusters.length;

  const resourceRows: ResourceRow[] = [
    {
      label: c.kind.CloudCompute,
      count: categoryCounts.cloud,
      models: gpuModelList.slice(0, 3).join(" / "),
      color: "blue",
    },
    {
      label: c.kind.EmbodiedCompute,
      count: categoryCounts.edge,
      models: "",
      color: "green",
    },
    {
      label: c.kind.Robot,
      count: categoryCounts.robot,
      models: robotModelList.slice(0, 3).join(" / "),
      color: "orange",
    },
  ];

  const robotNodes = displayNodes.filter((n) => hasNodeCategory(n, "robot"));
  return (
    <div
      className={`page-content resource-page overview-page refreshable-region page-refresh-region${refreshing ? " is-refreshing" : ""}`}
      aria-busy={refreshing}
    >
      <div className="section-heading">
        <div>
          <span className="eyebrow">{c.overview.eyebrow}</span>
          <h2>{c.overview.title}</h2>
          <p>{c.overview.desc}</p>
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
            ? isZh
              ? "刷新中..."
              : "Refreshing..."
            : c.common.refresh}
        </button>
      </div>
      <section className="metric-grid platform-metrics">
        <MetricCard
          icon={CloudCog}
          tone="blue"
          label={isZh ? "具身集群数量" : "Embodied clusters"}
          value={`${embodiedClusterCount}`}
          note={isZh ? "已纳管具身集群" : "Managed embodied clusters"}
          onClick={() => navigate("clusters-management")}
        />
        <MetricCard
          icon={Server}
          tone="mint"
          label={isZh ? "具身节点数量" : "Embodied nodes"}
          value={`${embodiedNodeCount}`}
          note={isZh ? "端算力与具身 Worker" : "Edge and embodied workers"}
          onClick={() => navigate("clusters-nodes")}
        />
        <MetricCard
          icon={Bot}
          tone="violet"
          label={isZh ? "具身种类数量" : "Embodied types"}
          value={`${embodiedModelCount}`}
          note={isZh ? "按设备型号去重" : "Unique device models"}
          onClick={() => navigate("clusters-nodes")}
        />
        <MetricCard
          icon={Workflow}
          tone="orange"
          label={c.overview.jobs}
          value={`${runningJobs} / ${realJobs.length}`}
          note={isZh ? "正在运行 / 任务总数" : "Running / total jobs"}
          onClick={() => navigate("jobs")}
        />
      </section>
      <OverviewChinaMap
        navigate={navigate}
        copy={c}
        nodes={displayNodes}
        jobs={realJobs}
        clusters={realClusters}
      />
      <section className="dashboard-grid">
        <div className="panel chart-panel">
          <div className="panel-title">
            <div>
              <span>{c.overview.compute}</span>
              <h3>{c.overview.computeDesc}</h3>
            </div>
            <button
              className="plain-button"
              onClick={() => navigate("clusters-nodes")}
            >
              {c.common.viewAll}
              <ArrowRight size={14} />
            </button>
          </div>
          <ResourceDistribution rows={resourceRows} />
        </div>
        <div className="panel workload-panel">
          <div className="panel-title">
            <div>
              <span>{c.overview.liveRobots}</span>
              <h3>{c.kind.Robot}</h3>
            </div>
            <button
              className="plain-button"
              onClick={() => navigate("clusters-nodes")}
            >
              {c.common.details}
              <ArrowRight size={14} />
            </button>
          </div>
          <div className="robot-state-list">
            {robotNodes.length === 0 ? (
              <p
                className="muted"
                style={{ padding: "16px 0", textAlign: "center" }}
              >
                {isZh ? "暂无真机" : "No robots"}
              </p>
            ) : (
              robotNodes.slice(0, 6).map((n) => {
                const phase = (n.status?.phase ?? "Offline") as Phase;
                const model =
                  getNodeDeviceModel(n) ||
                  n.metadata.labels?.["node.kubernetes.io/instance-type"] ||
                  "—";
                const reason = n.status?.reason || "—";
                return (
                  <div key={n.metadata.name}>
                    <span className={"node-status-ring " + phase.toLowerCase()}>
                      <Bot size={17} />
                    </span>
                    <div>
                      <strong>{n.metadata.name}</strong>
                      <small>
                        {model} · {reason}
                      </small>
                    </div>
                    <StatusBadge phase={phase} copy={c} />
                  </div>
                );
              })
            )}
          </div>
        </div>
      </section>
      <section className="bottom-grid">
        <div className="panel">
          <div className="panel-title">
            <div>
              <span>{c.overview.liveJobs}</span>
              <h3>{c.nav.jobs}</h3>
            </div>
            <button className="plain-button" onClick={() => navigate("jobs")}>
              {c.common.viewAll}
              <ArrowRight size={14} />
            </button>
          </div>
          <div className="workflow-list">
            {realJobs.slice(0, 4).map((job) => (
              <button key={job.id} onClick={() => navigate("jobs")}>
                <span className={"workflow-symbol " + job.phase.toLowerCase()}>
                  <Workflow size={17} />
                </span>
                <span className="workflow-info">
                  <strong>{job.name}</strong>
                  <small>
                    {c.jobType[job.type]} · {job.target}
                  </small>
                </span>
                <StatusBadge phase={job.phase} copy={c} />
                <span className="progress-cell">
                  <i>
                    <b style={{ width: job.progress + "%" }} />
                  </i>
                  <small>
                    {job.runningWorkers}/{job.workers}
                  </small>
                </span>
                <ChevronRight size={17} />
              </button>
            ))}
          </div>
        </div>
        <div className="panel activity-panel">
          <div className="panel-title">
            <div>
              <span>{c.overview.recent}</span>
              <h3>{c.common.production}</h3>
            </div>
          </div>
          <div className="activity-list">
            {activity.length === 0 ? (
              <p
                className="muted"
                style={{ padding: "16px 0", textAlign: "center" }}
              >
                {isZh ? "暂无活动" : "No recent activity"}
              </p>
            ) : (
              activity.map((item, i) => (
                <div key={i}>
                  <span className={"activity-dot " + item.type}>
                    <i />
                  </span>
                  <div>
                    <strong>{item.title}</strong>
                    <small>{item.meta}</small>
                  </div>
                  <time>{item.time}</time>
                </div>
              ))
            )}
          </div>
        </div>
      </section>
      <RefreshOverlay
        visible={refreshing}
        label={isZh ? "正在刷新总览数据" : "Refreshing overview data"}
      />
    </div>
  );
}
import { clustersApi, jobsApi, nodesApi } from "../backend";
