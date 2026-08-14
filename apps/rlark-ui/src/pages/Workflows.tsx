import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
} from "react";
import { Check, ChevronLeft, Plus, Trash2, X } from "lucide-react";
import { clusters, type Cluster, type JobType } from "../data";
import type { Copy } from "../i18n";
import type {
  CRDWorkflow,
  DAGEdge,
  DAGNode,
  RoleResource,
  WorkflowJobDef,
} from "../types";
import { useAutoRefresh } from "../hooks";
import { crdToWorkflow } from "../utils/crd";
import { hasCycle, makeDefaultRoleResources } from "../utils/dag";
import {
  computePvcStorageMap,
  generateJobCRD,
  ROLE_TEMPLATES,
} from "../utils/job";
import { toYaml } from "../utils/yaml";
import { formatChinaDateTime } from "../utils/time";
import { useNodeLabels } from "../utils/nodes";
import { NodeSelectorPicker, RoleNameInput } from "../components/create";
import { CodeEditorField } from "../components/CodeEditor";
import {
  compareSortValues,
  PageToolbar,
  Pagination,
  SortButton,
  StatusBadge,
  type SortDirection,
} from "../components/shared";

export function WorkflowDetailPage({
  wf,
  crd,
  copy: c,
  onBack,
  onJobClick,
}: {
  wf: ReturnType<typeof crdToWorkflow>;
  crd: CRDWorkflow;
  copy: Copy;
  onBack: () => void;
  onJobClick: (jobName: string) => void;
}) {
  const zh = c.nav.overview === "总览";
  const templates = wf.templates;
  const jobStatuses = wf.jobStatuses;

  const statusOf = (name: string) =>
    jobStatuses.find((s) => s.name === name)?.phase ?? "Pending";

  const phaseColor = (phase: string) =>
    phase === "Succeeded"
      ? "#22c55e"
      : phase === "Running"
        ? "var(--blue)"
        : phase === "Failed"
          ? "#ef4444"
          : "#94a3b8";

  const phaseLabel = (phase: string) =>
    zh
      ? phase === "Succeeded"
        ? "已完成"
        : phase === "Running"
          ? "运行中"
          : phase === "Failed"
            ? "失败"
            : "待执行"
      : phase === "Succeeded"
        ? "Done"
        : phase === "Running"
          ? "Running"
          : phase === "Failed"
            ? "Failed"
            : "Pending";

  const NODE_W = 200;
  const NODE_H = 56;
  const COL_GAP = 280;
  const ROW_GAP = 100;

  const canvasRef = useRef<HTMLDivElement>(null);
  const [portPositions, setPortPositions] = useState<
    Record<
      string,
      { in: { x: number; y: number }; out: { x: number; y: number } }
    >
  >({});

  const sorted = [...templates];
  const depMap = new Map(templates.map((t) => [t.name, t.dependencies ?? []]));
  const layerOf = new Map<string, number>();
  const calcLayer = (name: string): number => {
    if (layerOf.has(name)) return layerOf.get(name)!;
    const deps = depMap.get(name) ?? [];
    if (deps.length === 0) {
      layerOf.set(name, 0);
      return 0;
    }
    const max = Math.max(...deps.map(calcLayer));
    layerOf.set(name, max + 1);
    return max + 1;
  };
  templates.forEach((t) => calcLayer(t.name));

  const layers = new Map<number, string[]>();
  sorted.forEach((t) => {
    const l = layerOf.get(t.name) ?? 0;
    if (!layers.has(l)) layers.set(l, []);
    layers.get(l)!.push(t.name);
  });

  const layoutNodes = useMemo(
    () =>
      templates.map((t) => {
        const layer = layerOf.get(t.name) ?? 0;
        const col = layers.get(layer) ?? [];
        const idx = col.indexOf(t.name);
        return {
          id: t.name,
          name: t.name,
          phase: statusOf(t.name),
          x: 40 + layer * COL_GAP,
          y: 40 + idx * ROW_GAP,
        };
      }),
    [templates, jobStatuses],
  );

  const allEdges = useMemo(() => {
    const edges: { from: string; to: string }[] = [];
    templates.forEach((t) => {
      (t.dependencies ?? []).forEach((dep) => {
        edges.push({ from: dep, to: t.name });
      });
    });
    return edges;
  }, [templates]);

  useLayoutEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const cRect = canvas.getBoundingClientRect();
    const next: Record<
      string,
      { in: { x: number; y: number }; out: { x: number; y: number } }
    > = {};
    for (const n of layoutNodes) {
      const el = canvas.querySelector(
        `[data-node-id="${n.id}"]`,
      ) as HTMLElement | null;
      if (!el) continue;
      const r = el.getBoundingClientRect();
      const sx = r.left - cRect.left + canvas.scrollLeft;
      const sy = r.top - cRect.top + canvas.scrollTop;
      const cy = sy + r.height / 2;
      next[n.id] = { in: { x: sx, y: cy }, out: { x: sx + r.width, y: cy } };
    }
    setPortPositions(next);
  }, [layoutNodes]);

  const nodePos = (id: string, side: "in" | "out") => {
    const p = portPositions[id];
    if (p) return side === "in" ? p.in : p.out;
    const n = layoutNodes.find((x) => x.id === id);
    if (!n) return { x: 0, y: 0 };
    const cy = n.y + NODE_H / 2;
    return side === "in"
      ? { x: n.x + 1, y: cy }
      : { x: n.x + NODE_W - 1, y: cy };
  };

  const canvasW = 40 + (layers.size - 1) * COL_GAP + NODE_W + 40;
  const canvasH =
    40 + Math.max(...[...layers.values()].map((c) => c.length)) * ROW_GAP;

  return (
    <div className="page-content resource-page">
      <div className="section-heading">
        <div>
          <button
            className="secondary-button"
            onClick={onBack}
            style={{ marginBottom: 8 }}
          >
            <ChevronLeft size={14} />
            {zh ? "返回" : "Back"}
          </button>
          <span className="eyebrow">{c.workflows.eyebrow}</span>
          <h2>{wf.name}</h2>
          <p>
            <StatusBadge phase={wf.phase} copy={c} />
            <span
              style={{ marginLeft: 12, fontSize: 12, color: "var(--muted)" }}
            >
              {zh ? "任务" : "Jobs"}: {wf.runningJobs}/{wf.jobCount}
            </span>
          </p>
        </div>
      </div>
      <div className="form-section">
        <div className="form-section-head">
          <strong>{zh ? "DAG 执行状态" : "DAG Execution Status"}</strong>
        </div>
        <div
          className="dag-canvas"
          ref={canvasRef}
          style={{ overflow: "auto", maxHeight: 500 }}
        >
          <div
            className="dag-canvas-content"
            style={{ width: canvasW, height: canvasH }}
          >
            <svg className="dag-svg" width={canvasW} height={canvasH}>
              <defs>
                <marker
                  id="wf-arrow"
                  markerWidth="8"
                  markerHeight="8"
                  refX="7"
                  refY="3"
                  orient="auto"
                >
                  <path d="M0,0 L7,3 L0,6 Z" fill="var(--blue)" />
                </marker>
              </defs>
              {allEdges.map((e) => {
                const p1 = nodePos(e.from, "out");
                const p2 = nodePos(e.to, "in");
                const midX = (p1.x + p2.x) / 2;
                return (
                  <path
                    key={`${e.from}-${e.to}`}
                    d={`M ${p1.x} ${p1.y} C ${midX} ${p1.y}, ${midX} ${p2.y}, ${p2.x} ${p2.y}`}
                    stroke="var(--blue)"
                    strokeWidth="2"
                    fill="none"
                    markerEnd="url(#wf-arrow)"
                  />
                );
              })}
            </svg>
            {layoutNodes.map((n) => (
              <div
                key={n.id}
                data-node-id={n.id}
                className="dag-node"
                onClick={
                  n.phase === "Pending"
                    ? undefined
                    : () => onJobClick(`${wf.name}-${n.name}`)
                }
                style={{
                  left: n.x,
                  top: n.y,
                  borderColor: phaseColor(n.phase),
                  cursor: n.phase === "Pending" ? "default" : "pointer",
                  boxShadow:
                    n.phase === "Running"
                      ? `0 0 0 2px ${phaseColor(n.phase)}, 0 2px 12px rgba(124,58,237,0.2)`
                      : undefined,
                }}
              >
                <div
                  className="dag-node-port input"
                  style={{ borderColor: phaseColor(n.phase) }}
                />
                <div className="dag-node-body">
                  <strong>{n.name}</strong>
                  <span
                    className="dag-node-type"
                    style={{
                      color: phaseColor(n.phase),
                      background:
                        n.phase === "Running"
                          ? "rgba(124,58,237,0.1)"
                          : undefined,
                    }}
                  >
                    {phaseLabel(n.phase)}
                  </span>
                </div>
                <div
                  className="dag-node-port output"
                  style={{ borderColor: phaseColor(n.phase) }}
                />
              </div>
            ))}
          </div>
        </div>
      </div>
      <div className="table-panel" style={{ marginTop: 16 }}>
        <table>
          <thead>
            <tr>
              <th>{zh ? "任务名称" : "Job Name"}</th>
              <th>{zh ? "状态" : "Phase"}</th>
              <th>{zh ? "依赖" : "Dependencies"}</th>
              <th>{zh ? "消息" : "Message"}</th>
            </tr>
          </thead>
          <tbody>
            {templates.map((t) => {
              const st = jobStatuses.find((s) => s.name === t.name);
              return (
                <tr
                  key={t.name}
                  onClick={
                    (st?.phase ?? "Pending") === "Pending"
                      ? undefined
                      : () => onJobClick(`${wf.name}-${t.name}`)
                  }
                  style={{
                    cursor:
                      (st?.phase ?? "Pending") === "Pending"
                        ? "default"
                        : "pointer",
                  }}
                >
                  <td>
                    <strong>{t.name}</strong>
                  </td>
                  <td>
                    <span
                      style={{
                        color: phaseColor(st?.phase ?? "Pending"),
                        fontWeight: 600,
                      }}
                    >
                      {phaseLabel(st?.phase ?? "Pending")}
                    </span>
                  </td>
                  <td>
                    <small>{(t.dependencies ?? []).join(", ") || "—"}</small>
                  </td>
                  <td>
                    <small>{st?.message || "—"}</small>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export function WorkflowsPage({
  copy: c,
  onCreate,
  selectedName,
  onSelect,
  onJobClick,
}: {
  copy: Copy;
  onCreate: () => void;
  selectedName: string;
  onSelect: (name?: string) => void;
  onJobClick: (jobName: string) => void;
}) {
  const zh = c.nav.overview === "总览";
  const [workflows, setWorkflows] = useState<CRDWorkflow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [sort, setSort] = useState<{
    key: "name" | "phase" | "jobCount" | "created";
    direction: SortDirection;
  }>({ key: "created", direction: "desc" });
  const toggleSort = (key: typeof sort.key) =>
    setSort((current) => ({
      key,
      direction:
        current.key === key && current.direction === "asc" ? "desc" : "asc",
    }));

  const fetchWorkflows = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    try {
      const resp = await fetch("/api/v1/rlinf.io/v1alpha1/workflows");
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      setWorkflows(data.items ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchWorkflows, 10000);

  const handleDelete = async (name: string) => {
    if (
      !confirm(
        zh ? `确定删除工作流 "${name}" 吗?` : `Delete workflow "${name}"?`,
      )
    )
      return;
    try {
      const resp = await fetch(`/api/v1/rlinf.io/v1alpha1/workflows/${name}`, {
        method: "DELETE",
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      setWorkflows((prev) => prev.filter((w) => w.metadata.name !== name));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const items = workflows.map(crdToWorkflow);
  const filteredItems = items.filter((workflow) =>
    `${workflow.name} ${workflow.phase}`
      .toLowerCase()
      .includes(query.trim().toLowerCase()),
  );
  const sortedItems = [...filteredItems].sort((a, b) =>
    compareSortValues(
      a[sort.key],
      b[sort.key],
      sort.direction,
      zh ? "zh-CN" : "en",
    ),
  );
  const totalPages = Math.max(1, Math.ceil(sortedItems.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pagedItems = sortedItems.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );

  useEffect(() => setPage(1), [query, pageSize]);
  useEffect(() => {
    if (page > totalPages) setPage(totalPages);
  }, [page, totalPages]);

  const selected =
    selectedName && items.length > 0
      ? (items.find((w) => w.name === selectedName) ?? null)
      : null;

  if (selected) {
    return (
      <WorkflowDetailPage
        wf={selected}
        crd={workflows.find((w) => w.metadata.name === selectedName)!}
        copy={c}
        onBack={() => onSelect(undefined)}
        onJobClick={onJobClick}
      />
    );
  }

  return (
    <div className="page-content resource-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">{c.workflows.eyebrow}</span>
          <h2>{c.workflows.title}</h2>
          <p>{c.workflows.desc}</p>
        </div>
        <button className="primary-button" onClick={onCreate}>
          <Plus size={17} />
          {c.workflows.createTitle}
        </button>
      </div>
      <PageToolbar
        placeholder={c.workflows.search}
        value={query}
        onChange={setQuery}
        count={filteredItems.length}
        copy={c}
        onRefresh={() => fetchWorkflows()}
      />
      {error && (
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}
      <div className="table-panel">
        <table>
          <thead>
            <tr>
              <th>
                <SortButton
                  label={zh ? "工作流名称" : "Name"}
                  active={sort.key === "name"}
                  direction={sort.direction}
                  onClick={() => toggleSort("name")}
                />
              </th>
              <th>
                <SortButton
                  label={zh ? "状态" : "Status"}
                  active={sort.key === "phase"}
                  direction={sort.direction}
                  onClick={() => toggleSort("phase")}
                />
              </th>
              <th>
                <SortButton
                  label={zh ? "任务数" : "Jobs"}
                  active={sort.key === "jobCount"}
                  direction={sort.direction}
                  onClick={() => toggleSort("jobCount")}
                />
              </th>
              <th>
                <SortButton
                  label={zh ? "创建时间" : "Created"}
                  active={sort.key === "created"}
                  direction={sort.direction}
                  onClick={() => toggleSort("created")}
                />
              </th>
              <th />
            </tr>
          </thead>
          <tbody>
            {pagedItems.map((wf) => (
              <tr
                key={wf.name}
                onClick={() => onSelect(wf.name)}
                style={{ cursor: "pointer" }}
              >
                <td>
                  <strong>{wf.name}</strong>
                </td>
                <td>
                  <StatusBadge phase={wf.phase} copy={c} />
                </td>
                <td>
                  <span className="inline-progress">
                    <i>
                      <b
                        style={{
                          width:
                            wf.jobCount > 0
                              ? (wf.runningJobs / wf.jobCount) * 100 + "%"
                              : "0%",
                        }}
                      />
                    </i>
                    {wf.runningJobs}/{wf.jobCount}
                  </span>
                </td>
                <td>
                  <small>{formatChinaDateTime(wf.created)}</small>
                </td>
                <td>
                  <div className="row-actions">
                    <button
                      className="icon-button danger"
                      onClick={(event) => {
                        event.stopPropagation();
                        handleDelete(wf.name);
                      }}
                      title={zh ? "删除" : "Delete"}
                    >
                      <Trash2 size={15} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {filteredItems.length === 0 && !loading && (
              <tr>
                <td
                  colSpan={5}
                  style={{ textAlign: "center", padding: "32px" }}
                >
                  <small className="muted">
                    {query
                      ? zh
                        ? "没有匹配的工作流"
                        : "No matching workflows"
                      : zh
                        ? "暂无工作流"
                        : "No workflows"}
                  </small>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <Pagination
        page={currentPage}
        pageSize={pageSize}
        total={filteredItems.length}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
        zh={zh}
      />
    </div>
  );
}

export function CreateWorkflowModal({
  onClose,
  copy: c,
}: {
  onClose: () => void;
  copy: Copy;
}) {
  const zh = c.nav.overview === "总览";
  const [step, setStep] = useState(1);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [workflowName, setWorkflowName] = useState("rl-training-pipeline");

  useEffect(() => {
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !submitting) onClose();
    };
    document.addEventListener("keydown", handleEscape);
    return () => document.removeEventListener("keydown", handleEscape);
  }, [onClose, submitting]);
  const [domains, setDomains] = useState<{ name: string; cidr: string }[]>([]);
  const {
    clusterDisplayNames,
    nodes: allNodes,
    loading: nodesLoading,
  } = useNodeLabels();
  const [activeJobId, setActiveJobId] = useState<string>("");
  const [activeRoleTab, setActiveRoleTab] = useState<string>("");

  const [availableClusters, setAvailableClusters] = useState<Cluster[]>(
    clusters.slice(0, 4),
  );

  const [storageClasses, setStorageClasses] = useState<
    Array<{ name: string; description: string; bucket: string }>
  >([]);
  const [storageClassLoading, setStorageClassLoading] = useState(false);
  const [storageClassFetched, setStorageClassFetched] = useState(false);
  const [clustersLoaded, setClustersLoaded] = useState(false);
  const lastFetchedStorageClusterRef = useRef<string>("");

  const [dragNode, setDragNode] = useState<{
    id: string;
    offsetX: number;
    offsetY: number;
  } | null>(null);
  const [dragEdge, setDragEdge] = useState<{
    from: string;
    x: number;
    y: number;
  } | null>(null);
  const [editingNodeId, setEditingNodeId] = useState<string | null>(null);
  const canvasRef = useRef<HTMLDivElement>(null);

  const [nodes, setNodes] = useState<DAGNode[]>([
    { id: "n1", name: "job-1", x: 40, y: 40 },
  ]);
  const [edges, setEdges] = useState<DAGEdge[]>([]);

  const [jobs, setJobs] = useState<WorkflowJobDef[]>([
    {
      id: "n1",
      name: "job-1",
      dependencies: [],
      type: "RL",
      roles: ROLE_TEMPLATES["RL"],
      headerRole: ROLE_TEMPLATES["RL"][0],
      roleResources: makeDefaultRoleResources(
        "RL",
        ROLE_TEMPLATES["RL"],
        clusterDisplayNames,
      ),
      runScript:
        "python train.py --config /mnt/config/train.yaml --dataset /mnt/dataset --output /mnt/checkpoints",
      domain: "",
    },
  ]);

  useEffect(() => {
    fetch("/api/v1/rlinf.io/v1alpha1/domains")
      .then((r) =>
        r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)),
      )
      .then((data) =>
        setDomains(
          (data.items ?? []).map((d: any) => ({
            name: d.metadata?.name ?? "",
            cidr: d.spec?.cidr ?? "",
          })),
        ),
      )
      .catch(() => {});
  }, []);

  useEffect(() => {
    fetch("/api/v1/clusters")
      .then((r) =>
        r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)),
      )
      .then((data) => {
        const list: Cluster[] = (data.data ?? []).map((c: any) => ({
          id: c.id ?? c.name ?? "",
          name: c.name ?? c.id ?? "",
          type: c.type === "Embodied" ? "Embodied" : "Cloud",
          region: c.region ?? "",
          location: c.location ?? "",
          phase: c.phase ?? "Online",
          cloudNodes: c.cloudNodes ?? 0,
          embodiedNodes: c.embodiedNodes ?? 0,
          robots: c.robots ?? 0,
          gpuModels: c.gpuModels ?? [],
          robotModels: c.robotModels ?? [],
          cpuUsage: c.cpuUsage ?? 0,
          gpuUsage: c.gpuUsage ?? 0,
          robotUsage: c.robotUsage ?? 0,
          runningJobs: c.runningJobs ?? 0,
          description: c.description ?? "",
        }));
        const realList = list.length > 0 ? list : clusters.slice(0, 4);
        setAvailableClusters(realList);
        setClustersLoaded(list.length > 0);
        if (list.length > 0) {
          setJobs((prevJobs) =>
            prevJobs.map((job) => ({
              ...job,
              roleResources: Object.fromEntries(
                Object.entries(job.roleResources).map(([role, rr]) => {
                  const current = rr.cluster;
                  const match = list.find(
                    (c) => c.id === current || c.name === current,
                  );
                  return [
                    role,
                    { ...rr, cluster: match ? match.id : list[0].id },
                  ];
                }),
              ),
            })),
          );
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (!activeJobId && jobs.length > 0) setActiveJobId(jobs[0].id);
  }, [activeJobId, jobs]);

  const fetchStorageClasses = async (cluster?: string) => {
    if (storageClassLoading) return;
    const clusterKey = cluster ?? "";
    if (
      storageClassFetched &&
      lastFetchedStorageClusterRef.current === clusterKey
    )
      return;
    lastFetchedStorageClusterRef.current = clusterKey;
    setStorageClassLoading(true);
    setStorageClassFetched(false);
    try {
      const url = new URL(
        "/api/v1/storage/storageclass",
        window.location.origin,
      );
      if (cluster) {
        url.searchParams.set("clusters", cluster);
      }
      const resp = await fetch(url.pathname + url.search);
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const result = await resp.json();
      const list: Array<{ name: string; description: string; bucket: string }> =
        [];
      const data = result.data ?? {};
      for (const [key, value] of Object.entries(data)) {
        const item = value as any;
        list.push({
          name: item.name ?? key,
          description: item.description ?? "",
          bucket: item.bucket ?? "",
        });
      }
      setStorageClasses(list);
    } catch (e) {
      console.warn("Failed to fetch storage classes:", e);
    } finally {
      setStorageClassLoading(false);
      setStorageClassFetched(true);
    }
  };

  useEffect(() => {
    if (!activeRoleTab || !jobs.length) return;
    const job = jobs.find((j) => j.id === activeJobId);
    if (!job) return;
    const rr = job.roleResources[activeRoleTab];
    if (!rr) return;
    const hasStorageMount = rr.mounts.some((m) => m.type === "storage");
    if (hasStorageMount && rr.cluster) {
      fetchStorageClasses(rr.cluster);
    }
  }, [activeRoleTab, activeJobId, jobs]);

  useEffect(() => {
    if (!activeRoleTab && jobs.length > 0) {
      const job = jobs.find((j) => j.id === activeJobId);
      if (job && job.roles.length > 0) setActiveRoleTab(job.roles[0]);
    }
  }, [activeRoleTab, activeJobId, jobs]);

  const addJobToDAG = () => {
    const id = `n${Date.now()}`;
    let maxNum = 0;
    for (const j of jobs) {
      const m = j.name.match(/^job-(\d+)$/);
      if (m) maxNum = Math.max(maxNum, parseInt(m[1]));
    }
    const name = `job-${maxNum + 1}`;
    const x = 40 + (jobs.length % 3) * 260;
    const y = 40 + Math.floor(jobs.length / 3) * 160;
    const roles = ROLE_TEMPLATES["RL"];
    const newJob: WorkflowJobDef = {
      id,
      name,
      dependencies: [],
      type: "RL",
      roles,
      headerRole: roles[0],
      roleResources: makeDefaultRoleResources(
        "RL",
        roles,
        clusterDisplayNames,
        name,
      ),
      runScript:
        "python train.py --config /mnt/config/train.yaml --dataset /mnt/dataset --output /mnt/checkpoints",
      domain: "",
    };
    setJobs([...jobs, newJob]);
    setNodes([...nodes, { id, name, x, y }]);
    setActiveJobId(id);
    setActiveRoleTab(roles[0]);
  };

  const removeJobFromDAG = (id: string) => {
    if (jobs.length <= 1) return;
    const nextJobs = jobs.filter((j) => j.id !== id);
    const nextNodes = nodes.filter((n) => n.id !== id);
    const nextEdges = edges.filter((e) => e.from !== id && e.to !== id);
    setJobs(
      nextJobs.map((j) => ({
        ...j,
        dependencies: j.dependencies.filter((d) => {
          const depJob = jobs.find((j2) => j2.name === d);
          return depJob && depJob.id !== id;
        }),
      })),
    );
    setNodes(nextNodes);
    setEdges(nextEdges);
    if (activeJobId === id) setActiveJobId(nextJobs[0].id);
  };

  const updateJob = (id: string, patch: Partial<WorkflowJobDef>) => {
    setJobs((prev) => prev.map((j) => (j.id === id ? { ...j, ...patch } : j)));
  };

  const updateNodeName = (id: string, name: string) => {
    setNodes((prev) => prev.map((n) => (n.id === id ? { ...n, name } : n)));
  };

  const renameJob = (id: string, newName: string) => {
    const oldName = jobs.find((j) => j.id === id)?.name ?? "";
    updateJob(id, { name: newName });
    updateNodeName(id, newName);
    setJobs((prev) =>
      prev.map((j) => {
        if (j.dependencies.includes(oldName)) {
          return {
            ...j,
            dependencies: j.dependencies.map((d) =>
              d === oldName ? newName : d,
            ),
          };
        }
        return j;
      }),
    );
  };

  const addEdge = (from: string, to: string) => {
    if (from === to) return;
    if (edges.some((e) => e.from === from && e.to === to)) return;
    const nextEdges = [...edges, { from, to }];
    if (hasCycle(nextEdges, nodes)) {
      setError(zh ? "不能创建循环依赖" : "Cannot create circular dependency");
      setTimeout(() => setError(""), 3000);
      return;
    }
    setEdges(nextEdges);
    const fromName = jobs.find((j) => j.id === from)?.name ?? "";
    const toJob = jobs.find((j) => j.id === to);
    if (toJob) {
      updateJob(to, {
        dependencies: [...new Set([...toJob.dependencies, fromName])],
      });
    }
  };

  const removeEdge = (from: string, to: string) => {
    setEdges(edges.filter((e) => !(e.from === from && e.to === to)));
    const fromName = jobs.find((j) => j.id === from)?.name ?? "";
    const toJob = jobs.find((j) => j.id === to);
    if (toJob) {
      updateJob(to, {
        dependencies: toJob.dependencies.filter((d) => d !== fromName),
      });
    }
  };

  const getCanvasXY = (clientX: number, clientY: number) => {
    const el = canvasRef.current;
    if (!el) return { x: 0, y: 0 };
    const rect = el.getBoundingClientRect();
    return {
      x: clientX - rect.left + el.scrollLeft,
      y: clientY - rect.top + el.scrollTop,
    };
  };

  const onNodeMouseDown = (e: MouseEvent, id: string) => {
    if ((e.target as HTMLElement).classList.contains("dag-node-port")) return;
    const { x: mx, y: my } = getCanvasXY(e.clientX, e.clientY);
    const node = nodes.find((n) => n.id === id);
    if (!node) return;
    setDragNode({ id, offsetX: mx - node.x, offsetY: my - node.y });
  };

  const onCanvasMouseMove = (e: MouseEvent) => {
    const { x: mx, y: my } = getCanvasXY(e.clientX, e.clientY);
    if (dragNode) {
      setNodes((prev) =>
        prev.map((n) =>
          n.id === dragNode.id
            ? { ...n, x: mx - dragNode.offsetX, y: my - dragNode.offsetY }
            : n,
        ),
      );
    }
    if (dragEdge) {
      setDragEdge({ ...dragEdge, x: mx, y: my });
    }
  };

  const onCanvasMouseUp = (e: MouseEvent) => {
    if (dragEdge) {
      const target = (e.target as HTMLElement).closest(
        "[data-node-id]",
      ) as HTMLElement | null;
      if (target) {
        const toId = target.dataset.nodeId!;
        addEdge(dragEdge.from, toId);
      }
      setDragEdge(null);
    }
    setDragNode(null);
  };

  const onPortMouseDown = (e: MouseEvent, fromId: string) => {
    e.stopPropagation();
    const { x, y } = getCanvasXY(e.clientX, e.clientY);
    setDragEdge({ from: fromId, x, y });
  };

  const buildJobSpec = (job: WorkflowJobDef) => {
    const crd = generateJobCRD({
      name: job.name,
      type: job.type,
      headerRole: job.headerRole,
      roles: job.roles,
      roleResources: job.roleResources,
      runScript: job.runScript,
      domain: job.domain,
    });
    return { domain: crd.spec.domain, tasks: crd.spec.tasks };
  };

  const buildWorkflowCRD = () => {
    const jobTemplates = jobs.map((job) => {
      const depNames = edges
        .filter((e) => e.to === job.id)
        .map((e) => jobs.find((j) => j.id === e.from)?.name)
        .filter(Boolean) as string[];
      return {
        name: job.name,
        dependencies: depNames.length > 0 ? depNames : undefined,
        spec: buildJobSpec(job),
      };
    });
    return {
      apiVersion: "rlinf.io/v1alpha1",
      kind: "Workflow",
      metadata: { name: workflowName },
      spec: { jobTemplates },
    };
  };

  const crd = buildWorkflowCRD();
  const yaml = toYaml(crd);

  const handleSubmit = async () => {
    setSubmitting(true);
    setError("");
    try {
      const resp = await fetch("/api/v1/rlinf.io/v1alpha1/workflows", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(crd),
      });
      if (!resp.ok) {
        const body = await resp.text();
        throw new Error(`HTTP ${resp.status}: ${body}`);
      }
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  const activeJob = jobs.find((j) => j.id === activeJobId) ?? jobs[0];
  const effectiveHeader =
    activeJob && activeJob.roles.includes(activeJob.headerRole)
      ? activeJob.headerRole
      : (activeJob?.roles[0] ?? "");
  const steps = zh
    ? ["DAG 编排", "Job 详情", "YAML 预览"]
    : ["DAG Editor", "Job Details", "YAML Preview"];

  const NODE_W = 200;
  const NODE_H = 56;
  const [portPositions, setPortPositions] = useState<
    Record<
      string,
      { in: { x: number; y: number }; out: { x: number; y: number } }
    >
  >({});

  useLayoutEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const cRect = canvas.getBoundingClientRect();
    const next: Record<
      string,
      { in: { x: number; y: number }; out: { x: number; y: number } }
    > = {};
    for (const n of nodes) {
      const el = canvas.querySelector(
        `[data-node-id="${n.id}"]`,
      ) as HTMLElement | null;
      if (!el) continue;
      const r = el.getBoundingClientRect();
      const sx = r.left - cRect.left + canvas.scrollLeft;
      const sy = r.top - cRect.top + canvas.scrollTop;
      const cx = sx + r.width;
      const cy = sy + r.height / 2;
      next[n.id] = { in: { x: sx, y: cy }, out: { x: cx, y: cy } };
    }
    setPortPositions(next);
  }, [nodes]);

  const nodePortPos = (id: string, side: "in" | "out") => {
    const stored = portPositions[id];
    if (stored) return stored[side];
    const n = nodes.find((x) => x.id === id);
    if (!n) return { x: 0, y: 0 };
    const cy = n.y + NODE_H / 2;
    return side === "in"
      ? { x: n.x + 1, y: cy }
      : { x: n.x + NODE_W - 1, y: cy };
  };

  const updateRR = (role: string, field: keyof RoleResource, value: any) => {
    if (!activeJob) return;
    const rr = activeJob.roleResources[role];
    if (!rr) return;
    updateJob(activeJob.id, {
      roleResources: {
        ...activeJob.roleResources,
        [role]: { ...rr, [field]: value },
      },
    });
  };

  const updateRREnv = (
    role: string,
    index: number,
    field: "key" | "value",
    value: string,
  ) => {
    if (!activeJob) return;
    const rr = activeJob.roleResources[role];
    if (!rr) return;
    const envs = rr.envs.map((e, i) =>
      i === index ? { ...e, [field]: value } : e,
    );
    updateRR(role, "envs", envs);
  };

  const addRREnv = (role: string) => {
    if (!activeJob) return;
    const rr = activeJob.roleResources[role];
    if (!rr) return;
    updateRR(role, "envs", [...rr.envs, { key: "", value: "" }]);
  };

  const removeRREnv = (role: string, index: number) => {
    if (!activeJob) return;
    const rr = activeJob.roleResources[role];
    if (!rr) return;
    updateRR(
      role,
      "envs",
      rr.envs.filter((_, i) => i !== index),
    );
  };

  const updateRRMount = (
    role: string,
    index: number,
    field: "objectStorage" | "mountPath" | "type" | "hostPath",
    value: string,
  ) => {
    if (!activeJob) return;
    const rr = activeJob.roleResources[role];
    if (!rr) return;
    const mounts = rr.mounts.map((m, i) =>
      i === index ? { ...m, [field]: value } : m,
    );
    const pvcStorageMap = computePvcStorageMap(role, mounts, activeJob.name);
    updateJob(activeJob.id, {
      roleResources: {
        ...activeJob.roleResources,
        [role]: { ...rr, mounts, pvcStorageMap },
      },
    });
  };

  const addRRMount = (role: string) => {
    if (!activeJob) return;
    const rr = activeJob.roleResources[role];
    if (!rr) return;
    const newMounts = [
      ...rr.mounts,
      {
        type: "storage" as const,
        objectStorage: "",
        mountPath: "",
        hostPath: "",
      },
    ];
    const pvcStorageMap = computePvcStorageMap(role, newMounts, activeJob.name);
    updateJob(activeJob.id, {
      roleResources: {
        ...activeJob.roleResources,
        [role]: { ...rr, mounts: newMounts, pvcStorageMap },
      },
    });
  };

  const removeRRMount = (role: string, index: number) => {
    if (!activeJob) return;
    const rr = activeJob.roleResources[role];
    if (!rr) return;
    const newMounts = rr.mounts.filter((_, i) => i !== index);
    const pvcStorageMap = computePvcStorageMap(role, newMounts, activeJob.name);
    updateJob(activeJob.id, {
      roleResources: {
        ...activeJob.roleResources,
        [role]: { ...rr, mounts: newMounts, pvcStorageMap },
      },
    });
  };

  const onJobTypeChange = (next: JobType) => {
    if (!activeJob) return;
    const newRoles = ROLE_TEMPLATES[next];
    const newRR = makeDefaultRoleResources(
      next,
      newRoles,
      clusterDisplayNames,
      activeJob.name,
    );
    updateJob(activeJob.id, {
      type: next,
      roles: newRoles,
      headerRole: newRoles[0],
      roleResources: newRR,
    });
    if (newRoles.length > 0) setActiveRoleTab(newRoles[0]);
  };

  const addRole = () => {
    if (!activeJob) return;
    const newRole = `Role ${activeJob.roles.length + 1}`;
    const roles = [...activeJob.roles, newRole];
    const rr = { ...activeJob.roleResources };
    rr[newRole] = {
      role: newRole,
      cluster: clusterDisplayNames[0] ?? "",
      nodeSelector: "",
      replicas: 0,
      cpu: "",
      memory: "",
      gpu: "0",
      devices: [],
      image: "",
      prepareScript: "",
      envs: [{ key: "RLARK_TASK_ROLE", value: newRole }],
      mounts: [
        {
          type: "host" as const,
          objectStorage: "",
          mountPath: "/mnt/dataset",
          hostPath: "/host/dataset",
        },
      ],
    };
    updateJob(activeJob.id, { roles, roleResources: rr });
    setActiveRoleTab(newRole);
  };

  const removeRole = (role: string) => {
    if (!activeJob || activeJob.roles.length <= 1) return;
    const roles = activeJob.roles.filter((r) => r !== role);
    const rr = { ...activeJob.roleResources };
    delete rr[role];
    const headerRole =
      activeJob.headerRole === role ? roles[0] : activeJob.headerRole;
    updateJob(activeJob.id, { roles, roleResources: rr, headerRole });
    if (activeRoleTab === role) setActiveRoleTab(roles[0]);
  };

  const renameRole = (old: string, newName: string) => {
    if (!activeJob) return;
    if (!newName.trim()) return;
    const roles = activeJob.roles.map((r) => (r === old ? newName : r));
    const rr: Record<string, RoleResource> = {};
    for (const [k, v] of Object.entries(activeJob.roleResources)) {
      rr[k === old ? newName : k] = k === old ? { ...v, role: newName } : v;
    }
    const headerRole =
      activeJob.headerRole === old ? newName : activeJob.headerRole;
    updateJob(activeJob.id, { roles, roleResources: rr, headerRole });
    if (activeRoleTab === old) setActiveRoleTab(newName);
  };

  return (
    <div
      className="modal-backdrop"
      onMouseDown={(e) =>
        e.target === e.currentTarget && !submitting && onClose()
      }
    >
      <div className="modal create-job-modal" role="dialog" aria-modal="true">
        <div className="modal-head">
          <div>
            <span className="eyebrow">{c.workflows.eyebrow}</span>
            <h2>{c.workflows.createTitle}</h2>
          </div>
          <button
            className="icon-button"
            onClick={onClose}
            aria-label={zh ? "关闭创建工作流" : "Close workflow creator"}
            disabled={submitting}
          >
            ×
          </button>
        </div>
        <div className="modal-body create-job-body">
          <div className="create-stepper">
            {steps.map((label, index) => (
              <button
                key={label}
                className={step >= index + 1 ? "active" : ""}
                onClick={() => setStep(index + 1)}
                disabled={index + 1 > step}
                aria-current={step === index + 1 ? "step" : undefined}
              >
                <span>{index + 1}</span>
                {label}
              </button>
            ))}
          </div>

          {step === 1 && (
            <div className="form-section">
              <div className="form-section-head">
                <strong>{zh ? "工作流名称" : "Workflow Name"}</strong>
              </div>
              <input
                value={workflowName}
                onChange={(e) => setWorkflowName(e.target.value)}
                placeholder="rl-training-pipeline"
                style={{ marginBottom: 12 }}
              />
              <div className="dag-toolbar">
                <button className="secondary-button" onClick={addJobToDAG}>
                  <Plus size={14} />
                  {c.workflows.addJob}
                </button>
              </div>
              <div
                className="dag-canvas"
                ref={canvasRef}
                onMouseMove={onCanvasMouseMove}
                onMouseUp={onCanvasMouseUp}
                onMouseLeave={onCanvasMouseUp}
              >
                <div className="dag-canvas-content">
                  <svg className="dag-svg" width={900} height={500}>
                    <defs>
                      <marker
                        id="dag-arrow"
                        markerWidth="8"
                        markerHeight="8"
                        refX="7"
                        refY="3"
                        orient="auto"
                      >
                        <path d="M0,0 L7,3 L0,6 Z" fill="var(--blue)" />
                      </marker>
                    </defs>
                    {edges.map((e) => {
                      const p1 = nodePortPos(e.from, "out");
                      const p2 = nodePortPos(e.to, "in");
                      const midX = (p1.x + p2.x) / 2;
                      return (
                        <path
                          key={`${e.from}-${e.to}`}
                          d={`M ${p1.x} ${p1.y} C ${midX} ${p1.y}, ${midX} ${p2.y}, ${p2.x} ${p2.y}`}
                          stroke="var(--blue)"
                          strokeWidth="2"
                          fill="none"
                          markerEnd="url(#dag-arrow)"
                          className="dag-edge"
                          onClick={() => removeEdge(e.from, e.to)}
                        />
                      );
                    })}
                    {dragEdge &&
                      (() => {
                        const p1 = nodePortPos(dragEdge.from, "out");
                        return (
                          <path
                            d={`M ${p1.x} ${p1.y} C ${(p1.x + dragEdge.x) / 2} ${p1.y}, ${(p1.x + dragEdge.x) / 2} ${dragEdge.y}, ${dragEdge.x} ${dragEdge.y}`}
                            stroke="var(--blue)"
                            strokeWidth="2"
                            fill="none"
                            strokeDasharray="4 3"
                            className="dag-temp-line"
                          />
                        );
                      })()}
                  </svg>
                  {nodes.map((n) => (
                    <div
                      key={n.id}
                      className={
                        "dag-node" + (activeJobId === n.id ? " selected" : "")
                      }
                      style={{ left: n.x, top: n.y }}
                      data-node-id={n.id}
                      onMouseDown={(e) => onNodeMouseDown(e, n.id)}
                      onClick={() => {
                        setActiveJobId(n.id);
                        const j = jobs.find((j) => j.id === n.id);
                        if (j && j.roles.length > 0)
                          setActiveRoleTab(j.roles[0]);
                      }}
                    >
                      <div
                        className="dag-node-port input"
                        title={zh ? "输入" : "Input"}
                      />
                      <div className="dag-node-body">
                        {editingNodeId === n.id ? (
                          <input
                            className="dag-node-name"
                            value={n.name}
                            autoFocus
                            onClick={(e) => e.stopPropagation()}
                            onMouseDown={(e) => e.stopPropagation()}
                            onChange={(e) => renameJob(n.id, e.target.value)}
                            onBlur={() => {
                              if (!n.name.trim()) renameJob(n.id, "unnamed");
                              setEditingNodeId(null);
                            }}
                            onKeyDown={(e) => {
                              if (e.key === "Enter") setEditingNodeId(null);
                            }}
                          />
                        ) : (
                          <strong
                            onDoubleClick={(e) => {
                              e.stopPropagation();
                              setEditingNodeId(n.id);
                            }}
                          >
                            {n.name}
                          </strong>
                        )}
                        <span className="dag-node-type">
                          {jobs.find((j) => j.id === n.id)?.type ?? ""}
                        </span>
                      </div>
                      <div
                        className="dag-node-port output"
                        onMouseDown={(e) => onPortMouseDown(e, n.id)}
                        title={
                          zh
                            ? "拖拽到目标节点创建依赖"
                            : "Drag to target to create dependency"
                        }
                      />
                      {jobs.length > 1 && (
                        <button
                          className="dag-node-delete"
                          onClick={(e) => {
                            e.stopPropagation();
                            removeJobFromDAG(n.id);
                          }}
                        >
                          <X size={10} />
                        </button>
                      )}
                    </div>
                  ))}
                </div>
              </div>
              <div className="dag-hint">
                {zh
                  ? "拖拽节点右侧端口到目标节点创建依赖，点击连线可删除。点击节点切换 Job 详情。"
                  : "Drag output port (right) to target node to create dependency. Click edge to remove. Click node to edit."}
              </div>
            </div>
          )}

          {step === 2 && activeJob && (
            <>
              <div className="role-config-tabs" style={{ marginBottom: 12 }}>
                {jobs.map((job) => (
                  <button
                    key={job.id}
                    className={activeJobId === job.id ? "active" : ""}
                    onClick={() => {
                      setActiveJobId(job.id);
                      if (job.roles.length > 0) setActiveRoleTab(job.roles[0]);
                    }}
                  >
                    {job.name}
                  </button>
                ))}
              </div>
              <div className="form-section">
                <div className="form-row">
                  <label>
                    {c.workflows.jobName}
                    <input
                      value={activeJob.name}
                      onChange={(e) => renameJob(activeJob.id, e.target.value)}
                    />
                  </label>
                  <label>
                    {zh ? "任务类型" : "Job Type"}
                    <select
                      value={activeJob.type}
                      onChange={(e) =>
                        onJobTypeChange(e.target.value as JobType)
                      }
                    >
                      {(
                        [
                          "RL",
                          "DataCollection",
                          "Evaluation",
                          "Custom",
                        ] as JobType[]
                      ).map((t) => (
                        <option key={t} value={t}>
                          {c.jobType[t]}
                        </option>
                      ))}
                    </select>
                  </label>
                </div>
              </div>
              <div className="form-section">
                <div className="form-section-head">
                  <strong>{zh ? "角色管理" : "Roles"}</strong>
                  <button
                    className="secondary-button"
                    style={{ padding: "2px 10px", fontSize: 12 }}
                    onClick={addRole}
                  >
                    <Plus size={13} />
                    {zh ? "添加角色" : "Add Role"}
                  </button>
                </div>
                <div className="role-edit-list">
                  {activeJob.roles.map((role) => (
                    <div
                      key={role}
                      className={`role-edit-row ${effectiveHeader === role ? "active" : ""}`}
                      onClick={() =>
                        updateJob(activeJob.id, { headerRole: role })
                      }
                    >
                      <Check size={14} />
                      <RoleNameInput role={role} onRename={renameRole} />
                      <small>
                        {effectiveHeader === role ? "Header" : "Worker"}
                      </small>
                      {activeJob.roles.length > 1 && (
                        <button
                          className="icon-button danger"
                          onClick={(e) => {
                            e.stopPropagation();
                            removeRole(role);
                          }}
                        >
                          <X size={14} />
                        </button>
                      )}
                    </div>
                  ))}
                </div>
              </div>
              {activeJob.roles.length > 0 && (
                <>
                  <div className="role-config-tabs">
                    {activeJob.roles.map((role) => (
                      <button
                        key={role}
                        className={activeRoleTab === role ? "active" : ""}
                        onClick={() => setActiveRoleTab(role)}
                      >
                        {role}
                        {effectiveHeader === role && (
                          <span
                            className="role-chip"
                            style={{ marginLeft: 6, fontSize: 10 }}
                          >
                            Header
                          </span>
                        )}
                      </button>
                    ))}
                  </div>
                  {(() => {
                    const role = activeRoleTab || activeJob.roles[0];
                    if (!role) return null;
                    const rr = activeJob.roleResources[role];
                    if (!rr) return null;
                    return (
                      <div className="role-resource-card" key={role}>
                        <div className="form-section-head">
                          <strong>{role}</strong>
                          {effectiveHeader === role && (
                            <span
                              className="role-chip"
                              style={{
                                background: "#f3eefe",
                                color: "var(--blue)",
                              }}
                            >
                              Header
                            </span>
                          )}
                        </div>
                        <div className="form-row">
                          <label>
                            {zh ? "集群" : "Cluster"}
                            <select
                              value={rr.cluster}
                              onChange={(e) => {
                                const newCluster = e.target.value;
                                updateRR(role, "cluster", newCluster);
                                if (
                                  rr.mounts.some((m) => m.type === "storage")
                                ) {
                                  fetchStorageClasses(newCluster);
                                }
                              }}
                            >
                              {availableClusters.map((cl) => (
                                <option key={cl.id} value={cl.id}>
                                  {cl.name}
                                </option>
                              ))}
                            </select>
                          </label>
                        </div>
                        <div className="form-section" style={{ marginTop: 12 }}>
                          <div className="form-section-head">
                            <small>{zh ? "节点选择" : "Node Selector"}</small>
                          </div>
                          <NodeSelectorPicker
                            value={rr.nodeSelector}
                            onChange={(v) => updateRR(role, "nodeSelector", v)}
                            zh={zh}
                            cluster={rr.cluster}
                            nodes={allNodes}
                            loading={nodesLoading}
                            onMatchedCount={(n) =>
                              updateRR(role, "replicas", n)
                            }
                          />
                        </div>
                        <div
                          className="resource-input-row"
                          style={{ gridTemplateColumns: "1fr 1fr" }}
                        >
                          <label>
                            {zh ? "已选节点数" : "Number of selected nodes"}
                            <input
                              type="number"
                              value={rr.replicas}
                              readOnly
                              style={{ opacity: 0.6 }}
                            />
                          </label>
                          <label>
                            GPU
                            <input
                              value={rr.gpu}
                              onChange={(e) =>
                                updateRR(role, "gpu", e.target.value)
                              }
                              placeholder="4"
                            />
                          </label>
                        </div>
                        {(() => {
                          const clusterNodes = allNodes.filter((n) => {
                            const ns = n.metadata.namespace ?? "";
                            return ns === rr.cluster;
                          });
                          const deviceSet = new Set<string>();
                          for (const n of clusterNodes) {
                            const alloc = n.status?.allocatable ?? {};
                            for (const k of Object.keys(alloc)) {
                              if (k.startsWith("rlinf.io/")) deviceSet.add(k);
                            }
                          }
                          const availableDevices = Array.from(deviceSet).sort();
                          return (rr.devices ?? []).length > 0 ||
                            availableDevices.length > 0 ? (
                            <div
                              className="form-section"
                              style={{ marginTop: 12 }}
                            >
                              <div className="form-section-head">
                                <small>
                                  {zh ? "设备资源" : "Device Resources"}
                                </small>
                                {availableDevices.length > 0 && (
                                  <button
                                    type="button"
                                    className="secondary-button"
                                    style={{
                                      padding: "2px 10px",
                                      fontSize: 12,
                                    }}
                                    onClick={() => {
                                      const next = [
                                        ...(rr.devices ?? []),
                                        {
                                          name: availableDevices[0] ?? "",
                                          quantity: "1",
                                        },
                                      ];
                                      updateRR(role, "devices", next);
                                    }}
                                  >
                                    <Plus size={13} />
                                    {zh ? "添加" : "Add"}
                                  </button>
                                )}
                              </div>
                              {(rr.devices ?? []).map((dev, di) => (
                                <div key={di} className="device-row">
                                  <select
                                    value={dev.name}
                                    onChange={(e) => {
                                      const next = [...(rr.devices ?? [])];
                                      next[di] = {
                                        ...next[di],
                                        name: e.target.value,
                                      };
                                      updateRR(role, "devices", next);
                                    }}
                                  >
                                    <option value="">
                                      {zh ? "选择设备" : "Select device"}
                                    </option>
                                    {availableDevices.map((d) => (
                                      <option key={d} value={d}>
                                        {d}
                                      </option>
                                    ))}
                                    {dev.name &&
                                      !availableDevices.includes(dev.name) && (
                                        <option value={dev.name}>
                                          {dev.name}
                                        </option>
                                      )}
                                  </select>
                                  <input
                                    value={dev.quantity}
                                    onChange={(e) => {
                                      const next = [...(rr.devices ?? [])];
                                      next[di] = {
                                        ...next[di],
                                        quantity: e.target.value,
                                      };
                                      updateRR(role, "devices", next);
                                    }}
                                    placeholder="1"
                                  />
                                  <button
                                    type="button"
                                    className="icon-button danger"
                                    onClick={() => {
                                      const next = (rr.devices ?? []).filter(
                                        (_, j) => j !== di,
                                      );
                                      updateRR(role, "devices", next);
                                    }}
                                  >
                                    <X size={14} />
                                  </button>
                                </div>
                              ))}
                            </div>
                          ) : null;
                        })()}
                        <div className="form-section" style={{ marginTop: 12 }}>
                          <div className="form-section-head">
                            <small>{zh ? "镜像" : "Image"}</small>
                          </div>
                          <input
                            value={rr.image}
                            onChange={(e) =>
                              updateRR(role, "image", e.target.value)
                            }
                            placeholder=""
                          />
                        </div>
                        <div className="form-section" style={{ marginTop: 12 }}>
                          <div className="form-section-head">
                            <small>
                              {zh
                                ? "准备脚本 (Ray 启动前)"
                                : "Prepare Script (before Ray)"}
                            </small>
                          </div>
                          <CodeEditorField
                            value={rr.prepareScript}
                            onChange={(e) =>
                              updateRR(role, "prepareScript", e.target.value)
                            }
                            minHeight={92}
                            label={`${role}/prepare.sh`}
                          />
                        </div>
                        <div className="form-section" style={{ marginTop: 12 }}>
                          <div className="form-section-head">
                            <small>
                              {zh ? "环境变量" : "Environment Variables"}
                            </small>
                            <button
                              className="secondary-button"
                              onClick={() => addRREnv(role)}
                            >
                              <Plus size={14} />
                              {zh ? "添加" : "Add"}
                            </button>
                          </div>
                          {rr.envs.map((env, index) => (
                            <div className="env-row" key={index}>
                              <input
                                value={env.key}
                                onChange={(e) =>
                                  updateRREnv(
                                    role,
                                    index,
                                    "key",
                                    e.target.value,
                                  )
                                }
                              />
                              <input
                                value={env.value}
                                onChange={(e) =>
                                  updateRREnv(
                                    role,
                                    index,
                                    "value",
                                    e.target.value,
                                  )
                                }
                              />
                              <button
                                className="icon-button danger"
                                onClick={() => removeRREnv(role, index)}
                              >
                                <X size={14} />
                              </button>
                            </div>
                          ))}
                        </div>
                        <div className="form-section" style={{ marginTop: 12 }}>
                          <div className="form-section-head">
                            <small>{zh ? "存储挂载" : "Volume Mounts"}</small>
                            <button
                              className="secondary-button"
                              onClick={() => addRRMount(role)}
                            >
                              <Plus size={14} />
                              {zh ? "添加" : "Add"}
                            </button>
                          </div>
                          {rr.mounts.map((mount, index) => (
                            <div className="mount-row" key={index}>
                              <select
                                value={mount.type}
                                onChange={(e) => {
                                  updateRRMount(
                                    role,
                                    index,
                                    "type",
                                    e.target.value,
                                  );
                                  if (e.target.value === "storage") {
                                    fetchStorageClasses(rr.cluster);
                                  }
                                }}
                              >
                                <option value="storage">
                                  {zh ? "对象存储" : "Object storage"}
                                </option>
                                <option value="host">
                                  {zh ? "主机目录" : "Host directory"}
                                </option>
                              </select>
                              <label className="mount-field-box">
                                <span>
                                  {mount.type === "storage"
                                    ? zh
                                      ? "对象存储"
                                      : "Object storage"
                                    : zh
                                      ? "主机目录"
                                      : "Host directory"}
                                </span>
                                {mount.type === "storage" ? (
                                  <select
                                    value={mount.objectStorage}
                                    onChange={(e) =>
                                      updateRRMount(
                                        role,
                                        index,
                                        "objectStorage",
                                        e.target.value,
                                      )
                                    }
                                  >
                                    <option value="">
                                      {storageClassFetched &&
                                      storageClasses.length === 0
                                        ? zh
                                          ? "无可用存储类"
                                          : "No storage classes"
                                        : zh
                                          ? "选择存储类"
                                          : "Select storage class"}
                                    </option>
                                    {storageClasses.map((sc) => (
                                      <option key={sc.name} value={sc.name}>
                                        {sc.name}
                                      </option>
                                    ))}
                                  </select>
                                ) : (
                                  <input
                                    value={mount.hostPath}
                                    onChange={(e) =>
                                      updateRRMount(
                                        role,
                                        index,
                                        "hostPath",
                                        e.target.value,
                                      )
                                    }
                                    placeholder="/host/path"
                                  />
                                )}
                              </label>
                              <label className="mount-field-box">
                                <span>
                                  {zh ? "挂载到 Worker" : "Mount in worker"}
                                </span>
                                <input
                                  value={mount.mountPath}
                                  onChange={(e) =>
                                    updateRRMount(
                                      role,
                                      index,
                                      "mountPath",
                                      e.target.value,
                                    )
                                  }
                                  placeholder="/mnt/data"
                                />
                              </label>
                              <button
                                className="icon-button danger"
                                onClick={() => removeRRMount(role, index)}
                                title={zh ? "删除" : "Delete"}
                              >
                                <Trash2 size={14} />
                              </button>
                            </div>
                          ))}
                        </div>
                      </div>
                    );
                  })()}
                </>
              )}
              <div className="form-section">
                <div className="form-section-head">
                  <small>{zh ? "选择 Head 节点" : "Select Head"}</small>
                </div>
                <div className="role-template selectable">
                  {activeJob.roles.map((role) => (
                    <button
                      key={role}
                      className={effectiveHeader === role ? "active" : ""}
                      onClick={() =>
                        updateJob(activeJob.id, { headerRole: role })
                      }
                    >
                      <Check size={13} />
                      {role}
                      <small>
                        {effectiveHeader === role ? "Header" : "Worker"}
                      </small>
                    </button>
                  ))}
                </div>
              </div>
              <div className="form-section">
                <div className="form-section-head">
                  <small>
                    {zh
                      ? "跨集群网络域 (可选)"
                      : "Cross-cluster Network Domain (optional)"}
                  </small>
                </div>
                <select
                  value={activeJob.domain}
                  onChange={(e) =>
                    updateJob(activeJob.id, { domain: e.target.value })
                  }
                >
                  <option value="">
                    {zh ? "不使用跨集群网络" : "No cross-cluster network"}
                  </option>
                  {domains.map((d) => (
                    <option key={d.name} value={d.name}>
                      {d.name} ({d.cidr})
                    </option>
                  ))}
                </select>
              </div>
              <div className="form-section">
                <div className="form-section-head">
                  <small>{zh ? "运行脚本" : "Run Script"}</small>
                </div>
                <CodeEditorField
                  value={activeJob.runScript}
                  onChange={(e) =>
                    updateJob(activeJob.id, { runScript: e.target.value })
                  }
                  minHeight={112}
                  label="run.sh"
                />
              </div>
            </>
          )}

          {step === 3 && (
            <div className="yaml-preview">
              <div className="yaml-preview-head">
                <span>YAML</span>
                <button
                  className="secondary-button"
                  onClick={() => navigator.clipboard?.writeText(yaml)}
                >
                  {c.api.copy}
                </button>
              </div>
              <pre>{yaml}</pre>
            </div>
          )}

          {error && (
            <div className="cert-error" style={{ marginBottom: 12 }}>
              {error}
            </div>
          )}
          <div className="step-actions">
            <button
              className="secondary-button"
              onClick={() => (step > 1 ? setStep(step - 1) : onClose())}
            >
              {step > 1 ? (zh ? "上一步" : "Back") : zh ? "取消" : "Cancel"}
            </button>
            {step < 3 ? (
              <button
                className="primary-button"
                onClick={() => {
                  if (step === 2 && activeJob) {
                    const roles = activeJob.roles;
                    const currentRole = activeRoleTab || roles[0];
                    const roleIdx = roles.indexOf(currentRole);
                    if (roleIdx >= 0 && roleIdx < roles.length - 1) {
                      setActiveRoleTab(roles[roleIdx + 1]);
                      return;
                    }
                    const jobIdx = jobs.findIndex((j) => j.id === activeJobId);
                    if (jobIdx >= 0 && jobIdx < jobs.length - 1) {
                      const nextJob = jobs[jobIdx + 1];
                      setActiveJobId(nextJob.id);
                      if (nextJob.roles.length > 0) {
                        setActiveRoleTab(nextJob.roles[0]);
                      }
                      return;
                    }
                  }
                  setStep(step + 1);
                }}
                disabled={step === 1 && !workflowName.trim()}
              >
                {step === 2 && activeJob
                  ? (() => {
                      const roles = activeJob.roles;
                      const currentRole = activeRoleTab || roles[0];
                      const roleIdx = roles.indexOf(currentRole);
                      const isLastRole =
                        roleIdx < 0 || roleIdx === roles.length - 1;
                      const jobIdx = jobs.findIndex(
                        (j) => j.id === activeJobId,
                      );
                      const isLastJob =
                        jobIdx < 0 || jobIdx === jobs.length - 1;
                      if (!isLastRole) return zh ? "下一个角色" : "Next Role";
                      if (!isLastJob) return zh ? "下一个任务" : "Next Job";
                      return zh ? "下一步" : "Next";
                    })()
                  : zh
                    ? "下一步"
                    : "Next"}
              </button>
            ) : (
              <button
                className="primary-button"
                disabled={submitting}
                onClick={handleSubmit}
              >
                {submitting
                  ? zh
                    ? "提交中…"
                    : "Submitting…"
                  : zh
                    ? "创建"
                    : "Create"}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
