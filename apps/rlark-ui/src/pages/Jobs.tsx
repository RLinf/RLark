import type {
  CSSProperties,
  PointerEvent as ReactPointerEvent,
  UIEvent as ReactUIEvent,
} from "react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import {
  AlertTriangle,
  Check,
  ChevronRight,
  Copy,
  Download,
  Filter,
  Info,
  KeyRound,
  LoaderCircle,
  Maximize,
  Minimize,
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
  X,
  Zap,
} from "lucide-react";
import {
  type Job,
  type JobTag,
  type Phase,
  type PodInfo,
  type PullProgressEntry,
  type Worker as WorkerItem,
} from "../data";
import type { Copy as CopyType } from "../i18n";
import type { CRDNode, CRDTask, NodeEventEntry } from "../types";
import { useAutoRefresh } from "../hooks";
import { crdToJob } from "../utils/crd";
import { effectiveJobPhase, type JobDisplayPhase } from "../utils/jobPhase";
import { formatChinaDateTime } from "../utils/time";
import { resolveSSHKeyOwners, type SSHUserKey } from "../utils/sshKeys";
import { isDiskUsageWarning } from "../utils/nodeResources";
import {
  isValidJobDisplayName,
  JOB_DISPLAY_NAME_MAX_LENGTH,
} from "../utils/job";
import {
  ColumnFilterButton,
  compareSortValues,
  PageToolbar,
  Pagination,
  RefreshOverlay,
  SortButton,
  StatusBadge,
  useColumnFilter,
  type SortDirection,
} from "../components/shared";
import { ColumnFilterPopover } from "../components/ColumnFilterPopover";
import { JobTagPopover } from "../components/JobTagPopover";
import { TagFilterPopover } from "../components/TagFilterPopover";
import { TagEditor } from "../components/TagEditor";

function taskResourceName(jobName: string, taskName: string) {
  return `${jobName}-${taskName.toLowerCase().replace(/\s+/g, "-")}`
    .toLowerCase()
    .replace(/\s+/g, "-");
}

