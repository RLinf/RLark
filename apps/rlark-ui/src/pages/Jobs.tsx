import type { CSSProperties, PointerEvent as ReactPointerEvent } from "react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import {
  AlertTriangle,
  Check,
  ChevronRight,
  Copy,
  Download,
  ExternalLink,
  Info,
  KeyRound,
  LoaderCircle,
  MoreVertical,
  Network,
  Pencil,
  Play,
  Plus,
  RotateCcw,
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
  type PullProgressEntry,
  type Worker as WorkerItem,
} from "../data";
import type { Copy as CopyType } from "../i18n";
import type { CRDJob, CRDNode, NodeEventEntry } from "../types";
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

function effectiveJobPhase(job: Job, workerPhases?: string[]): Phase {
  if (job.stopped || job.phase === "Stopped") return "Stopped";
  const phases = (
    workerPhases ?? job.taskStatuses.map((task) => task.phase)
  ).filter(Boolean);
  if (phases.length === 0 || job.phase !== "Running") return job.phase;
  if (phases.some((phase) => phase === "Failed")) return "Failed";
  if (phases.every((phase) => phase === "Succeeded")) return "Succeeded";
  if (!phases.some((phase) => phase === "Running")) return "Pending";
  return "Running";
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

// aggregateJobPullProgress collects pullProgress entries from nodePullProgressMap
// for all nodes referenced by the job's taskStatuses.observedNodes. Used by the
// list/detail top StatusBadge hover to surface image pull progress while a job
// is Pending (mirrors WorkerRow's worker-level tooltip).
function aggregateJobPullProgress(
  job: Job,
  nodePullProgressMap: Record<string, PullProgressEntry[]>,
): PullProgressEntry[] {
  const nodeNames = new Set<string>();
  for (const ts of job.taskStatuses ?? []) {
    for (const n of ts.observedNodes ?? []) {
      if (n) nodeNames.add(n);
    }
  }
  const aggregated: PullProgressEntry[] = [];
  const seen = new Set<string>();
  for (const nodeName of nodeNames) {
    const entries = nodePullProgressMap[nodeName];
    if (!entries) continue;
    for (const p of entries) {
      // Dedup by image+status to avoid showing identical entries from
      // multiple nodes pulling the same image.
      const key = `${p.image}|${p.status}`;
      if (seen.has(key)) continue;
      seen.add(key);
      aggregated.push(p);
    }
  }
  return aggregated;
}

// aggregateJobEvents mirrors aggregateJobPullProgress but for Node.status.events
// (DiskPressure 等 Warning 事件)。在任务 Pending 期间，从 taskStatuses 中
// observedNodes 命中的节点上聚合 warning 事件，供列表/详情顶部 StatusBadge
// 的 "i" tooltip 展示。多节点同一事件按 (objectKind, objectName, reason)
// 去重，保留 lastTime 最新的一条。
function aggregateJobEvents(
  job: Job,
  nodeEventsMap: Record<string, NodeEventEntry[]>,
): NodeEventEntry[] {
  const nodeNames = new Set<string>();
  for (const ts of job.taskStatuses ?? []) {
    for (const n of ts.observedNodes ?? []) {
      if (n) nodeNames.add(n);
    }
  }
  const merged = new Map<string, NodeEventEntry>();
  for (const nodeName of nodeNames) {
    const entries = nodeEventsMap[nodeName];
    if (!entries) continue;
    for (const ev of entries) {
      const key = `${ev.objectKind ?? ""}|${ev.objectName ?? ""}|${ev.reason ?? ""}`;
      const existing = merged.get(key);
      if (!existing) {
        merged.set(key, ev);
        continue;
      }
      // keep the entry with the latest lastTime so a re-observed event
      // refreshes rather than duplicates.
      if (
        ev.lastTime &&
        (!existing.lastTime || ev.lastTime > existing.lastTime)
      ) {
        merged.set(key, ev);
      }
    }
  }
  const out = [...merged.values()];
  // newest-first by lastTime; fall back to reason for stable ordering when
  // timestamps tie (or are absent).
  out.sort((a, b) => {
    const ta = a.lastTime ?? "";
    const tb = b.lastTime ?? "";
    if (ta === tb) return (a.reason ?? "").localeCompare(b.reason ?? "");
    return tb.localeCompare(ta);
  });
  return out;
}

export function JobsPage({
  copy: c,
  isMockMode,
  selectedName,
  onSelect,
  onCreate,
  onClone,
  onEditAndRestart,
  adminMode = false,
}: {
  copy: CopyType;
  isMockMode: boolean;
  selectedName: string;
  onSelect: (name?: string) => void;
  onCreate?: () => void;
  onClone?: (job: Job) => void;
  onEditAndRestart?: (job: Job) => void;
  adminMode?: boolean;
}) {
  const zh = c.nav.overview === "总览";
  const [query, setQuery] = useState("");
  const [phaseFilter, setPhaseFilter] = useState<"All" | Phase>("All");
  const [realJobs, setRealJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [jobAction, setJobAction] = useState<
    "start" | "stop" | "restart" | "delete" | null
  >(null);
  const [restartTarget, setRestartTarget] = useState<Job | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Job | null>(null);
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
  // Per-node pull progress cache, refreshed alongside jobs. Keyed by node name
  // so list/detail top StatusBadge hover can aggregate progress for nodes
  // referenced by job.taskStatuses[].observedNodes.
  const [nodePullProgressMap, setNodePullProgressMap] = useState<
    Record<string, PullProgressEntry[]>
  >({});
  // Per-node warning events cache（Node.status.events），同样 keyed by node
  // name，供列表/详情顶部 StatusBadge 的 "i" tooltip 在 Pending 时聚合展示。
  const [nodeEventsMap, setNodeEventsMap] = useState<
    Record<string, NodeEventEntry[]>
  >({});
  const [nodeDeviceModelMap, setNodeDeviceModelMap] = useState<
    Record<string, { gpuModel?: string; deviceModel?: string }>
  >({});

  const fetchJobs = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    try {
      const [jobsResp, nodesResp] = await Promise.all([
        fetch("/api/v1/rlinf.io/v1alpha1/jobs"),
        fetch("/api/v1/rlinf.io/v1alpha1/nodes"),
      ]);
      if (!jobsResp.ok) throw new Error(`HTTP ${jobsResp.status}`);
      const data = await jobsResp.json();
      const items: CRDJob[] = data.items ?? [];
      setRealJobs(items.map(crdToJob));
      // Build nodeName -> pullProgress / nodeName -> events maps. Failures
      // here are non-fatal: the hover tooltip simply won't appear.
      if (nodesResp.ok) {
        const nodesData = await nodesResp.json();
        const nodeItems: CRDNode[] = nodesData.items ?? [];
        const progressMap: Record<string, PullProgressEntry[]> = {};
        const eventsMap: Record<string, NodeEventEntry[]> = {};
        const deviceModelMap: Record<
          string,
          { gpuModel?: string; deviceModel?: string }
        > = {};
        for (const n of nodeItems) {
          const pp = n.status?.pullProgress;
          if (Array.isArray(pp) && pp.length > 0) {
            progressMap[n.metadata.name] = pp;
          }
          const evs = n.status?.events;
          if (Array.isArray(evs) && evs.length > 0) {
            eventsMap[n.metadata.name] = evs;
          }
          const gpuModel = n.metadata.annotations?.["rlark.io/gpu-model"];
          const deviceModel = n.metadata.annotations?.["rlark.io/device-model"];
          if (gpuModel || deviceModel) {
            deviceModelMap[n.metadata.name] = { gpuModel, deviceModel };
          }
        }
        setNodePullProgressMap(progressMap);
        setNodeEventsMap(eventsMap);
        setNodeDeviceModelMap(deviceModelMap);
      }
    } catch (e) {
      setRealJobs([]);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchJobs, 10000);

  const handleDelete = async (job: Job) => {
    setJobAction("delete");
    setError("");
    try {
      const resp = await fetch(`/api/v1/rlinf.io/v1alpha1/jobs/${job.name}`, {
        method: "DELETE",
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      setRealJobs((prev) => prev.filter((j) => j.id !== job.id));
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return false;
    } finally {
      setJobAction(null);
    }
  };

  const handleSetStopped = async (job: Job, stopped: boolean) => {
    if (
      !confirm(
        stopped
          ? zh
            ? `确定停止任务 "${job.name}" 吗？`
            : `Stop job "${job.name}"?`
          : zh
            ? `确定启动任务 "${job.name}" 吗？`
            : `Start job "${job.name}"?`,
      )
    )
      return false;
    setJobAction(stopped ? "stop" : "start");
    setError("");
    try {
      const resp = await fetch(`/api/v1/rlinf.io/v1alpha1/jobs/${job.name}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/merge-patch+json" },
        body: JSON.stringify({ spec: { stopped } }),
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      setRealJobs((prev) =>
        prev.map((j) =>
          j.id === job.id
            ? {
                ...j,
                stopped,
                phase: stopped ? ("Stopped" as Phase) : ("Pending" as Phase),
              }
            : j,
        ),
      );
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return false;
    } finally {
      setJobAction(null);
    }
  };

  const handleRestart = async (job: Job) => {
    setJobAction("restart");
    setError("");
    try {
      const resp = await fetch(`/api/v1/rlinf.io/v1alpha1/jobs/${job.name}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/merge-patch+json" },
        body: JSON.stringify({
          metadata: {
            annotations: { "rlark.io/restarted-at": new Date().toISOString() },
          },
          spec: { stopped: false },
        }),
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      setRealJobs((prev) =>
        prev.map((item) =>
          item.id === job.id
            ? { ...item, stopped: false, phase: "Pending" as Phase }
            : item,
        ),
      );
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return false;
    } finally {
      setJobAction(null);
    }
  };

  const allJobs = realJobs;
  const filtered = allJobs.filter((j) => {
    const queryHit = `${j.id} ${j.displayName} ${j.type}`
      .toLowerCase()
      .includes(query.toLowerCase());
    const phaseHit =
      phaseFilter === "All" || effectiveJobPhase(j) === phaseFilter;
    return queryHit && phaseHit;
  });
  const sortedJobs = useMemo(
    () =>
      [...filtered].sort((a, b) => {
        const value = (job: Job) => {
          if (sort.key === "workers") return job.progress;
          if (sort.key === "phase") return effectiveJobPhase(job);
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
      <>
        <JobDetailPage
          job={selected}
          copy={c}
          isMockMode={isMockMode}
          onBack={() => onSelect(undefined)}
          onClone={adminMode ? undefined : () => onClone?.(selected)}
          lifecycleActions={{
            pending: jobAction,
            error,
            onStart: () => handleSetStopped(selected, false),
            onStop: () => handleSetStopped(selected, true),
            onRestart: () => setRestartTarget(selected),
            onDelete: () => setDeleteTarget(selected),
          }}
          nodePullProgressMap={nodePullProgressMap}
          nodeEventsMap={nodeEventsMap}
          nodeDeviceModelMap={nodeDeviceModelMap}
        />
        {restartTarget && (
          <RestartChoiceDialog
            job={restartTarget}
            zh={zh}
            onClose={() => setRestartTarget(null)}
            onRestart={() => {
              const job = restartTarget;
              setRestartTarget(null);
              void handleRestart(job);
            }}
            onEditRestart={
              adminMode || !onEditAndRestart
                ? undefined
                : () => {
                    const job = restartTarget;
                    setRestartTarget(null);
                    onEditAndRestart(job);
                  }
            }
          />
        )}
        {deleteTarget && (
          <DeleteJobDialog
            job={deleteTarget}
            zh={zh}
            pending={jobAction === "delete"}
            error={error}
            onClose={() => setDeleteTarget(null)}
            onConfirm={async () => {
              const deleted = await handleDelete(deleteTarget);
              if (!deleted) return;
              setDeleteTarget(null);
              onSelect(undefined);
            }}
          />
        )}
      </>
    );
  }

  return (
    <div className="page-content resource-page jobs-list-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">
            {adminMode
              ? zh
                ? "ADMIN / JOBS"
                : "ADMIN / JOBS"
              : c.jobs.eyebrow}
          </span>
          <h2>
            {adminMode ? (zh ? "任务管理" : "Job management") : c.jobs.title}
          </h2>
          <p>
            {adminMode
              ? zh
                ? "查看全平台任务状态，并执行停止、重启和删除等管理操作。"
                : "Review platform jobs and perform stop, restart, and delete operations."
              : c.jobs.desc}
          </p>
        </div>
        {!adminMode && (
          <button className="primary-button" onClick={onCreate}>
            <Plus size={17} />
            {c.common.createJob}
          </button>
        )}
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
                    {!adminMode && (
                      <button className="secondary-button" onClick={onCreate}>
                        <Plus size={15} />
                        {c.common.createJob}
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            )}
            {pagedJobs.map((job) => {
              const jobPullProgress =
                job.phase === "Pending"
                  ? aggregateJobPullProgress(job, nodePullProgressMap)
                  : [];
              const jobEvents =
                job.phase === "Pending"
                  ? aggregateJobEvents(job, nodeEventsMap)
                  : [];
              return (
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
                    <div className="status-with-info">
                      <StatusBadge phase={effectiveJobPhase(job)} copy={c} />
                      {(jobPullProgress.length > 0 || jobEvents.length > 0) && (
                        <PullProgressInfo
                          progress={jobPullProgress}
                          events={jobEvents}
                          zh={zh}
                        />
                      )}
                    </div>
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
                    {adminMode ? (
                      <AdminJobActions
                        job={job}
                        zh={zh}
                        onStop={() => handleSetStopped(job, true)}
                        onRestart={() => {
                          if (
                            confirm(
                              zh
                                ? `确定重新启动任务 "${job.name}" 吗？`
                                : `Restart job "${job.name}"?`,
                            )
                          ) {
                            void handleRestart(job);
                          }
                        }}
                        onDelete={() => setDeleteTarget(job)}
                      />
                    ) : (
                      <JobActionMenu
                        job={job}
                        zh={zh}
                        pending={jobAction !== null}
                        onClone={() => onClone?.(job)}
                        onDelete={() => setDeleteTarget(job)}
                        onToggleStop={() => handleSetStopped(job, !job.stopped)}
                        onRestart={() => setRestartTarget(job)}
                      />
                    )}
                  </td>
                </tr>
              );
            })}
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
      {restartTarget && (
        <RestartChoiceDialog
          job={restartTarget}
          zh={zh}
          onClose={() => setRestartTarget(null)}
          onRestart={() => {
            const job = restartTarget;
            setRestartTarget(null);
            void handleRestart(job);
          }}
          onEditRestart={
            !onEditAndRestart
              ? undefined
              : () => {
                  const job = restartTarget;
                  setRestartTarget(null);
                  onEditAndRestart(job);
                }
          }
        />
      )}
      {deleteTarget && (
        <DeleteJobDialog
          job={deleteTarget}
          zh={zh}
          pending={jobAction === "delete"}
          error={error}
          onClose={() => setDeleteTarget(null)}
          onConfirm={async () => {
            if (await handleDelete(deleteTarget)) setDeleteTarget(null);
          }}
        />
      )}
    </div>
  );
}

function JobActionMenu({
  job,
  zh,
  pending,
  onClone,
  onDelete,
  onToggleStop,
  onRestart,
}: {
  job: Job;
  zh: boolean;
  pending: boolean;
  onClone: () => void;
  onDelete: () => void;
  onToggleStop: () => void;
  onRestart: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [menuStyle, setMenuStyle] = useState<CSSProperties>({});
  const ref = useRef<HTMLDivElement>(null);
  const btnRef = useRef<HTMLButtonElement>(null);

  useLayoutEffect(() => {
    if (!open || !ref.current) return;
    const btn = btnRef.current;
    if (!btn) return;
    const rect = btn.getBoundingClientRect();
    const dropdownEl = ref.current.querySelector(
      ".action-dropdown",
    ) as HTMLElement | null;
    const ddHeight = dropdownEl?.offsetHeight ?? 200;
    const ddWidth = dropdownEl?.offsetWidth ?? 140;
    const spaceBelow = window.innerHeight - rect.bottom;
    const dropUp = spaceBelow < ddHeight + 8;
    const menuLeft = Math.max(
      8,
      Math.min(rect.right - ddWidth, window.innerWidth - ddWidth - 8),
    );
    setMenuStyle(
      dropUp
        ? {
            position: "fixed",
            left: menuLeft,
            bottom: window.innerHeight - rect.top + 4,
            right: "auto",
            top: "auto",
          }
        : {
            position: "fixed",
            left: menuLeft,
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
    const handleClose = () => setOpen(false);
    document.addEventListener("pointerdown", handlePointer);
    document.addEventListener("keydown", handleKey);
    window.addEventListener("scroll", handleClose, true);
    window.addEventListener("resize", handleClose);
    return () => {
      document.removeEventListener("pointerdown", handlePointer);
      document.removeEventListener("keydown", handleKey);
      window.removeEventListener("scroll", handleClose, true);
      window.removeEventListener("resize", handleClose);
    };
  }, [open]);

  const isStopped = job.stopped || job.phase === "Stopped";
  const isTerminal = ["Succeeded", "Failed"].includes(job.phase);

  const handleToggle = () => {
    setOpen((v) => !v);
  };

  return (
    <div className="row-actions" ref={ref} style={{ position: "relative" }}>
      <button
        className={`icon-button job-quick-lifecycle${isStopped ? " start" : " stop"}`}
        onClick={onToggleStop}
        disabled={pending || isTerminal}
        title={
          isTerminal
            ? zh
              ? "终态任务无法启动或停止"
              : "Terminal jobs cannot be started or stopped"
            : isStopped
              ? zh
                ? "启动任务"
                : "Start job"
              : zh
                ? "停止任务"
                : "Stop job"
        }
        aria-label={
          isStopped
            ? zh
              ? `启动任务 ${job.name}`
              : `Start ${job.name}`
            : zh
              ? `停止任务 ${job.name}`
              : `Stop ${job.name}`
        }
      >
        {isStopped ? <Play size={15} /> : <Square size={14} />}
      </button>
      <button
        ref={btnRef}
        className="icon-button"
        onClick={handleToggle}
        title={zh ? "操作" : "Actions"}
        aria-expanded={open}
        disabled={pending}
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
                onClone();
              }}
            >
              <Copy size={14} />
              {zh ? "复制" : "Clone"}
            </button>
            <button
              className="action-dropdown-item"
              onClick={() => {
                setOpen(false);
                onRestart();
              }}
            >
              <RotateCcw size={14} />
              {zh ? "重启" : "Restart"}
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

function AdminJobActions({
  job,
  zh,
  onStop,
  onRestart,
  onDelete,
}: {
  job: Job;
  zh: boolean;
  onStop: () => void;
  onRestart: () => void;
  onDelete: () => void;
}) {
  const canStop =
    !job.stopped && !["Stopped", "Succeeded", "Failed"].includes(job.phase);

  return (
    <div className="row-actions admin-job-actions">
      {canStop ? (
        <button
          className="icon-button"
          onClick={onStop}
          title={zh ? "停止任务" : "Stop job"}
          aria-label={zh ? `停止任务 ${job.name}` : `Stop ${job.name}`}
        >
          <Square size={15} />
        </button>
      ) : (
        <button
          className="icon-button"
          onClick={onRestart}
          title={zh ? "重启任务" : "Restart job"}
          aria-label={zh ? `重启任务 ${job.name}` : `Restart ${job.name}`}
        >
          <RotateCcw size={15} />
        </button>
      )}
      <button
        className="icon-button danger"
        onClick={onDelete}
        title={zh ? "删除任务" : "Delete job"}
        aria-label={zh ? `删除任务 ${job.name}` : `Delete ${job.name}`}
      >
        <Trash2 size={15} />
      </button>
    </div>
  );
}

function DeleteJobDialog({
  job,
  zh,
  pending,
  error,
  onClose,
  onConfirm,
}: {
  job: Job;
  zh: boolean;
  pending: boolean;
  error: string;
  onClose: () => void;
  onConfirm: () => Promise<void>;
}) {
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !pending) onClose();
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [onClose, pending]);

  return (
    <div
      className="modal-backdrop delete-job-backdrop"
      onMouseDown={(event) =>
        event.target === event.currentTarget && !pending && onClose()
      }
    >
      <section
        className="modal delete-job-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-job-title"
      >
        <div className="delete-job-head">
          <span className="delete-job-icon">
            <Trash2 size={20} />
          </span>
          <div>
            <span className="eyebrow">{zh ? "危险操作" : "Danger zone"}</span>
            <h2 id="delete-job-title">
              {zh ? "确认删除任务？" : "Delete this job?"}
            </h2>
          </div>
          <button
            className="icon-button"
            onClick={onClose}
            aria-label={zh ? "关闭" : "Close"}
            disabled={pending}
          >
            ×
          </button>
        </div>
        <div className="delete-job-body">
          <p>
            {zh
              ? "删除后，任务定义及其关联的运行实例将从平台移除。"
              : "The job definition and its associated runtime instances will be removed from the platform."}
          </p>
          <div className="delete-job-target">
            <span>{zh ? "即将删除" : "Job to delete"}</span>
            <strong>{job.name}</strong>
          </div>
          <div className="delete-job-warning">
            <AlertTriangle size={15} />
            <span>
              {zh
                ? "此操作不可撤销，请确认该任务已不再需要。"
                : "This action cannot be undone. Confirm that this job is no longer needed."}
            </span>
          </div>
          {error && (
            <div className="delete-job-error" role="alert">
              {zh ? "删除失败：" : "Delete failed: "}
              {error}
            </div>
          )}
        </div>
        <div className="delete-job-actions">
          <button
            className="secondary-button"
            onClick={onClose}
            disabled={pending}
          >
            {zh ? "取消" : "Cancel"}
          </button>
          <button
            className="delete-job-confirm"
            onClick={() => void onConfirm()}
            disabled={pending}
          >
            {pending ? (
              <LoaderCircle className="job-action-loading" size={15} />
            ) : (
              <Trash2 size={15} />
            )}
            {pending
              ? zh
                ? "删除中…"
                : "Deleting…"
              : zh
                ? "确认删除"
                : "Delete job"}
          </button>
        </div>
      </section>
    </div>
  );
}

function RestartChoiceDialog({
  job,
  zh,
  onClose,
  onRestart,
  onEditRestart,
}: {
  job: Job;
  zh: boolean;
  onClose: () => void;
  onRestart: () => void;
  onEditRestart?: () => void;
}) {
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [onClose]);

  return (
    <div
      className="modal-backdrop restart-choice-backdrop"
      onMouseDown={(event) => event.target === event.currentTarget && onClose()}
    >
      <section
        className="modal restart-choice-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="restart-choice-title"
      >
        <div className="restart-choice-head">
          <span className="restart-choice-icon">
            <RotateCcw size={19} />
          </span>
          <div>
            <span className="eyebrow">{zh ? "重启方式" : "Restart mode"}</span>
            <h2 id="restart-choice-title">{zh ? "重启任务" : "Restart job"}</h2>
            <p>{job.name}</p>
          </div>
          <button
            className="icon-button"
            onClick={onClose}
            aria-label={zh ? "关闭" : "Close"}
          >
            ×
          </button>
        </div>
        <div className="restart-choice-options">
          <button className="restart-choice-option primary" onClick={onRestart}>
            <span>
              <RotateCcw size={18} />
            </span>
            <strong>{zh ? "一键重启" : "Restart now"}</strong>
            <small>
              {zh
                ? "保持当前任务配置，立即重新创建 Worker。"
                : "Keep the current configuration and recreate workers now."}
            </small>
            <ChevronRight size={17} />
          </button>
          {onEditRestart && (
            <button className="restart-choice-option" onClick={onEditRestart}>
              <span>
                <Pencil size={18} />
              </span>
              <strong>{zh ? "编辑后重启" : "Edit and restart"}</strong>
              <small>
                {zh
                  ? "进入任务配置，保存修改后自动触发重启。"
                  : "Review the job configuration and restart after saving."}
              </small>
              <ChevronRight size={17} />
            </button>
          )}
        </div>
        <div className="restart-choice-note">
          {zh
            ? "重启会终止当前 Worker，运行中的连接将中断。"
            : "Restarting terminates current workers and interrupts active connections."}
        </div>
      </section>
    </div>
  );
}

type JobLifecycleActions = {
  pending: "start" | "stop" | "restart" | "delete" | null;
  error: string;
  onStart: () => void;
  onStop: () => void;
  onRestart: () => void;
  onDelete: () => void;
};

export function JobDetailPage({
  job,
  copy: c,
  isMockMode,
  onBack,
  onClone,
  lifecycleActions,
  nodePullProgressMap = {},
  nodeEventsMap = {},
  nodeDeviceModelMap = {},
}: {
  job: Job;
  copy: CopyType;
  isMockMode: boolean;
  onBack: () => void;
  onClone?: () => void;
  lifecycleActions: JobLifecycleActions;
  // Per-node pullProgress cache shared from JobsPage; used by the top
  // StatusBadge hover to surface image pull progress while the job is Pending.
  nodePullProgressMap?: Record<string, PullProgressEntry[]>;
  // Per-node warning events cache（Node.status.events），用于详情顶部
  // StatusBadge 的 "i" tooltip 及 worker 行 tooltip 在 Pending 时聚合展示。
  nodeEventsMap?: Record<string, NodeEventEntry[]>;
  nodeDeviceModelMap?: Record<
    string,
    { gpuModel?: string; deviceModel?: string }
  >;
}) {
  const zh = c.nav.overview === "总览";
  const [activeTab, setActiveTab] = useState<"workers" | "logs" | "metrics">(
    "workers",
  );
  const [taskNodes, setTaskNodes] = useState<Record<string, string>>({});
  const [tensorBoardProxy, setTensorBoardProxy] = useState<string>("");
  const [pullProgressMap, setPullProgressMap] = useState<
    Record<string, PullProgressEntry[]>
  >({});
  // Task.status.events 缓存，keyed by lowercased task 名；用于无 pod.node
  // 的 fallback worker 行展示 warning 事件。
  const [taskEventsMap, setTaskEventsMap] = useState<
    Record<string, NodeEventEntry[]>
  >({});
  const [podEventsMap, setPodEventsMap] = useState<
    Record<string, NodeEventEntry[]>
  >({});
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
  const [workerRefreshKey, setWorkerRefreshKey] = useState(0);
  const [workerRefreshing, setWorkerRefreshing] = useState(false);
  const [logRoleFilter, setLogRoleFilter] = useState("All");
  const [logWorkerFilter, setLogWorkerFilter] = useState("All");
  const [logQuery, setLogQuery] = useState("");
  const [logRange, setLogRange] = useState("1h");
  const [logStreamEnabled, setLogStreamEnabled] = useState(false);

  // Aggregate Node CR pullProgress for the top StatusBadge hover. Uses Node CR
  // (not the Task CR-derived pullProgressMap used by WorkerRow) so the tooltip
  // reflects raw node-agent reported progress while the job is Pending.
  const jobPullProgress =
    job.phase === "Pending"
      ? aggregateJobPullProgress(job, nodePullProgressMap)
      : [];
  // 同样从 Node CR events 聚合本 job 的 warning 事件，用于详情顶部
  // StatusBadge 的 "i" tooltip。worker 行 tooltip 则按节点取
  // nodeEventsMap[pod.node]，并在 pod.node 缺失时回退到 Task.status.events。
  const jobEvents =
    job.phase === "Pending" ? aggregateJobEvents(job, nodeEventsMap) : [];

  const { refresh: refreshTasks } = useAutoRefresh(
    async () => {
      const labelSelector = `rlinf.io/job=${job.name}`;
      const resp = await fetch(
        `/api/v1/rlinf.io/v1alpha1/tasks?labelSelector=${encodeURIComponent(labelSelector)}`,
      );
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      const items = data.items ?? [];
      const nodeMap: Record<string, string> = {};
      const progressMap: Record<string, PullProgressEntry[]> = {};
      const taskEventsMap: Record<string, NodeEventEntry[]> = {};
      let tbProxy = "";
      for (const item of items) {
        const taskName = item.metadata?.name ?? "";
        const observedNodes = item.status?.observedNodes ?? [];
        nodeMap[taskName] = observedNodes.join(", ") || "—";
        if (item.status?.tensorBoardProxy) {
          tbProxy = item.status.tensorBoardProxy;
        }
        const pp: PullProgressEntry[] = item.status?.pullProgress ?? [];
        if (Array.isArray(pp) && pp.length > 0) {
          // Key case-insensitively so it matches the lowercased lookup keys
          // built from job/task names below.
          progressMap[taskName.toLowerCase()] = pp;
        }
        // 控制面 task reconciler 已将各节点 events 聚合到
        // Task.status.events；按 task 名保存，供 fallback worker 行
        // （无 pod.node 时）展示事件。
        const evs: NodeEventEntry[] = item.status?.events ?? [];
        if (Array.isArray(evs) && evs.length > 0) {
          taskEventsMap[taskName.toLowerCase()] = evs;
        }
      }
      setTaskNodes(nodeMap);
      setTensorBoardProxy(tbProxy);
      setPullProgressMap(progressMap);
      setTaskEventsMap(taskEventsMap);
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
    setWorkerRefreshing(true);
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
      })
      .finally(() => {
        if (!cancelled) setWorkerRefreshing(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workerTaskNamesKey, workerRefreshKey]);

  const pendingPodNames = useMemo(
    () =>
      pods
        .filter((pod) => pod.phase !== "Running")
        .map((pod) => pod.name)
        .filter(Boolean)
        .sort(),
    [pods],
  );
  const pendingPodNamesKey = pendingPodNames.join(",");
  useAutoRefresh(
    async () => {
      if (pendingPodNames.length === 0) {
        setPodEventsMap({});
        return;
      }
      const entries = await Promise.all(
        pendingPodNames.map(async (podName) => {
          try {
            const response = await fetch(
              `/api/v1/rlinf.io/v1alpha1/pods/${encodeURIComponent(podName)}/events`,
            );
            if (!response.ok) return [podName, []] as const;
            const data = await response.json();
            return [
              podName,
              Array.isArray(data.events) ? data.events : [],
            ] as const;
          } catch {
            return [podName, []] as const;
          }
        }),
      );
      setPodEventsMap(Object.fromEntries(entries));
    },
    5000,
    [pendingPodNamesKey],
  );

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
            cpu: job.resources.find((item) => item.role === ts.name)?.cpu ?? "",
            memory:
              job.resources.find((item) => item.role === ts.name)?.memory ?? "",
            gpu: job.resources.find((item) => item.role === ts.name)?.gpu,
            logs: ts.message
              ? [ts.message]
              : [
                  `${ts.name}: worker state synced`,
                  `${ts.name}: waiting for runtime heartbeat`,
                ],
            pullProgress:
              pullProgressMap[jobChildName] ??
              pullProgressMap[ts.name.toLowerCase()] ??
              [],
            events:
              taskEventsMap[jobChildName] ??
              taskEventsMap[ts.name.toLowerCase()] ??
              [],
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
          const phase = (pod.phase || "Pending") as Phase;
          // When the worker is still Pending, query the hosting node's
          // pullProgress (reported by node-agent) so the StatusBadge "i"
          // hover surfaces live image pull progress for this worker.
          const nodePullProgress =
            phase === "Pending" && pod.node
              ? (nodePullProgressMap[pod.node] ?? [])
              : [];
          // 同样从 Node.status.events 取节点 warning 事件；当 pod.node
          // 缺失时回退到 Task.status.events，让 Pending worker 行 tooltip
          // 在调度前就能展示 DiskPressure 等原因。
          const workerEvents =
            phase === "Pending"
              ? (podEventsMap[pod.name] ?? []).length > 0
                ? (podEventsMap[pod.name] ?? [])
                : pod.node && (nodeEventsMap[pod.node] ?? []).length > 0
                  ? (nodeEventsMap[pod.node] ?? [])
                  : (taskEventsMap[pod.taskName?.toLowerCase() ?? ""] ?? [])
              : [];
          return {
            id: `${pod.namespace}/${pod.podNamespace}/${pod.podName}`,
            name: pod.podName || `${role}-${index}`,
            jobId: job.id,
            role,
            node: pod.node || "—",
            phase: (pod.phase || "Pending") as Phase,
            cpu: resource?.cpu ?? "",
            memory: resource?.memory ?? "",
            gpu: resource?.gpu,
            logs: pod.message
              ? [pod.message]
              : [
                  `${role}: worker state synced`,
                  `${role}: waiting for runtime heartbeat`,
                ],
            pullProgress: nodePullProgress,
            events: workerEvents,
          };
        })
      : fallbackWorkers;
  const runningWorkerCount = jobWorkers.filter(
    (worker) => worker.phase === "Running",
  ).length;
  const displayPhase = effectiveJobPhase(
    job,
    jobWorkers.map((worker) => worker.phase),
  );
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
        displayPhase={displayPhase}
        tensorBoardProxy={tensorBoardProxy}
        onClone={onClone}
        lifecycleActions={lifecycleActions}
        jobPullProgress={jobPullProgress}
        jobEvents={jobEvents}
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
            <div className="worker-panel-actions">
              <span className="worker-total-count">
                {filteredWorkers.length} {zh ? "个 Worker" : "workers"}
              </span>
              <button
                className="secondary-button worker-refresh-button"
                onClick={() => {
                  refreshTasks();
                  setWorkerRefreshKey((key) => key + 1);
                }}
                disabled={workerRefreshing}
                title={zh ? "刷新 Worker 列表" : "Refresh worker list"}
              >
                <RotateCcw
                  size={14}
                  className={workerRefreshing ? "job-action-loading" : ""}
                />
                {zh ? "刷新" : "Refresh"}
              </button>
            </div>
          </div>
          <RoleRuntimeConfig
            job={job}
            copy={c}
            roles={workerRoles}
            selectedRole={workerRoleFilter}
            nodeDeviceModelMap={nodeDeviceModelMap}
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
            <div className="log-loading-state" role="status" aria-live="polite">
              <span className="log-loading-icon">
                <LoaderCircle size={20} />
              </span>
              <div>
                <strong>
                  {zh ? "正在连接 Worker 日志" : "Connecting to worker logs"}
                </strong>
                <small>
                  {zh
                    ? "正在汇总各实例的最新输出…"
                    : "Collecting the latest output from each instance…"}
                </small>
              </div>
              <i className="log-loading-shimmer" aria-hidden="true" />
            </div>
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
                  <>
                    <div className="log-list-head" aria-hidden="true">
                      <span>{zh ? "角色" : "Role"}</span>
                      <span>Worker</span>
                      <span>{zh ? "日志内容" : "Log message"}</span>
                    </div>
                    {filteredLogEntries.map((entry) => (
                      <div className="log-list-row" key={entry.id}>
                        <span className="log-role-name">{entry.role}</span>
                        <span className="log-worker-name">{entry.worker}</span>
                        <p>{entry.message}</p>
                      </div>
                    ))}
                  </>
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
  displayPhase,
  tensorBoardProxy,
  onClone,
  lifecycleActions,
  jobPullProgress = [],
  jobEvents = [],
}: {
  job: Job;
  copy: CopyType;
  onBack: () => void;
  runningWorkerCount: number;
  totalWorkers: number;
  displayPhase: Phase;
  tensorBoardProxy?: string;
  onClone?: () => void;
  lifecycleActions: JobLifecycleActions;
  // Per-node pullProgress cache shared from JobsPage; surfaced next to the
  // top StatusBadge while the job is still Pending.
  jobPullProgress?: PullProgressEntry[];
  // 节点级 Warning 事件聚合，在 Pending 时与 pullProgress 一并展示。
  jobEvents?: NodeEventEntry[];
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
          <div className="status-with-info">
            <StatusBadge phase={displayPhase} copy={c} />
            {(jobPullProgress.length > 0 || jobEvents.length > 0) && (
              <PullProgressInfo
                progress={jobPullProgress}
                events={jobEvents}
                zh={zh}
              />
            )}
          </div>
          <small>
            {job.stopped
              ? zh
                ? "任务已终止"
                : "Job stopped"
              : displayPhase === "Pending"
                ? zh
                  ? "Worker 正在等待调度或启动"
                  : "Workers are waiting to schedule or start"
                : displayPhase === "Succeeded"
                  ? zh
                    ? "所有 Worker 已完成"
                    : "All workers completed"
                  : zh
                    ? "任务保持活跃"
                    : "Job active"}
          </small>
          <div className="job-detail-actions">
            {onClone && (
              <button
                className="secondary-button job-detail-clone-button"
                onClick={onClone}
                disabled={lifecycleActions.pending !== null}
              >
                <Copy size={15} />
                {zh ? "复制" : "Clone"}
              </button>
            )}
            {job.stopped || job.phase === "Stopped" ? (
              <button
                className="secondary-button primary-action"
                onClick={lifecycleActions.onStart}
                disabled={lifecycleActions.pending !== null}
              >
                {lifecycleActions.pending === "start" ? (
                  <LoaderCircle className="job-action-loading" size={15} />
                ) : (
                  <Play size={15} />
                )}
                {zh ? "启动" : "Start"}
              </button>
            ) : !["Succeeded", "Failed"].includes(job.phase) ? (
              <button
                className="secondary-button"
                onClick={lifecycleActions.onStop}
                disabled={lifecycleActions.pending !== null}
              >
                {lifecycleActions.pending === "stop" ? (
                  <LoaderCircle className="job-action-loading" size={15} />
                ) : (
                  <Square size={15} />
                )}
                {zh ? "停止" : "Stop"}
              </button>
            ) : null}
            <button
              className="secondary-button"
              onClick={lifecycleActions.onRestart}
              disabled={lifecycleActions.pending !== null}
            >
              {lifecycleActions.pending === "restart" ? (
                <LoaderCircle className="job-action-loading" size={15} />
              ) : (
                <RotateCcw size={15} />
              )}
              {zh ? "重启" : "Restart"}
            </button>
            <button
              className="secondary-button danger"
              onClick={lifecycleActions.onDelete}
              disabled={lifecycleActions.pending !== null}
            >
              {lifecycleActions.pending === "delete" ? (
                <LoaderCircle className="job-action-loading" size={15} />
              ) : (
                <Trash2 size={15} />
              )}
              {zh ? "删除" : "Delete"}
            </button>
          </div>
          {lifecycleActions.error && (
            <span className="job-detail-action-error">
              {zh ? "操作失败：" : "Action failed: "}
              {lifecycleActions.error}
            </span>
          )}
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

function RoleRuntimeConfig({
  job,
  copy: c,
  roles,
  selectedRole,
  onRoleChange,
  nodeDeviceModelMap,
}: {
  job: Job;
  copy: CopyType;
  roles: string[];
  selectedRole: string;
  onRoleChange: (role: string) => void;
  nodeDeviceModelMap: Record<
    string,
    { gpuModel?: string; deviceModel?: string }
  >;
}) {
  const zh = c.nav.overview === "总览";
  const [expanded, setExpanded] = useState(false);
  const resource = job.resources.find((item) => item.role === selectedRole);
  const taskName = resource ? taskResourceName(job.name, resource.role) : "";
  const taskStatus = job.taskStatuses.find(
    (status) =>
      status.name.toLowerCase() === taskName ||
      status.name.toLowerCase() === selectedRole.toLowerCase(),
  );
  const hostnameSelector = resource?.nodeSelector.match(
    /(?:^|,)kubernetes\.io\/hostname=([^=]+?)(?=,[^,=]+=|$)/,
  )?.[1];
  const selectedNodes = hostnameSelector
    ? hostnameSelector
        .split(",")
        .map((node) => node.trim())
        .filter(Boolean)
    : [];
  const resourceNodes =
    taskStatus?.observedNodes && taskStatus.observedNodes.length > 0
      ? taskStatus.observedNodes
      : selectedNodes;
  const gpuModel = resourceNodes
    .map((node) => nodeDeviceModelMap[node]?.gpuModel)
    .find(Boolean);
  const deviceModel = resourceNodes
    .map((node) => nodeDeviceModelMap[node]?.deviceModel)
    .find(Boolean);

  const deviceSummary = resource?.devices.map((device) => {
    const model = deviceModel || device.name;
    return zh
      ? `${model} · ${device.quantity} 个设备`
      : `${model} · ${device.quantity} devices`;
  });
  const gpuSummary =
    resource && Number(resource.gpu) > 0
      ? zh
        ? `${gpuModel || "GPU"} · ${resource.gpu} GPU`
        : `${gpuModel || "GPU"} · ${resource.gpu} GPU`
      : "";

  const resourceSummary = resource
    ? [
        gpuSummary,
        ...(deviceSummary ?? []),
        resource.cpu ? `${resource.cpu} CPU` : "",
        resource.memory,
      ]
        .filter(Boolean)
        .join(" / ") || (zh ? "未申请设备" : "No device requested")
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
  if (value.includes("robot")) return "具身真机";
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

// PullProgressInfo renders an "i" icon at the top-right of a task status badge.
// Hovering (or focusing) it reveals the live image pull progress / speed for
// the task's images while its pods have not yet reached Running, plus any
// node-level Warning events (DiskPressure 等) aggregated from
// Node.status.events / Task.status.events so operators can see why a pod is
// stuck Pending even when no image pull is in flight.
//
// The tooltip uses position: fixed (instead of position: absolute) so it can
// escape the overflow:auto / overflow:hidden of its ancestor containers
// (notably .worker-table-scroll, where overflow-x:auto forces overflow-y to
// compute to auto per the CSS spec, clipping any absolutely-positioned
// descendant). The icon's viewport position is measured on hover/focus and
// the tooltip is placed above the icon, or below if there isn't enough room
// above (e.g. when the icon sits in the first row of the Jobs list table).
function PullProgressInfo({
  progress,
  events = [],
  zh,
  emptyMessage,
}: {
  progress: PullProgressEntry[];
  events?: NodeEventEntry[];
  zh: boolean;
  emptyMessage?: string;
}) {
  const wrapperRef = useRef<HTMLSpanElement | null>(null);
  const tooltipRef = useRef<HTMLSpanElement | null>(null);
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{
    top: number;
    left: number;
    above: boolean;
    arrowLeft: number;
  } | null>(null);

  const measure = () => {
    const icon = wrapperRef.current;
    const tooltip = tooltipRef.current;
    if (!icon || !tooltip) return;
    const iconRect = icon.getBoundingClientRect();
    const tooltipH = tooltip.offsetHeight;
    const tooltipW = tooltip.offsetWidth;
    const gap = 10;
    const margin = 12;

    const spaceAbove = iconRect.top;
    const spaceBelow = window.innerHeight - iconRect.bottom;
    const above = spaceAbove >= tooltipH + gap || spaceAbove >= spaceBelow;

    const top = above ? iconRect.top - tooltipH - gap : iconRect.bottom + gap;

    const iconCenter = iconRect.left + iconRect.width / 2;
    let left = iconCenter - tooltipW / 2;
    left = Math.max(
      margin,
      Math.min(left, window.innerWidth - tooltipW - margin),
    );
    const arrowLeft = Math.max(16, Math.min(iconCenter - left, tooltipW - 16));

    setPos((current) => {
      if (
        current &&
        Math.abs(current.top - top) < 0.1 &&
        Math.abs(current.left - left) < 0.1 &&
        current.above === above &&
        Math.abs(current.arrowLeft - arrowLeft) < 0.1
      ) {
        return current;
      }
      return { top, left, above, arrowLeft };
    });
  };

  const show = () => {
    setPos(null);
    setOpen(true);
  };
  const clear = () => {
    setOpen(false);
    setPos(null);
  };

  useLayoutEffect(() => {
    if (!open) return;
    // A portal's child ref can still be unset during the parent's first layout
    // effect. Start on the next frame, then keep tracking while the tooltip is
    // open: live worker polling can change table column widths and move the
    // icon without producing a window scroll or resize event.
    let frame = 0;
    const track = () => {
      measure();
      frame = window.requestAnimationFrame(track);
    };
    frame = window.requestAnimationFrame(track);
    return () => window.cancelAnimationFrame(frame);
  }, [open, progress, events, emptyMessage]);

  const tooltipStyle: CSSProperties = pos
    ? {
        position: "fixed",
        top: pos.top,
        left: pos.left,
        bottom: "auto",
        right: "auto",
      }
    : {
        position: "fixed",
        top: 0,
        left: 0,
        visibility: "hidden",
      };

  return (
    <>
      <span
        className="status-info"
        tabIndex={0}
        ref={wrapperRef}
        onMouseEnter={show}
        onMouseLeave={clear}
        onFocus={show}
        onBlur={clear}
      >
        <Info size={13} />
      </span>
      {open &&
        createPortal(
          <span
            ref={tooltipRef}
            className={`status-info-tooltip status-info-tooltip-open${pos && !pos.above ? " status-info-tooltip-below" : ""}`}
            style={tooltipStyle}
            role="status"
          >
            <i
              className="status-info-tooltip-arrow"
              aria-hidden="true"
              style={{ left: pos ? pos.arrowLeft - 6 : 0 }}
            />
            {progress.length === 0 && events.length === 0 && emptyMessage && (
              <>
                <strong>{zh ? "Worker 等待中" : "Worker pending"}</strong>
                <span className="pending-empty-message">{emptyMessage}</span>
              </>
            )}
            {progress.length > 0 && (
              <>
                <strong>{zh ? "镜像拉取进度" : "Image Pull Progress"}</strong>
                {progress.map((p, i) => {
                  const pct =
                    p.total > 0
                      ? Math.min(
                          100,
                          Math.round((p.downloaded / p.total) * 100),
                        )
                      : 0;
                  return (
                    <span key={`p-${i}`} className="pull-entry">
                      <code>{p.image}</code>
                      <span className="pull-status">
                        {p.message || p.status}
                        {p.status === "pulling" && p.total > 0
                          ? ` · ${pct}%`
                          : ""}
                      </span>
                      {p.total > 0 && (
                        <span className="pull-detail">
                          {formatBytes(p.downloaded)} / {formatBytes(p.total)}
                        </span>
                      )}
                      {p.speed > 0 && (
                        <span className="pull-detail">
                          {formatBytes(p.speed)}/s
                        </span>
                      )}
                    </span>
                  );
                })}
              </>
            )}
            {events.length > 0 && (
              <>
                <strong className="status-info-tooltip-section">
                  {zh ? "Worker 事件" : "Worker Events"}
                </strong>
                {events.map((ev, i) => (
                  <span key={`e-${i}`} className="pull-entry event-entry">
                    <span
                      className={`event-chip event-${ev.type?.toLowerCase() ?? "normal"}`}
                    >
                      {ev.reason || ev.type || "Event"}
                    </span>
                    {ev.objectName && (
                      <code className="event-object">{ev.objectName}</code>
                    )}
                    {ev.message && (
                      <span className="event-message">{ev.message}</span>
                    )}
                    {ev.lastTime && (
                      <span className="pull-detail">
                        {formatChinaDateTime(ev.lastTime)}
                      </span>
                    )}
                  </span>
                ))}
              </>
            )}
          </span>,
          document.body,
        )}
    </>
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
                  : [
                      worker.cpu ? `CPU ${worker.cpu}` : "",
                      worker.gpu && worker.gpu !== "0"
                        ? `GPU ${worker.gpu}`
                        : "",
                    ]
                      .filter(Boolean)
                      .join(" · ") || (zh ? "未申请资源" : "No requests")}
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
          <div className="status-with-info">
            <StatusBadge phase={worker.phase} copy={c} />
            {worker.phase !== "Running" &&
              (worker.phase === "Pending" ||
                (worker.pullProgress && worker.pullProgress.length > 0) ||
                (worker.events && worker.events.length > 0)) && (
                <PullProgressInfo
                  progress={worker.pullProgress ?? []}
                  events={worker.events ?? []}
                  zh={zh}
                  emptyMessage={
                    worker.phase === "Pending"
                      ? worker.node && worker.node !== "—"
                        ? zh
                          ? `已调度到 ${worker.node}，正在等待容器创建或节点上报镜像拉取状态。`
                          : `Scheduled to ${worker.node}; waiting for container creation or image-pull status from the node.`
                        : zh
                          ? "正在等待节点调度；调度完成后将展示镜像拉取或节点事件。"
                          : "Waiting for node scheduling. Image-pull progress or node events will appear after placement."
                      : undefined
                  }
                />
              )}
          </div>
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
                  <span>{zh ? "申请 CPU" : "CPU request"}</span>
                  <strong>
                    {worker.cpu || (zh ? "未申请" : "Not requested")}
                  </strong>
                </div>
                <div>
                  <span>{zh ? "申请内存" : "Memory request"}</span>
                  <strong>
                    {worker.memory || (zh ? "未申请" : "Not requested")}
                  </strong>
                </div>
                <div>
                  <span>{zh ? "申请 GPU" : "GPU request"}</span>
                  <strong>
                    {worker.gpu && worker.gpu !== "0"
                      ? worker.gpu
                      : zh
                        ? "未申请"
                        : "Not requested"}
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
