import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import {
  AlertCircle,
  Boxes,
  ChevronLeft,
  ChevronRight,
  Database,
  Download,
  FileText,
  Folder,
  FolderOpen,
  HardDrive,
  Home,
  Link2,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Server,
  Trash2,
  Upload,
  X,
} from "lucide-react";
import { type StorageClass, type StorageProvider } from "../data";
import { copy, type Copy } from "../i18n";
import {
  ColumnFilterButton,
  PageToolbar,
  Pagination,
  RefreshOverlay,
  useColumnFilter,
} from "../components/shared";
import { ColumnFilterPopover } from "../components/ColumnFilterPopover";
import { formatChinaDateTime } from "../utils/time";

type StorageClassFormState = {
  name: string;
  clusters: string[];
  provider: StorageProvider;
  endpoint: string;
  region: string;
  bucket: string;
  access_key_id: string;
  access_key_secret: string;
  path_style: boolean;
  description: string;
};

type ProviderOption = {
  name: string;
  value: string;
};

type GatewayErrorBody = {
  code?: string;
  details?: { fields?: unknown };
};

const storageClassFieldLabels: Record<string, { zh: string; en: string }> = {
  name: { zh: "名称", en: "Name" },
  bucket: { zh: "存储桶", en: "Bucket" },
  access_key_secret: { zh: "Access Key Secret", en: "Access Key Secret" },
};

function storageClassSubmissionError(
  error: unknown,
  isChinese: boolean,
  fallback: string,
) {
  if (!(error instanceof ApiError)) return fallback;

  const body = error.body as GatewayErrorBody | undefined;
  if (body?.code === "STORAGE_CLASS_REQUIRED_FIELDS") {
    const fields = Array.isArray(body.details?.fields)
      ? body.details.fields.filter(
          (field): field is string =>
            typeof field === "string" && field in storageClassFieldLabels,
        )
      : [];
    if (fields.length > 0) {
      const labels = fields.map(
        (field) => storageClassFieldLabels[field][isChinese ? "zh" : "en"],
      );
      return isChinese
        ? `请填写必填字段：${labels.join("、")}。`
        : `Required fields: ${labels.join(", ")}.`;
    }
  }

  return errorMessage(error.body) || error.message || fallback;
}

function normalizeStorageClass(raw: any): StorageClass {
  const name = raw?.name ?? raw?.id ?? "";
  return {
    id: raw?.id ?? name,
    name,
    namespace: raw?.namespace ?? "default",
    clusters: Array.isArray(raw?.clusters) ? raw.clusters : [],
    provider: (raw?.provider ?? "") as StorageProvider,
    endpoint: raw?.endpoint ?? "",
    region: raw?.region ?? "",
    bucket: raw?.bucket ?? "",
    accessKeyId: raw?.accessKeyId ?? raw?.access_key_id ?? "",
    pathStyle: Boolean(raw?.pathStyle ?? raw?.path_style),
    description: raw?.description ?? "",
    createdAt: raw?.createdAt ?? raw?.created_at ?? "",
  };
}

function isMaskedAccessKeyID(value: string): boolean {
  return /^.{2}\*+.{2}$/.test(value);
}

function storageClassToForm(
  storageClass?: StorageClass,
): StorageClassFormState {
  const accessKeyID = storageClass?.accessKeyId ?? "";
  return {
    name: storageClass?.name ?? "",
    clusters: storageClass?.clusters ?? [],
    provider: storageClass?.provider ?? "",
    endpoint: storageClass?.endpoint ?? "",
    region: storageClass?.region ?? "",
    bucket: storageClass?.bucket ?? "",
    access_key_id: accessKeyID,
    access_key_secret: "",
    path_style: storageClass?.pathStyle ?? false,
    description: storageClass?.description ?? "",
  };
}

