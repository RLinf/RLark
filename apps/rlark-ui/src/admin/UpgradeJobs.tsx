import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type FormEvent,
} from "react";
import {
  AlertTriangle,
  ArrowLeft,
  ArrowUpCircle,
  Copy as CopyIcon,
  Loader2,
  MoreVertical,
  Plus,
  Trash2,
} from "lucide-react";
import type { Copy, Lang } from "../i18n";
import {
  ColumnFilterButton,
  PageToolbar,
  Pagination,
  RefreshOverlay,
  SortButton,
  compareSortValues,
  useColumnFilter,
  type SortDirection,
} from "../components/shared";
import { ColumnFilterPopover } from "../components/ColumnFilterPopover";
import { clustersApi, upgradeJobsApi, type UpgradeJobItem } from "../backend";

const phaseColor = (phase: string): string => {
  switch (phase) {
    case "Succeeded":
      return "var(--green, #26b985)";
    case "Failed":
      return "#ef4444";
    case "Upgrading":
      return "var(--accent, #8b5cf6)";
    default:
      return "#7f8897";
  }
};

const phaseLabel = (phase: string, zh: boolean): string => {
  if (!zh) return phase || "Pending";
  switch (phase) {
    case "Succeeded":
      return "成功";
    case "Failed":
      return "失败";
    case "Upgrading":
      return "升级中";
    case "RollingBack":
      return "已回滚";
    case "Pending":
    case "":
      return "等待中";
    default:
      return phase;
  }
};

const conditionLabel = (status: string, zh: boolean): string => {
  if (!zh) return status;
  switch (status) {
    case "Succeeded":
      return "成功";
    case "Failed":
      return "失败";
    case "RolledBack":
      return "已回滚";
    case "Upgrading":
      return "升级中";
    default:
      return status;
  }
};

function PhaseChip({ phase, zh }: { phase: string; zh: boolean }) {
  const color = phaseColor(phase);
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 5,
        fontSize: 12,
        fontWeight: 600,
        color,
      }}
    >
      <i
        style={{
          width: 7,
          height: 7,
          borderRadius: "50%",
          background: color,
          display: "inline-block",
        }}
      />
      {phaseLabel(phase, zh)}
    </span>
  );
}

export function UpgradeJobsPage({
  lang,
  copy: c,
  selectedJob,
  onSelectJob,
}: {
  lang: Lang;
  copy: Copy;
  selectedJob?: string;
  onSelectJob: (name?: string) => void;
}) {
  const [createInitial, setCreateInitial] = useState<
    UpgradeJobFormInitial | undefined
  >();

  if (selectedJob === "create") {
    return (
      <UpgradeJobCreatePage
        lang={lang}
        copy={c}
        initial={createInitial}
        onBack={() => {
          setCreateInitial(undefined);
          onSelectJob(undefined);
        }}
        onCreated={() => {
          setCreateInitial(undefined);
          onSelectJob(undefined);
        }}
      />
    );
  }

  if (selectedJob) {
    return (
      <UpgradeJobDetail
        lang={lang}
        copy={c}
        jobName={selectedJob}
        onBack={() => onSelectJob(undefined)}
        onCopyCreate={(initial) => {
          setCreateInitial(initial);
          onSelectJob("create");
        }}
      />
    );
  }
  return (
    <UpgradeJobsList
      lang={lang}
      copy={c}
      onSelectJob={onSelectJob}
      onCreate={() => onSelectJob("create")}
      onCopyCreate={(initial) => {
        setCreateInitial(initial);
        onSelectJob("create");
      }}
    />
  );
}

export interface UpgradeJobFormInitial {
  description: string;
  image: string;
  configPatch: string;
  clusterNames: string[];
  concurrency: string;
  failureTolerate: string;
  timeoutSeconds: string;
  autoRollback: boolean;
}

// Build a create-form initial state from an existing job item.
function buildCopyInitial(job: UpgradeJobItem): UpgradeJobFormInitial {
  return {
    description: job.metadata.annotations?.["rlark.io/description"] ?? "",
    image: job.spec.image,
    configPatch: job.spec.configPatch ?? "",
    clusterNames: job.spec.clusterNames ?? [],
    concurrency: String(job.spec.strategy?.concurrency ?? 1),
    failureTolerate: String(job.spec.strategy?.failureTolerate ?? 0),
    timeoutSeconds: String(job.spec.strategy?.timeoutSeconds ?? 600),
    autoRollback: job.spec.strategy?.autoRollback ?? true,
  };
}

