import type { CSSProperties, PointerEvent as ReactPointerEvent } from "react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import {
  Check,
  ChevronRight,
  Copy,
  Download,
  ExternalLink,
  KeyRound,
  MoreVertical,
  Network,
  Pencil,
  Play,
  Plus,
  Square,
  Search,
  TerminalSquare,
  Trash2,
  Workflow,
  Zap,
} from "lucide-react";
import {
  type Job,
  type Phase,
  type PodInfo,
  type Worker as WorkerItem,
} from "../data";
import type { Copy as CopyType } from "../i18n";
import type { CRDJob } from "../types";
import { useAutoRefresh } from "../hooks";
import { crdToJob } from "../utils/crd";
import { formatChinaDateTime } from "../utils/time";
import {
  compareSortValues,
  PageToolbar,
  Pagination,
  SortButton,
  StatusBadge,
  type SortDirection,
} from "../components/shared";

function taskResourceName(jobName: string, taskName: string) {
  return `${jobName}-${taskName.toLowerCase().replace(/\s+/g, "-")}`
    .toLowerCase()
    .replace(/\s+/g, "-");
}

async function copyText(value: string) {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {
    // Fall through for browsers that block Clipboard API on local HTTP pages.
  }

  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.focus({ preventScroll: true });
  textarea.select();
  textarea.setSelectionRange(0, value.length);
  const copied = document.execCommand("copy");
  textarea.remove();
  return copied;
}