async function copyText(value: string) {
  if (!value) return false;

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
  onSelectNode,
  onSelectCluster,
  onCreate,
  onClone,
  onEditAndRestart,
  adminMode = false,
}: {
  copy: CopyType;
  isMockMode: boolean;
  selectedName: string;
  onSelect: (name?: string) => void;
  onSelectNode?: (name: string) => void;
  onSelectCluster?: (id: string) => void;
  onCreate?: () => void;
  onClone?: (job: Job) => void;
  onEditAndRestart?: (job: Job) => void;
  adminMode?: boolean;
}) {
  const zh = c.nav.overview === "总览";
  const [query, setQuery] = useState("");
  // 表头列多选筛选；空数组 = 全部
  const [phaseFilter, setPhaseFilter] = useState<string[]>([]);
  const [typeFilter, setTypeFilter] = useState<string[]>([]);
  const [tagFilter, setTagFilter] = useState<Record<string, string[]>>({});
  const [allJobTags, setAllJobTags] = useState<
    Array<{ key: string; values: string[] }>
  >([]);
  const [tagFilterOpen, setTagFilterOpen] = useState(false);
  const [tagFilterAnchor, setTagFilterAnchor] = useState<DOMRect | null>(null);
  const [realJobs, setRealJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [listRefreshing, setListRefreshing] = useState(false);
  const [copiedJobId, setCopiedJobId] = useState("");
  const [tagPopover, setTagPopover] = useState<{
    tags: JobTag[];
    anchor: DOMRect;
  } | null>(null);
  const [error, setError] = useState("");
  const [actionNotice, setActionNotice] = useState("");
  const [jobAction, setJobAction] = useState<
    "start" | "stop" | "restart" | "delete" | null
  >(null);
  const [restartTarget, setRestartTarget] = useState<Job | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Job | null>(null);
  const [lifecycleConfirm, setLifecycleConfirm] = useState<{
    job: Job;
    action: "start" | "stop" | "clean-start" | "restart";
  } | null>(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [sort, setSort] = useState<{
    key: "submittedAt" | "stoppedAt";
    direction: SortDirection;
  }>({ key: "submittedAt", direction: "desc" });
  const columnFilter = useColumnFilter();
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
  const [nodeDiskWarningMap, setNodeDiskWarningMap] = useState<
    Record<string, boolean>
  >({});

  const fetchJobs = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    try {
      const tagSelector = Object.entries(tagFilter)
        .flatMap(([key, values]) => values.map((value) => `${key}=${value}`))
        .join(",");
      const [items, tags] = await Promise.all([
        jobsApi.list({ tagSelector: tagSelector || undefined }),
        jobsApi.listTags(),
      ]);
      setRealJobs(items.map(crdToJob));
      setAllJobTags(tags);

      const nodeNames = new Set<string>();
      for (const job of items) {
        for (const task of job.status?.tasks ?? []) {
          for (const nodeName of task.observedNodes ?? []) {
            if (nodeName) nodeNames.add(nodeName);
          }
        }
      }

      // Build nodeName -> pullProgress / nodeName -> events maps. Failures
      // here are non-fatal: the hover tooltip simply won't appear.
      const nodeResponses = await Promise.all(
        [...nodeNames].map(async (nodeName) => {
          return nodesApi.get(nodeName).catch(() => null);
        }),
      );
      {
        const nodeItems: CRDNode[] = nodeResponses.filter(
          (node): node is CRDNode => node !== null,
        );
        const progressMap: Record<string, PullProgressEntry[]> = {};
        const eventsMap: Record<string, NodeEventEntry[]> = {};
        const deviceModelMap: Record<
          string,
          { gpuModel?: string; deviceModel?: string }
        > = {};
        const diskWarningMap: Record<string, boolean> = {};
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
          if (isDiskUsageWarning(n)) {
            diskWarningMap[n.metadata.name] = true;
          }
        }
        setNodePullProgressMap(progressMap);
        setNodeEventsMap(eventsMap);
        setNodeDeviceModelMap(deviceModelMap);
        setNodeDiskWarningMap(diskWarningMap);
      }
    } catch (e) {
      setRealJobs([]);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchJobs, 10000);

  const handleListRefresh = async () => {
    if (listRefreshing) return;
    setListRefreshing(true);
    try {
      await fetchJobs(false);
    } finally {
      setListRefreshing(false);
    }
  };

  const handleCopyJobId = async (jobId: string) => {
    if (!(await copyText(jobId))) return;
    setCopiedJobId(jobId);
    window.setTimeout(() => setCopiedJobId(""), 1600);
  };

  const handleDelete = async (job: Job) => {
    setJobAction("delete");
    setError("");
    try {
      await jobsApi.remove(job.name);
      setRealJobs((prev) =>
        prev.map((j) =>
          j.id === job.id ? { ...j, phase: "Deleting" as Phase } : j,
        ),
      );
      setActionNotice(zh ? "任务正在删除" : "Job deletion started");
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return false;
    } finally {
      setJobAction(null);
    }
  };

  const handleSetStopped = async (job: Job, stopped: boolean) => {
    setJobAction(stopped ? "stop" : "start");
    setError("");
    try {
      await jobsApi.setStopped(job.name, stopped);
      setRealJobs((prev) =>
        prev.map((j) =>
          j.id === job.id
            ? {
                ...j,
                stopped,
                phase: stopped ? j.phase : ("Pending" as Phase),
                stoppedAt: stopped ? j.stoppedAt : "—",
              }
            : j,
        ),
      );
      setActionNotice(
        stopped
          ? zh
            ? "任务已提交停止"
            : "Job stop submitted"
          : zh
            ? "任务已提交启动"
            : "Job start submitted",
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
      await jobsApi.patch(job.name, {
        metadata: {
          annotations: {
            "rlark.io/restarted-at": new Date().toISOString(),
          },
        },
        spec: { stopped: false },
      });
      setRealJobs((prev) =>
        prev.map((item) =>
          item.id === job.id
            ? { ...item, stopped: false, phase: "Pending" as Phase }
            : item,
        ),
      );
      setActionNotice(zh ? "任务已提交重启" : "Job restart submitted");
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return false;
    } finally {
      setJobAction(null);
    }
  };

  const waitForFailedJobCleanup = async (job: Job) => {
    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
      const current = crdToJob(await jobsApi.get(job.name));
      if (current.phase === "Stopped" && current.runningWorkers === 0) return;
      await new Promise((resolve) => window.setTimeout(resolve, 1000));
    }
    throw new Error(
      zh
        ? "等待残留 Worker 清理超时，任务未启动。"
        : "Timed out waiting for residual workers to stop; the job was not started.",
    );
  };

  const handleCleanStart = async (job: Job) => {
    setJobAction("restart");
    setError("");
    try {
      await jobsApi.setStopped(job.name, true);
      await waitForFailedJobCleanup(job);

      await jobsApi.setStopped(job.name, false);
      setRealJobs((prev) =>
        prev.map((item) =>
          item.id === job.id
            ? { ...item, stopped: false, phase: "Pending" as Phase }
            : item,
        ),
      );
      setActionNotice(
        zh ? "任务已清理并提交启动" : "Job cleaned and submitted",
      );
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return false;
    } finally {
      setJobAction(null);
    }
  };

  const confirmLifecycleAction = async () => {
    if (!lifecycleConfirm) return;
    const { job, action } = lifecycleConfirm;
    const succeeded =
      action === "stop"
        ? await handleSetStopped(job, true)
        : action === "start"
          ? await handleSetStopped(job, false)
          : action === "clean-start"
            ? await handleCleanStart(job)
            : await handleRestart(job);
    if (succeeded) {
      setLifecycleConfirm(null);
      if (selectedName) onSelect(job.name);
    }
  };

  const allJobs = realJobs;

  // 轻量更新 job（tags 或 displayName），使用 PATCH
  const handlePatchJob = async (
    jobName: string,
    patchBody: Record<string, any>,
  ) => {
    const updated = await jobsApi.patch(jobName, patchBody);
    const parsed = crdToJob(updated);
    setRealJobs((prev) => prev.map((j) => (j.id === jobName ? parsed : j)));
    setAllJobTags((prev) => {
      const valuesByKey = new Map(
        prev.map((tag) => [tag.key, new Set(tag.values)]),
      );
      for (const tag of parsed.tags ?? []) {
        if (!valuesByKey.has(tag.key)) valuesByKey.set(tag.key, new Set());
        valuesByKey.get(tag.key)!.add(tag.value);
      }
      return [...valuesByKey].map(([key, values]) => ({
        key,
        values: [...values],
      }));
    });
    return parsed;
  };

  const filtered = allJobs.filter((j) => {
    const queryHit =
      `${j.id} ${j.displayName} ${j.type} ${(j.tags ?? []).map((t) => `${t.key}:${t.value}`).join(" ")}`
        .toLowerCase()
        .includes(query.toLowerCase());
    const phaseHit =
      phaseFilter.length === 0 || phaseFilter.includes(effectiveJobPhase(j));
    const typeHit = typeFilter.length === 0 || typeFilter.includes(j.type);
    return queryHit && phaseHit && typeHit;
  });
  const sortedJobs = useMemo(
    () =>
      [...filtered].sort((a, b) => {
        const comparison = compareSortValues(
          a[sort.key],
          b[sort.key],
          sort.direction,
          zh ? "zh-CN" : "en",
        );
        return comparison !== 0
          ? comparison
          : a.id.localeCompare(b.id, zh ? "zh-CN" : "en");
      }),
    [filtered, sort, zh],
  );
  const totalPages = Math.max(1, Math.ceil(sortedJobs.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pagedJobs = sortedJobs.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );

  useEffect(
    () => setPage(1),
    [query, phaseFilter, typeFilter, tagFilter, pageSize],
  );
  useEffect(() => {
    void fetchJobs(false);
  }, [tagFilter]);
  useEffect(() => {
    if (page > totalPages) setPage(totalPages);
  }, [page, totalPages]);
  useEffect(() => {
    if (!actionNotice) return;
    const timer = window.setTimeout(() => setActionNotice(""), 4000);
    return () => window.clearTimeout(timer);
  }, [actionNotice]);

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
            onStart: () =>
              setLifecycleConfirm({
                job: selected,
                action: selected.phase === "Failed" ? "clean-start" : "start",
              }),
            onStop: () =>
              setLifecycleConfirm({ job: selected, action: "stop" }),
            onRestart: () => setRestartTarget(selected),
            onDelete: () => setDeleteTarget(selected),
          }}
          onSelectNode={onSelectNode}
          onSelectCluster={onSelectCluster}
          nodePullProgressMap={nodePullProgressMap}
          nodeEventsMap={nodeEventsMap}
          nodeDeviceModelMap={nodeDeviceModelMap}
          nodeDiskWarningMap={nodeDiskWarningMap}
          allJobTags={allJobTags}
          onPatchJob={handlePatchJob}
        />
        {restartTarget && (
          <RestartChoiceDialog
            job={restartTarget}
            zh={zh}
            onClose={() => setRestartTarget(null)}
            onRestart={() => {
              const job = restartTarget;
              setRestartTarget(null);
              void handleRestart(job).then((succeeded) => {
                if (succeeded) onSelect(job.name);
              });
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
        {lifecycleConfirm && (
          <JobLifecycleConfirmDialog
            job={lifecycleConfirm.job}
            action={lifecycleConfirm.action}
            zh={zh}
            pending={jobAction !== null}
            error={error}
            onClose={() => setLifecycleConfirm(null)}
            onConfirm={confirmLifecycleAction}
          />
        )}
      </>
    );
  }

  return (
    <div className="page-content resource-page jobs-list-page">
      {actionNotice && (
        <div className="job-action-notice" role="status">
          <Check size={16} />
          <span>{actionNotice}</span>
          <button
            type="button"
            onClick={() => setActionNotice("")}
            aria-label={zh ? "关闭提示" : "Dismiss notification"}
          >
            ×
          </button>
        </div>
      )}
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
        onRefresh={handleListRefresh}
        refreshing={listRefreshing}
      />
      {error && (
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}
      <div
        className={`table-panel jobs-table-panel refreshable-region${listRefreshing ? " is-refreshing" : ""}`}
        aria-busy={listRefreshing}
      >
        <table>
          <thead>
            <tr>
              <th>{zh ? "名称/ID" : "Name / ID"}</th>
              <th>
                <ColumnFilterButton
                  label={zh ? "类型" : "Type"}
                  selectedCount={typeFilter.length}
                  onClick={columnFilter.openFor("type")}
                />
              </th>
              <th className="job-table-tag-col">
                <button
                  type="button"
                  className={`job-table-tag-head${Object.keys(tagFilter).length > 0 ? " has-filter" : ""}`}
                  onClick={(e) => {
                    const rect = (
                      e.currentTarget as HTMLElement
                    ).getBoundingClientRect();
                    setTagFilterAnchor(rect);
                    setTagFilterOpen(true);
                  }}
                >
                  <span>{zh ? "标签" : "Tags"}</span>
                  <Filter size={13} />
                </button>
              </th>
              <th>
                <ColumnFilterButton
                  label={zh ? "状态" : "Status"}
                  selectedCount={phaseFilter.length}
                  onClick={columnFilter.openFor("phase")}
                />
              </th>
              <th>Worker</th>
              <th>{zh ? "角色数量" : "Roles"}</th>
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
                <td colSpan={9}>
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
              const jobFailedMessage =
                effectiveJobPhase(job) === "Failed"
                  ? [
                      ...new Set(
                        job.taskStatuses
                          .filter((ts) => ts.phase === "Failed")
                          .map((ts) => jobFailureMessage(ts.message, zh)),
                      ),
                    ].join("\n")
                  : undefined;
              return (
                <tr key={job.id}>
                  <td>
                    <div className="job-id-cell-wrap">
                      <button
                        className={`link-cell job-id-cell${job.id.length > 28 ? " is-long" : ""}`}
                        title={job.id}
                        onClick={() => onSelect(job.id)}
                      >
                        <strong>{job.displayName}</strong>
                      </button>
                      <button
                        type="button"
                        className="plain-button job-id-copy"
                        onClick={() => handleCopyJobId(job.id)}
                        title={zh ? "复制资源 ID" : "Copy resource ID"}
                        aria-label={zh ? "复制资源 ID" : "Copy resource ID"}
                      >
                        {copiedJobId === job.id ? (
                          <Check size={13} />
                        ) : (
                          <Copy size={13} />
                        )}
                        <small>{job.id}</small>
                      </button>
                    </div>
                  </td>
                  <td>
                    <span className="role-chip">{c.jobType[job.type]}</span>
                  </td>
                  <td className="job-table-tag-cell">
                    {(job.tags ?? []).length > 0 ? (
                      <div className="job-tags-cell">
                        {(job.tags ?? []).slice(0, 2).map((t) => (
                          <span
                            key={t.id}
                            className="job-tag-chip"
                            title={`${t.key}: ${t.value}`}
                          >
                            {t.key}: {t.value}
                          </span>
                        ))}
                        {(job.tags ?? []).length > 2 && (
                          <button
                            type="button"
                            className="job-tag-chip job-tag-overflow"
                            onClick={(event) => {
                              setTagPopover({
                                tags: job.tags ?? [],
                                anchor:
                                  event.currentTarget.getBoundingClientRect(),
                              });
                            }}
                          >
                            +{(job.tags ?? []).length - 2}
                          </button>
                        )}
                      </div>
                    ) : (
                      <span className="job-no-tag">—</span>
                    )}
                  </td>
                  <td>
                    <div className="status-with-info">
                      <StatusBadge phase={effectiveJobPhase(job)} copy={c} />
                      {(jobPullProgress.length > 0 ||
                        jobEvents.length > 0 ||
                        jobFailedMessage) && (
                        <PullProgressInfo
                          progress={jobPullProgress}
                          events={jobEvents}
                          zh={zh}
                          statusMessage={jobFailedMessage}
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
                        onStop={() =>
                          setLifecycleConfirm({ job, action: "stop" })
                        }
                        onRestart={() =>
                          setLifecycleConfirm({ job, action: "restart" })
                        }
                        onDelete={() => setDeleteTarget(job)}
                      />
                    ) : (
                      <JobActionMenu
                        job={job}
                        zh={zh}
                        pending={jobAction !== null}
                        onClone={() => onClone?.(job)}
                        onDelete={() => setDeleteTarget(job)}
                        onStart={() =>
                          setLifecycleConfirm({
                            job,
                            action:
                              job.phase === "Failed" ? "clean-start" : "start",
                          })
                        }
                        onStop={() =>
                          setLifecycleConfirm({ job, action: "stop" })
                        }
                        onRestart={() => setRestartTarget(job)}
                      />
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        <RefreshOverlay
          visible={listRefreshing}
          label={zh ? "正在刷新任务列表" : "Refreshing job list"}
        />
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
      {lifecycleConfirm && (
        <JobLifecycleConfirmDialog
          job={lifecycleConfirm.job}
          action={lifecycleConfirm.action}
          zh={zh}
          pending={jobAction !== null}
          error={error}
          onClose={() => setLifecycleConfirm(null)}
          onConfirm={confirmLifecycleAction}
        />
      )}
      {tagPopover &&
        typeof document !== "undefined" &&
        createPortal(
          <JobTagPopover
            tags={tagPopover.tags}
            anchorRect={tagPopover.anchor}
            zh={zh}
            onClose={() => setTagPopover(null)}
          />,
          document.body,
        )}
      {tagFilterOpen &&
        tagFilterAnchor &&
        typeof document !== "undefined" &&
        createPortal(
          <TagFilterPopover
            allTags={allJobTags}
            selection={tagFilter}
            onChange={setTagFilter}
            onReset={() => {
              setTagFilterOpen(false);
              void fetchJobs(false);
            }}
            zh={zh}
            anchorRect={tagFilterAnchor}
            onClose={() => setTagFilterOpen(false)}
          />,
          document.body,
        )}
      {columnFilter.openKey === "type" &&
        typeof document !== "undefined" &&
        createPortal(
          <ColumnFilterPopover
            label={zh ? "类型" : "Type"}
            options={(
              ["RL", "DataCollection", "Evaluation", "Custom"] as const
            ).map((v) => ({ value: v, label: c.jobType[v] ?? v }))}
            selected={typeFilter}
            onChange={setTypeFilter}
            anchorRect={columnFilter.anchorRect}
            onClose={columnFilter.close}
            zh={zh}
          />,
          document.body,
        )}
      {columnFilter.openKey === "phase" &&
        typeof document !== "undefined" &&
        createPortal(
          <ColumnFilterPopover
            label={zh ? "状态" : "Status"}
            options={[
              { value: "Running", label: c.status.Running },
              { value: "Pending", label: c.status.Pending },
              { value: "Succeeded", label: c.status.Succeeded },
              { value: "Failed", label: c.status.Failed },
              { value: "Stopping", label: c.status.Stopping },
              { value: "Stopped", label: c.status.Stopped },
              { value: "Deleting", label: c.status.Deleting },
            ]}
            selected={phaseFilter}
            onChange={setPhaseFilter}
            anchorRect={columnFilter.anchorRect}
            onClose={columnFilter.close}
            zh={zh}
          />,
          document.body,
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
  onStart,
  onStop,
  onRestart,
}: {
  job: Job;
  zh: boolean;
  pending: boolean;
  onClone: () => void;
  onDelete: () => void;
  onStart: () => void;
  onStop: () => void;
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

  const isStartable =
    job.stopped || job.phase === "Stopped" || job.phase === "Succeeded";
  const isSucceeded = job.phase === "Succeeded";
  const isFailed = job.phase === "Failed";
  const isDeleting = job.phase === "Deleting";
  const isStopping = job.stopped && job.phase !== "Stopped";
  const lifecycleLabel = isSucceeded
    ? zh
      ? "已成功完成的任务不能再次启动"
      : "Succeeded jobs cannot be started again"
    : isStartable
      ? zh
        ? "启动任务"
        : "Start job"
      : zh
        ? "停止任务"
        : "Stop job";

  const handleToggle = () => {
    setOpen((v) => !v);
  };

  return (
    <div className="row-actions" ref={ref} style={{ position: "relative" }}>
      <button
        className="job-row-action"
        onClick={onClone}
        disabled={pending || isDeleting}
      >
        <Copy size={14} />
        {zh ? "复制" : "Clone"}
      </button>
      <button
        className="job-row-action"
        onClick={onRestart}
        disabled={pending || isDeleting || isStopping}
      >
        <RotateCcw size={14} />
        {zh ? "重启" : "Restart"}
      </button>
      <button
        className={`job-row-action job-quick-lifecycle${isStartable && !isFailed ? " start" : " stop"}`}
        onClick={isStartable && !isFailed ? onStart : onStop}
        disabled={pending || isSucceeded || isDeleting || isStopping}
        title={lifecycleLabel}
      >
        {isStartable && !isFailed ? <Play size={14} /> : <Square size={13} />}
        {isStartable && !isFailed
          ? zh
            ? "启动"
            : "Start"
          : zh
            ? "停止"
            : "Stop"}
      </button>
      <span
        className="action-tooltip"
        data-tooltip={zh ? "更多操作" : "More actions"}
      >
        <button
          ref={btnRef}
          className="icon-button"
          onClick={handleToggle}
          aria-label={zh ? `更多操作 ${job.name}` : `More actions ${job.name}`}
          aria-expanded={open}
          disabled={pending || isDeleting || isStopping}
        >
          <MoreVertical size={16} />
        </button>
      </span>
      {open && (
        <>
          <div className="action-dropdown" style={menuStyle}>
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
  const isDeleting = job.phase === "Deleting";
  const isStopping = job.stopped && job.phase !== "Stopped";
  const canStop =
    !job.stopped && !["Stopped", "Succeeded", "Deleting"].includes(job.phase);

  return (
    <div className="row-actions admin-job-actions">
      {canStop ? (
        <button
          className="icon-button"
          onClick={onStop}
          disabled={isDeleting || isStopping}
          title={zh ? "停止任务" : "Stop job"}
          aria-label={zh ? `停止任务 ${job.name}` : `Stop ${job.name}`}
        >
          <Square size={15} />
        </button>
      ) : (
        <button
          className="icon-button"
          onClick={onRestart}
          disabled={isDeleting || isStopping}
          title={zh ? "重启任务" : "Restart job"}
          aria-label={zh ? `重启任务 ${job.name}` : `Restart ${job.name}`}
        >
          <RotateCcw size={15} />
        </button>
      )}
      <button
        className="icon-button danger"
        onClick={onDelete}
        disabled={isDeleting || isStopping}
        title={zh ? "删除任务" : "Delete job"}
        aria-label={zh ? `删除任务 ${job.name}` : `Delete ${job.name}`}
      >
        <Trash2 size={15} />
      </button>
    </div>
  );
}

function JobLifecycleConfirmDialog({
  job,
  action,
  zh,
  pending,
  error,
  onClose,
  onConfirm,
}: {
  job: Job;
  action: "start" | "stop" | "clean-start" | "restart";
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

  const content = {
    start: {
      eyebrow: zh ? "任务启动" : "Start job",
      title: zh ? "确认启动任务？" : "Start this job?",
      description: zh
        ? "任务将按当前配置重新进入调度队列。"
        : "The job will re-enter the scheduling queue with its current configuration.",
      confirm: zh ? "确认启动" : "Start job",
      pending: zh ? "启动中…" : "Starting…",
      icon: <Play size={19} />,
    },
    stop: {
      eyebrow: zh ? "任务停止" : "Stop job",
      title: zh ? "确认停止任务？" : "Stop this job?",
      description: zh
        ? "平台将停止该任务的 Worker，当前运行连接会中断。"
        : "The platform will stop this job's workers and interrupt active connections.",
      confirm: zh ? "确认停止" : "Stop job",
      pending: zh ? "停止中…" : "Stopping…",
      icon: <Square size={18} />,
    },
    "clean-start": {
      eyebrow: zh ? "安全启动" : "Clean start",
      title: zh ? "清理后启动任务？" : "Clean up and start?",
      description: zh
        ? "平台会先停止并清理失败任务残留的 Worker，确认清理完成后再启动。"
        : "Residual workers will be stopped and cleaned up before the job starts.",
      confirm: zh ? "清理后启动" : "Clean and start",
      pending: zh ? "清理并启动中…" : "Cleaning and starting…",
      icon: <RotateCcw size={19} />,
    },
    restart: {
      eyebrow: zh ? "任务重启" : "Restart job",
      title: zh ? "确认重启任务？" : "Restart this job?",
      description: zh
        ? "任务将使用当前配置重新启动，现有运行连接会中断。"
        : "The job will restart with its current configuration and interrupt active connections.",
      confirm: zh ? "确认重启" : "Restart job",
      pending: zh ? "重启中…" : "Restarting…",
      icon: <RotateCcw size={19} />,
    },
  }[action];

  return (
    <div
      className="modal-backdrop job-lifecycle-backdrop"
      onMouseDown={(event) =>
        event.target === event.currentTarget && !pending && onClose()
      }
    >
      <section
        className={`modal job-lifecycle-modal ${action}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby="job-lifecycle-title"
      >
        <div className="job-lifecycle-head">
          <span className="job-lifecycle-icon">{content.icon}</span>
          <div>
            <span className="eyebrow">{content.eyebrow}</span>
            <h2 id="job-lifecycle-title">{content.title}</h2>
          </div>
          <button
            className="icon-button"
            onClick={onClose}
            disabled={pending}
            aria-label={zh ? "关闭" : "Close"}
          >
            ×
          </button>
        </div>
        <div className="job-lifecycle-body">
          <p>{content.description}</p>
          <div className="delete-job-target">
            <span>{zh ? "目标任务" : "Target job"}</span>
            <strong>{job.name}</strong>
          </div>
          {error && (
            <div className="delete-job-error" role="alert">
              {error}
            </div>
          )}
        </div>
        <div className="job-lifecycle-actions">
          <button
            className="secondary-button"
            onClick={onClose}
            disabled={pending}
          >
            {zh ? "取消" : "Cancel"}
          </button>
          <button
            className="primary-button"
            onClick={() => void onConfirm()}
            disabled={pending}
          >
            {pending ? (
              <LoaderCircle className="job-action-loading" size={15} />
            ) : (
              content.icon
            )}
            {pending ? content.pending : content.confirm}
          </button>
        </div>
      </section>
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
                ? "正在停止并清理…"
                : "Stopping and cleaning up…"
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
  onSelectNode,
  onSelectCluster,
  nodePullProgressMap = {},
  nodeEventsMap = {},
  nodeDeviceModelMap = {},
  nodeDiskWarningMap = {},
  allJobTags = [],
  onPatchJob,
}: {
  job: Job;
  copy: CopyType;
  isMockMode: boolean;
  onBack: () => void;
  onClone?: () => void;
  lifecycleActions: JobLifecycleActions;
  onSelectNode?: (name: string) => void;
  onSelectCluster?: (id: string) => void;
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
  nodeDiskWarningMap?: Record<string, boolean>;
  allJobTags?: Array<{ key: string; values: string[] }>;
  onPatchJob?: (
    jobName: string,
    patchBody: Record<string, any>,
  ) => Promise<Job>;
}) {
  const zh = c.nav.overview === "总览";
  // 是否处于编辑/重启中（任务名称编辑需置灰）
  const isDeleting = job.phase === "Deleting";
  const isStopping = job.stopped && job.phase !== "Stopped";
  const isUpdating = lifecycleActions.pending !== null || isDeleting;
  const [jobIdCopied, setJobIdCopied] = useState(false);

  // 任务名称内联编辑状态
  const [nameEditing, setNameEditing] = useState(false);
  const [nameDraft, setNameDraft] = useState(job.displayName);
  const [nameSaving, setNameSaving] = useState(false);
  const [nameError, setNameError] = useState("");
  // 与创建任务一致：输入过程中立即校验名称格式
  const nameInvalid =
    nameDraft.trim().length > 0 && !isValidJobDisplayName(nameDraft.trim());

  // 当 job 切换时重置
  useEffect(() => {
    setNameDraft(job.displayName);
    setNameEditing(false);
    setNameError("");
  }, [job.id]);

  const startNameEdit = () => {
    if (isUpdating) return;
    setNameDraft(job.displayName);
    setNameEditing(true);
    setNameError("");
  };

  const cancelNameEdit = () => {
    setNameDraft(job.displayName);
    setNameEditing(false);
    setNameError("");
  };

  const saveNameEdit = async () => {
    const trimmed = nameDraft.trim();
    if (!trimmed) {
      setNameError(zh ? "任务名称不能为空" : "Name cannot be empty");
      return;
    }
    if (trimmed.length > JOB_DISPLAY_NAME_MAX_LENGTH) {
      setNameError(
        zh
          ? `任务名称不能超过 ${JOB_DISPLAY_NAME_MAX_LENGTH} 个字符`
          : `Name too long (max ${JOB_DISPLAY_NAME_MAX_LENGTH})`,
      );
      return;
    }
    if (!isValidJobDisplayName(trimmed)) {
      setNameError(
        zh
          ? "名称格式不正确，仅支持中英文、数字以及-_."
          : "Invalid name format. Only Chinese/English letters, digits, -, _ and . are allowed.",
      );
      return;
    }
    if (trimmed === job.displayName) {
      setNameEditing(false);
      return;
    }
    if (!onPatchJob) {
      setNameError(zh ? "当前环境不支持修改" : "Editing not available");
      return;
    }
    setNameSaving(true);
    setNameError("");
    try {
      await onPatchJob(job.name, {
        metadata: {
          annotations: { "rlark.io/display-name": trimmed },
        },
      });
      setNameEditing(false);
    } catch (e) {
      setNameError(e instanceof Error ? e.message : String(e));
    } finally {
      setNameSaving(false);
    }
  };
  const handleCopyResourceId = async () => {
    if (!(await copyText(job.id))) return;
    setJobIdCopied(true);
    window.setTimeout(() => setJobIdCopied(false), 1600);
  };
  const [activeTab, setActiveTab] = useState<"workers" | "logs" | "metrics">(
    "workers",
  );
  const [taskNodes, setTaskNodes] = useState<Record<string, string>>({});
  const [taskClusters, setTaskClusters] = useState<Record<string, string>>({});
  const [tensorBoardProxy, setTensorBoardProxy] = useState<string>("");
  const [pullProgressMap, setPullProgressMap] = useState<
    Record<string, PullProgressEntry[]>
  >({});
  // Task.status.events 缓存，keyed by lowercased task 名；用于无 pod.node
  // 的 fallback worker 行展示 warning 事件。
  const [taskEventsMap, setTaskEventsMap] = useState<
    Record<string, NodeEventEntry[]>
  >({});
  // Task 列表缓存，用于获取节点 RANK
  const [tasks, setTasks] = useState<CRDTask[]>([]);
  const [detailNodeDiskWarningMap, setDetailNodeDiskWarningMap] = useState<
    Record<string, boolean>
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
  const [backendLogs, setBackendLogs] = useState<
    Array<{
      timestamp: string;
      line: string;
      labels?: Record<string, string>;
      fields?: Record<string, any>;
    }>
  >([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsError, setLogsError] = useState<string | null>(null);
  // 无限滚动：内部游标状态，用户不感知
  const [logsHasMore, setLogsHasMore] = useState(false);
  const [logsNextCursor, setLogsNextCursor] = useState("");
  const [logsLoadingMore, setLogsLoadingMore] = useState(false);
  // 角色配置区块的角色选择（默认第一个角色）
  const [workerRoleFilter, setWorkerRoleFilter] = useState(
    job.resources.length > 0 ? job.resources[0].role : "All",
  );
  // Worker 列表的角色筛选（独立于角色配置区块，默认 All）
  const [workerListRoleFilter, setWorkerListRoleFilter] = useState("All");
  const [workerPage, setWorkerPage] = useState(1);
  const [workerSort, setWorkerSort] = useState<{
    key: "createdAt";
    direction: SortDirection;
  }>({ key: "createdAt", direction: "desc" });
  // Worker 表表头多选筛选；空数组 = 全部
  const [workerRoleFilterValues, setWorkerRoleFilterValues] = useState<
    string[]
  >([]);
  const [workerPhaseFilter, setWorkerPhaseFilter] = useState<string[]>([]);
  const [workerClusterFilter, setWorkerClusterFilter] = useState<string[]>([]);
  const [workerKindFilter, setWorkerKindFilter] = useState<string[]>([]);
  const workerColumnFilter = useColumnFilter();
  const workerTableRef = useRef<HTMLDivElement>(null);
  const workerTableDrag = useRef({ active: false, x: 0, scrollLeft: 0 });
  const [workerTableDragging, setWorkerTableDragging] = useState(false);
  const [workerRefreshKey, setWorkerRefreshKey] = useState(0);
  const [workerRefreshing, setWorkerRefreshing] = useState(false);
  // 默认选中第一个角色（不再支持"所有角色"）
  const [logRoleFilter, setLogRoleFilter] = useState(() =>
    job.resources.length > 0 ? job.resources[0].role : "",
  );
  const [logWorkerFilter, setLogWorkerFilter] = useState("All");
  const [logQuery, setLogQuery] = useState("");
  const [logQueryInput, setLogQueryInput] = useState(""); // 输入框的临时值
  const logQueryInputRef = useRef<HTMLInputElement>(null); // 输入框引用
  const [logRange, setLogRange] = useState("1h");
  const [logCustomRange, setLogCustomRange] = useState(false);
  const [logCustomFrom, setLogCustomFrom] = useState("");
  const [logCustomTo, setLogCustomTo] = useState("");
  const [logOrder, setLogOrder] = useState<"desc" | "asc">("desc");
  const [logFullscreen, setLogFullscreen] = useState(false);
  const [logCopied, setLogCopied] = useState(false);

  // 全屏时锁定 body 滚动，防止背景跟着滚
  useEffect(() => {
    if (!logFullscreen) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [logFullscreen]);
  // 从后端获取的历史 Worker 列表（用于已停止任务的日志查询）
  const [logWorkersFromBackend, setLogWorkersFromBackend] = useState<string[]>(
    [],
  );

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
      const items = await tasksApi.list({ labelSelector });
      const nodeMap: Record<string, string> = {};
      const clusterMap: Record<string, string> = {};
      const progressMap: Record<string, PullProgressEntry[]> = {};
      const taskEventsMap: Record<string, NodeEventEntry[]> = {};
      const observedNodes = new Map<string, string>();
      let tbProxy = "";
      for (const item of items) {
        const taskName = item.metadata?.name ?? "";
        const taskNamespace = item.metadata?.namespace ?? "";
        const taskObservedNodes = item.status?.observedNodes ?? [];
        for (const nodeName of taskObservedNodes) {
          if (nodeName)
            observedNodes.set(`${taskNamespace}/${nodeName}`, taskNamespace);
        }
        nodeMap[taskName] = taskObservedNodes.join(", ") || "—";
        clusterMap[taskName] = item.metadata?.namespace ?? "—";
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
      const nodeResponses = await Promise.all(
        [...observedNodes].map(async ([nodeKey, namespace]) => {
          const nodeName = nodeKey.slice(namespace.length + 1);
          try {
            return await nodesApi.get(nodeName, { namespace });
          } catch {
            return null;
          }
        }),
      );
      const diskWarningMap: Record<string, boolean> = {};
      for (const node of nodeResponses) {
        const nodeName = node?.metadata?.name;
        if (nodeName) {
          diskWarningMap[nodeName] = isDiskUsageWarning(node as CRDNode);
        }
      }

      setTaskNodes(nodeMap);
      setTaskClusters(clusterMap);
      setTensorBoardProxy(tbProxy);
      setPullProgressMap(progressMap);
      setTaskEventsMap(taskEventsMap);
      // 保存 Task 列表，用于获取节点 RANK
      setTasks(items);
      setDetailNodeDiskWarningMap(diskWarningMap);
    },
    10000,
    [job.name],
  );

  const workerTaskNames = job.taskStatuses.map((ts) => {
    const childTaskName = ts.name.toLowerCase().replace(/\s+/g, "-");
    return `${job.name}-${childTaskName}`.toLowerCase().replace(/\s+/g, "-");
  });
  const workerTaskNamesKey = workerTaskNames.join(",");

  const handleWorkerRefresh = async () => {
    if (workerRefreshing) return;
    setWorkerRefreshing(true);
    await refreshTasks();
    setWorkerRefreshKey((key) => key + 1);
  };

  useEffect(() => {
    if (!workerTaskNamesKey) {
      setPods([]);
      setDomainIPMap({});
      setWorkerRefreshing(false);
      return;
    }
    let cancelled = false;
    setWorkerRefreshing(true);
    const labelSelector = `rlark.io/task-name in (${workerTaskNamesKey})`;

    const domainsPromise = domainsApi.list();
    const podsPromise = podsApi.list({ labelSelector });

    Promise.all([podsPromise, domainsPromise])
      .then(([podItems, domainItems]) => {
        if (cancelled) return;
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
            const data = await podsApi.events<{ events?: NodeEventEntry[] }>(
              podName,
            );
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

  // 返回有效的时间范围；如果自定义时间 from >= to，返回 null 表示无效
  const getLogTimeRange = (): { from: string; to: string } | null => {
    if (logCustomRange) {
      if (logCustomFrom && logCustomTo) {
        // datetime-local 返回的是本地时间字符串（如 "2026-09-16T14:30"），
        // 需要手动添加时区偏移，确保被正确解析为本地时间，再转换为 UTC
        const fromDate = new Date(logCustomFrom + ":00");
        const toDate = new Date(logCustomTo + ":00");
        // 校验开始时间必须早于结束时间
        if (fromDate.getTime() >= toDate.getTime()) {
          return null;
        }
        return { from: fromDate.toISOString(), to: toDate.toISOString() };
      }
      // 自定义时间不完整时返回 null，不发起查询
      return null;
    }
    const to = new Date();
    const from = new Date();
    switch (logRange) {
      case "15m":
        from.setMinutes(from.getMinutes() - 15);
        break;
      case "1h":
        from.setHours(from.getHours() - 1);
        break;
      case "6h":
        from.setHours(from.getHours() - 6);
        break;
      case "24h":
        from.setHours(from.getHours() - 24);
        break;
      case "7d":
        from.setDate(from.getDate() - 7);
        break;
      case "30d":
        from.setDate(from.getDate() - 30);
        break;
      default:
        from.setHours(from.getHours() - 1);
    }
    return { from: from.toISOString(), to: to.toISOString() };
  };

  // 判断当前自定义时间范围是否有效（用于 UI 提示）
  const isCustomRangeInvalid =
    logCustomRange &&
    logCustomFrom &&
    logCustomTo &&
    new Date(logCustomFrom).getTime() >= new Date(logCustomTo).getTime();

  const fetchLogs = async (isInitial = true, cursor = "") => {
    if (activeTab !== "logs") return;
    // 如果时间范围无效（自定义时间 from >= to），直接报错，不发起请求
    const timeRange = getLogTimeRange();
    if (!timeRange) {
      setLogsError(
        zh ? "开始时间必须早于结束时间" : "Start time must be before end time",
      );
      return;
    }
    if (isInitial) setLogsLoading(true);
    if (cursor) setLogsLoadingMore(true);
    setLogsError(null);
    try {
      const params = new URLSearchParams();
      params.set("from", timeRange.from);
      params.set("to", timeRange.to);

      // Always send the first task as the base filter (backend requires it)
      let taskName = "";
      if (logRoleFilter !== "All") {
        const resource = job.resources.find((r) => r.role === logRoleFilter);
        taskName = resource
          ? taskResourceName(job.name, resource.role)
          : logRoleFilter;
      } else if (logWorkerFilter !== "All") {
        const matchedPod = pods.find((p) => p.podName === logWorkerFilter);
        if (matchedPod && matchedPod.taskName) {
          taskName = matchedPod.taskName;
        }
      }

      if (!taskName && job.resources.length > 0) {
        // Default to the first resource's task if nothing selected
        taskName = taskResourceName(job.name, job.resources[0].role);
      }

      if (taskName) {
        params.set("task", taskName);
      }

      if (logWorkerFilter !== "All") {
        params.set("pod", logWorkerFilter);
      }

      if (logQuery.trim()) {
        params.set("query", logQuery.trim());
      }

      // 传递排序方式
      params.set("order", logOrder);

      if (cursor) {
        params.set("cursor", cursor);
      }

      const data = await jobsApi.logs<any>(
        job.name,
        Object.fromEntries(params.entries()),
      );
      if (data.source === "backend" && Array.isArray(data.entries)) {
        // 无限滚动：首次替换，追加时拼接
        if (cursor) {
          setBackendLogs((prev) => [...prev, ...data.entries]);
        } else {
          setBackendLogs(data.entries);
        }
        setPodLogs([]);
        setLogsHasMore(Boolean(data.hasMore));
        setLogsNextCursor(data.nextCursor || "");
      } else {
        setPodLogs(Array.isArray(data.pods) ? data.pods : []);
        setBackendLogs([]);
        setLogsHasMore(false);
        setLogsNextCursor("");
      }
    } catch (e) {
      if (!cursor) {
        setPodLogs([]);
        setBackendLogs([]);
      }
      setLogsError(e instanceof Error ? e.message : String(e));
    } finally {
      setLogsLoading(false);
      setLogsLoadingMore(false);
    }
  };

  // 滚动到底部附近时自动加载下一页
  const handleLogScroll = (e: ReactUIEvent<HTMLDivElement>) => {
    if (!logsHasMore || !logsNextCursor || logsLoadingMore || logsLoading) {
      return;
    }
    const el = e.currentTarget;
    const distanceToBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    if (distanceToBottom < 100) {
      fetchLogs(false, logsNextCursor);
    }
  };

  // 查询条件变化时重新查询
  useEffect(() => {
    fetchLogs(true);
    fetchLogWorkers(); // 时间范围变化时重新获取 Worker 列表
  }, [
    logRange,
    logCustomRange,
    logCustomFrom,
    logCustomTo,
    logRoleFilter,
    logWorkerFilter,
    logQuery,
    logOrder,
  ]);

  // 首次进入日志页面时自动加载数据
  useEffect(() => {
    if (activeTab === "logs") {
      fetchLogs(true);
      fetchLogWorkers(); // 同时获取历史 Worker 列表
    }
  }, [activeTab]);

  // 从后端获取历史 Worker 列表（用于已停止任务的日志查询）
  const fetchLogWorkers = async () => {
    try {
      const timeRange = getLogTimeRange();
      if (!timeRange) return;
      const params = new URLSearchParams();
      params.set("from", timeRange.from);
      params.set("to", timeRange.to);
      params.set("label", "pod");
      // 如果选择了特定角色，则传递 task 参数
      if (logRoleFilter !== "All") {
        const resource = job.resources.find((r) => r.role === logRoleFilter);
        const taskName = resource
          ? taskResourceName(job.name, resource.role)
          : logRoleFilter;
        params.set("task", taskName);
      }
      const data = await jobsApi.logLabelValues<{ values?: string[] }>(
        job.name,
        Object.fromEntries(params.entries()),
      );
      if (Array.isArray(data.values)) {
        setLogWorkersFromBackend(data.values);
      }
    } catch (e) {
      // 静默失败，不影响日志查询主流程
      console.error("failed to fetch log workers:", e);
    }
  };

  const workerStatusSummary = (
    message: string | undefined,
    events: NodeEventEntry[],
    failed: boolean,
  ) => {
    if (
      message === "FailedScheduling" ||
      events.some((event) => event.reason === "FailedScheduling")
    ) {
      return zh
        ? "没有合适的节点可调度，资源可能被占用"
        : "No suitable node is available; resources may be occupied";
    }
    return failed ? jobFailureMessage(message, zh) : message;
  };

  const fallbackWorkers: WorkerItem[] = [];
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
              ? (nodePullProgressMap[pod.node] ?? []).filter(
                  (progress) => progress.image === resource?.image,
                )
              : [];
          // 同样从 Node.status.events 取节点 warning 事件；当 pod.node
          // 缺失时回退到 Task.status.events，让 Pending worker 行 tooltip
          // 在调度前就能展示 DiskPressure 等原因。
          const workerEvents =
            phase === "Pending" || phase === "Failed"
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
            cluster: pod.taskNamespace || pod.namespace || "—",
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
            statusMessage: workerStatusSummary(
              pod.message,
              workerEvents,
              phase === "Failed",
            ),
            pullProgress: nodePullProgress,
            events: workerEvents,
          };
        })
      : fallbackWorkers;
  const runningWorkerCount = jobWorkers.filter(
    (worker) => worker.phase === "Running",
  ).length;
  const displayPhase = effectiveJobPhase(job);
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
  const workerRoles = [
    ...new Set(job.resources.map((resource) => resource.role)),
  ];
  const filteredWorkers = jobWorkers.filter(
    (worker) =>
      workerListRoleFilter === "All" || worker.role === workerListRoleFilter,
  );
  const toggleWorkerSort = (key: typeof workerSort.key) => {
    setWorkerSort((current) => ({
      key,
      direction:
        current.key === key && current.direction === "asc" ? "desc" : "asc",
    }));
    setWorkerPage(1);
  };
  // 表头筛选 options（从 jobWorkers 去重）
  const workerRoleOptions = useMemo(() => {
    const set = new Set<string>();
    jobWorkers.forEach((w) => w.role && set.add(w.role));
    return [...set].sort().map((v) => ({ value: v, label: v }));
  }, [jobWorkers]);
  const workerPhaseOptions = useMemo(() => {
    const set = new Set<string>();
    jobWorkers.forEach((w) => w.phase && set.add(w.phase));
    return [...set].sort().map((v) => ({
      value: v,
      label: (c.status as Record<string, string>)[v] ?? v,
    }));
  }, [jobWorkers, c]);
  const workerClusterOptions = useMemo(() => {
    const set = new Set<string>();
    jobWorkers.forEach((w) => w.cluster && set.add(w.cluster));
    return [...set].sort().map((v) => ({ value: v, label: v }));
  }, [jobWorkers]);
  const workerKindOptions = useMemo(() => {
    const set = new Set<string>();
    jobWorkers.forEach((w) => set.add(getNodeKindLabel(w)));
    return [...set].sort().map((v) => ({ value: v, label: v }));
  }, [jobWorkers]);
  // 在过滤 + 排序前叠加表头多选筛选（角色/状态/集群/节点类型）
  const columnFilteredWorkers = filteredWorkers.filter((worker) => {
    const roleHit =
      workerRoleFilterValues.length === 0 ||
      workerRoleFilterValues.includes(worker.role);
    const phaseHit =
      workerPhaseFilter.length === 0 ||
      workerPhaseFilter.includes(worker.phase);
    const clusterHit =
      workerClusterFilter.length === 0 ||
      workerClusterFilter.includes(worker.cluster ?? "");
    const kindHit =
      workerKindFilter.length === 0 ||
      workerKindFilter.includes(getNodeKindLabel(worker));
    return roleHit && phaseHit && clusterHit && kindHit;
  });
  const sortedWorkers = columnFilteredWorkers
    .map((worker) => ({ worker, index: jobWorkers.indexOf(worker) }))
    .sort((left, right) =>
      compareSortValues(
        formatWorkerCreatedAt(job.startedAt, left.index),
        formatWorkerCreatedAt(job.startedAt, right.index),
        workerSort.direction,
        zh ? "zh-CN" : "en",
      ),
    );
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
  const logEntries =
    backendLogs.length > 0
      ? backendLogs.map((entry, index) => {
          const taskName = entry.labels?.task || entry.fields?.task || "";
          const podName = entry.labels?.pod || entry.fields?.pod || "";
          const node = entry.labels?.node || entry.fields?.node || "";
          const role =
            resourceForTask(taskName)?.role ??
            job.resources.find((resource) => resource.role === taskName)
              ?.role ??
            taskName;
          return {
            id: `${podName}-${entry.timestamp}-${index}`,
            worker: podName,
            role,
            phase: "Running", // Backend logs don't have phase, assume running
            node,
            message: entry.line,
            timestamp: entry.timestamp,
          };
        })
      : podLogs.flatMap((pod) => {
          const role =
            resourceForTask(pod.taskName)?.role ??
            job.resources.find((resource) => resource.role === pod.taskName)
              ?.role ??
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
              timestamp: undefined,
            }));
        });
  // Derive available roles and workers from Job metadata (resources and pods)
  // so the dropdowns remain populated and selectable even when log query returns 0 entries.
  const logRoles = useMemo(() => {
    const rolesFromResources = job.resources.map((r) => r.role);
    const rolesFromPods = pods.map((p) => {
      const resource = resourceForTask(p.taskName);
      return resource?.role ?? p.taskName;
    });
    return [...new Set([...rolesFromResources, ...rolesFromPods])].filter(
      Boolean,
    );
  }, [job.resources, pods]);

  const logWorkers = useMemo(() => {
    // 优先使用从后端获取的历史 Worker 列表（支持已停止任务）
    if (logWorkersFromBackend.length > 0) {
      return logWorkersFromBackend;
    }
    // 否则从当前运行中的 Pod 获取
    return [
      ...new Set(
        pods
          .filter((p) => {
            if (logRoleFilter === "All") return true;
            const resource = resourceForTask(p.taskName);
            const role = resource?.role ?? p.taskName;
            return role === logRoleFilter;
          })
          .map((p) => p.podName),
      ),
    ].filter(Boolean);
  }, [pods, logRoleFilter, logWorkersFromBackend]);

  // 任务日志可查的最晚结束时刻（本地 datetime-local 字符串）。
  // 运行中/Pending 等状态：当前时间；
  // 已停止/失败/成功：min(stoppedAt, 当前时间)，停止之后没有日志。
  const logMaxEndLocal = useMemo(() => {
    const pad = (n: number) => String(n).padStart(2, "0");
    const toLocalInput = (d: Date) =>
      `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    const now = new Date();
    const isTerminated =
      displayPhase === "Stopped" ||
      displayPhase === "Failed" ||
      displayPhase === "Succeeded";
    if (isTerminated && job.stoppedAt && job.stoppedAt !== "—") {
      const stopped = new Date(job.stoppedAt);
      if (
        !Number.isNaN(stopped.getTime()) &&
        stopped.getTime() < now.getTime()
      ) {
        return toLocalInput(stopped);
      }
    }
    return toLocalInput(now);
  }, [displayPhase, job.stoppedAt]);

  const filteredLogEntries = logEntries.filter(
    (entry) =>
      (logRoleFilter === "All" || entry.role === logRoleFilter) &&
      (logWorkerFilter === "All" || entry.worker === logWorkerFilter) &&
      `${entry.message} ${entry.role} ${entry.worker}`
        .toLowerCase()
        .includes(logQuery.toLowerCase()),
  );

  // 复制全部日志到剪贴板：每行 "[时间] [worker] message"
  const copyAllLogs = async () => {
    const lines = filteredLogEntries.map((entry) => {
      const time = entry.timestamp
        ? new Date(entry.timestamp).toLocaleString(zh ? "zh-CN" : "en-US", {
            year: "numeric",
            month: "2-digit",
            day: "2-digit",
            hour: "2-digit",
            minute: "2-digit",
            second: "2-digit",
            hour12: false,
          })
        : "";
      const parts: string[] = [];
      if (time) parts.push(`[${time}]`);
      if (entry.worker) parts.push(`[${entry.worker}]`);
      parts.push(entry.message);
      return parts.join(" ");
    });
    try {
      await navigator.clipboard.writeText(lines.join("\n"));
      setLogCopied(true);
      window.setTimeout(() => setLogCopied(false), 2000);
    } catch {
      // 剪贴板不可用（非安全上下文等）时静默失败
    }
  };
  const tabs: Array<{ id: typeof activeTab; label: string }> = [
    { id: "workers", label: zh ? "详情" : "Details" },
    { id: "logs", label: c.common.logs },
    { id: "metrics", label: zh ? "监控" : "Metrics" },
  ];
  const jobFailedMessage =
    displayPhase === "Failed"
      ? [
          ...new Set(
            job.taskStatuses
              .filter((ts) => ts.phase === "Failed")
              .map((ts) => jobFailureMessage(ts.message, zh)),
          ),
        ].join("\n")
      : undefined;
  return (
    <div className="page-content resource-page job-detail-page">
      {/* 顶部返回与操作栏 */}
      <div className="job-detail-summary-head">
        <div>
          <button className="plain-button back-button" onClick={onBack}>
            ← {zh ? "返回任务列表" : "Back"}
          </button>
          <span className="eyebrow">{c.jobs.selected}</span>
          {nameEditing ? (
            <div className="job-name-edit">
              <input
                value={nameDraft}
                maxLength={JOB_DISPLAY_NAME_MAX_LENGTH}
                autoFocus
                disabled={nameSaving}
                className={nameInvalid ? "input-invalid" : undefined}
                placeholder={
                  zh
                    ? "请输入名称,支持1-64字符,中英文、数字以及-_."
                    : "Enter a name (1-64 chars; Chinese/English, digits, -, _ and .)"
                }
                onChange={(e) => setNameDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void saveNameEdit();
                  else if (e.key === "Escape") cancelNameEdit();
                }}
                onBlur={() => {
                  // 失焦自动保存（如果有改动的话）
                  if (nameDraft.trim() !== job.displayName) void saveNameEdit();
                  else cancelNameEdit();
                }}
              />
              {nameSaving && <span className="job-name-saving">...</span>}
              {nameInvalid && (
                <span className="job-name-edit-error">
                  {zh
                    ? "名称格式不正确，仅支持中英文、数字以及-_."
                    : "Invalid name format. Only Chinese/English letters, digits, -, _ and . are allowed."}
                </span>
              )}
              {nameError && (
                <span className="job-name-edit-error">{nameError}</span>
              )}
            </div>
          ) : (
            <h2
              className={`job-title-row${isUpdating ? " is-disabled" : ""}`}
              onClick={startNameEdit}
            >
              <span>{job.displayName}</span>
              <button
                type="button"
                className="job-title-edit-btn"
                onClick={(e) => {
                  e.stopPropagation();
                  startNameEdit();
                }}
                disabled={isUpdating}
                title={
                  isUpdating
                    ? zh
                      ? "重启或执行中，无法编辑名称"
                      : "Editing disabled during lifecycle operation"
                    : zh
                      ? "编辑任务名称"
                      : "Edit job name"
                }
                aria-label={zh ? "编辑任务名称" : "Edit job name"}
              >
                <Pencil size={20} />
              </button>
            </h2>
          )}
          <div className="job-detail-resource-line">
            <button
              type="button"
              className="plain-button job-id-copy"
              onClick={handleCopyResourceId}
              title={zh ? "复制资源 ID" : "Copy resource ID"}
              aria-label={zh ? "复制资源 ID" : "Copy resource ID"}
            >
              {jobIdCopied ? <Check size={13} /> : <Copy size={13} />}
              <span>{job.id}</span>
            </button>
            <span>
              {c.jobType[job.type]} · {job.cluster}
            </span>
          </div>
        </div>
        <div className="job-detail-summary-status">
          <div className="status-with-info">
            <StatusBadge phase={displayPhase} copy={c} />
            {(jobPullProgress.length > 0 ||
              jobEvents.length > 0 ||
              jobFailedMessage) && (
              <PullProgressInfo
                progress={jobPullProgress}
                events={jobEvents}
                zh={zh}
                statusMessage={jobFailedMessage}
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
                disabled={
                  lifecycleActions.pending !== null || isDeleting || isStopping
                }
              >
                <Copy size={15} />
                {zh ? "复制" : "Clone"}
              </button>
            )}
            {job.stopped || job.phase === "Stopped" ? (
              <button
                className="secondary-button primary-action"
                onClick={lifecycleActions.onStart}
                disabled={
                  lifecycleActions.pending !== null || isDeleting || isStopping
                }
              >
                {lifecycleActions.pending === "start" ? (
                  <LoaderCircle className="job-action-loading" size={15} />
                ) : (
                  <Play size={15} />
                )}
                {zh ? "启动" : "Start"}
              </button>
            ) : !["Succeeded"].includes(job.phase) ? (
              <button
                className="secondary-button"
                onClick={lifecycleActions.onStop}
                disabled={
                  lifecycleActions.pending !== null || isDeleting || isStopping
                }
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
              disabled={
                lifecycleActions.pending !== null || isDeleting || isStopping
              }
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
              disabled={
                lifecycleActions.pending !== null || isDeleting || isStopping
              }
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

      {/* Tab 导航切换（切换 详情 / 日志 / 监控），与下方内容融合为一个整体卡片 */}
      <div className="job-tab-panel">
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
          <>
            {/* 第一块：角色配置信息（平铺展示，支持切换角色） */}
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

            {/* 第二块：公共配置 */}
            <JobPublicOverview
              job={job}
              copy={c}
              onBack={onBack}
              runningWorkerCount={runningWorkerCount}
              totalWorkers={pods.length || job.workers}
              displayPhase={displayPhase}
              tensorBoardProxy={tensorBoardProxy}
              onClone={onClone}
              lifecycleActions={lifecycleActions}
              jobPullProgress={jobPullProgress}
              jobEvents={jobEvents}
              jobFailedMessage={jobFailedMessage}
              allJobTags={allJobTags}
              onPatchJob={onPatchJob}
            />

            {/* 第三块：Pod 实例列表 */}
            <div className="worker-primary-panel worker-console-panel">
              <div className="worker-panel-head">
                <div>
                  <span className="eyebrow">
                    {zh ? "Worker 列表" : "Runtime workers"}
                  </span>
                  <h3>{zh ? "Worker 维度查看实例信息" : "Worker list"}</h3>
                </div>
                <div className="worker-panel-actions">
                  <div
                    className="role-runtime-tabs worker-list-role-tabs"
                    aria-label={zh ? "筛选角色" : "Filter by role"}
                  >
                    <button
                      className={workerListRoleFilter === "All" ? "active" : ""}
                      onClick={() => {
                        setWorkerListRoleFilter("All");
                        setWorkerPage(1);
                      }}
                    >
                      All
                    </button>
                    {workerRoles.map((role) => (
                      <button
                        key={role}
                        className={
                          role === workerListRoleFilter ? "active" : ""
                        }
                        onClick={() => {
                          setWorkerListRoleFilter(role);
                          setWorkerPage(1);
                        }}
                      >
                        {role}
                      </button>
                    ))}
                  </div>
                  <span className="worker-total-count">
                    {filteredWorkers.length} {zh ? "个 Pod" : "pods"}
                  </span>
                  <button
                    className="secondary-button worker-refresh-button"
                    onClick={handleWorkerRefresh}
                    disabled={workerRefreshing}
                    aria-busy={workerRefreshing}
                    title={zh ? "刷新 Worker 列表" : "Refresh worker list"}
                  >
                    <RotateCcw
                      size={14}
                      className={workerRefreshing ? "job-action-loading" : ""}
                    />
                    {workerRefreshing
                      ? zh
                        ? "刷新中..."
                        : "Refreshing..."
                      : zh
                        ? "刷新"
                        : "Refresh"}
                  </button>
                </div>
              </div>
              <div
                ref={workerTableRef}
                className={`worker-table worker-console-table worker-table-scroll refreshable-region${workerTableDragging ? " dragging" : ""}${workerRefreshing ? " is-refreshing" : ""}`}
                aria-busy={workerRefreshing}
                onPointerDown={handleWorkerTablePointerDown}
                onPointerMove={handleWorkerTablePointerMove}
                onPointerUp={stopWorkerTableDrag}
                onPointerCancel={stopWorkerTableDrag}
              >
                <table>
                  <thead>
                    <tr>
                      <th>{zh ? "实例名称" : "Worker name"}</th>
                      <th>
                        <ColumnFilterButton
                          label={zh ? "角色" : "Role"}
                          selectedCount={workerRoleFilterValues.length}
                          onClick={workerColumnFilter.openFor("role")}
                        />
                      </th>
                      <th>
                        <ColumnFilterButton
                          label={zh ? "状态" : "Status"}
                          selectedCount={workerPhaseFilter.length}
                          onClick={workerColumnFilter.openFor("phase")}
                        />
                      </th>
                      <th>
                        <ColumnFilterButton
                          label={zh ? "集群" : "Cluster"}
                          selectedCount={workerClusterFilter.length}
                          onClick={workerColumnFilter.openFor("cluster")}
                        />
                      </th>
                      <th>{zh ? "节点" : "Node"}</th>
                      <th>
                        <ColumnFilterButton
                          label={zh ? "节点类型" : "Node type"}
                          selectedCount={workerKindFilter.length}
                          onClick={workerColumnFilter.openFor("kind")}
                        />
                      </th>
                      <th>{zh ? "节点 RANK" : "Node RANK"}</th>
                      <th>{zh ? "实例 IP" : "Worker IP"}</th>
                      <th>{zh ? "网络域 IP" : "Domain IP"}</th>
                      <th>{zh ? "申请 GPU" : "GPU"}</th>
                      <th>
                        <SortButton
                          label={zh ? "创建时间" : "Created"}
                          active={workerSort.key === "createdAt"}
                          direction={workerSort.direction}
                          onClick={() => toggleWorkerSort("createdAt")}
                        />
                      </th>
                      <th
                        className="worker-sticky-header-col"
                        aria-label={zh ? "操作" : "Actions"}
                      >
                        {zh ? "操作" : "Actions"}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {visibleWorkers.length > 0 ? (
                      visibleWorkers.map(({ worker, index }) => (
                        <WorkerTableRow
                          key={worker.id}
                          jobName={job.name}
                          worker={worker}
                          copy={c}
                          pods={workerPodsByTask.get(worker.id) ?? []}
                          domainIPMap={domainIPMap}
                          isHeader={worker.role === job.headerRole}
                          createdAt={formatWorkerCreatedAt(
                            job.startedAt,
                            index,
                          )}
                          onSelectNode={onSelectNode}
                          onSelectCluster={onSelectCluster}
                          diskWarning={
                            detailNodeDiskWarningMap[worker.node] ??
                            nodeDiskWarningMap[worker.node] ??
                            false
                          }
                          tasks={tasks}
                        />
                      ))
                    ) : (
                      <tr>
                        <td colSpan={11} className="empty-cell">
                          {zh
                            ? "当前没有运行中的 Worker 实例"
                            : "No running worker instances"}
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
                <RefreshOverlay
                  visible={workerRefreshing}
                  label={zh ? "正在刷新 Worker 列表" : "Refreshing worker list"}
                />
              </div>
              {visibleWorkers.length > 0 && (
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
                      onClick={() =>
                        setWorkerPage((page) => Math.max(1, page - 1))
                      }
                    >
                      {zh ? "上一页" : "Previous"}
                    </button>
                    <button
                      className="secondary-button"
                      disabled={workerPage >= workerPageCount}
                      onClick={() =>
                        setWorkerPage((page) =>
                          Math.min(workerPageCount, page + 1),
                        )
                      }
                    >
                      {zh ? "下一页" : "Next"}
                    </button>
                  </div>
                </div>
              )}
            </div>
          </>
        )}

        {activeTab === "logs" && (
          <div className="job-observe-panel">
            <div className="observe-panel-head">
              <div>
                <span className="eyebrow">{zh ? "任务日志" : "Job logs"}</span>
                <h3>{zh ? "Worker 日志流" : "Worker log stream"}</h3>
              </div>
            </div>
            {logsError && (
              <div className="log-error-banner" role="alert">
                <AlertTriangle size={16} />
                <span>{logsError}</span>
                <button
                  className="log-error-banner-close"
                  onClick={() => setLogsError(null)}
                  aria-label={zh ? "关闭" : "Close"}
                >
                  ×
                </button>
              </div>
            )}
            {logsLoading && (
              <div
                className="log-loading-state"
                role="status"
                aria-live="polite"
              >
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
            )}
            {!logsLoading && (
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
                      onChange={(event) =>
                        setLogWorkerFilter(event.target.value)
                      }
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
                      value={logCustomRange ? "custom" : logRange}
                      onChange={(event) => {
                        if (event.target.value === "custom") {
                          setLogCustomRange(true);
                          // 默认与「最近 1 小时」一致：from = to - 1h。
                          // to 取 logMaxEndLocal（运行中=现在；已停止/失败=任务停止时刻）。
                          const toDate = new Date(logMaxEndLocal);
                          const fromDate = new Date(
                            toDate.getTime() - 3600 * 1000,
                          );
                          const pad = (n: number) => String(n).padStart(2, "0");
                          const toLocalInput = (d: Date) =>
                            `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
                          setLogCustomTo(toLocalInput(toDate));
                          setLogCustomFrom(toLocalInput(fromDate));
                        } else {
                          setLogCustomRange(false);
                          setLogRange(event.target.value);
                        }
                      }}
                    >
                      <option value="15m">
                        {zh ? "最近 15 分钟" : "Last 15m"}
                      </option>
                      <option value="1h">
                        {zh ? "最近 1 小时" : "Last 1h"}
                      </option>
                      <option value="6h">
                        {zh ? "最近 6 小时" : "Last 6h"}
                      </option>
                      <option value="24h">
                        {zh ? "最近 24 小时" : "Last 24h"}
                      </option>
                      <option value="7d">{zh ? "最近 7 天" : "Last 7d"}</option>
                      <option value="30d">
                        {zh ? "最近 30 天" : "Last 30d"}
                      </option>
                      <option value="custom">
                        {zh ? "自定义时间" : "Custom range"}
                      </option>
                    </select>
                  </label>
                  {logCustomRange && (
                    <div
                      className="log-custom-range-inputs"
                      style={{
                        display: "flex",
                        gap: "6px",
                        alignItems: "center",
                      }}
                    >
                      <input
                        type="datetime-local"
                        className="log-custom-datetime"
                        value={logCustomFrom}
                        max={logMaxEndLocal}
                        onChange={(e) => setLogCustomFrom(e.target.value)}
                        style={
                          isCustomRangeInvalid
                            ? { borderColor: "var(--error, #ef4444)" }
                            : undefined
                        }
                      />
                      <span
                        style={{
                          fontSize: "12px",
                          color: "var(--text-secondary)",
                        }}
                      >
                        至
                      </span>
                      <input
                        type="datetime-local"
                        className="log-custom-datetime"
                        value={logCustomTo}
                        max={logMaxEndLocal}
                        onChange={(e) => setLogCustomTo(e.target.value)}
                        style={
                          isCustomRangeInvalid
                            ? { borderColor: "var(--error, #ef4444)" }
                            : undefined
                        }
                      />
                      {isCustomRangeInvalid && (
                        <span
                          style={{
                            fontSize: "12px",
                            color: "var(--error, #ef4444)",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {zh ? "开始时间需早于结束时间" : "Invalid range"}
                        </span>
                      )}
                    </div>
                  )}
                  <label className="log-order-filter">
                    <span>{zh ? "排序" : "Order"}</span>
                    <select
                      value={logOrder}
                      onChange={(event) =>
                        setLogOrder(event.target.value as "desc" | "asc")
                      }
                    >
                      <option value="desc">
                        {zh ? "最新在前" : "Newest first"}
                      </option>
                      <option value="asc">
                        {zh ? "最旧在前" : "Oldest first"}
                      </option>
                    </select>
                  </label>
                  <label className="log-search-field">
                    <Search size={15} />
                    <input
                      ref={logQueryInputRef}
                      value={logQueryInput}
                      onChange={(event) => setLogQueryInput(event.target.value)}
                      onBlur={() => setLogQuery(logQueryInput)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter") {
                          setLogQuery(logQueryInput);
                        }
                      }}
                      placeholder={
                        zh
                          ? "搜索日志内容（仅支持完整单词/词组）"
                          : "Search logs (full words only)"
                      }
                      title={
                        zh
                          ? "由于日志索引分词限制，请搜索完整的单词或词组"
                          : "Search with complete words due to log indexing rules"
                      }
                    />
                  </label>
                  <button
                    className="secondary-button"
                    onClick={() => fetchLogs(true)}
                    disabled={logsLoading}
                    title={zh ? "刷新日志" : "Refresh logs"}
                  >
                    <RotateCcw
                      size={14}
                      className={logsLoading ? "job-action-loading" : ""}
                    />
                    {logsLoading
                      ? zh
                        ? "刷新中…"
                        : "Refreshing…"
                      : zh
                        ? "刷新"
                        : "Refresh"}
                  </button>
                  <button
                    className="secondary-button log-export-button"
                    onClick={() => exportLogs(filteredLogEntries, job.name)}
                  >
                    <Download size={15} />
                    {zh ? "导出" : "Export"}
                  </button>
                </div>

                {logsLoading ? (
                  <div
                    className="log-loading-state"
                    role="status"
                    aria-live="polite"
                  >
                    <span className="log-loading-icon">
                      <LoaderCircle size={20} />
                    </span>
                    <div>
                      <strong>
                        {zh
                          ? "正在连接 Worker 日志"
                          : "Connecting to worker logs"}
                      </strong>
                      <small>
                        {zh
                          ? "正在汇总各实例的最新输出…"
                          : "Collecting the latest output from each instance…"}
                      </small>
                    </div>
                    <i className="log-loading-shimmer" aria-hidden="true" />
                  </div>
                ) : (
                  (() => {
                    const terminalNode = (
                      <div
                        className={`log-terminal ${logFullscreen ? "log-terminal-fullscreen" : ""}`}
                        aria-live="polite"
                      >
                        <div className="log-terminal-header">
                          <div className="log-terminal-title">
                            <span className="log-terminal-dot" />
                            <span className="log-terminal-dot" />
                            <span className="log-terminal-dot" />
                            <strong>{zh ? "任务日志" : "Job logs"}</strong>
                          </div>
                          <div className="log-terminal-actions">
                            <button
                              className="icon-button"
                              onClick={() => setLogFullscreen(!logFullscreen)}
                              title={
                                logFullscreen
                                  ? zh
                                    ? "退出全屏"
                                    : "Exit fullscreen"
                                  : zh
                                    ? "全屏"
                                    : "Fullscreen"
                              }
                            >
                              {logFullscreen ? (
                                <Minimize size={14} />
                              ) : (
                                <Maximize size={14} />
                              )}
                            </button>
                            <button
                              className="icon-button"
                              onClick={copyAllLogs}
                              title={
                                logCopied
                                  ? zh
                                    ? "已复制"
                                    : "Copied"
                                  : zh
                                    ? "复制全部日志"
                                    : "Copy all logs"
                              }
                            >
                              {logCopied ? (
                                <Check size={14} />
                              ) : (
                                <Copy size={14} />
                              )}
                            </button>
                          </div>
                        </div>
                        <div
                          className="log-terminal-content"
                          onScroll={handleLogScroll}
                        >
                          {filteredLogEntries.length > 0 ? (
                            filteredLogEntries.map((entry) => (
                              <div className="log-terminal-line" key={entry.id}>
                                <span className="log-terminal-time">
                                  {entry.timestamp
                                    ? new Date(entry.timestamp).toLocaleString(
                                        zh ? "zh-CN" : "en-US",
                                        {
                                          year: "numeric",
                                          month: "2-digit",
                                          day: "2-digit",
                                          hour: "2-digit",
                                          minute: "2-digit",
                                          second: "2-digit",
                                          hour12: false,
                                        },
                                      )
                                    : ""}
                                </span>
                                {logWorkerFilter === "All" && (
                                  <span className="log-terminal-worker">
                                    {entry.worker}
                                  </span>
                                )}
                                <span className="log-terminal-message">
                                  {entry.message}
                                </span>
                              </div>
                            ))
                          ) : (
                            <div className="empty-inline">
                              {zh
                                ? "未找到匹配的日志。"
                                : "No matching logs found."}
                            </div>
                          )}
                          {logsLoadingMore && (
                            <div className="log-terminal-loading-more">
                              {zh ? "加载中…" : "Loading…"}
                            </div>
                          )}
                        </div>
                      </div>
                    );
                    // 全屏时挂到 body 下，避免侧边栏/顶栏的 stacking context
                    // （半透明 + backdrop-filter）与全屏终端叠出脏视觉。
                    return logFullscreen
                      ? createPortal(terminalNode, document.body)
                      : terminalNode;
                  })()
                )}
              </>
            )}
          </div>
        )}

        {activeTab === "metrics" && (
          <div className="job-observe-panel">
            <div className="observe-panel-head">
              <div>
                <span className="eyebrow">
                  {zh ? "任务监控" : "Job metrics"}
                </span>
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
      {workerColumnFilter.openKey === "role" &&
        typeof document !== "undefined" &&
        createPortal(
          <ColumnFilterPopover
            label={zh ? "角色" : "Role"}
            options={workerRoleOptions}
            selected={workerRoleFilterValues}
            onChange={setWorkerRoleFilterValues}
            anchorRect={workerColumnFilter.anchorRect}
            onClose={workerColumnFilter.close}
            zh={zh}
          />,
          document.body,
        )}
      {workerColumnFilter.openKey === "phase" &&
        typeof document !== "undefined" &&
        createPortal(
          <ColumnFilterPopover
            label={zh ? "状态" : "Status"}
            options={workerPhaseOptions}
            selected={workerPhaseFilter}
            onChange={setWorkerPhaseFilter}
            anchorRect={workerColumnFilter.anchorRect}
            onClose={workerColumnFilter.close}
            zh={zh}
          />,
          document.body,
        )}
      {workerColumnFilter.openKey === "cluster" &&
        typeof document !== "undefined" &&
        createPortal(
          <ColumnFilterPopover
            label={zh ? "集群" : "Cluster"}
            options={workerClusterOptions}
            selected={workerClusterFilter}
            onChange={setWorkerClusterFilter}
            anchorRect={workerColumnFilter.anchorRect}
            onClose={workerColumnFilter.close}
            zh={zh}
          />,
          document.body,
        )}
      {workerColumnFilter.openKey === "kind" &&
        typeof document !== "undefined" &&
        createPortal(
          <ColumnFilterPopover
            label={zh ? "节点类型" : "Node type"}
            options={workerKindOptions}
            selected={workerKindFilter}
            onChange={setWorkerKindFilter}
            anchorRect={workerColumnFilter.anchorRect}
            onClose={workerColumnFilter.close}
            zh={zh}
          />,
          document.body,
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
  jobFailedMessage,
  allJobTags = [],
  onPatchJob,
}: {
  job: Job;
  copy: CopyType;
  onBack: () => void;
  runningWorkerCount: number;
  totalWorkers: number;
  displayPhase: JobDisplayPhase;
  tensorBoardProxy?: string;
  onClone?: () => void;
  lifecycleActions: JobLifecycleActions;
  jobPullProgress?: PullProgressEntry[];
  jobEvents?: NodeEventEntry[];
  jobFailedMessage?: string;
  allJobTags?: Array<{ key: string; values: string[] }>;
  onPatchJob?: (
    jobName: string,
    patchBody: Record<string, any>,
  ) => Promise<Job>;
}) {
  const zh = c.nav.overview === "总览";
  const [sshKeys, setSSHKeys] = useState<SSHUserKey[]>([]);
  const [sshKeysLoaded, setSSHKeysLoaded] = useState(false);

  // tags 内联编辑
  const [tagsEditing, setTagsEditing] = useState(false);
  const [tagsDraft, setTagsDraft] = useState<JobTag[]>(job.tags ?? []);
  const [tagsSaving, setTagsSaving] = useState(false);
  const [tagsError, setTagsError] = useState("");
  const [tagPopover, setTagPopover] = useState<{
    tags: JobTag[];
    anchor: DOMRect;
  } | null>(null);

  useEffect(() => {
    setTagsDraft(job.tags ?? []);
    setTagsEditing(false);
    setTagsError("");
    // 仅在切换任务时重置：列表自动刷新会让 job.tags 变成新引用，不能因此打断编辑
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [job.id]);

  useEffect(() => {
    if (!job.sshPublicKey) {
      setSSHKeys([]);
      setSSHKeysLoaded(true);
      return;
    }

    const controller = new AbortController();
    setSSHKeysLoaded(false);
    sshKeysApi
      .list(controller.signal)
      .then((data) => setSSHKeys(Array.isArray(data) ? data : []))
      .catch((error) => {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        setSSHKeys([]);
      })
      .finally(() => {
        if (!controller.signal.aborted) setSSHKeysLoaded(true);
      });

    return () => controller.abort();
  }, [job.sshPublicKey]);

  const resolvedSSHKeys = resolveSSHKeyOwners(job.sshPublicKey, sshKeys);
  const baseConfigRows = [
    {
      label: zh ? "Worker 数量" : "Worker count",
      value: `${runningWorkerCount} / ${totalWorkers}`,
    },
    {
      label: zh ? "创建时间" : "Created",
      value: formatTaskTime(job.startedAt),
    },
    {
      label: "Header Worker",
      value: job.headerRole || "—",
    },
    {
      label: zh ? "网络域" : "Network domain",
      value: job.domain || (zh ? "未配置" : "Not configured"),
      fullValue: job.domain,
      className: job.domain ? "public-config-truncated-value" : undefined,
    },
    {
      label: "TensorBoard",
      value: job.tensorBoardDir || (zh ? "未配置" : "Not configured"),
    },
  ];

  const tagGroups = Array.from(
    (job.tags ?? []).reduce((groups, tag) => {
      const values = groups.get(tag.key);
      if (values) {
        values.push(tag.value);
      } else {
        groups.set(tag.key, [tag.value]);
      }
      return groups;
    }, new Map<string, string[]>()),
  );
  const flatTags = tagGroups.flatMap(([key, values]) =>
    values.map((value) => ({ key, value })),
  );

  const saveTags = async () => {
    if (!onPatchJob) return;
    setTagsSaving(true);
    setTagsError("");
    try {
      await onPatchJob(job.name, {
        spec: {
          tags: [
            ...new Map(
              tagsDraft
                .filter((t) => t.key.trim() && t.value.trim())
                .map((t) => [
                  t.key.trim(),
                  {
                    key: t.key.trim(),
                    values: tagsDraft
                      .filter((item) => item.key.trim() === t.key.trim())
                      .map((item) => item.value.trim())
                      .filter(
                        (value, index, values) =>
                          value && values.indexOf(value) === index,
                      ),
                  },
                ]),
            ).values(),
          ],
        },
      });
      setTagsEditing(false);
    } catch (e) {
      setTagsError(e instanceof Error ? e.message : String(e));
    } finally {
      setTagsSaving(false);
    }
  };

  const cancelTagsEdit = () => {
    setTagsDraft(job.tags ?? []);
    setTagsEditing(false);
    setTagsError("");
  };

  const isUpdating =
    lifecycleActions.pending !== null || job.phase === "Deleting";
  return (
    <section className="job-detail-summary-card">
      <div className="role-runtime-heading" style={{ marginBottom: "16px" }}>
        <div>
          <span className="eyebrow">
            {zh ? "公共配置" : "Shared configuration"}
          </span>
          <strong>
            {zh ? "任务维度查看公共配置" : "Job-level attributes configuration"}
          </strong>
        </div>
      </div>
      <div className="public-runtime-card" style={{ width: "100%" }}>
        <div className="public-runtime-topology">
          <div className="public-command-card">
            <span className="public-config-title">
              {zh ? "启动命令" : "Start command"}
            </span>
            <CommandCodeBlock value={job.command || "—"} copy={c} />
          </div>
          <div className="public-basic-config-card">
            <span className="public-config-title">
              {zh ? "公共属性" : "Shared attributes"}
            </span>
            <div className="public-basic-config-list">
              {baseConfigRows.map((row) => (
                <div key={row.label}>
                  <span>{row.label}</span>
                  <code
                    className={row.className}
                    title={row.fullValue || row.value}
                  >
                    {row.value}
                  </code>
                </div>
              ))}
              <div className="public-ssh-keys-row">
                <span>{zh ? "SSH 公钥" : "SSH Public Keys"}</span>
                {resolvedSSHKeys.length > 0 ? (
                  <ul className="job-ssh-key-list">
                    {resolvedSSHKeys.map(({ publicKey, owners }, index) => {
                      const ownerLabel =
                        owners.length > 0
                          ? owners.map(({ user }) => user).join(", ")
                          : sshKeysLoaded
                            ? zh
                              ? "未知用户"
                              : "Unknown user"
                            : zh
                              ? "加载用户中…"
                              : "Loading user…";

                      return (
                        <li key={`${publicKey}-${index}`} title={publicKey}>
                          <strong title={ownerLabel}>{ownerLabel}</strong>
                          <code>{publicKey}</code>
                        </li>
                      );
                    })}
                  </ul>
                ) : (
                  <code>{zh ? "未配置" : "Not configured"}</code>
                )}
              </div>
              <div className="public-tags-row">
                <span className="public-tags-label">
                  {zh ? "标签" : "Tags"}
                </span>
                <div className="public-tags-content">
                  {flatTags.length > 0 ? (
                    <div className="job-tags-cell">
                      {flatTags.slice(0, 2).map(({ key, value }) => (
                        <span
                          key={`${key}-${value}`}
                          className="job-tag-chip"
                          title={`${key}: ${value}`}
                        >
                          {key}: {value}
                        </span>
                      ))}
                      {flatTags.length > 2 && (
                        <button
                          type="button"
                          className="job-tag-chip job-tag-overflow"
                          onClick={(event) => {
                            setTagPopover({
                              tags: job.tags ?? [],
                              anchor:
                                event.currentTarget.getBoundingClientRect(),
                            });
                          }}
                        >
                          +{flatTags.length - 2}
                        </button>
                      )}
                    </div>
                  ) : (
                    <span className="public-config-empty">
                      {zh ? "未配置" : "Not configured"}
                    </span>
                  )}
                  <button
                    type="button"
                    className="job-tags-edit-btn"
                    onClick={() => {
                      setTagsDraft(job.tags ?? []);
                      setTagsError("");
                      setTagsEditing(true);
                    }}
                    disabled={isUpdating || !onPatchJob}
                    title={
                      isUpdating
                        ? zh
                          ? "执行中，无法编辑"
                          : "Editing disabled during operation"
                        : zh
                          ? "编辑标签"
                          : "Edit tags"
                    }
                    aria-label={zh ? "编辑标签" : "Edit tags"}
                  >
                    <Pencil size={13} />
                  </button>
                </div>
              </div>
              {tagPopover &&
                typeof document !== "undefined" &&
                createPortal(
                  <JobTagPopover
                    tags={tagPopover.tags}
                    anchorRect={tagPopover.anchor}
                    zh={zh}
                    onClose={() => setTagPopover(null)}
                  />,
                  document.body,
                )}
              {tagsEditing &&
                typeof document !== "undefined" &&
                createPortal(
                  <div
                    className="modal-backdrop job-tags-modal-backdrop"
                    onMouseDown={(event) =>
                      event.target === event.currentTarget &&
                      !tagsSaving &&
                      cancelTagsEdit()
                    }
                  >
                    <section
                      className="modal job-tags-modal"
                      role="dialog"
                      aria-modal="true"
                      aria-labelledby="job-tags-modal-title"
                    >
                      <div className="modal-head">
                        <div>
                          <h2 id="job-tags-modal-title">
                            {zh ? "编辑标签" : "Edit tags"}
                          </h2>
                        </div>
                        <button
                          type="button"
                          className="icon-button"
                          onClick={cancelTagsEdit}
                          disabled={tagsSaving}
                          aria-label={zh ? "关闭" : "Close"}
                        >
                          <X size={18} />
                        </button>
                      </div>
                      <div className="job-tags-modal-body">
                        <TagEditor
                          tags={tagsDraft}
                          onChange={setTagsDraft}
                          suggestions={allJobTags}
                          zh={zh}
                          sectioned
                        />
                        {tagsError && (
                          <span className="public-tags-error">{tagsError}</span>
                        )}
                      </div>
                      <div className="modal-footer job-tags-modal-actions">
                        <button
                          type="button"
                          className="secondary-button"
                          onClick={cancelTagsEdit}
                          disabled={tagsSaving}
                        >
                          {zh ? "取消" : "Cancel"}
                        </button>
                        <button
                          type="button"
                          className="primary-button"
                          onClick={saveTags}
                          disabled={tagsSaving || isUpdating}
                        >
                          {tagsSaving
                            ? zh
                              ? "保存中…"
                              : "Saving…"
                            : zh
                              ? "确定"
                              : "Confirm"}
                        </button>
                      </div>
                    </section>
                  </div>,
                  document.querySelector(".app-shell") ?? document.body,
                )}
            </div>
          </div>
        </div>
      </div>
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

  return (
    <section className="role-runtime-config job-detail-summary-card">
      <div className="role-runtime-heading">
        <div>
          <span className="eyebrow">
            {zh ? "角色配置" : "Roles configuration"}
          </span>
          <strong>
            {zh
              ? "角色维度查看运行配置"
              : "Instances and configuration by role"}
          </strong>
        </div>
        <div
          className="role-runtime-tabs"
          aria-label={zh ? "选择角色" : "Select role"}
        >
          {roles.map((role) => (
            <button
              key={role}
              className={role === selectedRole ? "active" : ""}
              onClick={() => onRoleChange(role)}
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
        </div>
      ) : (
        <p className="role-runtime-all-summary">
          {zh
            ? `显示全部 ${roles.length} 个角色的 Worker`
            : `Showing workers from all ${roles.length} roles`}
        </p>
      )}
      {resource && (
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
              <span>{zh ? "节点选择" : "Node selection"}</span>
              {resourceNodes.length ? (
                <div>
                  {resourceNodes.map((node, index) => (
                    <p key={`${node}-${index}`}>
                      <code>{node}</code>
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
                zh ? "大小" : "Size",
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
                mount.type === "storage" ? `${mount.pvcSizeGb} Gi` : "",
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
    timestamp?: string;
  }>,
  jobName: string,
) {
  const header = "timestamp,worker,role,message";
  const rows = entries.map((entry) =>
    [entry.timestamp || "", entry.worker, entry.role, entry.message]
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

const workerEventReasonLabels: Record<string, { zh: string; en: string }> = {
  FailedScheduling: { zh: "调度失败", en: "Scheduling failed" },
  FailedMount: { zh: "存储挂载失败", en: "Storage mount failed" },
  FailedAttachVolume: { zh: "存储卷连接失败", en: "Volume attachment failed" },
  FailedBinding: { zh: "存储卷绑定失败", en: "Volume binding failed" },
  FailedMapVolume: { zh: "存储卷映射失败", en: "Volume mapping failed" },
  FailedUnMount: { zh: "存储卷卸载失败", en: "Volume unmount failed" },
  FailedMountOnFilesystemMismatch: {
    zh: "存储卷文件系统不匹配",
    en: "Volume filesystem mismatch",
  },
  VolumeResizeFailed: { zh: "存储卷扩容失败", en: "Volume resize failed" },
  FileSystemResizeFailed: {
    zh: "文件系统扩容失败",
    en: "Filesystem resize failed",
  },
  FailedCreatePodSandBox: {
    zh: "运行环境创建失败",
    en: "Pod sandbox creation failed",
  },
  FailedCreatePodContainer: {
    zh: "容器创建失败",
    en: "Container creation failed",
  },
  SandboxChanged: {
    zh: "运行环境已变更，正在重建",
    en: "Pod sandbox changed; recreating",
  },
  FailedCreate: { zh: "Worker 创建失败", en: "Worker creation failed" },
  Failed: { zh: "Worker 启动失败", en: "Worker startup failed" },
  FailedSync: {
    zh: "Worker 状态同步失败",
    en: "Worker state synchronization failed",
  },
  FailedKillPod: { zh: "Worker 停止失败", en: "Worker termination failed" },
  FailedPostStartHook: {
    zh: "容器启动钩子执行失败",
    en: "Container post-start hook failed",
  },
  FailedPreStopHook: {
    zh: "容器停止钩子执行失败",
    en: "Container pre-stop hook failed",
  },
  ErrImagePull: { zh: "镜像拉取失败", en: "Image pull failed" },
  ImagePullBackOff: { zh: "镜像拉取重试中", en: "Retrying image pull" },
  InvalidImageName: { zh: "镜像地址无效", en: "Invalid image name" },
  FailedToRetrieveImagePullSecret: {
    zh: "镜像凭据不可用",
    en: "Image credentials unavailable",
  },
  BackOff: { zh: "容器启动重试中", en: "Retrying container startup" },
  CrashLoopBackOff: {
    zh: "容器反复启动失败",
    en: "Container repeatedly failed to start",
  },
  OOMKilled: {
    zh: "容器内存不足被终止",
    en: "Container terminated due to insufficient memory",
  },
  Unhealthy: { zh: "健康检查失败", en: "Health check failed" },
  Evicted: { zh: "Worker 已被节点驱逐", en: "Worker evicted from node" },
  Preempted: {
    zh: "Worker 已被高优先级任务抢占",
    en: "Worker preempted by a higher-priority workload",
  },
  NodeNotReady: { zh: "节点不可用", en: "Node unavailable" },
  NodeNotReachable: { zh: "节点无法连接", en: "Node unreachable" },
  NodeNotSchedulable: { zh: "节点不可调度", en: "Node unschedulable" },
  DiskPressure: { zh: "节点磁盘空间不足", en: "Node disk pressure" },
  MemoryPressure: { zh: "节点内存压力", en: "Node memory pressure" },
  PIDPressure: { zh: "节点进程资源不足", en: "Node PID pressure" },
  OutOfDisk: { zh: "节点磁盘空间耗尽", en: "Node out of disk space" },
  NetworkUnavailable: { zh: "节点网络不可用", en: "Node network unavailable" },
  Rebooted: { zh: "节点已重启", en: "Node rebooted" },
  FreeDiskSpaceFailed: {
    zh: "镜像清理失败，磁盘空间不足",
    en: "Image cleanup failed; insufficient disk space",
  },
  ContainerGCFailed: { zh: "容器清理失败", en: "Container cleanup failed" },
  ImageGCFailed: { zh: "镜像清理失败", en: "Image cleanup failed" },
  FailedNodeAllocatableEnforcement: {
    zh: "节点资源限制配置失败",
    en: "Node resource enforcement failed",
  },
  Pulling: { zh: "正在拉取镜像", en: "Pulling image" },
  Pulled: { zh: "镜像已拉取", en: "Image pulled" },
};

function workerEventReasonLabel(
  reason: string,
  zh: boolean,
  failed = false,
): string {
  const label = workerEventReasonLabels[reason];
  if (label) return zh ? label.zh : label.en;
  if (failed) return zh ? "Worker 运行失败" : "Worker failed";
  return zh ? "等待 Worker 启动" : "Waiting for Worker startup";
}

function jobFailureMessage(message: string | undefined, zh: boolean): string {
  if (!message) return zh ? "Worker 运行失败" : "Worker failed";
  if (/oomkilled|out of memory/i.test(message)) {
    return workerEventReasonLabel("OOMKilled", zh, true);
  }
  if (
    /crashloopbackoff|back-off .*restarting failed container/i.test(message)
  ) {
    return workerEventReasonLabel("CrashLoopBackOff", zh, true);
  }
  if (/imagepullbackoff/i.test(message)) {
    return workerEventReasonLabel("ImagePullBackOff", zh, true);
  }
  if (/errimagepull/i.test(message)) {
    return workerEventReasonLabel("ErrImagePull", zh, true);
  }
  for (const reason of Object.keys(workerEventReasonLabels)) {
    if (message.includes(reason))
      return workerEventReasonLabel(reason, zh, true);
  }
  return workerEventReasonLabel("", zh, true);
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
export function PullProgressInfo({
  progress,
  events = [],
  zh,
  emptyMessage,
  statusMessage,
  statusTitle,
  eventTitle,
  failed = false,
  variant = "default",
}: {
  progress: PullProgressEntry[];
  events?: NodeEventEntry[];
  zh: boolean;
  emptyMessage?: string;
  statusMessage?: string;
  statusTitle?: string;
  eventTitle?: string;
  failed?: boolean;
  variant?: "default" | "danger";
}) {
  const wrapperRef = useRef<HTMLSpanElement | null>(null);
  const tooltipRef = useRef<HTMLSpanElement | null>(null);
  const closeTimerRef = useRef<number | null>(null);
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{
    top: number;
    left: number;
    above: boolean;
    arrowLeft: number;
  } | null>(null);
  const recentEvents = [
    ...new Map(
      [...events]
        .sort((left, right) => {
          const leftTime = Date.parse(left.lastTime ?? "") || 0;
          const rightTime = Date.parse(right.lastTime ?? "") || 0;
          return rightTime - leftTime;
        })
        .map((event) => [
          workerEventReasonLabel(event.reason, zh, failed),
          event,
        ]),
    ).values(),
  ].slice(0, 4);

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

  const cancelClose = () => {
    if (closeTimerRef.current !== null) {
      window.clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
  };
  const show = () => {
    cancelClose();
    setPos(null);
    setOpen(true);
  };
  const clear = () => {
    cancelClose();
    setOpen(false);
    setPos(null);
  };
  const handleMouseLeave = (event: React.MouseEvent<HTMLSpanElement>) => {
    const nextTarget = event.relatedTarget;
    if (
      nextTarget instanceof Node &&
      (wrapperRef.current?.contains(nextTarget) ||
        tooltipRef.current?.contains(nextTarget))
    ) {
      return;
    }
    cancelClose();
    closeTimerRef.current = window.setTimeout(clear, 250);
  };

  useEffect(() => () => cancelClose(), []);

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
  }, [open, progress, events, emptyMessage, statusMessage, statusTitle]);

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
        className={`status-info${variant === "danger" ? " status-info-danger" : ""}`}
        tabIndex={0}
        ref={wrapperRef}
        onMouseEnter={show}
        onMouseLeave={handleMouseLeave}
        onFocus={show}
        onBlur={clear}
      >
        <Info size={13} />
      </span>
      {open &&
        createPortal(
          <span
            ref={tooltipRef}
            className={`status-info-tooltip status-info-tooltip-open${variant === "danger" ? " status-info-tooltip-danger" : ""}${pos && !pos.above ? " status-info-tooltip-below" : ""}`}
            style={tooltipStyle}
            role="status"
            onMouseEnter={show}
            onMouseLeave={handleMouseLeave}
          >
            <i
              className="status-info-tooltip-arrow"
              aria-hidden="true"
              style={{ left: pos ? pos.arrowLeft - 6 : 0 }}
            />
            {progress.length === 0 &&
              events.length === 0 &&
              !statusMessage &&
              emptyMessage && (
                <>
                  <strong>{zh ? "Worker 等待中" : "Worker pending"}</strong>
                  <span className="pending-empty-message">{emptyMessage}</span>
                </>
              )}
            {statusMessage && (
              <>
                <strong>
                  {statusTitle ?? (zh ? "异常原因" : "Failure Reason")}
                </strong>
                <span className="pull-entry status-message-entry">
                  {statusMessage}
                </span>
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
            {recentEvents.length > 0 && (
              <>
                <strong className="status-info-tooltip-section">
                  {eventTitle ??
                    (failed
                      ? zh
                        ? "失败原因"
                        : "Failure Reasons"
                      : zh
                        ? "等待原因"
                        : "Pending Reasons")}
                  {events.length > recentEvents.length && (
                    <small>
                      {zh
                        ? `最近 ${recentEvents.length} 条`
                        : `Latest ${recentEvents.length}`}
                    </small>
                  )}
                </strong>
                {recentEvents.map((ev, i) => (
                  <span key={`e-${i}`} className="pull-entry event-entry">
                    <span
                      className={`event-chip ${
                        ev.type === "Normal"
                          ? "event-normal"
                          : failed
                            ? "event-failed"
                            : "event-pending"
                      }`}
                    >
                      {workerEventReasonLabel(ev.reason, zh, failed)}
                    </span>
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
  onSelectNode,
  onSelectCluster,
  diskWarning,
  tasks,
}: {
  jobName: string;
  worker: WorkerItem;
  copy: CopyType;
  pods: PodInfo[];
  domainIPMap: Record<string, string>;
  isHeader?: boolean;
  createdAt: string;
  onSelectNode?: (name: string) => void;
  onSelectCluster?: (id: string) => void;
  diskWarning: boolean;
  tasks: CRDTask[];
}) {
  const zh = c.nav.overview === "总览";
  const [copied, setCopied] = useState(false);
  const [sshConfig, setSSHConfig] = useState<{
    jumpHost: string;
    jumpPort: string;
  } | null>(null);
  useEffect(() => {
    systemConfigApi
      .get()
      .then((d) => {
        setSSHConfig({
          jumpHost: d.ssh?.jumpHost || d.sshJumpHost || "",
          jumpPort: d.ssh?.jumpPort || d.sshJumpPort || "",
        });
      })
      .catch(() => {});
  }, []);
  const sshJump = sshConfig?.jumpHost
    ? `${sshConfig.jumpHost}${sshConfig.jumpPort ? ":" + sshConfig.jumpPort : ""}`
    : "";
  // head 节点直连 Pod 的 22 端口；非 head 节点通过节点 sshd 的 2222 端口转发到目标 Pod。
  const sshCommand = sshJump
    ? isHeader
      ? `ssh -J ${sshJump} root@${worker.name}`
      : `ssh -J ${sshJump} root@${worker.name} -p 2222`
    : "";
  const handleCopy = async () => {
    if (!(await copyText(sshCommand))) return;
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  const pod = pods[0];
  const domainIP =
    domainIPMap[`${pod?.namespace}/${pod?.podNamespace}/${pod?.podName}`] ??
    "—";

  // 获取节点 RANK：优先从环境变量 RLINF_NODE_RANK 获取，否则从 task 的 ray-node-rank-start annotation 加上 worker index 计算
  const getNodeRank = (worker: WorkerItem, pods: PodInfo[]) => {
    const pod = pods[0];
    if (!pod) return "—";

    // 方式一：从环境变量获取
    const envRank = pod.env?.find((e) => e.name === "RLINF_NODE_RANK")?.value;
    if (envRank) return envRank;

    // 方式二：从 task 的 annotation 计算
    // 从 worker 名称中提取 task 名称（去掉 -0, -1, -2 等后缀）
    const taskNameMatch = worker.name.match(/^(.+)-(\d+)$/);
    if (taskNameMatch) {
      const taskName = taskNameMatch[1];
      const index = parseInt(taskNameMatch[2], 10);

      // 从 tasks 列表中查找对应的 task
      const task = tasks.find((t) => t.metadata?.name === taskName);
      if (task?.metadata?.annotations?.["rlark.io/ray-node-rank-start"]) {
        const startRank = parseInt(
          task.metadata.annotations["rlark.io/ray-node-rank-start"],
          10,
        );
        if (!isNaN(startRank)) {
          return String(startRank + index);
        }
      }
    }

    return "—";
  };

  return (
    <>
      <tr>
        <td>
          <span className="table-primary">
            <span className="row-icon">
              <Zap size={16} />
            </span>
            <span className="worker-name-block">
              <strong className="worker-name" title={worker.name}>
                {worker.name}
              </strong>
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
          <div className="status-with-info">
            <StatusBadge phase={worker.phase} copy={c} />
            {worker.phase !== "Running" &&
              (worker.phase === "Pending" ||
                worker.phase === "Failed" ||
                (worker.pullProgress && worker.pullProgress.length > 0) ||
                (worker.events && worker.events.length > 0)) && (
                <PullProgressInfo
                  progress={worker.pullProgress ?? []}
                  events={worker.events ?? []}
                  zh={zh}
                  statusMessage={
                    worker.phase === "Failed"
                      ? jobFailureMessage(worker.statusMessage, zh)
                      : undefined
                  }
                  failed={worker.phase === "Failed"}
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
          <WorkerClusterLink
            cluster={worker.cluster}
            onSelectCluster={onSelectCluster}
          />
        </td>
        <td>
          <span
            className={`worker-node-with-warning${diskWarning ? " is-warning" : ""}`}
          >
            <WorkerNodeLink node={worker.node} onSelectNode={onSelectNode} />
            {diskWarning && (
              <PullProgressInfo
                progress={[]}
                zh={zh}
                statusMessage={
                  zh
                    ? "磁盘即将用满，请及时清理空间"
                    : "Disk is almost full. Please clean up space."
                }
                statusTitle={
                  zh ? "健康与容量告警" : "Health and Capacity Alert"
                }
                variant="danger"
              />
            )}
          </span>
        </td>
        <td>
          <span className="node-kind-chip">{getNodeKindLabel(worker)}</span>
        </td>
        <td>
          <span className="node-rank-chip">{getNodeRank(worker, pods)}</span>
        </td>
        <td>
          <code className="inline-code">{pod?.ip || "—"}</code>
        </td>
        <td>
          <code className="inline-code">{domainIP}</code>
        </td>
        <td>
          <strong>
            {worker.gpu && worker.gpu !== "0"
              ? `${worker.gpu} GPU`
              : zh
                ? "未申请"
                : "None"}
          </strong>
        </td>
        <td>
          <span className="table-date">{createdAt}</span>
        </td>
        <td className="worker-sticky-actions">
          <div className="worker-table-actions">
            <span
              className="action-tooltip"
              data-tooltip={
                !sshCommand
                  ? zh
                    ? "未配置 SSH 跳板地址"
                    : "SSH jump host is not configured"
                  : copied
                    ? c.jobs.sshCopied
                    : zh
                      ? "复制 SSH 命令"
                      : "Copy SSH command"
              }
            >
              <button
                className="icon-button worker-copy-ssh-icon"
                onClick={handleCopy}
                disabled={!sshCommand}
                aria-label={
                  !sshCommand
                    ? zh
                      ? "未配置 SSH 跳板地址"
                      : "SSH jump host is not configured"
                    : copied
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
          </div>
        </td>
      </tr>
    </>
  );
}

function WorkerClusterLink({
  cluster,
  onSelectCluster,
}: {
  cluster?: string;
  onSelectCluster?: (id: string) => void;
}) {
  if (!onSelectCluster || !cluster || cluster === "—") {
    return (
      <span className="worker-chip" title={cluster || "—"}>
        <span className="worker-link-label">{cluster || "—"}</span>
      </span>
    );
  }
  return (
    <button
      type="button"
      className="worker-chip worker-chip-link"
      onClick={() => onSelectCluster(cluster)}
      title={cluster}
    >
      <span className="worker-link-label">{cluster}</span>
    </button>
  );
}

function WorkerNodeLink({
  node,
  onSelectNode,
}: {
  node: string;
  onSelectNode?: (name: string) => void;
}) {
  if (!onSelectNode || !node || node === "—" || node.includes(",")) {
    return (
      <span className="worker-chip" title={node || "—"}>
        <span className="worker-link-label">{node || "—"}</span>
      </span>
    );
  }
  return (
    <button
      type="button"
      className="worker-chip worker-chip-link"
      onClick={() => onSelectNode(node)}
      title={node}
    >
      <span className="worker-link-label">{node}</span>
    </button>
  );
}
import {
  domainsApi,
  jobsApi,
  nodesApi,
  podsApi,
  sshKeysApi,
  systemConfigApi,
  tasksApi,
} from "../backend";