function UpgradeJobsList({
  lang,
  copy: c,
  onSelectJob,
  onCreate,
  onCopyCreate,
}: {
  lang: Lang;
  copy: Copy;
  onSelectJob: (name?: string) => void;
  onCreate: () => void;
  onCopyCreate: (initial: UpgradeJobFormInitial) => void;
}) {
  const zh = lang === "zh";
  const [jobs, setJobs] = useState<UpgradeJobItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");

  const [deleteJob, setDeleteJob] = useState<UpgradeJobItem | null>(null);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState("");

  // Pagination state
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  // Sort and filter state
  const [sort, setSort] = useState<{
    key: "creationTimestamp";
    direction: SortDirection;
  }>({ key: "creationTimestamp", direction: "desc" });
  const [phaseFilter, setPhaseFilter] = useState<string[]>([]);
  const columnFilter = useColumnFilter();

  const toggleSort = () =>
    setSort((current) => ({
      key: "creationTimestamp",
      direction: current.direction === "asc" ? "desc" : "asc",
    }));

  const fetchJobs = () => {
    upgradeJobsApi
      .list()
      .then(setJobs)
      .catch((e) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    fetchJobs();
    const interval = setInterval(fetchJobs, 5000);
    return () => clearInterval(interval);
  }, []);

  const filteredJobs = useMemo(() => {
    const q = query.trim().toLowerCase();
    const filtered = jobs.filter((job) => {
      const queryHit =
        !q ||
        job.metadata.name.toLowerCase().includes(q) ||
        (job.metadata.annotations?.["rlark.io/description"] ?? "")
          .toLowerCase()
          .includes(q);
      const phaseHit =
        phaseFilter.length === 0 || phaseFilter.includes(job.status.phase);
      return queryHit && phaseHit;
    });
    return [...filtered].sort((a, b) => {
      const comparison = compareSortValues(
        a.metadata.creationTimestamp ?? "",
        b.metadata.creationTimestamp ?? "",
        sort.direction,
        zh ? "zh-CN" : "en",
      );
      return comparison !== 0
        ? comparison
        : a.metadata.name.localeCompare(b.metadata.name);
    });
  }, [jobs, query, phaseFilter, sort, zh]);

  // Pagination
  const totalPages = Math.max(1, Math.ceil(filteredJobs.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pagedJobs = filteredJobs.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );

  const canDelete = (phase: string) => phase !== "Upgrading";

  const handleDeleteConfirm = () => {
    if (!deleteJob) return;
    setDeletePending(true);
    setDeleteError("");
    upgradeJobsApi
      .remove(deleteJob.metadata.name)
      .then(() => {
        setDeleteJob(null);
        setDeletePending(false);
        fetchJobs();
      })
      .catch((e) => {
        setDeletePending(false);
        setDeleteError(e instanceof Error ? e.message : String(e));
      });
  };

  return (
    <div className="page-content resource-page upgrade-jobs-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">
            <ArrowUpCircle size={13} />
            {zh ? "集群升级" : "Upgrades"}
          </span>
          <h2>{zh ? "集群升级任务" : "Cluster Upgrade Jobs"}</h2>
          <p>
            {zh
              ? "批量升级数据面集群到目标版本，支持自动回滚"
              : "Batch upgrade data-plane clusters with automatic rollback"}
          </p>
        </div>
        <button type="button" className="primary-button" onClick={onCreate}>
          <Plus size={15} />
          <span>{zh ? "创建升级任务" : "Create Upgrade"}</span>
        </button>
      </div>

      {error && (
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}

      <PageToolbar
        placeholder={zh ? "搜索升级任务..." : "Search upgrade jobs..."}
        value={query}
        onChange={setQuery}
        count={filteredJobs.length}
        copy={c}
        onRefresh={fetchJobs}
        refreshing={loading}
      />

      <section
        className={`table-panel refreshable-region${loading ? " is-refreshing" : ""}`}
        aria-busy={loading}
      >
        <div className="storage-table-heading">
          <div>
            <strong>{zh ? "升级任务" : "Upgrade Jobs"}</strong>
            <small>
              {zh
                ? "升级任务按集群并发执行，失败可自动回滚"
                : "Jobs run concurrently per cluster with auto rollback"}
            </small>
          </div>
          <span>
            {zh
              ? `共 ${filteredJobs.length} 项`
              : `${filteredJobs.length} items`}
          </span>
        </div>

        <table>
          <thead>
            <tr>
              <th>{zh ? "任务名称" : "Name"}</th>
              <th>
                <ColumnFilterButton
                  label={zh ? "状态" : "Phase"}
                  selectedCount={phaseFilter.length}
                  onClick={columnFilter.openFor("phase")}
                />
              </th>
              <th>{zh ? "升级镜像" : "Upgrade Image"}</th>
              <th>{zh ? "进度" : "Progress"}</th>
              <th>
                <SortButton
                  label={zh ? "创建时间" : "Created"}
                  active={true}
                  direction={sort.direction}
                  onClick={toggleSort}
                />
              </th>
              <th className="table-actions-col">{zh ? "操作" : "Actions"}</th>
            </tr>
          </thead>
          <tbody>
            {filteredJobs.length === 0 ? (
              <tr>
                <td
                  colSpan={6}
                  className="muted"
                  style={{ padding: "20px", textAlign: "center" }}
                >
                  {loading
                    ? zh
                      ? "加载中…"
                      : "Loading…"
                    : zh
                      ? "暂无升级任务"
                      : "No upgrade jobs"}
                </td>
              </tr>
            ) : (
              pagedJobs.map((job) => (
                <tr key={job.metadata.name}>
                  <td>
                    <div className="job-id-cell-wrap">
                      <button
                        className="link-cell job-id-cell"
                        title={job.metadata.name}
                        onClick={() => onSelectJob(job.metadata.name)}
                      >
                        <strong>{job.metadata.name}</strong>
                      </button>
                      {job.metadata.annotations?.["rlark.io/description"] && (
                        <small
                          className="muted"
                          style={{
                            fontSize: 11,
                            marginTop: 1,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                            maxWidth: 200,
                          }}
                          title={
                            job.metadata.annotations?.["rlark.io/description"]
                          }
                        >
                          {job.metadata.annotations?.["rlark.io/description"]}
                        </small>
                      )}
                    </div>
                  </td>
                  <td>
                    <PhaseChip phase={job.status.phase} zh={zh} />
                  </td>
                  <td>
                    <code
                      style={{
                        fontFamily: "var(--font-mono, monospace)",
                        fontSize: 11,
                        wordBreak: "break-all",
                      }}
                    >
                      {job.spec.image}
                    </code>
                  </td>
                  <td className="muted">
                    {job.status.upgradedClusters ?? 0}/
                    {job.status.totalClusters}
                    {job.status.failedClusters > 0 && (
                      <span style={{ color: "#ef4444" }}>
                        {" "}
                        ({job.status.failedClusters} {zh ? "失败" : "failed"})
                      </span>
                    )}
                    {job.status.rollbackClusters > 0 && (
                      <span style={{ color: "#f59e35" }}>
                        {" "}
                        ({job.status.rollbackClusters}{" "}
                        {zh ? "回滚" : "rolled back"})
                      </span>
                    )}
                  </td>
                  <td className="muted">
                    {job.status.startTime
                      ? new Date(job.status.startTime).toLocaleString()
                      : "-"}
                  </td>
                  <td
                    className="table-actions-col"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <UpgradeJobRowActions
                      job={job}
                      zh={zh}
                      canDelete={canDelete(job.status.phase)}
                      onCopyCreate={() => onCopyCreate(buildCopyInitial(job))}
                      onDelete={() => setDeleteJob(job)}
                    />
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </section>

      <Pagination
        page={currentPage}
        pageSize={pageSize}
        total={filteredJobs.length}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
        zh={zh}
      />

      <RefreshOverlay
        visible={loading && jobs.length === 0}
        label={zh ? "加载中…" : "Loading…"}
      />

      {/* Phase filter popover */}
      {columnFilter.openKey === "phase" && (
        <ColumnFilterPopover
          label={zh ? "状态" : "Phase"}
          options={[
            "Pending",
            "Upgrading",
            "Succeeded",
            "Failed",
            "RollingBack",
          ].map((p) => ({
            value: p,
            label: phaseLabel(p, zh),
          }))}
          selected={phaseFilter}
          onChange={setPhaseFilter}
          anchorRect={columnFilter.anchorRect}
          onClose={columnFilter.close}
          zh={zh}
        />
      )}

      {/* Delete confirmation modal */}
      {deleteJob && (
        <div
          className="modal-backdrop delete-job-backdrop"
          onMouseDown={(e) =>
            e.target === e.currentTarget && !deletePending && setDeleteJob(null)
          }
        >
          <section
            className="modal delete-job-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="delete-upgrade-job-title"
          >
            <div className="delete-job-head">
              <span className="delete-job-icon">
                <Trash2 size={20} />
              </span>
              <div>
                <span className="eyebrow">
                  {zh ? "危险操作" : "Danger zone"}
                </span>
                <h2 id="delete-upgrade-job-title">
                  {zh ? "确认删除任务？" : "Delete this upgrade job?"}
                </h2>
              </div>
              <button
                className="icon-button"
                onClick={() => !deletePending && setDeleteJob(null)}
                aria-label={zh ? "关闭" : "Close"}
                disabled={deletePending}
              >
                ×
              </button>
            </div>
            <div className="delete-job-body">
              <p>
                {zh
                  ? "删除后，该升级任务将从平台移除。"
                  : "This upgrade job will be removed from the platform."}
              </p>
              <div className="delete-job-target">
                <span>{zh ? "即将删除" : "Job to delete"}</span>
                <strong>{deleteJob.metadata.name}</strong>
              </div>
              <div className="delete-job-warning">
                <AlertTriangle size={15} />
                <span>
                  {zh
                    ? "此操作不可撤销，请确认该任务已不再需要。"
                    : "This action cannot be undone. Confirm that this job is no longer needed."}
                </span>
              </div>
              {deleteError && (
                <div className="delete-job-error" role="alert">
                  {zh ? "删除失败：" : "Delete failed: "}
                  {deleteError}
                </div>
              )}
            </div>
            <div className="delete-job-actions">
              <button
                className="secondary-button"
                onClick={() => setDeleteJob(null)}
                disabled={deletePending}
              >
                {zh ? "取消" : "Cancel"}
              </button>
              <button
                className="delete-job-confirm"
                onClick={handleDeleteConfirm}
                disabled={deletePending}
              >
                {deletePending ? (
                  <Loader2 className="job-action-loading" size={15} />
                ) : (
                  <Trash2 size={15} />
                )}
                {deletePending
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
      )}
    </div>
  );
}

// ─── Create Page ──────────────────────────────────────────────────────────────

function UpgradeJobCreatePage({
  lang,
  copy: c,
  initial,
  onBack,
  onCreated,
}: {
  lang: Lang;
  copy: Copy;
  initial?: UpgradeJobFormInitial;
  onBack: () => void;
  onCreated: () => void;
}) {
  const zh = lang === "zh";
  const [clusters, setClusters] = useState<{ id: string; name: string }[]>([]);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");
  const errorRef = useRef<HTMLDivElement>(null);

  const [form, setForm] = useState<UpgradeJobFormInitial>(
    initial ?? {
      description: "",
      image: "",
      configPatch: "",
      clusterNames: [],
      concurrency: "1",
      failureTolerate: "0",
      timeoutSeconds: "600",
      autoRollback: true,
    },
  );

  useEffect(() => {
    clustersApi
      .list<{ id: string; name: string }>()
      .then(setClusters)
      .catch(() => setClusters([]));
  }, []);

  const toggleCluster = (id: string) => {
    setForm((prev) => ({
      ...prev,
      clusterNames: prev.clusterNames.includes(id)
        ? prev.clusterNames.filter((c) => c !== id)
        : [...prev.clusterNames, id],
    }));
  };

  const handleCreate = (e: FormEvent) => {
    e.preventDefault();
    setError("");
    let msg = "";
    if (!form.image.trim()) {
      msg = zh ? "请填写升级镜像" : "Upgrade image is required";
    } else if (form.clusterNames.length === 0) {
      msg = zh ? "请至少选择一个集群" : "Select at least one cluster";
    } else {
      const ft = Number(form.failureTolerate);
      if (isNaN(ft) || ft < 0 || ft > 1) {
        msg = zh
          ? "失败容忍比例必须在 0-1 之间"
          : "Failure tolerate must be between 0 and 1";
      }
      const conc = Number(form.concurrency);
      if (!msg && (isNaN(conc) || conc < 1)) {
        msg = zh ? "并发数必须大于等于 1" : "Concurrency must be at least 1";
      }
      const timeout = Number(form.timeoutSeconds);
      if (!msg && (isNaN(timeout) || timeout < 120)) {
        msg = zh
          ? "超时时间必须大于等于 120 秒"
          : "Timeout must be at least 120 seconds";
      }
    }
    if (msg) {
      setError(msg);
      // Scroll to the error message so the user can see it.
      setTimeout(() => {
        errorRef.current?.scrollIntoView({
          behavior: "smooth",
          block: "start",
        });
      }, 0);
      return;
    }
    setCreating(true);
    upgradeJobsApi
      .create({
        metadata: form.description.trim()
          ? { annotations: { "rlark.io/description": form.description.trim() } }
          : undefined,
        spec: {
          image: form.image.trim(),
          configPatch: form.configPatch.trim() || undefined,
          clusterNames: form.clusterNames,
          strategy: {
            concurrency: Number(form.concurrency) || 1,
            failureTolerate: Number(form.failureTolerate) || 0,
            timeoutSeconds: Number(form.timeoutSeconds) || 600,
            autoRollback: form.autoRollback,
          },
        },
      })
      .then(() => onCreated())
      .catch((e) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setCreating(false));
  };

  return (
    <div
      className="page-content resource-page upgrade-jobs-page"
      style={{ display: "flex", flexDirection: "column", gap: 18 }}
    >
      <button
        onClick={onBack}
        style={{
          border: "none",
          background: "transparent",
          cursor: "pointer",
          fontSize: 13,
          color: "var(--blue)",
          display: "flex",
          alignItems: "center",
          gap: 4,
          padding: 0,
          width: "fit-content",
        }}
      >
        ← {zh ? "返回升级任务" : "Back to upgrade jobs"}
      </button>

      <div className="section-heading">
        <div>
          <span className="eyebrow">
            <ArrowUpCircle size={13} />
            {zh ? "集群升级" : "Upgrades"}
          </span>
          <h2>{zh ? "创建升级任务" : "Create Upgrade Job"}</h2>
          <p>
            {zh
              ? "选择目标集群并指定升级镜像，升级失败时可自动回滚"
              : "Select target clusters and specify the upgrade image. Auto rollback on failure."}
          </p>
        </div>
      </div>

      {error && (
        <div ref={errorRef} className="cert-error" role="alert">
          {error}
        </div>
      )}

      <form
        onSubmit={handleCreate}
        style={{ display: "flex", flexDirection: "column", gap: 20 }}
      >
        {/* Basic info */}
        <div className="form-section">
          <div className="form-section-head">
            <strong>{zh ? "基本信息" : "Basic"}</strong>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <label
                style={{ fontSize: 13, fontWeight: 600, color: "var(--text)" }}
              >
                {zh ? "备注（可选）" : "Description (optional)"}
              </label>
              <input
                value={form.description}
                onChange={(e) =>
                  setForm({ ...form, description: e.target.value })
                }
                style={{
                  padding: "10px 12px",
                  borderRadius: 8,
                  border: "1px solid var(--line)",
                  fontSize: 14,
                  boxSizing: "border-box",
                  width: "100%",
                }}
              />
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <label
                style={{ fontSize: 13, fontWeight: 600, color: "var(--text)" }}
              >
                {zh ? "升级镜像" : "Upgrade image"}
                <span style={{ color: "#ef4444" }}> *</span>
              </label>
              <span className="muted" style={{ fontSize: 12 }}>
                {zh
                  ? "rlark-upgrade 镜像；镜像 tag 即目标升级版本"
                  : "rlark-upgrade image; its tag is the upgrade target"}
              </span>
              <input
                value={form.image}
                onChange={(e) => setForm({ ...form, image: e.target.value })}
                placeholder="registry/rlark-upgrade:v0.0.2"
                required
                style={{
                  padding: "10px 12px",
                  borderRadius: 8,
                  border: "1px solid var(--line)",
                  fontSize: 14,
                  boxSizing: "border-box",
                  width: "100%",
                }}
              />
            </div>
          </div>
        </div>

        {/* Target clusters */}
        <div className="form-section">
          <div className="form-section-head">
            <strong>
              {zh ? "目标集群" : "Target clusters"}
              <span style={{ color: "#ef4444" }}> *</span>
            </strong>
          </div>
          {clusters.length === 0 ? (
            <p className="muted" style={{ margin: 0, fontSize: 13 }}>
              {zh ? "暂无可选集群" : "No clusters available"}
            </p>
          ) : (
            <div className="upgrade-cluster-options">
              {clusters.map((cluster) => (
                <label
                  key={cluster.id}
                  className={
                    form.clusterNames.includes(cluster.id) ? "active" : ""
                  }
                >
                  <input
                    type="checkbox"
                    checked={form.clusterNames.includes(cluster.id)}
                    onChange={() => toggleCluster(cluster.id)}
                  />
                  <span>{cluster.name || cluster.id}</span>
                </label>
              ))}
            </div>
          )}
        </div>

        {/* Advanced options */}
        <div className="form-section">
          <div className="form-section-head">
            <strong>{zh ? "高级选项" : "Advanced"}</strong>
          </div>
          <div style={{ display: "flex", gap: 16 }}>
            <div
              style={{
                display: "flex",
                flexDirection: "column",
                gap: 6,
                flex: 1,
              }}
            >
              <label
                style={{
                  fontSize: 13,
                  fontWeight: 600,
                  color: "var(--text)",
                  display: "flex",
                  alignItems: "center",
                  gap: 4,
                }}
              >
                {zh ? "并发数" : "Concurrency"}
                <span
                  className="muted"
                  style={{ fontSize: 11, fontWeight: 400 }}
                >
                  {zh ? "(个)" : "(clusters)"}
                </span>
              </label>
              <input
                type="number"
                min={1}
                value={form.concurrency}
                onChange={(e) =>
                  setForm({
                    ...form,
                    concurrency: e.target.value,
                  })
                }
                style={{
                  padding: "10px 12px",
                  borderRadius: 8,
                  border: "1px solid var(--line)",
                  fontSize: 14,
                  boxSizing: "border-box",
                  width: "100%",
                }}
              />
            </div>
            <div
              style={{
                display: "flex",
                flexDirection: "column",
                gap: 6,
                flex: 1,
              }}
            >
              <label
                style={{
                  fontSize: 13,
                  fontWeight: 600,
                  color: "var(--text)",
                  display: "flex",
                  alignItems: "center",
                  gap: 4,
                }}
              >
                {zh ? "失败容忍比例" : "Failure tolerate"}
                <span
                  className="muted"
                  style={{ fontSize: 11, fontWeight: 400 }}
                >
                  {zh ? "(0-1)" : "(0-1)"}
                </span>
              </label>
              <input
                type="number"
                min={0}
                max={1}
                step={0.1}
                value={form.failureTolerate}
                onChange={(e) =>
                  setForm({
                    ...form,
                    failureTolerate: e.target.value,
                  })
                }
                style={{
                  padding: "10px 12px",
                  borderRadius: 8,
                  border: "1px solid var(--line)",
                  fontSize: 14,
                  boxSizing: "border-box",
                  width: "100%",
                }}
              />
            </div>
            <div
              style={{
                display: "flex",
                flexDirection: "column",
                gap: 6,
                flex: 1,
              }}
            >
              <label
                style={{
                  fontSize: 13,
                  fontWeight: 600,
                  color: "var(--text)",
                  display: "flex",
                  alignItems: "center",
                  gap: 4,
                }}
              >
                {zh ? "超时" : "Timeout"}
                <span
                  className="muted"
                  style={{ fontSize: 11, fontWeight: 400 }}
                >
                  {zh ? "(秒)" : "(s)"}
                </span>
              </label>
              <input
                type="number"
                min={120}
                value={form.timeoutSeconds}
                onChange={(e) =>
                  setForm({
                    ...form,
                    timeoutSeconds: e.target.value,
                  })
                }
                style={{
                  padding: "10px 12px",
                  borderRadius: 8,
                  border: "1px solid var(--line)",
                  fontSize: 14,
                  boxSizing: "border-box",
                  width: "100%",
                }}
              />
            </div>
          </div>
          <label
            style={{
              marginTop: 12,
              display: "flex",
              alignItems: "center",
              gap: 10,
              fontSize: 13,
            }}
          >
            <input
              type="checkbox"
              checked={form.autoRollback}
              onChange={(e) =>
                setForm({ ...form, autoRollback: e.target.checked })
              }
            />
            {zh ? "失败时自动回滚" : "Auto rollback on failure"}
          </label>
        </div>

        {/* Full deploy config */}
        <div className="form-section">
          <div className="form-section-head">
            <strong>
              {zh ? "部署配置（可选）" : "Deploy config (optional)"}
            </strong>
          </div>
          <textarea
            className="upgrade-config-textarea"
            value={form.configPatch}
            onChange={(e) => setForm({ ...form, configPatch: e.target.value })}
            rows={10}
            spellCheck={false}
            placeholder={
              zh
                ? "完整配置，例如：\napiVersion: rlark.io/v1alpha1\nkind: DeployConfig\nplane: data\nkubernetes:\n  agent-image: rlark:v0.0.67"
                : "Full config, e.g.:\napiVersion: rlark.io/v1alpha1\nkind: DeployConfig\nplane: data\nkubernetes:\n  agent-image: rlark:v0.0.67"
            }
          />
        </div>

        {/* Actions */}
        <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
          <button type="button" className="secondary-button" onClick={onBack}>
            {zh ? "取消" : "Cancel"}
          </button>
          <button type="submit" className="primary-button" disabled={creating}>
            {creating ? (
              <Loader2 size={15} className="job-action-loading" />
            ) : (
              <ArrowUpCircle size={15} />
            )}
            <span>{zh ? "创建升级任务" : "Create Upgrade"}</span>
          </button>
        </div>
      </form>
    </div>
  );
}

// ─── Job Detail View ──────────────────────────────────────────────────────────

function UpgradeJobDetail({
  lang,
  copy: c,
  jobName,
  onBack,
  onCopyCreate,
}: {
  lang: Lang;
  copy: Copy;
  jobName: string;
  onBack: () => void;
  onCopyCreate: (initial: UpgradeJobFormInitial) => void;
}) {
  const zh = lang === "zh";
  const [job, setJob] = useState<UpgradeJobItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const fetchJob = () => {
    upgradeJobsApi
      .get(jobName)
      .then(setJob)
      .catch((e) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    fetchJob();
    const interval = setInterval(fetchJob, 5000);
    return () => clearInterval(interval);
  }, [jobName]);

  // Delete modal state
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const canDelete = job && job.status.phase !== "Upgrading";

  const handleDeleteConfirm = () => {
    setDeletePending(true);
    setDeleteError("");
    upgradeJobsApi
      .remove(jobName)
      .then(() => onBack())
      .catch((e) => {
        setDeletePending(false);
        setDeleteError(e instanceof Error ? e.message : String(e));
      });
  };

  const handleCopyCreate = () => {
    if (!job) return;
    onCopyCreate(buildCopyInitial(job));
  };

  // Derive cluster list: prefer explicit ClusterNames, fall back to Conditions
  const clusterList =
    job?.spec.clusterNames && job.spec.clusterNames.length > 0
      ? job.spec.clusterNames
      : (job?.status.conditions?.map((c: { cluster: string }) => c.cluster) ??
        []);

  if (loading && !job) {
    return (
      <div className="page-content resource-page upgrade-jobs-page">
        <RefreshOverlay visible label={zh ? "加载中…" : "Loading…"} />
      </div>
    );
  }

  if (error && !job) {
    return (
      <div className="page-content resource-page upgrade-jobs-page">
        <div className="cert-error">{error}</div>
        <button
          type="button"
          className="secondary-button"
          style={{ marginTop: 12 }}
          onClick={onBack}
        >
          <ArrowLeft size={14} />
          {zh ? "返回列表" : "Back to List"}
        </button>
      </div>
    );
  }

  if (!job) return null;

  return (
    <div className="page-content resource-page upgrade-jobs-page">
      {/* Header */}
      <div className="section-heading">
        <div>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              marginBottom: 8,
            }}
          >
            <span className="eyebrow">
              <ArrowUpCircle size={13} />
              {zh ? "集群升级" : "Upgrades"}
            </span>
            <PhaseChip phase={job.status.phase} zh={zh} />
          </div>
          <h2 style={{ margin: "0 0 2px" }}>{job.metadata.name}</h2>
          {job.metadata.annotations?.["rlark.io/description"] && (
            <p className="muted" style={{ margin: 0, fontSize: 13 }}>
              {job.metadata.annotations?.["rlark.io/description"]}
            </p>
          )}
        </div>
        <div className="row-actions" style={{ gap: 8 }}>
          {/* Copy & Create */}
          <button
            type="button"
            className="secondary-button"
            onClick={handleCopyCreate}
          >
            <CopyIcon size={13} />
            {zh ? "复制创建" : "Copy & Create"}
          </button>

          {/* Delete */}
          {canDelete && (
            <button
              type="button"
              className="icon-button danger"
              title={zh ? "删除" : "Delete"}
              aria-label={zh ? "删除" : "Delete"}
              onClick={() => setShowDeleteModal(true)}
            >
              <Trash2 size={15} />
            </button>
          )}
        </div>
      </div>

      {/* Back button */}
      <button
        type="button"
        className="secondary-button"
        style={{ marginBottom: 16 }}
        onClick={onBack}
      >
        <ArrowLeft size={13} />
        {zh ? "返回列表" : "Back to List"}
      </button>

      {/* Progress summary */}
      <section className="table-panel" style={{ marginBottom: 16 }}>
        <div className="storage-table-heading">
          <strong>{zh ? "进度" : "Progress"}</strong>
          <span className="muted" style={{ fontSize: 13 }}>
            {job.status.upgradedClusters ?? 0}/{job.status.totalClusters}
            {(job.status.upgradingClusters ?? 0) > 0 && (
              <span style={{ color: "#8b5cf6" }}>
                {" "}
                ({job.status.upgradingClusters} {zh ? "升级中" : "upgrading"})
              </span>
            )}
            {job.status.failedClusters > 0 && (
              <span style={{ color: "#ef4444" }}>
                {" "}
                ({job.status.failedClusters} {zh ? "失败" : "failed"})
              </span>
            )}
            {job.status.rollbackClusters > 0 && (
              <span style={{ color: "#f59e35" }}>
                {" "}
                ({job.status.rollbackClusters} {zh ? "回滚" : "rolled back"})
              </span>
            )}
          </span>
        </div>
        {job.status.totalClusters > 0 && (
          <div style={{ padding: "0 16px 14px" }}>
            <div
              style={{
                height: 6,
                borderRadius: 3,
                background: "rgba(127,136,151,0.12)",
                overflow: "hidden",
                display: "flex",
              }}
            >
              {job.status.upgradedClusters > 0 && (
                <div
                  style={{
                    width: `${((job.status.upgradedClusters ?? 0) / job.status.totalClusters) * 100}%`,
                    background: "#26b985",
                  }}
                />
              )}
              {(job.status.upgradingClusters ?? 0) > 0 && (
                <div
                  style={{
                    width: `${((job.status.upgradingClusters ?? 0) / job.status.totalClusters) * 100}%`,
                    background: "#8b5cf6",
                  }}
                />
              )}
              {job.status.failedClusters > 0 && (
                <div
                  style={{
                    width: `${(job.status.failedClusters / job.status.totalClusters) * 100}%`,
                    background: "#ef4444",
                  }}
                />
              )}
              {job.status.rollbackClusters > 0 && (
                <div
                  style={{
                    width: `${(job.status.rollbackClusters / job.status.totalClusters) * 100}%`,
                    background: "#f59e35",
                  }}
                />
              )}
            </div>
          </div>
        )}
      </section>

      {/* Info section */}
      <section className="table-panel" style={{ marginBottom: 16 }}>
        <div className="storage-table-heading">
          <strong>{zh ? "任务信息" : "Job Info"}</strong>
        </div>
        <table style={{ margin: 0 }}>
          <tbody>
            {[
              {
                label: zh ? "备注" : "Description",
                value: job.metadata.annotations?.["rlark.io/description"] || (
                  <span className="muted">-</span>
                ),
              },
              {
                label: zh ? "升级镜像" : "Upgrade Image",
                value: (
                  <code
                    style={{
                      fontFamily: "var(--font-mono, monospace)",
                      fontSize: 12,
                      wordBreak: "break-all",
                    }}
                  >
                    {job.spec.image}
                  </code>
                ),
              },
              {
                label: zh ? "目标集群" : "Target Clusters",
                value:
                  clusterList.length > 0 ? (
                    clusterList.join(", ")
                  ) : (
                    <span className="muted">-</span>
                  ),
              },
              {
                label: zh ? "并发数" : "Concurrency",
                value: job.spec.strategy?.concurrency ?? 1,
              },
              {
                label: zh ? "失败容忍比例" : "Failure Tolerate",
                value: job.spec.strategy?.failureTolerate ?? 0,
              },
              {
                label: zh ? "超时（秒）" : "Timeout (s)",
                value: job.spec.strategy?.timeoutSeconds ?? 600,
              },
              {
                label: zh ? "自动回滚" : "Auto Rollback",
                value: job.spec.strategy?.autoRollback
                  ? zh
                    ? "是"
                    : "Yes"
                  : zh
                    ? "否"
                    : "No",
              },
              {
                label: zh ? "开始时间" : "Started",
                value: job.status.startTime
                  ? new Date(job.status.startTime).toLocaleString()
                  : "-",
              },
              {
                label: zh ? "完成时间" : "Completed",
                value: job.status.completionTime
                  ? new Date(job.status.completionTime).toLocaleString()
                  : "-",
              },
            ].map(({ label, value }) => (
              <tr key={label}>
                <td
                  style={{
                    width: 140,
                    padding: "8px 16px",
                    fontSize: 12,
                    fontWeight: 600,
                    color: "var(--muted)",
                    verticalAlign: "middle",
                  }}
                >
                  {label}
                </td>
                <td
                  style={{
                    padding: "8px 16px",
                    fontSize: 13,
                    verticalAlign: "middle",
                  }}
                >
                  {value}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      {/* ConfigPatch — always show with YAML syntax highlighting */}
      <section className="table-panel" style={{ marginBottom: 16 }}>
        <div className="storage-table-heading">
          <strong>{zh ? "配置覆盖" : "Config Patch"}</strong>
        </div>
        <div style={{ padding: "12px 16px" }}>
          {job.spec.configPatch ? (
            <YamlCodeBlock code={job.spec.configPatch} />
          ) : (
            <span className="muted" style={{ fontSize: 13 }}>
              {zh ? "无" : "None"}
            </span>
          )}
        </div>
      </section>

      {/* Per-cluster conditions */}
      {job.status.conditions && job.status.conditions.length > 0 && (
        <section className="table-panel" style={{ marginBottom: 16 }}>
          <div className="storage-table-heading">
            <div>
              <strong>{zh ? "集群明细" : "Cluster Details"}</strong>
              <small>
                {zh
                  ? `共 ${job.status.conditions.length} 个集群`
                  : `${job.status.conditions.length} clusters`}
              </small>
            </div>
          </div>
          <table>
            <thead>
              <tr>
                <th>{zh ? "集群" : "Cluster"}</th>
                <th>{zh ? "状态" : "Status"}</th>
                <th>{zh ? "完成时间" : "Finished"}</th>
                <th>{zh ? "信息" : "Message"}</th>
              </tr>
            </thead>
            <tbody>
              {job.status.conditions.map((cond) => (
                <tr key={cond.cluster}>
                  <td style={{ fontWeight: 500 }}>{cond.cluster}</td>
                  <td>
                    <span
                      style={{
                        color: phaseColor(cond.status),
                        fontWeight: 500,
                        fontSize: 12,
                      }}
                    >
                      {conditionLabel(cond.status, zh)}
                    </span>
                  </td>
                  <td className="muted">
                    {cond.upgradeTime
                      ? new Date(cond.upgradeTime).toLocaleString()
                      : "-"}
                  </td>
                  <td className="muted">{cond.reason || "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      {/* Delete confirmation modal */}
      {showDeleteModal && (
        <div
          className="modal-backdrop delete-job-backdrop"
          onMouseDown={(e) =>
            e.target === e.currentTarget &&
            !deletePending &&
            setShowDeleteModal(false)
          }
        >
          <section
            className="modal delete-job-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="delete-upgrade-detail-title"
          >
            <div className="delete-job-head">
              <span className="delete-job-icon">
                <Trash2 size={20} />
              </span>
              <div>
                <span className="eyebrow">
                  {zh ? "危险操作" : "Danger zone"}
                </span>
                <h2 id="delete-upgrade-detail-title">
                  {zh ? "确认删除任务？" : "Delete this upgrade job?"}
                </h2>
              </div>
              <button
                className="icon-button"
                onClick={() => !deletePending && setShowDeleteModal(false)}
                aria-label={zh ? "关闭" : "Close"}
                disabled={deletePending}
              >
                ×
              </button>
            </div>
            <div className="delete-job-body">
              <p>
                {zh
                  ? "删除后，该升级任务将从平台移除。"
                  : "This upgrade job will be removed from the platform."}
              </p>
              <div className="delete-job-target">
                <span>{zh ? "即将删除" : "Job to delete"}</span>
                <strong>{job.metadata.name}</strong>
              </div>
              <div className="delete-job-warning">
                <AlertTriangle size={15} />
                <span>
                  {zh
                    ? "此操作不可撤销，请确认该任务已不再需要。"
                    : "This action cannot be undone. Confirm that this job is no longer needed."}
                </span>
              </div>
              {deleteError && (
                <div className="delete-job-error" role="alert">
                  {zh ? "删除失败：" : "Delete failed: "}
                  {deleteError}
                </div>
              )}
            </div>
            <div className="delete-job-actions">
              <button
                className="secondary-button"
                onClick={() => setShowDeleteModal(false)}
                disabled={deletePending}
              >
                {zh ? "取消" : "Cancel"}
              </button>
              <button
                className="delete-job-confirm"
                onClick={handleDeleteConfirm}
                disabled={deletePending}
              >
                {deletePending ? (
                  <Loader2 className="job-action-loading" size={15} />
                ) : (
                  <Trash2 size={15} />
                )}
                {deletePending
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
      )}
    </div>
  );
}

// ─── YAML Code Block with lightweight syntax highlighting ─────────────────────

const YAML_KEY_COLOR = "#7c3aed";
const YAML_VALUE_COLOR = "#1f2937";
const YAML_COMMENT_COLOR = "#9ca3af";
const YAML_PUNCT_COLOR = "#6b7280";
const YAML_STRING_COLOR = "#059669";
const YAML_NUMBER_COLOR = "#d97706";
const YAML_BOOL_COLOR = "#2563eb";

interface YamlLine {
  indent: number;
  key?: string;
  value?: string;
  comment?: string;
  isSeparator: boolean;
  raw: string;
}

function parseYamlLine(line: string): YamlLine {
  const trimmed = line.trimEnd();
  if (trimmed.trimStart().startsWith("#")) {
    return {
      indent: line.length - line.trimStart().length,
      comment: trimmed.trim(),
      isSeparator: false,
      raw: line,
    };
  }
  if (!trimmed) {
    return { indent: 0, isSeparator: true, raw: line };
  }

  const indent = line.length - line.trimStart().length;
  const content = trimmed.trimStart();
  const colonIdx = content.indexOf(":");

  if (colonIdx > 0) {
    const key = content.slice(0, colonIdx).trim();
    const after = content.slice(colonIdx + 1).trim();
    let value = after;
    let comment: string | undefined;
    const commentIdx = findCommentStart(after);
    if (commentIdx >= 0) {
      value = after.slice(0, commentIdx).trim();
      comment = after.slice(commentIdx).trim();
    }
    return {
      indent,
      key,
      value: value || undefined,
      comment,
      isSeparator: false,
      raw: line,
    };
  }

  return { indent, value: content, isSeparator: false, raw: line };
}

function findCommentStart(s: string): number {
  let inSingle = false;
  let inDouble = false;
  for (let i = 0; i < s.length; i++) {
    const ch = s[i];
    if (ch === "'" && !inDouble) inSingle = !inSingle;
    else if (ch === '"' && !inSingle) inDouble = !inDouble;
    else if (ch === "#" && !inSingle && !inDouble) {
      if (i === 0 || s[i - 1] === " " || s[i - 1] === "\t") return i;
    }
  }
  return -1;
}

function YamlValue({ value }: { value: string }) {
  if (!value) return null;
  if (/^(true|false|yes|no|on|off)$/i.test(value)) {
    return <span style={{ color: YAML_BOOL_COLOR }}>{value}</span>;
  }
  if (/^-?[\d.]+$/.test(value)) {
    return <span style={{ color: YAML_NUMBER_COLOR }}>{value}</span>;
  }
  if (
    (value.startsWith('"') && value.endsWith('"')) ||
    (value.startsWith("'") && value.endsWith("'"))
  ) {
    return <span style={{ color: YAML_STRING_COLOR }}>{value}</span>;
  }
  if (value.startsWith("- ")) {
    return (
      <>
        <span style={{ color: YAML_PUNCT_COLOR }}>- </span>
        <YamlValue value={value.slice(2)} />
      </>
    );
  }
  return <span style={{ color: YAML_VALUE_COLOR }}>{value}</span>;
}

function YamlCodeBlock({ code }: { code: string }) {
  const lines = code.split("\n");
  return (
    <pre
      style={{
        margin: 0,
        padding: "10px 12px",
        background: "rgba(127,136,151,0.06)",
        borderRadius: 6,
        fontSize: 12,
        fontFamily: '"JetBrains Mono", var(--font-mono, monospace)',
        lineHeight: 1.65,
        whiteSpace: "pre-wrap",
        wordBreak: "break-all",
        overflowX: "auto",
      }}
    >
      {lines.map((line, i) => {
        const parsed = parseYamlLine(line);
        if (parsed.isSeparator) return <div key={i}> </div>;
        return (
          <div key={i}>
            <span>{" ".repeat(parsed.indent)}</span>
            {parsed.comment && !parsed.key && !parsed.value && (
              <span style={{ color: YAML_COMMENT_COLOR }}>
                {parsed.comment}
              </span>
            )}
            {parsed.key && (
              <>
                <span style={{ color: YAML_KEY_COLOR }}>{parsed.key}</span>
                <span style={{ color: YAML_PUNCT_COLOR }}>:</span>
                {parsed.value !== undefined && <span> </span>}
              </>
            )}
            {parsed.value && <YamlValue value={parsed.value} />}
            {parsed.comment && (parsed.key || parsed.value) && (
              <>
                {" "}
                <span style={{ color: YAML_COMMENT_COLOR }}>
                  {parsed.comment}
                </span>
              </>
            )}
          </div>
        );
      })}
    </pre>
  );
}

// ─── Row Actions (参考 Jobs 页面 JobActionMenu) ───────────────────────────────

function UpgradeJobRowActions({
  job,
  zh,
  canDelete,
  onCopyCreate,
  onDelete,
}: {
  job: UpgradeJobItem;
  zh: boolean;
  canDelete: boolean;
  onCopyCreate: () => void;
  onDelete: () => void;
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

  const handleToggle = () => {
    setOpen((v) => !v);
  };

  return (
    <div className="row-actions" ref={ref} style={{ position: "relative" }}>
      <button className="job-row-action" onClick={onCopyCreate}>
        <CopyIcon size={13} />
        {zh ? "复制创建" : "Clone"}
      </button>
      <span
        className="action-tooltip"
        data-tooltip={zh ? "更多操作" : "More actions"}
      >
        <button
          ref={btnRef}
          className="icon-button"
          onClick={handleToggle}
          aria-label={
            zh
              ? `更多操作 ${job.metadata.name}`
              : `More actions ${job.metadata.name}`
          }
          aria-expanded={open}
          disabled={!canDelete}
        >
          <MoreVertical size={15} />
        </button>
      </span>
      {open && (
        <div className="action-dropdown" style={menuStyle}>
          <button
            className="action-dropdown-item danger"
            onClick={() => {
              setOpen(false);
              onDelete();
            }}
          >
            <Trash2 size={13} />
            {zh ? "删除" : "Delete"}
          </button>
        </div>
      )}
    </div>
  );
}