export function JobsPage({
  copy: c,
  isMockMode,
  selectedName,
  onSelect,
  onCreate,
  onClone,
  onEdit,
}: {
  copy: CopyType;
  isMockMode: boolean;
  selectedName: string;
  onSelect: (name?: string) => void;
  onCreate: () => void;
  onClone: (job: Job) => void;
  onEdit: (job: Job) => void;
}) {
  const zh = c.nav.overview === "总览";
  const [query, setQuery] = useState("");
  const [phaseFilter, setPhaseFilter] = useState<"All" | Phase>("All");
  const [realJobs, setRealJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [sort, setSort] = useState<{
    key:
      | "id"
      | "type"
      | "phase"
      | "workers"
      | "roleCount"
      | "submittedAt"
      | "stoppedAt";
    direction: SortDirection;
  }>({ key: "submittedAt", direction: "desc" });
  const toggleSort = (key: typeof sort.key) =>
    setSort((current) => ({
      key,
      direction:
        current.key === key && current.direction === "asc" ? "desc" : "asc",
    }));

  const fetchJobs = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    try {
      const resp = await fetch("/api/v1/rlinf.io/v1alpha1/jobs");
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      const items: CRDJob[] = data.items ?? [];
      setRealJobs(items.map(crdToJob));
    } catch (e) {
      setRealJobs([]);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchJobs, 10000);

  const handleDelete = async (job: Job) => {
    if (
      !confirm(
        zh ? `确定删除任务 "${job.name}" 吗?` : `Delete job "${job.name}"?`,
      )
    )
      return;
    try {
      const resp = await fetch(`/api/v1/rlinf.io/v1alpha1/jobs/${job.name}`, {
        method: "DELETE",
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      setRealJobs((prev) => prev.filter((j) => j.id !== job.id));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const handleToggleStop = async (job: Job) => {
    const stopped = !job.stopped;
    try {
      const resp = await fetch(`/api/v1/rlinf.io/v1alpha1/jobs/${job.name}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/merge-patch+json" },
        body: JSON.stringify({ spec: { stopped } }),
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      setRealJobs((prev) =>
        prev.map((j) => (j.id === job.id ? { ...j, stopped } : j)),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const allJobs = realJobs;
  const filtered = allJobs.filter((j) => {
    const queryHit = `${j.id} ${j.displayName} ${j.type}`
      .toLowerCase()
      .includes(query.toLowerCase());
    const phaseHit = phaseFilter === "All" || j.phase === phaseFilter;
    return queryHit && phaseHit;
  });
  const sortedJobs = useMemo(
    () =>
      [...filtered].sort((a, b) => {
        const value = (job: Job) => {
          if (sort.key === "workers") return job.progress;
          return job[sort.key];
        };
        return compareSortValues(
          value(a),
          value(b),
          sort.direction,
          zh ? "zh-CN" : "en",
        );
      }),
    [filtered, sort, zh],
  );
  const totalPages = Math.max(1, Math.ceil(sortedJobs.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pagedJobs = sortedJobs.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );

  useEffect(() => setPage(1), [query, phaseFilter, pageSize]);
  useEffect(() => {
    if (page > totalPages) setPage(totalPages);
  }, [page, totalPages]);

  const selected =
    selectedName && allJobs.length > 0
      ? (allJobs.find((j) => j.name === selectedName) ?? null)
      : null;

  if (selected) {
    return (
      <JobDetailPage
        job={selected}
        copy={c}
        isMockMode={isMockMode}
        onBack={() => onSelect(undefined)}
        onClone={() => onClone(selected)}
      />
    );
  }

  return (
    <div className="page-content resource-page jobs-list-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">{c.jobs.eyebrow}</span>
          <h2>{c.jobs.title}</h2>
          <p>{c.jobs.desc}</p>
        </div>
        <button className="primary-button" onClick={onCreate}>
          <Plus size={17} />
          {c.common.createJob}
        </button>
      </div>
      <PageToolbar
        placeholder={c.jobs.search}
        value={query}
        onChange={setQuery}
        count={filtered.length}
        copy={c}
        onRefresh={() => fetchJobs()}
        filterValue={phaseFilter}
        onFilterChange={(value) => setPhaseFilter(value as "All" | Phase)}
        filterOptions={[
          { value: "All", label: zh ? "全部状态" : "All statuses" },
          { value: "Running", label: c.status.Running },
          { value: "Pending", label: c.status.Pending },
          { value: "Succeeded", label: c.status.Succeeded },
          { value: "Failed", label: c.status.Failed },
          { value: "Stopped", label: c.status.Stopped },
        ]}
      />
      {error && (
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}
      <div className="table-panel jobs-table-panel">
        <table>
          <thead>
            <tr>
              <th>
                <SortButton
                  label={zh ? "任务 ID" : "Job ID"}
                  active={sort.key === "id"}
                  direction={sort.direction}
                  onClick={() => toggleSort("id")}
                />
              </th>
              <th>
                <SortButton
                  label={zh ? "任务类型" : "Type"}
                  active={sort.key === "type"}
                  direction={sort.direction}
                  onClick={() => toggleSort("type")}
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
                  label="Worker"
                  active={sort.key === "workers"}
                  direction={sort.direction}
                  onClick={() => toggleSort("workers")}
                />
              </th>
              <th>
                <SortButton
                  label={zh ? "角色数量" : "Roles"}
                  active={sort.key === "roleCount"}
                  direction={sort.direction}
                  onClick={() => toggleSort("roleCount")}
                />
              </th>
              <th>
                <SortButton
                  label={zh ? "创建时间" : "Created"}
                  active={sort.key === "submittedAt"}
                  direction={sort.direction}
                  onClick={() => toggleSort("submittedAt")}
                />
              </th>
              <th>
                <SortButton
                  label={zh ? "停止时间" : "Stopped"}
                  active={sort.key === "stoppedAt"}
                  direction={sort.direction}
                  onClick={() => toggleSort("stoppedAt")}
                />
              </th>
              <th>{zh ? "操作" : "Actions"}</th>
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 && !loading && (
              <tr>
                <td colSpan={8}>
                  <div className="table-empty-state">
                    <span>
                      <Workflow size={22} />
                    </span>
                    <strong>{zh ? "还没有任务" : "No jobs yet"}</strong>
                    <small>
                      {zh
                        ? "创建强化学习、数据采集、评测或自定义任务。"
                        : "Create an RL, data collection, evaluation, or custom job."}
                    </small>
                    <button className="secondary-button" onClick={onCreate}>
                      <Plus size={15} />
                      {c.common.createJob}
                    </button>
                  </div>
                </td>
              </tr>
            )}
            {pagedJobs.map((job) => (
              <tr key={job.id}>
                <td>
                  <button
                    className="link-cell"
                    onClick={() => onSelect(job.id)}
                  >
                    <strong>{job.id}</strong>
                    {job.displayName !== job.id && (
                      <small>{job.displayName}</small>
                    )}
                  </button>
                </td>
                <td>
                  <span className="role-chip">{c.jobType[job.type]}</span>
                </td>
                <td>
                  <StatusBadge phase={job.phase} copy={c} />
                </td>
                <td>
                  <span className="inline-progress">
                    <i>
                      <b style={{ width: job.progress + "%" }} />
                    </i>
                    {job.runningWorkers}/{job.workers}
                  </span>
                </td>
                <td>{job.roleCount}</td>
                <td>{formatTaskTime(job.submittedAt)}</td>
                <td>{formatTaskTime(job.stoppedAt)}</td>
                <td>
                  <JobActionMenu
                    job={job}
                    zh={zh}
                    onEdit={() => onEdit(job)}
                    onClone={() => onClone(job)}
                    onDelete={() => handleDelete(job)}
                    onToggleStop={() => handleToggleStop(job)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <Pagination
        page={currentPage}
        pageSize={pageSize}
        total={filtered.length}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
        zh={zh}
      />
    </div>
  );
}

function JobActionMenu({
  job,
  zh,
  onEdit,
  onClone,
  onDelete,
  onToggleStop,
}: {
  job: Job;
  zh: boolean;
  onEdit: () => void;
  onClone: () => void;
  onDelete: () => void;
  onToggleStop: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [menuStyle, setMenuStyle] = useState<CSSProperties>({});
  const ref = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    if (!open || !ref.current) return;
    const btn = ref.current.querySelector("button");
    if (!btn) return;
    const rect = btn.getBoundingClientRect();
    const dropdownEl = ref.current.querySelector(
      ".action-dropdown",
    ) as HTMLElement | null;
    const ddHeight = dropdownEl?.offsetHeight ?? 200;
    const spaceBelow = window.innerHeight - rect.bottom;
    const dropUp = spaceBelow < ddHeight + 8;
    setMenuStyle(
      dropUp
        ? {
            position: "fixed",
            left: rect.left,
            bottom: window.innerHeight - rect.top + 4,
            right: "auto",
            top: "auto",
          }
        : {
            position: "fixed",
            left: rect.left,
            top: rect.bottom + 4,
            right: "auto",
            bottom: "auto",
          },
    );
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handlePointer = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", handlePointer);
    document.addEventListener("keydown", handleKey);
    return () => {
      document.removeEventListener("pointerdown", handlePointer);
      document.removeEventListener("keydown", handleKey);
    };
  }, [open]);

  const isStopped = job.stopped || job.phase === "Stopped";
  const isTerminal = job.phase === "Succeeded";

  return (
    <div className="row-actions" ref={ref} style={{ position: "relative" }}>
      <button
        className="icon-button"
        onClick={() => setOpen((v) => !v)}
        title={zh ? "操作" : "Actions"}
        aria-expanded={open}
      >
        <MoreVertical size={16} />
      </button>
      {open && (
        <>
          <div className="action-dropdown" style={menuStyle}>
            <button
              className="action-dropdown-item"
              onClick={() => {
                setOpen(false);
                onEdit();
              }}
            >
              <Pencil size={14} />
              {zh ? "编辑" : "Edit"}
            </button>
            <button
              className="action-dropdown-item"
              onClick={() => {
                setOpen(false);
                onClone();
              }}
            >
              <Copy size={14} />
              {zh ? "复制" : "Clone"}
            </button>
            <button
              className="action-dropdown-item"
              disabled={isTerminal}
              onClick={() => {
                if (isTerminal) return;
                setOpen(false);
                onToggleStop();
              }}
              title={
                isTerminal
                  ? zh
                    ? "终态任务无法操作"
                    : "Terminal job cannot be toggled"
                  : ""
              }
            >
              {isStopped ? (
                <>
                  <Play size={14} />
                  {zh ? "启动" : "Start"}
                </>
              ) : (
                <>
                  <Square size={14} />
                  {zh ? "停止" : "Stop"}
                </>
              )}
            </button>
            <button
              className="action-dropdown-item danger"
              onClick={() => {
                setOpen(false);
                onDelete();
              }}
            >
              <Trash2 size={14} />
              {zh ? "删除" : "Delete"}
            </button>
          </div>
        </>
      )}
    </div>
  );
}

export function JobDetailPage({
  job,
  copy: c,
  isMockMode,
  onBack,
  onClone,
}: {
  job: Job;
  copy: CopyType;
  isMockMode: boolean;
  onBack: () => void;
  onClone: () => void;
}) {
  const zh = c.nav.overview === "总览";
  const [activeTab, setActiveTab] = useState<"workers" | "logs" | "metrics">(
    "workers",
  );
  const [taskNodes, setTaskNodes] = useState<Record<string, string>>({});
  const [tensorBoardProxy, setTensorBoardProxy] = useState<string>("");
  const [pods, setPods] = useState<PodInfo[]>([]);
  const [domainIPMap, setDomainIPMap] = useState<Record<string, string>>({});
  const [podLogs, setPodLogs] = useState<
    Array<{
      taskName: string;
      podName: string;
      phase: string;
      node: string;
      logs: string;
    }>
  >([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsError, setLogsError] = useState<string | null>(null);
  const [workerRoleFilter, setWorkerRoleFilter] = useState("All");
  const [workerPage, setWorkerPage] = useState(1);
  const [workerSort, setWorkerSort] = useState<{
    key: "name" | "role" | "node" | "kind" | "ip" | "createdAt" | "phase";
    direction: SortDirection;
  }>({ key: "name", direction: "asc" });
  const workerTableRef = useRef<HTMLDivElement>(null);
  const workerTableDrag = useRef({ active: false, x: 0, scrollLeft: 0 });
  const [workerTableDragging, setWorkerTableDragging] = useState(false);
  const [logRoleFilter, setLogRoleFilter] = useState("All");
  const [logWorkerFilter, setLogWorkerFilter] = useState("All");
  const [logQuery, setLogQuery] = useState("");
  const [logRange, setLogRange] = useState("1h");
  const [logStreamEnabled, setLogStreamEnabled] = useState(false);

  useAutoRefresh(
    async () => {
      const labelSelector = `rlinf.io/job=${job.name}`;
      const resp = await fetch(
        `/api/v1/rlinf.io/v1alpha1/tasks?labelSelector=${encodeURIComponent(labelSelector)}`,
      );
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      const items = data.items ?? [];
      const nodeMap: Record<string, string> = {};
      let tbProxy = "";
      for (const item of items) {
        const taskName = item.metadata?.name ?? "";
        const observedNodes = item.status?.observedNodes ?? [];
        nodeMap[taskName] = observedNodes.join(", ") || "—";
        if (item.status?.tensorBoardProxy) {
          tbProxy = item.status.tensorBoardProxy;
        }
      }
      setTaskNodes(nodeMap);
      setTensorBoardProxy(tbProxy);
    },
    10000,
    [job.name],
  );

  const workerTaskNames = job.taskStatuses.map((ts) => {
    const childTaskName = ts.name.toLowerCase().replace(/\s+/g, "-");
    return `${job.name}-${childTaskName}`.toLowerCase().replace(/\s+/g, "-");
  });
  const workerTaskNamesKey = workerTaskNames.join(",");

  useEffect(() => {
    if (!workerTaskNamesKey) {
      setPods([]);
      setDomainIPMap({});
      return;
    }
    let cancelled = false;
    const labelSelector = `rlark.io/task-name in (${workerTaskNamesKey})`;

    const domainsPromise = fetch(`/api/v1/rlinf.io/v1alpha1/domains`).then(
      (resp) =>
        resp.ok
          ? resp.json()
          : Promise.reject(new Error(`HTTP ${resp.status}`)),
    );

    const podsPromise = fetch(
      `/api/v1/rlinf.io/v1alpha1/pods?labelSelector=${encodeURIComponent(labelSelector)}`,
    ).then((resp) =>
      resp.ok ? resp.json() : Promise.reject(new Error(`HTTP ${resp.status}`)),
    );

    Promise.all([podsPromise, domainsPromise])
      .then(([podData, domainData]) => {
        if (cancelled) return;
        const podItems = podData.items ?? [];
        const uniquePods = new Map<string, PodInfo>();
        for (const item of podItems) {
          const pod: PodInfo = {
            name: item.metadata?.name ?? "",
            namespace: item.metadata?.namespace ?? "",
            taskName: item.spec?.taskName ?? "",
            taskNamespace: item.spec?.taskNamespace ?? "",
            podName: item.spec?.podName ?? "",
            podNamespace: item.spec?.podNamespace ?? "",
            domain: item.spec?.domain ?? "",
            phase: item.status?.phase ?? "Pending",
            node: item.status?.node ?? "",
            ip: item.status?.ip ?? "",
            message: item.status?.message ?? "",
          };
          // The control plane can briefly return duplicate/history Pod CRs for
          // one runtime Worker. Keep one row per actual Pod identity.
          const identity = `${pod.namespace}/${pod.podNamespace}/${pod.podName || pod.name}`;
          uniquePods.set(identity, pod);
        }
        const podList = [...uniquePods.values()];
        setPods(podList);

        const domainItems = domainData.items ?? [];
        const ipMap: Record<string, string> = {};
        for (const d of domainItems) {
          const allocs = d.status?.ipAllocations ?? [];
          for (const a of allocs) {
            if (a.pod) ipMap[a.pod] = a.ip;
          }
        }
        setDomainIPMap(ipMap);
      })
      .catch(() => {
        if (cancelled) return;
        setPods([]);
        setDomainIPMap({});
      });
    return () => {
      cancelled = true;
    };
  }, [workerTaskNamesKey]);

  const fetchLogs = async (isInitial = true) => {
    if (activeTab !== "logs") return;
    if (!isInitial && !logStreamEnabled) return;
    if (isInitial) setLogsLoading(true);
    setLogsError(null);
    try {
      const resp = await fetch(
        `/api/v1/rlinf.io/v1alpha1/jobs/${encodeURIComponent(job.name)}/logs`,
      );
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      setPodLogs(Array.isArray(data.pods) ? data.pods : []);
    } catch (e) {
      setPodLogs([]);
      setLogsError(e instanceof Error ? e.message : String(e));
    } finally {
      setLogsLoading(false);
    }
  };

  useAutoRefresh(fetchLogs, 5000, [activeTab, job.name, logStreamEnabled]);

  const fallbackWorkers: WorkerItem[] =
    job.taskStatuses.length > 0
      ? job.taskStatuses.map((ts, i) => {
          const childTaskName = ts.name.toLowerCase().replace(/\s+/g, "-");
          const jobChildName = `${job.name}-${childTaskName}`
            .toLowerCase()
            .replace(/\s+/g, "-");
          return {
            id: `${job.id}-${i}`,
            name: ts.name,
            jobId: job.id,
            role: ts.name,
            node:
              taskNodes[jobChildName] ??
              taskNodes[ts.name] ??
              ts.observedNodes?.join(", ") ??
              "—",
            phase: (ts.phase || "Pending") as Phase,
            cpu: Math.min(96, 38 + i * 9 + (ts.phase === "Running" ? 12 : 0)),
            memory: Math.min(92, 42 + i * 7 + (ts.phase === "Running" ? 8 : 0)),
            gpu: job.resources.find((item) => item.role === ts.name)?.gpu,
            latency: i % 2 === 0 ? `${42 + i * 7} ms` : undefined,
            fps: i % 2 === 1 ? 24 + i * 3 : undefined,
            logs: ts.message
              ? [ts.message]
              : [
                  `${ts.name}: worker state synced`,
                  `${ts.name}: waiting for runtime heartbeat`,
                ],
          };
        })
      : [];
  const resourceForTask = (taskName: string) =>
    job.resources.find(
      (resource) => taskResourceName(job.name, resource.role) === taskName,
    );
  const jobWorkers: WorkerItem[] =
    pods.length > 0
      ? pods.map((pod, index) => {
          const resource = resourceForTask(pod.taskName);
          const role = resource?.role ?? pod.taskName;
          return {
            id: `${pod.namespace}/${pod.podNamespace}/${pod.podName}`,
            name: pod.podName || `${role}-${index}`,
            jobId: job.id,
            role,
            node: pod.node || "—",
            phase: (pod.phase || "Pending") as Phase,
            cpu: Math.min(
              96,
              38 + (index % 6) * 9 + (pod.phase === "Running" ? 12 : 0),
            ),
            memory: Math.min(
              92,
              42 + (index % 6) * 7 + (pod.phase === "Running" ? 8 : 0),
            ),
            gpu: resource?.gpu,
            latency: index % 2 === 0 ? `${42 + (index % 5) * 7} ms` : undefined,
            fps: index % 2 === 1 ? 24 + (index % 5) * 3 : undefined,
            logs: pod.message
              ? [pod.message]
              : [
                  `${role}: worker state synced`,
                  `${role}: waiting for runtime heartbeat`,
                ],
          };
        })
      : fallbackWorkers;
  const runningWorkerCount = jobWorkers.filter(
    (worker) => worker.phase === "Running",
  ).length;
  const workerPodsByTask = new Map<string, PodInfo[]>();
  for (const worker of jobWorkers) {
    workerPodsByTask.set(
      worker.id,
      pods.filter(
        (pod) =>
          pod.podName === worker.name ||
          `${pod.namespace}/${pod.podNamespace}/${pod.podName}` === worker.id,
      ),
    );
  }
  const workerRoles = [...new Set(jobWorkers.map((worker) => worker.role))];
  const filteredWorkers = jobWorkers.filter(
    (worker) => workerRoleFilter === "All" || worker.role === workerRoleFilter,
  );
  const toggleWorkerSort = (key: typeof workerSort.key) => {
    setWorkerSort((current) => ({
      key,
      direction:
        current.key === key && current.direction === "asc" ? "desc" : "asc",
    }));
    setWorkerPage(1);
  };
  const sortedWorkers = filteredWorkers
    .map((worker) => ({ worker, index: jobWorkers.indexOf(worker) }))
    .sort((left, right) => {
      const value = ({ worker, index }: typeof left) => {
        const pod = workerPodsByTask.get(worker.id)?.[0];
        switch (workerSort.key) {
          case "name":
            return worker.name;
          case "role":
            return worker.role;
          case "node":
            return worker.node;
          case "kind":
            return getNodeKindLabel(worker);
          case "ip":
            return pod?.ip ?? "";
          case "createdAt":
            return formatWorkerCreatedAt(job.startedAt, index);
          case "phase":
            return worker.phase;
        }
      };
      return compareSortValues(
        value(left),
        value(right),
        workerSort.direction,
        zh ? "zh-CN" : "en",
      );
    });
  const workersPerPage = 8;
  const workerPageCount = Math.max(
    1,
    Math.ceil(sortedWorkers.length / workersPerPage),
  );
  const visibleWorkers = sortedWorkers.slice(
    (workerPage - 1) * workersPerPage,
    workerPage * workersPerPage,
  );
  const handleWorkerTablePointerDown = (
    event: ReactPointerEvent<HTMLDivElement>,
  ) => {
    if (event.button !== 0 || !workerTableRef.current) return;
    const target = event.target as HTMLElement;
    if (
      target.closest(
        "button, a, input, select, textarea, [role='button'], [contenteditable='true']",
      )
    ) {
      return;
    }
    workerTableDrag.current = {
      active: true,
      x: event.clientX,
      scrollLeft: workerTableRef.current.scrollLeft,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
    setWorkerTableDragging(true);
  };
  const handleWorkerTablePointerMove = (
    event: ReactPointerEvent<HTMLDivElement>,
  ) => {
    if (!workerTableDrag.current.active || !workerTableRef.current) return;
    workerTableRef.current.scrollLeft =
      workerTableDrag.current.scrollLeft -
      (event.clientX - workerTableDrag.current.x);
  };
  const stopWorkerTableDrag = () => {
    workerTableDrag.current.active = false;
    setWorkerTableDragging(false);
  };
  const logEntries = podLogs.flatMap((pod) => {
    const role =
      resourceForTask(pod.taskName)?.role ??
      job.resources.find((resource) => resource.role === pod.taskName)?.role ??
      pod.taskName;
    return pod.logs
      .split("\n")
      .filter(Boolean)
      .map((message, index) => ({
        id: `${pod.podName}-${index}-${message}`,
        worker: pod.podName,
        role,
        phase: pod.phase,
        node: pod.node,
        message,
      }));
  });
  const logRoles = [...new Set(logEntries.map((entry) => entry.role))];
  const logWorkers = [
    ...new Set(
      logEntries
        .filter(
          (entry) => logRoleFilter === "All" || entry.role === logRoleFilter,
        )
        .map((entry) => entry.worker),
    ),
  ];
  const filteredLogEntries = logEntries.filter(
    (entry) =>
      (logRoleFilter === "All" || entry.role === logRoleFilter) &&
      (logWorkerFilter === "All" || entry.worker === logWorkerFilter) &&
      `${entry.message} ${entry.role} ${entry.worker}`
        .toLowerCase()
        .includes(logQuery.toLowerCase()),
  );
  const tabs: Array<{ id: typeof activeTab; label: string }> = [
    { id: "workers", label: zh ? "Worker" : "Workers" },
    { id: "logs", label: c.common.logs },
    { id: "metrics", label: zh ? "监控" : "Metrics" },
  ];
  return (
    <div className="page-content resource-page job-detail-page">
      <JobPublicOverview
        job={job}
        copy={c}
        onBack={onBack}
        runningWorkerCount={runningWorkerCount}
        totalWorkers={jobWorkers.length || job.workers}
        tensorBoardProxy={tensorBoardProxy}
        onClone={onClone}
      />
      <div className="sub-tabs">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            className={activeTab === tab.id ? "active" : ""}
            onClick={() => setActiveTab(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>
      {activeTab === "workers" && (
        <div className="worker-primary-panel worker-console-panel">
          <div className="worker-panel-head">
            <div>
              <span className="eyebrow">
                {zh ? "运行实例" : "Runtime instances"}
              </span>
              <h3>{zh ? "Worker 列表" : "Worker list"}</h3>
            </div>
            <span className="worker-total-count">
              {filteredWorkers.length} {zh ? "个 Worker" : "workers"}
            </span>
          </div>
          <RoleRuntimeConfig
            job={job}
            copy={c}
            roles={workerRoles}
            selectedRole={workerRoleFilter}
            onRoleChange={(role) => {
              setWorkerRoleFilter(role);
              setWorkerPage(1);
            }}
          />
          <div
            ref={workerTableRef}
            className={`worker-table worker-console-table worker-table-scroll${workerTableDragging ? " dragging" : ""}`}
            onPointerDown={handleWorkerTablePointerDown}
            onPointerMove={handleWorkerTablePointerMove}
            onPointerUp={stopWorkerTableDrag}
            onPointerCancel={stopWorkerTableDrag}
          >
            <table>
              <thead>
                <tr>
                  {(
                    [
                      ["name", zh ? "实例名称" : "Worker name"],
                      ["role", zh ? "角色" : "Role"],
                      ["node", zh ? "节点" : "Node"],
                      ["kind", zh ? "节点类型" : "Node type"],
                      ["ip", zh ? "实例 IP" : "Worker IP"],
                      ["createdAt", zh ? "创建时间" : "Created"],
                      ["phase", zh ? "状态" : "Status"],
                    ] as const
                  ).map(([key, label]) => (
                    <th key={key}>
                      <SortButton
                        label={label}
                        active={workerSort.key === key}
                        direction={workerSort.direction}
                        onClick={() => toggleWorkerSort(key)}
                      />
                    </th>
                  ))}
                  <th aria-label={zh ? "操作" : "Actions"} />
                </tr>
              </thead>
              <tbody>
                {visibleWorkers.map(({ worker, index }) => (
                  <WorkerTableRow
                    key={worker.id}
                    jobName={job.name}
                    worker={worker}
                    copy={c}
                    pods={workerPodsByTask.get(worker.id) ?? []}
                    domainIPMap={domainIPMap}
                    isHeader={
                      worker.role === job.headerRole &&
                      worker.name === job.headerWorker
                    }
                    createdAt={formatWorkerCreatedAt(job.startedAt, index)}
                  />
                ))}
              </tbody>
            </table>
          </div>
          <div className="worker-pagination">
            <span>
              {zh
                ? `第 ${workerPage} / ${workerPageCount} 页`
                : `Page ${workerPage} of ${workerPageCount}`}
            </span>
            <div>
              <button
                className="secondary-button"
                disabled={workerPage <= 1}
                onClick={() => setWorkerPage((page) => Math.max(1, page - 1))}
              >
                {zh ? "上一页" : "Previous"}
              </button>
              <button
                className="secondary-button"
                disabled={workerPage >= workerPageCount}
                onClick={() =>
                  setWorkerPage((page) => Math.min(workerPageCount, page + 1))
                }
              >
                {zh ? "下一页" : "Next"}
              </button>
            </div>
          </div>
        </div>
      )}
      {activeTab === "logs" && (
        <div className="job-observe-panel">
          <div className="observe-panel-head">
            <div>
              <span className="eyebrow">{zh ? "任务日志" : "Job logs"}</span>
              <h3>{zh ? "Worker 日志流" : "Worker log stream"}</h3>
            </div>
          </div>
          {logsLoading ? (
            <code>{zh ? "加载日志中…" : "Loading logs…"}</code>
          ) : logsError ? (
            <code className="log-error">{logsError}</code>
          ) : podLogs.length === 0 ? (
            <code>{zh ? "暂无日志" : "No logs available"}</code>
          ) : (
            <>
              <div className="log-console-toolbar">
                <label className="log-role-filter">
                  <span>{zh ? "角色" : "Role"}</span>
                  <select
                    value={logRoleFilter}
                    onChange={(event) => {
                      setLogRoleFilter(event.target.value);
                      setLogWorkerFilter("All");
                    }}
                  >
                    <option value="All">{zh ? "全部角色" : "All roles"}</option>
                    {logRoles.map((role) => (
                      <option key={role} value={role}>
                        {role}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="log-worker-filter">
                  <span>Worker</span>
                  <select
                    value={logWorkerFilter}
                    onChange={(event) => setLogWorkerFilter(event.target.value)}
                  >
                    <option value="All">
                      {zh ? "全部 Worker" : "All workers"}
                    </option>
                    {logWorkers.map((worker) => (
                      <option key={worker} value={worker}>
                        {worker}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="log-range-filter">
                  <span>{zh ? "时间范围" : "Time range"}</span>
                  <select
                    value={logRange}
                    onChange={(event) => setLogRange(event.target.value)}
                  >
                    <option value="15m">15m</option>
                    <option value="1h">1h</option>
                    <option value="6h">6h</option>
                    <option value="24h">24h</option>
                  </select>
                </label>
                <label className="log-search-field">
                  <Search size={15} />
                  <input
                    value={logQuery}
                    onChange={(event) => setLogQuery(event.target.value)}
                    placeholder={zh ? "搜索日志内容" : "Search logs"}
                  />
                </label>
                <button
                  className={
                    "stream-toggle" + (logStreamEnabled ? " active" : "")
                  }
                  onClick={() => setLogStreamEnabled((enabled) => !enabled)}
                >
                  <i />
                  {logStreamEnabled
                    ? zh
                      ? "实时输出中"
                      : "Streaming"
                    : zh
                      ? "已暂停"
                      : "Paused"}
                </button>
                <button
                  className="secondary-button log-export-button"
                  onClick={() => exportLogs(filteredLogEntries, job.name)}
                >
                  <Download size={15} />
                  {zh ? "导出" : "Export"}
                </button>
              </div>
              <div
                className="log-list"
                aria-live={logStreamEnabled ? "polite" : "off"}
              >
                {filteredLogEntries.length > 0 ? (
                  filteredLogEntries.map((entry) => (
                    <div className="log-list-row" key={entry.id}>
                      <span className="log-worker-name">{entry.worker}</span>
                      <span className="log-role-name">{entry.role}</span>
                      <p>{entry.message}</p>
                    </div>
                  ))
                ) : (
                  <div className="empty-inline">
                    {zh ? "未找到匹配的日志。" : "No matching logs found."}
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      )}
      {activeTab === "metrics" && (
        <div className="job-observe-panel">
          <div className="observe-panel-head">
            <div>
              <span className="eyebrow">{zh ? "任务监控" : "Job metrics"}</span>
              <h3>
                {zh
                  ? "Worker 资源与具身通道"
                  : "Worker resources and live channel"}
              </h3>
            </div>
          </div>
          <MetricsDashboard
            workers={jobWorkers}
            copy={c}
            isMockMode={isMockMode}
          />
        </div>
      )}
    </div>
  );
}

function JobPublicOverview({
  job,
  copy: c,
  onBack,
  runningWorkerCount,
  totalWorkers,
  tensorBoardProxy,
  onClone,
}: {
  job: Job;
  copy: CopyType;
  onBack: () => void;
  runningWorkerCount: number;
  totalWorkers: number;
  tensorBoardProxy?: string;
  onClone: () => void;
}) {
  const zh = c.nav.overview === "总览";
  const baseConfigRows = [
    {
      label: zh ? "网络域" : "Network domain",
      value: job.domain || (zh ? "未配置" : "Not configured"),
    },
    {
      label: "TensorBoard",
      value: job.tensorBoardDir || (zh ? "未配置" : "Not configured"),
    },
  ];
  return (
    <section className="job-detail-summary-card">
      <div className="job-detail-summary-head">
        <div>
          <button className="plain-button back-button" onClick={onBack}>
            ← {zh ? "返回任务列表" : "Back"}
          </button>
          <span className="eyebrow">{c.jobs.selected}</span>
          <h2>{job.name}</h2>
          <p>
            {c.jobType[job.type]} · {job.cluster}
          </p>
        </div>
        <div className="job-detail-summary-status">
          <StatusBadge phase={job.phase} copy={c} />
          <small>
            {job.stopped
              ? zh
                ? "任务已终止"
                : "Job stopped"
              : zh
                ? "任务保持活跃"
                : "Job active"}
          </small>
          <button
            className="secondary-button job-detail-clone-button"
            onClick={onClone}
          >
            <Copy size={15} />
            {zh ? "复制任务" : "Clone job"}
          </button>
        </div>
      </div>

      <div className="job-detail-summary-grid">
        <SummaryMetric
          label={zh ? "Worker" : "Workers"}
          value={`${runningWorkerCount} / ${totalWorkers}`}
          hint={zh ? "运行中 / 总数" : "running / total"}
          tone="blue"
        />
        <SummaryMetric
          label={zh ? "创建时间" : "Created"}
          value={formatTaskTime(job.startedAt)}
          hint={zh ? "提交到控制面" : "submitted to control plane"}
        />
        <SummaryMetric
          label={zh ? "已运行" : "Running for"}
          value={job.duration || "—"}
          hint={
            job.stopped
              ? zh
                ? "已停止"
                : "stopped"
              : zh
                ? "持续运行中"
                : "still running"
          }
        />
        <div className="task-summary-metric header-summary-metric">
          <span>Header Worker</span>
          <strong>{job.headerRole || "—"}</strong>
          <small>{zh ? "访问入口" : "Access entry"}</small>
          {tensorBoardProxy && (
            <a
              href={tensorBoardProxy}
              target="_blank"
              rel="noopener noreferrer"
              title="TensorBoard"
            >
              <ExternalLink size={14} />
            </a>
          )}
        </div>
      </div>

      <div className="job-detail-summary-columns">
        <section className="job-detail-summary-section header-worker-card-legacy">
          <div className="public-card-head">
            <div>
              <span className="eyebrow">{zh ? "访问入口" : "Access"}</span>
              <h3>Header Worker</h3>
            </div>
            {tensorBoardProxy && (
              <a
                href={tensorBoardProxy}
                target="_blank"
                rel="noopener noreferrer"
                title="TensorBoard"
              >
                <ExternalLink size={16} />
              </a>
            )}
          </div>
          <div className="header-worker-identity">
            <span className="role-chip">{job.headerRole || "—"}</span>
            {job.headerWorker && job.headerWorker !== job.headerRole && (
              <strong>{job.headerWorker}</strong>
            )}
          </div>
        </section>

        <div className="public-runtime-card">
          <div className="public-runtime-topology">
            <div className="public-command-card">
              <span className="public-config-title">
                {zh ? "启动命令" : "Start command"}
              </span>
              <CommandCodeBlock value={job.command || "—"} copy={c} />
            </div>
            <div className="public-basic-config-card">
              <span className="public-config-title">
                {zh ? "公共配置" : "Shared settings"}
              </span>
              <div className="public-basic-config-list">
                {baseConfigRows.map((row) => (
                  <div key={row.label}>
                    <span>{row.label}</span>
                    <code>{row.value}</code>
                  </div>
                ))}
              </div>
            </div>
          </div>
          <div className="public-runtime-tables">
            <PublicCompactConfigTable
              title={zh ? "环境变量" : "Environment variables"}
              firstHeader={zh ? "变量名" : "Name"}
              secondHeader={zh ? "变量值" : "Value"}
              rows={
                job.env.length
                  ? job.env.map((item) => ({
                      key: item.key,
                      value: item.value,
                    }))
                  : []
              }
              empty={
                zh ? "未配置环境变量" : "No environment variables configured"
              }
            />
          </div>
        </div>
      </div>
    </section>
  );
}

function PublicCompactConfigTable({
  title,
  firstHeader,
  secondHeader,
  rows,
  empty,
}: {
  title: string;
  firstHeader: string;
  secondHeader: string;
  rows: Array<{ key: string; value: string }>;
  empty: string;
}) {
  return (
    <section className="public-compact-config-card">
      <span className="public-config-title">{title}</span>
      <table className="public-compact-config-table">
        <thead>
          <tr>
            <th>{firstHeader}</th>
            <th>{secondHeader}</th>
          </tr>
        </thead>
        <tbody>
          {rows.length ? (
            rows.map((row, index) => (
              <tr key={`${row.key}-${row.value}-${index}`}>
                <td>
                  <code>{row.key || "—"}</code>
                </td>
                <td>
                  <code>{row.value || "—"}</code>
                </td>
              </tr>
            ))
          ) : (
            <tr>
              <td colSpan={2}>
                <small>{empty}</small>
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </section>
  );
}

function CommandCodeBlock({
  value,
  copy: c,
}: {
  value: string;
  copy: CopyType;
}) {
  const [copied, setCopied] = useState(false);
  const lines = value.split(/\r?\n/);
  const copyValue = async () => {
    if (!(await copyText(value))) return;
    setCopied(true);
    setTimeout(() => setCopied(false), 1600);
  };
  return (
    <div className="command-code-block">
      <button className="icon-button" onClick={copyValue} title={c.api.copy}>
        {copied ? <Check size={15} /> : <Copy size={15} />}
      </button>
      <pre>
        {lines.map((line, index) => (
          <code key={`${index}-${line}`}>
            <span className="command-line-number">{index + 1}</span>
            <span className="command-line-content">
              {highlightCommandLine(line || " ")}
            </span>
          </code>
        ))}
      </pre>
    </div>
  );
}

function highlightCommandLine(line: string) {
  const parts = line.split(/(\s+|&&|\|\||[|;])/);
  const commandKeywords = new Set([
    "python",
    "python3",
    "bash",
    "sh",
    "node",
    "npm",
    "pnpm",
    "yarn",
    "rlark",
    "torchrun",
  ]);
  return parts.map((part, index) => {
    if (!part) return null;
    const className =
      commandKeywords.has(part) || part.startsWith("-m")
        ? "command-token-keyword"
        : part.startsWith("-")
          ? "command-token-flag"
          : part.startsWith("/") || part.includes("=")
            ? "command-token-value"
            : "";
    return className ? (
      <span className={className} key={`${part}-${index}`}>
        {part}
      </span>
    ) : (
      <span key={`${part}-${index}`}>{part}</span>
    );
  });
}

function SummaryMetric({
  label,
  value,
  hint,
  tone,
}: {
  label: string;
  value: string;
  hint: string;
  tone?: "blue";
}) {
  return (
    <div className={"task-summary-metric" + (tone ? ` tone-${tone}` : "")}>
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{hint}</small>
    </div>
  );
}

function CopyableCodeBlock({
  label,
  value,
  copy: c,
}: {
  label: string;
  value: string;
  copy: CopyType;
}) {
  const [copied, setCopied] = useState(false);
  const copyValue = async () => {
    if (!(await copyText(value))) return;
    setCopied(true);
    setTimeout(() => setCopied(false), 1600);
  };
  return (
    <div className="copyable-code-block">
      <div>
        <span>{label}</span>
        <button className="icon-button" onClick={copyValue} title={c.api.copy}>
          {copied ? <Check size={15} /> : <Copy size={15} />}
        </button>
      </div>
      <code>{value}</code>
    </div>
  );
}

function RoleRuntimeConfig({
  job,
  copy: c,
  roles,
  selectedRole,
  onRoleChange,
}: {
  job: Job;
  copy: CopyType;
  roles: string[];
  selectedRole: string;
  onRoleChange: (role: string) => void;
}) {
  const zh = c.nav.overview === "总览";
  const [expanded, setExpanded] = useState(false);
  const resource = job.resources.find((item) => item.role === selectedRole);

  const resourceSummary = resource
    ? [
        `${resource.cpu} CPU`,
        resource.memory,
        `${resource.gpu} GPU`,
        ...resource.devices.map(
          (device) => `${device.name} x ${device.quantity}`,
        ),
      ].join(" · ")
    : "";
  const nodeSelectors = resource?.nodeSelector
    ? resource.nodeSelector.split(",").map((selector) => {
        const [key, ...value] = selector.split("=");
        return { key: key.trim(), value: value.join("=").trim() };
      })
    : [];

  return (
    <section className="role-runtime-config">
      <div className="role-runtime-heading">
        <div>
          <span className="eyebrow">{zh ? "Worker 角色" : "Worker roles"}</span>
          <strong>
            {zh
              ? "按角色查看实例与运行配置"
              : "Instances and configuration by role"}
          </strong>
        </div>
        <div
          className="role-runtime-tabs"
          aria-label={zh ? "选择角色" : "Select role"}
        >
          <button
            className={selectedRole === "All" ? "active" : ""}
            onClick={() => {
              onRoleChange("All");
              setExpanded(false);
            }}
          >
            All
          </button>
          {roles.map((role) => (
            <button
              key={role}
              className={role === selectedRole ? "active" : ""}
              onClick={() => {
                onRoleChange(role);
                setExpanded(false);
              }}
            >
              {role}
              {role === job.headerRole && <span>Header</span>}
            </button>
          ))}
        </div>
      </div>
      {resource ? (
        <div className="role-runtime-summary">
          <div className="role-runtime-image">
            <span>{zh ? "镜像" : "Image"}</span>
            <code>{resource.image || "—"}</code>
            <button
              className="icon-button"
              onClick={() => copyText(resource.image || "")}
              aria-label={zh ? "复制镜像地址" : "Copy image"}
              disabled={!resource.image}
            >
              <Copy size={14} />
            </button>
          </div>
          <div className="role-runtime-resource-summary">
            <span>{zh ? "资源规格" : "Resources"}</span>
            <strong>{resourceSummary}</strong>
          </div>
          <div className="role-runtime-meta">
            <span>
              {resource.replicas} {zh ? "个副本" : "replicas"}
            </span>
            <span>{resource.cluster || "—"}</span>
          </div>
          <button
            className="secondary-button role-runtime-toggle"
            onClick={() => setExpanded((value) => !value)}
            aria-expanded={expanded}
          >
            {expanded
              ? zh
                ? "收起配置"
                : "Collapse"
              : zh
                ? "展开配置"
                : "Show configuration"}
            <ChevronRight
              size={15}
              style={{ transform: expanded ? "rotate(90deg)" : "none" }}
            />
          </button>
        </div>
      ) : (
        <p className="role-runtime-all-summary">
          {zh
            ? `显示全部 ${roles.length} 个角色的 Worker`
            : `Showing workers from all ${roles.length} roles`}
        </p>
      )}
      {resource && expanded && (
        <div className="role-runtime-details">
          <div className="role-runtime-command-row">
            <section className="role-runtime-command-card">
              <span>{zh ? "准备命令" : "Prepare command"}</span>
              <CommandCodeBlock
                value={
                  resource.prepareScript || (zh ? "未配置" : "Not configured")
                }
                copy={c}
              />
            </section>
            <section className="role-runtime-selector-card">
              <span>{zh ? "节点选择" : "Node selectors"}</span>
              {nodeSelectors.length ? (
                <div>
                  {nodeSelectors.map((selector, index) => (
                    <p key={`${selector.key}-${index}`}>
                      <code>{selector.key}</code>
                      <b>=</b>
                      <code>{selector.value || "—"}</code>
                    </p>
                  ))}
                </div>
              ) : (
                <small>{zh ? "未配置" : "Not configured"}</small>
              )}
            </section>
          </div>
          <div className="role-runtime-config-tables">
            <RoleRuntimeDataTable
              className="role-runtime-env-table"
              title={zh ? "环境变量" : "Environment variables"}
              headers={[zh ? "变量名" : "Name", zh ? "变量值" : "Value"]}
              rows={resource.env.map((item) => [item.key, item.value])}
              empty={
                zh ? "未配置环境变量" : "No environment variables configured"
              }
            />
            <RoleRuntimeDataTable
              className="role-runtime-mount-table"
              title={zh ? "数据挂载" : "Data mounts"}
              headers={[
                zh ? "挂载类型" : "Mount type",
                zh ? "来源" : "Source",
                zh ? "挂载到 Worker" : "Mount in worker",
              ]}
              rows={resource.mounts.map((mount) => [
                mount.type === "storage"
                  ? zh
                    ? "对象存储"
                    : "Object storage"
                  : zh
                    ? "主机目录"
                    : "Host directory",
                mount.type === "storage" ? mount.objectStorage : mount.hostPath,
                mount.mountPath,
              ])}
              empty={zh ? "未配置数据挂载" : "No data mounts configured"}
            />
          </div>
        </div>
      )}
    </section>
  );
}

function RoleRuntimeDataTable({
  className,
  title,
  headers,
  rows,
  empty,
}: {
  className: string;
  title: string;
  headers: string[];
  rows: string[][];
  empty: string;
}) {
  return (
    <section className={`role-runtime-table-card ${className}`}>
      <span>{title}</span>
      <table>
        <thead>
          <tr>
            {headers.map((header) => (
              <th key={header}>{header}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.length ? (
            rows.map((row, rowIndex) => (
              <tr key={`${title}-${rowIndex}`}>
                {row.map((cell, cellIndex) => (
                  <td key={`${title}-${rowIndex}-${cellIndex}`}>
                    <code>{cell || "—"}</code>
                  </td>
                ))}
              </tr>
            ))
          ) : (
            <tr>
              <td className="empty-cell" colSpan={headers.length}>
                {empty}
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </section>
  );
}

function formatTaskTime(value: string) {
  return formatChinaDateTime(value);
}

function formatWorkerCreatedAt(startedAt: string, index: number) {
  const date = new Date(startedAt);
  if (Number.isNaN(date.getTime())) return startedAt || "—";
  date.setSeconds(date.getSeconds() + index * 12);
  return formatChinaDateTime(date.toISOString());
}

function getNodeKindLabel(worker: WorkerItem) {
  const value = `${worker.node} ${worker.role}`.toLowerCase();
  if (value.includes("robot") || value.includes("environment"))
    return "具身真机";
  if (value.includes("edge") || value.includes("camera")) return "具身算力";
  return "云算力";
}

function exportLogs(
  entries: Array<{
    worker: string;
    role: string;
    message: string;
  }>,
  jobName: string,
) {
  const header = "worker,role,message";
  const rows = entries.map((entry) =>
    [entry.worker, entry.role, entry.message]
      .map((value) => `"${value.replaceAll('"', '""')}"`)
      .join(","),
  );
  const blob = new Blob([[header, ...rows].join("\n")], {
    type: "text/csv;charset=utf-8",
  });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `${jobName}-logs.csv`;
  anchor.click();
  URL.revokeObjectURL(url);
}

function MetricsDashboard({
  workers,
  copy: c,
  isMockMode,
}: {
  workers: WorkerItem[];
  copy: CopyType;
  isMockMode: boolean;
}) {
  const zh = c.nav.overview === "总览";
  const [scope, setScope] = useState<"task" | "worker">("task");
  const [workerRole, setWorkerRole] = useState("All");
  const [workerName, setWorkerName] = useState("All");
  const [range, setRange] = useState("1h");
  const workerRoles = [...new Set(workers.map((worker) => worker.role))];
  const roleWorkers =
    workerRole === "All"
      ? []
      : workers.filter((worker) => worker.role === workerRole);
  const selectedWorker = workers.find((worker) => worker.name === workerName);
  const selectedLabel =
    scope === "task"
      ? zh
        ? "全部 Worker"
        : "All workers"
      : (selectedWorker?.name ??
        (workerRole === "All"
          ? zh
            ? "选择角色"
            : "Select role"
          : workerRole));
  const metricCards = [
    {
      label: "CPU",
      unit: "%",
      color: "#4f7cff",
      values: [39, 48, 43, 58, 55, 67, 61, 72, 63, 69, 59, 65],
    },
    {
      label: zh ? "内存" : "Memory",
      unit: "%",
      color: "#8b5cf6",
      values: [45, 43, 51, 55, 53, 61, 64, 59, 68, 66, 71, 69],
    },
    {
      label: "GPU",
      unit: "%",
      color: "#22b991",
      values: [58, 66, 62, 74, 78, 72, 85, 82, 87, 79, 83, 89],
    },
    {
      label: zh ? "跨集群网络" : "Cross-cluster network",
      unit: "Mbps",
      color: "#f59e0b",
      values: [220, 280, 260, 340, 310, 420, 390, 460, 410, 490, 440, 530],
    },
    {
      label: "RDMA",
      unit: "Gbps",
      color: "#ec4899",
      values: [12, 17, 15, 23, 21, 28, 25, 31, 29, 35, 32, 38],
    },
  ];
  if (!isMockMode) {
    return (
      <div className="metrics-dashboard metrics-integration-state">
        <span className="metrics-integration-icon">
          <Network size={20} />
        </span>
        <strong>{zh ? "指标待接入" : "Metrics integration pending"}</strong>
        <p>
          {zh
            ? "当前环境暂无可用的 Prometheus 指标数据"
            : "Prometheus metrics are not available in this environment"}
        </p>
      </div>
    );
  }
  return (
    <div className="metrics-dashboard">
      <div className="metrics-filter-bar">
        <div className="metrics-scope-toggle">
          <button
            className={scope === "task" ? "active" : ""}
            onClick={() => setScope("task")}
          >
            {zh ? "任务聚合" : "Task aggregate"}
          </button>
          <button
            className={scope === "worker" ? "active" : ""}
            onClick={() => setScope("worker")}
          >
            {zh ? "单 Worker" : "Per worker"}
          </button>
        </div>
        {scope === "worker" && (
          <>
            <label>
              <span>{zh ? "角色" : "Role"}</span>
              <select
                value={workerRole}
                onChange={(event) => {
                  setWorkerRole(event.target.value);
                  setWorkerName("All");
                }}
              >
                <option value="All">{zh ? "选择角色" : "Select role"}</option>
                {workerRoles.map((role) => (
                  <option key={role} value={role}>
                    {role}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span>Worker</span>
              <select
                value={workerName}
                disabled={workerRole === "All"}
                onChange={(event) => setWorkerName(event.target.value)}
              >
                <option value="All">
                  {zh ? "选择 Worker" : "Select worker"}
                </option>
                {roleWorkers.map((worker) => (
                  <option key={worker.id} value={worker.name}>
                    {worker.name}
                  </option>
                ))}
              </select>
            </label>
          </>
        )}
        <label>
          <span>{zh ? "时间范围" : "Time range"}</span>
          <select
            value={range}
            onChange={(event) => setRange(event.target.value)}
          >
            <option value="15m">15m</option>
            <option value="1h">1h</option>
            <option value="6h">6h</option>
            <option value="24h">24h</option>
          </select>
        </label>
        <span className="metrics-source-label">
          Prometheus · {selectedLabel}
        </span>
      </div>
      <div className="metrics-overview-row">
        <span>
          {zh
            ? `${range} 内的资源与网络时序`
            : `Resource and network time series over ${range}`}
        </span>
        <strong>
          {workers.length} {zh ? "个 Worker 已接入指标" : "workers reporting"}
        </strong>
      </div>
      <div className="time-series-grid">
        {metricCards.map((metric) => (
          <TimeSeriesCard key={metric.label} {...metric} />
        ))}
      </div>
    </div>
  );
}

function TimeSeriesCard({
  label,
  unit,
  color,
  values,
}: {
  label: string;
  unit: string;
  color: string;
  values: number[];
}) {
  const max = Math.max(...values);
  const min = Math.min(...values);
  const range = Math.max(max - min, 1);
  const points = values
    .map(
      (value, index) =>
        `${(index / (values.length - 1)) * 100},${88 - ((value - min) / range) * 62}`,
    )
    .join(" ");
  const latest = values.at(-1) ?? 0;
  return (
    <section className="time-series-card">
      <div className="time-series-head">
        <div>
          <span>{label}</span>
          <strong>
            {latest} <small>{unit}</small>
          </strong>
        </div>
        <i style={{ background: color }} />
      </div>
      <svg
        viewBox="0 0 100 100"
        preserveAspectRatio="none"
        aria-label={`${label} time series`}
      >
        <line x1="0" x2="100" y1="25" y2="25" />
        <line x1="0" x2="100" y1="55" y2="55" />
        <line x1="0" x2="100" y1="85" y2="85" />
        <polyline points={points} style={{ stroke: color }} />
      </svg>
      <div className="time-series-foot">
        <span>-60m</span>
        <span>-30m</span>
        <span>now</span>
      </div>
    </section>
  );
}

function WorkerTableRow({
  jobName,
  worker,
  copy: c,
  pods,
  domainIPMap,
  isHeader,
  createdAt,
}: {
  jobName: string;
  worker: WorkerItem;
  copy: CopyType;
  pods: PodInfo[];
  domainIPMap: Record<string, string>;
  isHeader?: boolean;
  createdAt: string;
}) {
  const zh = c.nav.overview === "总览";
  const [copied, setCopied] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [sshConfig, setSSHConfig] = useState<{
    sshJumpHost: string;
    sshJumpPort: string;
  } | null>(null);
  useEffect(() => {
    fetch("/api/v1/system-config")
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => {
        if (d) setSSHConfig(d);
      })
      .catch(() => {});
  }, []);
  const sshUser = sessionStorage.getItem("rlark-user-name") || "<ssh-user>";
  const sshJump = sshConfig?.sshJumpHost
    ? `${sshUser}@${sshConfig.sshJumpHost}${sshConfig.sshJumpPort ? ":" + sshConfig.sshJumpPort : ""}`
    : "";
  const sshCommand = sshJump ? `ssh -J ${sshJump} root@${worker.name}` : "";
  const handleCopy = async () => {
    if (!(await copyText(sshCommand))) return;
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  const pod = pods[0];
  return (
    <>
      <tr>
        <td>
          <span className="table-primary">
            <span className="row-icon">
              <Zap size={16} />
            </span>
            <span>
              <strong>{worker.name}</strong>
              <small>
                {isHeader
                  ? "Header Worker"
                  : `${worker.latency ?? `${worker.fps ?? "-"} fps`}`}
              </small>
            </span>
          </span>
        </td>
        <td>
          <span className="role-chip">{worker.role}</span>
        </td>
        <td>
          <strong className="table-node-name">{worker.node}</strong>
        </td>
        <td>
          <span className="node-kind-chip">{getNodeKindLabel(worker)}</span>
        </td>
        <td>
          <code className="inline-code">{pod?.ip || "—"}</code>
        </td>
        <td>
          <span className="table-date">{createdAt}</span>
        </td>
        <td>
          <StatusBadge phase={worker.phase} copy={c} />
        </td>
        <td>
          <div className="worker-table-actions">
            <span
              className="action-tooltip"
              data-tooltip={
                copied
                  ? c.jobs.sshCopied
                  : zh
                    ? "复制 SSH 命令"
                    : "Copy SSH command"
              }
            >
              <button
                className="icon-button worker-copy-ssh-icon"
                onClick={handleCopy}
                aria-label={
                  copied
                    ? c.jobs.sshCopied
                    : zh
                      ? "复制 SSH 命令"
                      : "Copy SSH command"
                }
              >
                {copied ? <Check size={16} /> : <KeyRound size={16} />}
              </button>
            </span>
            <span
              className="action-tooltip"
              data-tooltip={zh ? "打开 WebTerminal" : "Open WebTerminal"}
            >
              <button
                className="icon-button worker-terminal-icon"
                disabled={!pod}
                onClick={() => {
                  if (!pod) return;
                  const params = new URLSearchParams({
                    job: jobName,
                    worker: pod.name,
                    status: worker.phase,
                  });
                  // Keep the same-origin opener just long enough for the browser
                  // to clone sessionStorage into the new tab, then sever it so
                  // the terminal cannot navigate or control the parent page.
                  const terminalWindow = window.open(
                    `/terminal?${params.toString()}`,
                    "_blank",
                  );
                  if (terminalWindow) terminalWindow.opener = null;
                }}
                aria-label={zh ? "打开 WebTerminal" : "Open WebTerminal"}
              >
                <TerminalSquare size={16} />
              </button>
            </span>
            <span
              className="action-tooltip"
              data-tooltip={
                expanded
                  ? zh
                    ? "收起详情"
                    : "Collapse details"
                  : zh
                    ? "查看详情"
                    : "View details"
              }
            >
              <button
                className="icon-button worker-detail-icon"
                onClick={() => setExpanded((value) => !value)}
                aria-label={
                  expanded
                    ? zh
                      ? "收起详情"
                      : "Collapse details"
                    : zh
                      ? "查看详情"
                      : "View details"
                }
              >
                <ChevronRight
                  size={17}
                  style={{ transform: expanded ? "rotate(90deg)" : "none" }}
                />
              </button>
            </span>
          </div>
        </td>
      </tr>
      {expanded && (
        <tr className="worker-expanded-row">
          <td colSpan={8}>
            <div className="worker-detail-drawer">
              <div className="worker-detail-head">
                <div>
                  <span className="eyebrow">
                    {zh ? "Worker 详情" : "Worker details"}
                  </span>
                  <strong>{worker.name}</strong>
                </div>
                <div className="worker-ssh-inline">
                  <KeyRound size={15} />
                  <code title={sshCommand}>{sshCommand}</code>
                  <button
                    className="icon-button"
                    onClick={handleCopy}
                    aria-label={zh ? "复制 SSH 地址" : "Copy SSH address"}
                  >
                    {copied ? <Check size={15} /> : <Copy size={15} />}
                  </button>
                </div>
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
                  <span>CPU / {zh ? "内存" : "Memory"}</span>
                  <strong>
                    {worker.cpu}% / {worker.memory}%
                  </strong>
                </div>
                <div>
                  <span>{zh ? "通信状态" : "Runtime"}</span>
                  <strong>
                    {worker.latency ?? `${worker.fps ?? "-"} fps`}
                  </strong>
                </div>
              </div>
              {pods.length > 0 ? (
                <div className="pod-subtable">
                  <table>
                    <thead>
                      <tr>
                        <th>{zh ? "实例名称" : "Worker name"}</th>
                        <th>{zh ? "节点" : "Node"}</th>
                        <th>{zh ? "实例 IP" : "Worker IP"}</th>
                        <th>{zh ? "网络域" : "Domain"}</th>
                        <th>{zh ? "状态" : "Status"}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {pods.map((pod) => {
                        const domainIP =
                          domainIPMap[
                            `${pod.namespace}/${pod.podNamespace}/${pod.podName}`
                          ] ?? "";
                        return (
                          <tr key={pod.name}>
                            <td>
                              <code className="inline-code">{pod.podName}</code>
                            </td>
                            <td>{pod.node || "—"}</td>
                            <td>{pod.ip || "—"}</td>
                            <td>
                              {pod.domain ? (
                                <>
                                  <Network
                                    size={13}
                                    style={{ marginRight: 4 }}
                                  />
                                  {pod.domain}
                                  {domainIP && (
                                    <code
                                      className="inline-code"
                                      style={{ marginLeft: 6, fontSize: 12 }}
                                    >
                                      {domainIP}
                                    </code>
                                  )}
                                </>
                              ) : (
                                "—"
                              )}
                            </td>
                            <td>
                              <StatusBadge
                                phase={pod.phase as Phase}
                                copy={c}
                              />
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              ) : (
                <div className="empty-inline">
                  {zh ? "暂无 Pod 详情，等待任务同步。" : "No pod details yet."}
                </div>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  );
}