export function StorageClassesPage({
  copy: c,
  selectedName,
  onSelect,
  onCreate,
  onBrowseFiles,
  refreshKey = 0,
}: {
  copy: Copy;
  selectedName?: string;
  onSelect: (name?: string) => void;
  onCreate: () => void;
  onBrowseFiles: (storageClass: string) => void;
  refreshKey?: number;
}) {
  const zh = c === copy.zh;
  const [realClasses, setRealClasses] = useState<StorageClass[]>([]);
  const [loading, setLoading] = useState(false);
  const [fetched, setFetched] = useState(false);
  const [search, setSearch] = useState("");
  // 表头列多选筛选；空数组 = 全部
  const [providerFilter, setProviderFilter] = useState<string[]>([]);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const columnFilter = useColumnFilter();
  const [editingClass, setEditingClass] = useState<StorageClass | null>(null);

  const selected = useMemo(
    () => realClasses.find((sc) => sc.id === selectedName),
    [realClasses, selectedName],
  );

  const fetchClasses = async () => {
    if (loading) return;
    setLoading(true);
    try {
      const data = await storageClassesApi.list();
      const list: StorageClass[] = Object.values(
        data as Record<string, unknown>,
      ).map(normalizeStorageClass);
      setRealClasses(list);
    } catch {
      setRealClasses([]);
    } finally {
      setLoading(false);
      setFetched(true);
    }
  };

  useEffect(() => {
    if (!fetched) fetchClasses();
  }, [fetched]);

  useEffect(() => {
    setFetched(false);
  }, [refreshKey]);

  useEffect(() => setPage(1), [search, providerFilter, pageSize]);

  const handleDelete = async (id: string, name: string) => {
    if (
      !confirm(
        zh ? `确定删除存储类 "${name}" 吗?` : `Delete storage class "${name}"?`,
      )
    )
      return;
    try {
      await storageClassesApi.remove(id || name);
      fetchClasses();
    } catch (e) {
      console.error("Failed to delete storage class:", e);
    }
  };

  if (selected) {
    return (
      <StorageClassDetailPage
        copy={c}
        storageClass={selected}
        onBack={() => onSelect()}
      />
    );
  }

  const filtered = realClasses.filter((sc) => {
    const query = search.toLowerCase();
    const matchesSearch =
      sc.name.toLowerCase().includes(query) ||
      sc.provider.toLowerCase().includes(query) ||
      sc.bucket.toLowerCase().includes(query);
    const providerHit =
      providerFilter.length === 0 || providerFilter.includes(sc.provider);
    return matchesSearch && providerHit;
  });
  const providers = Array.from(new Set(realClasses.map((sc) => sc.provider)));
  // 列表页列不再支持排序，按名称做稳定 tiebreak 即可
  const sortedClasses = [...filtered].sort((a, b) =>
    a.name.localeCompare(b.name, zh ? "zh-CN" : "en", { numeric: true }),
  );
  const totalPages = Math.max(1, Math.ceil(sortedClasses.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pagedClasses = sortedClasses.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );

  return (
    <div className="page-content resource-page storage-class-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">
            <HardDrive size={13} />
            {c.storageClass.eyebrow}
          </span>
          <h2>{c.storageClass.title}</h2>
          <p>{c.storageClass.desc}</p>
        </div>
        <div className="section-actions">
          <button className="primary-button" onClick={onCreate}>
            <Plus size={17} />
            {c.storageClass.createBtn}
          </button>
        </div>
      </div>
      <section
        className="storage-overview-grid"
        aria-label={zh ? "存储资源概况" : "Storage overview"}
      >
        <div className="storage-overview-card purple">
          <span>
            <Database size={18} />
          </span>
          <div>
            <small>{zh ? "存储类" : "Storage classes"}</small>
            <strong>{realClasses.length}</strong>
            <em>{zh ? "个可用存储配置" : "available configurations"}</em>
          </div>
        </div>
        <div className="storage-overview-card orange">
          <span>
            <Boxes size={18} />
          </span>
          <div>
            <small>{zh ? "存储提供商" : "Providers"}</small>
            <strong>{providers.length}</strong>
            <em>{providers.join(" · ") || "—"}</em>
          </div>
        </div>
      </section>
      <PageToolbar
        placeholder={c.storageClass.search}
        value={search}
        onChange={setSearch}
        count={filtered.length}
        copy={c}
        onRefresh={fetchClasses}
        refreshing={loading}
      />
      <section
        className={`table-panel storage-class-table-panel refreshable-region${loading ? " is-refreshing" : ""}`}
        aria-busy={loading}
      >
        <div className="storage-table-heading">
          <div>
            <strong>{zh ? "存储资源" : "Storage resources"}</strong>
            <small>
              {zh
                ? "管理存储桶和访问配置"
                : "Manage buckets and access settings"}
            </small>
          </div>
          <span>
            {zh ? `共 ${filtered.length} 项` : `${filtered.length} items`}
          </span>
        </div>
        <table>
          <thead>
            <tr>
              <th>{c.storageClass.name}</th>
              <th>
                <ColumnFilterButton
                  label={c.storageClass.provider}
                  selectedCount={providerFilter.length}
                  onClick={columnFilter.openFor("provider")}
                />
              </th>
              <th>{c.storageClass.bucket}</th>
              <th>{c.storageClass.description}</th>
              <th className="storage-actions-col">{zh ? "操作" : "Actions"}</th>
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 && (
              <tr>
                <td colSpan={5} className="storage-empty-cell">
                  {loading
                    ? zh
                      ? "加载中..."
                      : "Loading..."
                    : c.storageClass.noData}
                </td>
              </tr>
            )}
            {pagedClasses.map((sc) => (
              <tr key={sc.id}>
                <td>
                  <button
                    className="storage-name-cell"
                    onClick={() => onSelect(sc.id)}
                    aria-label={`${zh ? "查看存储类" : "View storage class"} ${sc.name}`}
                  >
                    <span>
                      <HardDrive size={16} />
                    </span>
                    <span>
                      <strong>{sc.name}</strong>
                    </span>
                  </button>
                </td>
                <td>
                  <span className="storage-provider-chip">{sc.provider}</span>
                </td>
                <td>
                  <code className="inline-code">{sc.bucket}</code>
                </td>
                <td>
                  <span className="storage-description">
                    {sc.description || "—"}
                  </span>
                </td>
                <td className="storage-actions-col">
                  <div className="row-actions">
                    <button
                      className="icon-button"
                      title={c.files.eyebrow}
                      onClick={(e) => {
                        e.stopPropagation();
                        onBrowseFiles(sc.id);
                      }}
                    >
                      <FolderOpen size={14} />
                    </button>
                    <button
                      className="icon-button"
                      title={zh ? "编辑存储类" : "Edit storage class"}
                      onClick={(e) => {
                        e.stopPropagation();
                        setEditingClass(sc);
                      }}
                    >
                      <Pencil size={14} />
                    </button>
                    <button
                      className="icon-button danger"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDelete(sc.id, sc.name);
                      }}
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <RefreshOverlay
          visible={loading}
          label={zh ? "正在刷新存储类列表" : "Refreshing storage class list"}
        />
      </section>
      <Pagination
        page={currentPage}
        pageSize={pageSize}
        total={filtered.length}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
        zh={zh}
      />
      {columnFilter.openKey === "provider" && (
        <ColumnFilterPopover
          label={c.storageClass.provider}
          options={providers.map((p) => ({ value: p, label: p }))}
          selected={providerFilter}
          onChange={setProviderFilter}
          anchorRect={columnFilter.anchorRect}
          onClose={columnFilter.close}
          zh={zh}
        />
      )}
      {editingClass && (
        <StorageClassCreatePage
          copy={c}
          initialStorageClass={editingClass}
          onCreated={() => {
            setEditingClass(null);
            fetchClasses();
          }}
          onBack={() => setEditingClass(null)}
        />
      )}
    </div>
  );
}

export function StorageClassDetailPage({
  copy: c,
  storageClass,
  onBack,
}: {
  copy: Copy;
  storageClass: StorageClass;
  onBack: () => void;
}) {
  const zh = c === copy.zh;
  return (
    <div className="page-content resource-page storage-detail-page">
      <div className="section-heading storage-detail-hero">
        <div>
          <button
            type="button"
            className="plain-button back-button"
            onClick={onBack}
          >
            ← {zh ? "返回存储列表" : "Back to storage"}
          </button>
          <span className="eyebrow">
            <HardDrive size={13} />
            {c.storageClass.eyebrow}
          </span>
          <h2>{storageClass.name}</h2>
          <p>
            {storageClass.provider} · {storageClass.bucket}
          </p>
          <div className="storage-detail-badges">
            <span>{storageClass.namespace}</span>
            <span>
              {storageClass.pathStyle
                ? zh
                  ? "Path Style 已启用"
                  : "Path Style enabled"
                : zh
                  ? "标准访问模式"
                  : "Standard access"}
            </span>
          </div>
        </div>
      </div>
      <div className="node-detail-body storage-detail-layout">
        <section className="node-detail-section storage-detail-panel">
          <span className="node-detail-label">
            <Database size={15} />
            {c.storageClass.basicInfo}
          </span>
          <p className="storage-detail-section-copy">
            {zh
              ? "存储资源的归属与基础配置"
              : "Ownership and base configuration"}
          </p>
          <div className="node-detail-grid storage-detail-info-grid">
            <div>
              <span className="muted">{c.storageClass.provider}</span>
              <strong>{storageClass.provider}</strong>
            </div>
            <div>
              <span className="muted">{c.storageClass.bucket}</span>
              <strong>{storageClass.bucket}</strong>
            </div>
            <div>
              <span className="muted">{c.storageClass.region}</span>
              <strong>{storageClass.region || "—"}</strong>
            </div>
            <div>
              <span className="muted">{c.storageClass.createdAt}</span>
              <strong>{storageClass.createdAt || "—"}</strong>
            </div>
          </div>
        </section>
        <section className="node-detail-section storage-detail-panel">
          <span className="node-detail-label">
            <Link2 size={15} />
            {c.storageClass.connection}
          </span>
          <p className="storage-detail-section-copy">
            {zh ? "访问端点与连接方式" : "Endpoint and connection mode"}
          </p>
          <div className="node-detail-grid storage-detail-info-grid">
            <div>
              <span className="muted">{c.storageClass.endpoint}</span>
              <strong>{storageClass.endpoint || "—"}</strong>
            </div>
            <div>
              <span className="muted">{c.storageClass.pathStyle}</span>
              <strong>
                {storageClass.pathStyle
                  ? zh
                    ? "启用"
                    : "Enabled"
                  : zh
                    ? "禁用"
                    : "Disabled"}
              </strong>
            </div>
            <div>
              <span className="muted">{c.storageClass.description}</span>
              <strong>{storageClass.description || "—"}</strong>
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}

export function StorageClassCreatePage({
  copy: c,
  onBack,
  onCreated,
  initialStorageClass,
}: {
  copy: Copy;
  onBack: () => void;
  onCreated?: () => void;
  initialStorageClass?: StorageClass;
}) {
  const zh = c === copy.zh;
  const isEdit = Boolean(initialStorageClass);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [form, setForm] = useState<StorageClassFormState>(() =>
    storageClassToForm(initialStorageClass),
  );
  const [providerOptions, setProviderOptions] = useState<ProviderOption[]>([]);
  const [clusterOptions, setClusterOptions] = useState<
    Array<{ id: string; name: string }>
  >([]);
  const [clustersMenuOpen, setClustersMenuOpen] = useState(false);
  const clustersMenuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setForm(storageClassToForm(initialStorageClass));
  }, [initialStorageClass]);

  // 提供商目录来自后端，取值必须与后端校验的 canonical 值一致，否则下发到
  // 数据面的 rclone 配置无法识别。
  useEffect(() => {
    let cancelled = false;
    storageClassesApi
      .providers()
      .then((data) => {
        if (cancelled) return;
        setProviderOptions(
          data
            .map((provider) => ({
              name: provider.name ?? provider.value ?? "",
              value: provider.value ?? "",
            }))
            .filter((provider) => provider.value),
        );
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    clustersApi
      .list<{ id?: string; name?: string }>()
      .then((clusters) => {
        if (cancelled) return;
        setClusterOptions(
          clusters
            .map((cluster) => {
              const id = cluster.id || cluster.name || "";
              return { id, name: cluster.name || id };
            })
            .filter((cluster) => cluster.id),
        );
      })
      .catch(() => {
        if (!cancelled) setClusterOptions([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const handlePointerDown = (event: PointerEvent) => {
      if (
        clustersMenuRef.current &&
        !clustersMenuRef.current.contains(event.target as Node)
      ) {
        setClustersMenuOpen(false);
      }
    };
    document.addEventListener("pointerdown", handlePointerDown);
    return () => document.removeEventListener("pointerdown", handlePointerDown);
  }, []);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        if (clustersMenuOpen) {
          setClustersMenuOpen(false);
          return;
        }
        if (!submitting) onBack();
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [clustersMenuOpen, onBack, submitting]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!isEdit && !form.access_key_secret.trim()) {
      setError(
        zh ? "Access Key Secret 不能为空。" : "Access Key Secret is required.",
      );
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      if (isEdit)
        await storageClassesApi.update(initialStorageClass!.name, form);
      else await storageClassesApi.create(form);
      onCreated?.();
      onBack();
    } catch (err) {
      setError(
        storageClassSubmissionError(err, zh, c.storageClass.createFailed),
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    // 表单内容无法恢复，遮罩点击不关闭弹层；关闭入口为右上角按钮、取消按钮和 Esc。
    <div className="modal-backdrop storage-create-backdrop">
      <div
        className="modal storage-create-modal"
        role="dialog"
        aria-modal="true"
      >
        <div className="modal-head storage-create-head">
          <div>
            <span className="eyebrow">
              <HardDrive size={13} />
              {c.storageClass.eyebrow}
            </span>
            <h2>
              {isEdit
                ? zh
                  ? "编辑存储类"
                  : "Edit storage class"
                : c.storageClass.createTitle}
            </h2>
            <p>
              {isEdit
                ? zh
                  ? "调整对象存储配置和访问参数。"
                  : "Update object storage settings and access parameters."
                : c.storageClass.desc}
            </p>
          </div>
          <button
            type="button"
            className="icon-button storage-create-close"
            aria-label={zh ? "关闭" : "Close"}
            onClick={onBack}
          >
            <X size={18} />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="storage-create-form">
          <div className="form-section">
            <strong>{c.storageClass.basicInfo}</strong>
            <div className="form-grid">
              <label>
                {c.storageClass.name} *
                <input
                  required
                  disabled={isEdit}
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder={zh ? "my-storage-class" : "my-storage-class"}
                />
              </label>
              <label>
                {c.storageClass.provider} *
                <select
                  required
                  value={form.provider}
                  onChange={(e) =>
                    setForm({
                      ...form,
                      provider: e.target.value as StorageProvider,
                    })
                  }
                >
                  <option value="">{zh ? "请选择" : "Select"}</option>
                  {providerOptions.map((p, index) => (
                    <option key={`${p.value}-${index}`} value={p.value}>
                      {p.name}
                    </option>
                  ))}
                  {form.provider &&
                    !providerOptions.some((p) => p.value === form.provider) && (
                      <option value={form.provider}>{form.provider}</option>
                    )}
                </select>
              </label>
            </div>
          </div>
          <div className="form-section">
            <strong>{zh ? "关联集群" : "Associated clusters"}</strong>
            <div className="registry-cluster-select" ref={clustersMenuRef}>
              <button
                type="button"
                className="registry-cluster-trigger"
                aria-expanded={clustersMenuOpen}
                aria-haspopup="listbox"
                onClick={() => setClustersMenuOpen((open) => !open)}
              >
                <span>
                  {form.clusters.length === 0
                    ? zh
                      ? "请选择集群"
                      : "Select clusters"
                    : zh
                      ? `已选择 ${form.clusters.length} 个集群`
                      : `${form.clusters.length} cluster${
                          form.clusters.length === 1 ? "" : "s"
                        } selected`}
                </span>
                <ChevronRight
                  size={16}
                  className={clustersMenuOpen ? "open" : ""}
                />
              </button>
              {clustersMenuOpen && (
                <div className="registry-cluster-options" role="listbox">
                  {Array.from(
                    new Set([
                      ...clusterOptions.map((cluster) => cluster.id),
                      ...form.clusters,
                    ]),
                  )
                    .sort()
                    .map((id) => {
                      const option = clusterOptions.find(
                        (cluster) => cluster.id === id,
                      );
                      return (
                        <label
                          key={id}
                          className={form.clusters.includes(id) ? "active" : ""}
                        >
                          <input
                            type="checkbox"
                            checked={form.clusters.includes(id)}
                            onChange={() =>
                              setForm({
                                ...form,
                                clusters: form.clusters.includes(id)
                                  ? form.clusters.filter(
                                      (cluster) => cluster !== id,
                                    )
                                  : [...form.clusters, id].sort(),
                              })
                            }
                          />
                          <span>
                            <strong>{option?.name || id}</strong>
                          </span>
                        </label>
                      );
                    })}
                  {clusterOptions.length === 0 &&
                    form.clusters.length === 0 && (
                      <p>{zh ? "暂无可选集群" : "No clusters available"}</p>
                    )}
                </div>
              )}
            </div>
          </div>
          <div className="form-section">
            <strong>{c.storageClass.connection}</strong>
            <div className="form-grid">
              <label>
                {c.storageClass.endpoint} *
                <input
                  required
                  value={form.endpoint}
                  onChange={(e) =>
                    setForm({ ...form, endpoint: e.target.value })
                  }
                  placeholder="https://s3.amazonaws.com"
                />
              </label>
              <label>
                {c.storageClass.region} *
                <input
                  required
                  value={form.region}
                  onChange={(e) => setForm({ ...form, region: e.target.value })}
                  placeholder="us-east-1"
                />
              </label>
              <label>
                {c.storageClass.bucket} *
                <input
                  required
                  value={form.bucket}
                  onChange={(e) => setForm({ ...form, bucket: e.target.value })}
                  placeholder="my-bucket"
                />
              </label>
              <label>
                {c.storageClass.pathStyle}
                <input
                  type="checkbox"
                  checked={form.path_style}
                  onChange={(e) =>
                    setForm({ ...form, path_style: e.target.checked })
                  }
                />
              </label>
              <label>
                {c.storageClass.accessKeyId} *
                <input
                  required={!isEdit}
                  autoComplete="off"
                  value={form.access_key_id}
                  onChange={(e) =>
                    setForm({ ...form, access_key_id: e.target.value })
                  }
                  placeholder={
                    isEdit
                      ? zh
                        ? "留空则保持原 Access Key ID"
                        : "Leave blank to keep the existing Access Key ID"
                      : "AKIAIOSFODNN7EXAMPLE"
                  }
                />
              </label>
              <label>
                {c.storageClass.accessKeySecret} *
                <input
                  required={!isEdit}
                  type="password"
                  autoComplete="new-password"
                  value={form.access_key_secret}
                  onChange={(e) =>
                    setForm({ ...form, access_key_secret: e.target.value })
                  }
                  placeholder={
                    isEdit
                      ? zh
                        ? "留空则保持原密钥"
                        : "Leave blank to keep the existing secret"
                      : "••••••••••••"
                  }
                />
              </label>
            </div>
          </div>
          <div className="form-section">
            <label>
              {c.storageClass.description}
              <textarea
                value={form.description}
                onChange={(e) =>
                  setForm({ ...form, description: e.target.value })
                }
                placeholder={
                  zh
                    ? "描述存储类的用途和特性"
                    : "Describe the purpose and characteristics of the storage class"
                }
                rows={3}
              />
            </label>
          </div>
          {error && (
            <div className="storage-create-error" role="alert">
              <AlertCircle size={18} aria-hidden="true" />
              <span>{error}</span>
              <button
                type="button"
                className="storage-create-error-close"
                aria-label={zh ? "关闭错误提示" : "Dismiss error"}
                onClick={() => setError("")}
              >
                <X size={16} aria-hidden="true" />
              </button>
            </div>
          )}
          <div className="form-actions">
            <button type="button" className="secondary-button" onClick={onBack}>
              {zh ? "取消" : "Cancel"}
            </button>
            <button
              type="submit"
              className="primary-button"
              disabled={submitting}
            >
              <Plus size={17} />
              {submitting
                ? zh
                  ? isEdit
                    ? "保存中..."
                    : "创建中..."
                  : isEdit
                    ? "Saving..."
                    : "Creating..."
                : isEdit
                  ? zh
                    ? "保存修改"
                    : "Save changes"
                  : c.storageClass.createBtn}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export function StorageClassFilesPage({
  copy: c,
  sub,
  onBack,
}: {
  copy: Copy;
  sub: string;
  onBack: () => void;
}) {
  const zh = c === copy.zh;
  const name = decodeURIComponent(sub);
  const [prefix, setPrefix] = useState("");
  const [objects, setObjects] = useState<any[]>([]);
  const [commonPrefixes, setCommonPrefixes] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const fetchFiles = async () => {
    if (!name) {
      setError(
        zh ? "参数错误：缺少存储类名称" : "Error: missing storage class name",
      );
      return;
    }
    setLoading(true);
    setError("");
    try {
      const data = await storageObjectsApi.list<{
        objects?: any[];
        common_prefixes?: string[];
      }>(name, prefix);
      setObjects(data.objects || []);
      setCommonPrefixes(data.common_prefixes || []);
    } catch (e: any) {
      setError(
        e?.message || (zh ? "加载文件列表失败" : "Failed to load file list"),
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchFiles();
  }, [name, prefix]);

  const handleUpload = async (files: FileList) => {
    if (!name) return;
    setUploading(true);
    setError("");
    try {
      for (let i = 0; i < files.length; i++) {
        const file = files[i];
        const formData = new FormData();
        const key = prefix ? `${prefix}${file.name}` : file.name;
        formData.append("file", file, key);
        setUploadProgress(
          `${zh ? "上传中" : "Uploading"}: ${file.name} (${i + 1}/${files.length})`,
        );
        await storageObjectsApi.upload(name, formData);
      }
      setUploadProgress("");
      fetchFiles();
    } catch (e: any) {
      setError(e?.message || (zh ? "上传失败" : "Upload failed"));
    } finally {
      setUploading(false);
    }
  };

  const handleDownload = async (key: string) => {
    try {
      const data = await storageObjectsApi.download<{ url?: string }>(
        name,
        key,
      );
      if (data.url) {
        window.open(data.url, "_blank");
      }
    } catch (e: any) {
      alert(
        e?.message || (zh ? "获取下载链接失败" : "Failed to get download URL"),
      );
    }
  };

  const handleDelete = async (key: string) => {
    if (!confirm(zh ? `确定删除文件 "${key}" 吗?` : `Delete file "${key}"?`))
      return;
    try {
      await storageObjectsApi.remove(name, key);
      fetchFiles();
    } catch (e: any) {
      alert(e?.message || (zh ? "删除失败" : "Delete failed"));
    }
  };

  const normalizeDirectoryPrefix = (directoryPrefix: string) => {
    const normalized = directoryPrefix.replace(/^\/+|\/+$/g, "");
    return normalized ? `${normalized}/` : "";
  };

  const navigateFolder = (folderPrefix: string) => {
    setPrefix(normalizeDirectoryPrefix(folderPrefix));
  };

  const goUp = () => {
    const currentDirectory = normalizeDirectoryPrefix(prefix).slice(0, -1);
    const parentEnd = currentDirectory.lastIndexOf("/");
    setPrefix(
      parentEnd >= 0 ? `${currentDirectory.slice(0, parentEnd + 1)}` : "",
    );
  };

  const filteredObjects = objects.filter((obj) =>
    obj.key.toLowerCase().includes(search.toLowerCase()),
  );

  const filteredPrefixes = commonPrefixes.filter((folder) =>
    folder.toLowerCase().includes(search.toLowerCase()),
  );
  const totalPages = Math.max(1, Math.ceil(filteredObjects.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pagedObjects = filteredObjects.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );

  useEffect(() => setPage(1), [prefix, search, pageSize]);

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
    if (bytes < 1024 * 1024 * 1024)
      return (bytes / (1024 * 1024)).toFixed(1) + " MB";
    return (bytes / (1024 * 1024 * 1024)).toFixed(2) + " GB";
  };

  const formatDate = (dateStr: string) => {
    return formatChinaDateTime(dateStr);
  };

  const breadcrumbs = () => {
    if (!prefix) return [<span key="root">{c.files.rootPath}</span>];
    const parts = prefix.split("/").filter(Boolean);
    const crumbs = [
      <button type="button" key="root" onClick={() => setPrefix("")}>
        {c.files.rootPath}
      </button>,
    ];
    let path = "";
    for (const part of parts) {
      path += part + "/";
      const breadcrumbPath = path;
      crumbs.push(
        <button
          type="button"
          key={breadcrumbPath}
          onClick={() => setPrefix(breadcrumbPath)}
        >
          {part}
        </button>,
      );
    }
    return crumbs;
  };

  return (
    <div className="page-content resource-page files-page storage-files-page">
      <div className="section-heading storage-files-hero">
        <div>
          <span className="eyebrow">
            <FolderOpen size={13} />
            {c.files.eyebrow}
          </span>
          <h2>{name || c.files.title}</h2>
          <p>
            {c.files.storageClass}: {name || "—"}
          </p>
          <div className="storage-detail-badges">
            <span>
              {prefix
                ? zh
                  ? "子目录"
                  : "Subfolder"
                : zh
                  ? "根目录"
                  : "Root directory"}
            </span>
          </div>
        </div>
        <div className="section-actions">
          <button className="secondary-button" onClick={onBack}>
            <ChevronLeft size={17} />
            {c.files.backToStorage}
          </button>
        </div>
      </div>

      <section
        className="storage-files-summary"
        aria-label={zh ? "文件资源概况" : "File resource overview"}
      >
        <div>
          <span>
            <HardDrive size={17} />
          </span>
          <small>{zh ? "存储类" : "Storage class"}</small>
          <strong>{name || "—"}</strong>
        </div>
        <div>
          <span>
            <FolderOpen size={17} />
          </span>
          <small>{zh ? "当前目录内容" : "Current contents"}</small>
          <strong>
            {commonPrefixes.length + objects.length}{" "}
            <em>{zh ? "项" : "items"}</em>
          </strong>
        </div>
      </section>

      <div className="files-breadcrumb">
        <Home size={15} />
        <span className="breadcrumb-label">{c.files.currentPath}:</span>
        {breadcrumbs()}
      </div>

      <div className="page-toolbar storage-files-toolbar">
        <div className="search-field">
          <Search size={16} />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={c.files.search}
          />
        </div>
        <button
          className="primary-button"
          onClick={() => {
            const input = document.createElement("input");
            input.type = "file";
            input.multiple = true;
            input.onchange = (e) => {
              const files = (e.target as HTMLInputElement).files;
              if (files && files.length > 0) handleUpload(files);
            };
            input.click();
          }}
          disabled={uploading}
        >
          <Upload size={16} />
          {uploading ? uploadProgress || "..." : c.files.uploadBtn}
        </button>
        <button
          className="secondary-button"
          onClick={fetchFiles}
          disabled={loading}
        >
          <RefreshCw
            size={16}
            className={loading ? "job-action-loading" : ""}
          />
          {loading ? (zh ? "刷新中..." : "Refreshing...") : c.common.refresh}
        </button>
      </div>

      {error && (
        <div className="storage-files-error" role="alert">
          <AlertCircle size={18} />
          <div>
            <strong>
              {zh ? "目录内容加载失败" : "Failed to load directory"}
            </strong>
            <span>{error}</span>
          </div>
          <button
            type="button"
            className="secondary-button"
            onClick={fetchFiles}
          >
            <RefreshCw size={14} />
            {zh ? "重试" : "Retry"}
          </button>
        </div>
      )}

      <section
        className={`table-panel files-table storage-files-table-panel refreshable-region${loading ? " is-refreshing" : ""}`}
        aria-busy={loading}
      >
        <div className="storage-table-heading">
          <div>
            <strong>{zh ? "目录内容" : "Directory contents"}</strong>
            <small>
              {zh
                ? "浏览文件夹并管理当前路径下的对象"
                : "Browse folders and manage objects in the current path"}
            </small>
          </div>
          <span>
            {zh
              ? `共 ${filteredPrefixes.length + filteredObjects.length} 项`
              : `${filteredPrefixes.length + filteredObjects.length} items`}
          </span>
        </div>
        <table>
          <thead>
            <tr>
              <th>{c.files.fileName}</th>
              <th>{c.files.size}</th>
              <th>{c.files.lastModified}</th>
              <th>{c.files.actions}</th>
            </tr>
          </thead>
          <tbody>
            {prefix && (
              <tr className="clickable storage-go-up-row" onClick={goUp}>
                <td colSpan={4}>
                  <ChevronLeft size={15} />
                  {zh ? "返回上级目录" : "Go up"}
                </td>
              </tr>
            )}
            {filteredPrefixes.map((folder) => {
              const folderName = folder.endsWith("/")
                ? folder.slice(0, -1)
                : folder;
              const displayName = prefix
                ? folderName.replace(prefix, "")
                : folderName;
              return (
                <tr
                  key={folder}
                  className="clickable"
                  onClick={() => navigateFolder(folder)}
                >
                  <td>
                    <span className="storage-file-name folder">
                      <i>
                        <Folder size={17} />
                      </i>
                      <span>
                        <strong>{displayName}</strong>
                        <small>{zh ? "文件夹" : "Folder"}</small>
                      </span>
                    </span>
                  </td>
                  <td className="muted">—</td>
                  <td className="muted">—</td>
                  <td>
                    <button
                      className="icon-button"
                      title={zh ? "进入目录" : "Enter folder"}
                    >
                      <ChevronRight size={14} />
                    </button>
                  </td>
                </tr>
              );
            })}
            {pagedObjects.map((obj) => {
              const fileName = obj.key.split("/").pop() || obj.key;
              return (
                <tr key={obj.key}>
                  <td>
                    <span className="storage-file-name file">
                      <i>
                        <FileText size={17} />
                      </i>
                      <span>
                        <strong>{fileName}</strong>
                        <small>{zh ? "对象文件" : "Object"}</small>
                      </span>
                    </span>
                  </td>
                  <td>{formatSize(obj.size || 0)}</td>
                  <td>{formatDate(obj.last_modified)}</td>
                  <td>
                    <div className="row-actions">
                      <button
                        className="icon-button"
                        title={c.files.download}
                        onClick={() => handleDownload(obj.key)}
                      >
                        <Download size={14} />
                      </button>
                      <button
                        className="icon-button danger"
                        title={c.files.delete}
                        onClick={() => handleDelete(obj.key)}
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
            {!loading &&
              filteredObjects.length === 0 &&
              filteredPrefixes.length === 0 &&
              !error && (
                <tr>
                  <td colSpan={4}>
                    <div className="storage-files-state empty">
                      <span>
                        <FolderOpen size={22} />
                      </span>
                      <strong>{c.files.noData}</strong>
                      <small>
                        {zh
                          ? "可以上传文件，或返回上级目录继续浏览。"
                          : "Upload files or return to the parent directory."}
                      </small>
                    </div>
                  </td>
                </tr>
              )}
          </tbody>
        </table>
        <RefreshOverlay
          visible={loading}
          label={zh ? "正在刷新目录内容" : "Refreshing directory contents"}
        />
      </section>
      {!loading && filteredObjects.length > 0 && (
        <Pagination
          page={currentPage}
          pageSize={pageSize}
          total={filteredObjects.length}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
          zh={zh}
        />
      )}
    </div>
  );
}
import { clustersApi, storageClassesApi, storageObjectsApi } from "../backend";
import { ApiError, errorMessage } from "../api";
