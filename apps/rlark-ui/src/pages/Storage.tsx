import { useEffect, useMemo, useState, type FormEvent } from "react";
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
import {
  clusters as mockClusters,
  type StorageClass,
  type StorageProvider,
} from "../data";
import { copy, type Copy } from "../i18n";
import {
  compareSortValues,
  PageToolbar,
  Pagination,
  SortButton,
  type SortDirection,
} from "../components/shared";
import { formatChinaDateTime } from "../utils/time";

type StorageClassFormState = {
  name: string;
  namespace: string;
  provider: StorageProvider;
  clusters: string[];
  endpoint: string;
  region: string;
  bucket: string;
  access_key_id: string;
  access_key_secret: string;
  path_style: boolean;
  description: string;
};

type ClusterOption = {
  id: string;
  name: string;
  description?: string;
};

const storageProviders: StorageProvider[] = [
  "AWS S3",
  "Google Cloud",
  "Azure Blob",
  "MinIO",
  "Aliyun OSS",
  "Tencent COS",
];

function normalizeStorageClass(raw: any): StorageClass {
  const name = raw?.name ?? raw?.id ?? "";
  return {
    id: raw?.id ?? name,
    name,
    namespace: raw?.namespace ?? "default",
    provider: (raw?.provider ?? "MinIO") as StorageProvider,
    clusters: Array.isArray(raw?.clusters) ? raw.clusters : [],
    endpoint: raw?.endpoint ?? "",
    region: raw?.region ?? "",
    bucket: raw?.bucket ?? "",
    accessKeyId: raw?.accessKeyId ?? raw?.access_key_id ?? "",
    pathStyle: Boolean(raw?.pathStyle ?? raw?.path_style),
    description: raw?.description ?? "",
    createdAt: raw?.createdAt ?? raw?.created_at ?? "",
  };
}

function storageClassToForm(
  storageClass?: StorageClass,
): StorageClassFormState {
  return {
    name: storageClass?.name ?? "",
    namespace: storageClass?.namespace ?? "default",
    provider: storageClass?.provider ?? "MinIO",
    clusters: storageClass?.clusters ?? [],
    endpoint: storageClass?.endpoint ?? "",
    region: storageClass?.region ?? "",
    bucket: storageClass?.bucket ?? "",
    access_key_id: storageClass?.accessKeyId ?? "",
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
  refreshKey = 0,
}: {
  copy: Copy;
  selectedName?: string;
  onSelect: (name?: string) => void;
  onCreate: () => void;
  refreshKey?: number;
}) {
  const zh = c === copy.zh;
  const [realClasses, setRealClasses] = useState<StorageClass[]>([]);
  const [loading, setLoading] = useState(false);
  const [fetched, setFetched] = useState(false);
  const [search, setSearch] = useState("");
  const [providerFilter, setProviderFilter] = useState("All");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [sort, setSort] = useState<{
    key: "name" | "provider" | "bucket" | "clusters" | "description";
    direction: SortDirection;
  }>({ key: "name", direction: "asc" });
  const toggleSort = (key: typeof sort.key) =>
    setSort((current) => ({
      key,
      direction:
        current.key === key && current.direction === "asc" ? "desc" : "asc",
    }));
  const [editingClass, setEditingClass] = useState<StorageClass | null>(null);

  const selected = useMemo(
    () => realClasses.find((sc) => sc.name === selectedName),
    [realClasses, selectedName],
  );

  const fetchClasses = async () => {
    if (loading) return;
    setLoading(true);
    try {
      const resp = await fetch("/api/v1/storage/storageclass");
      if (resp.ok) {
        const data = await resp.json();
        const list: StorageClass[] = Object.values(data.data || {}).map(
          normalizeStorageClass,
        );
        setRealClasses(list);
      } else {
        setRealClasses([]);
      }
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
      const resp = await fetch(
        `/api/v1/storage/storageclass/${encodeURIComponent(id || name)}`,
        {
          method: "DELETE",
        },
      );
      if (resp.ok) fetchClasses();
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
      sc.bucket.toLowerCase().includes(query) ||
      sc.clusters.some((cluster) => cluster.toLowerCase().includes(query));
    return (
      matchesSearch &&
      (providerFilter === "All" || sc.provider === providerFilter)
    );
  });
  const providers = Array.from(new Set(realClasses.map((sc) => sc.provider)));
  const associatedClusters = new Set(realClasses.flatMap((sc) => sc.clusters));
  const sortedClasses = [...filtered].sort((a, b) => {
    const value = (storageClass: StorageClass) =>
      sort.key === "clusters"
        ? storageClass.clusters.length
        : storageClass[sort.key];
    return compareSortValues(
      value(a),
      value(b),
      sort.direction,
      zh ? "zh-CN" : "en",
    );
  });
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
        <div className="storage-overview-card green">
          <span>
            <Server size={18} />
          </span>
          <div>
            <small>{zh ? "关联集群" : "Linked clusters"}</small>
            <strong>{associatedClusters.size}</strong>
            <em>{zh ? "个集群已接入存储" : "clusters with storage"}</em>
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
        filterValue={providerFilter}
        onFilterChange={setProviderFilter}
        filterOptions={[
          { value: "All", label: zh ? "全部提供商" : "All providers" },
          ...providers.map((provider) => ({
            value: provider,
            label: provider,
          })),
        ]}
      />
      <section className="table-panel storage-class-table-panel">
        <div className="storage-table-heading">
          <div>
            <strong>{zh ? "存储资源" : "Storage resources"}</strong>
            <small>
              {zh
                ? "管理存储桶、集群关联和访问配置"
                : "Manage buckets, cluster links and access settings"}
            </small>
          </div>
          <span>
            {zh ? `共 ${filtered.length} 项` : `${filtered.length} items`}
          </span>
        </div>
        <table>
          <thead>
            <tr>
              <th>
                <SortButton
                  label={c.storageClass.name}
                  active={sort.key === "name"}
                  direction={sort.direction}
                  onClick={() => toggleSort("name")}
                />
              </th>
              <th>
                <SortButton
                  label={c.storageClass.provider}
                  active={sort.key === "provider"}
                  direction={sort.direction}
                  onClick={() => toggleSort("provider")}
                />
              </th>
              <th>
                <SortButton
                  label={c.storageClass.bucket}
                  active={sort.key === "bucket"}
                  direction={sort.direction}
                  onClick={() => toggleSort("bucket")}
                />
              </th>
              <th>
                <SortButton
                  label={c.storageClass.clusters}
                  active={sort.key === "clusters"}
                  direction={sort.direction}
                  onClick={() => toggleSort("clusters")}
                />
              </th>
              <th>
                <SortButton
                  label={c.storageClass.description}
                  active={sort.key === "description"}
                  direction={sort.direction}
                  onClick={() => toggleSort("description")}
                />
              </th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 && (
              <tr>
                <td colSpan={6} className="storage-empty-cell">
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
                    onClick={() => onSelect(sc.name)}
                    aria-label={`${zh ? "查看存储类" : "View storage class"} ${sc.name}`}
                  >
                    <span>
                      <HardDrive size={16} />
                    </span>
                    <span>
                      <strong>{sc.name}</strong>
                      <small>{sc.namespace}</small>
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
                  <div className="storage-cluster-list">
                    {sc.clusters.length > 0
                      ? sc.clusters.map((cluster) => (
                          <span key={cluster}>{cluster}</span>
                        ))
                      : "—"}
                  </div>
                </td>
                <td>
                  <span className="storage-description">
                    {sc.description || "—"}
                  </span>
                </td>
                <td>
                  <div className="row-actions">
                    <button
                      className="icon-button"
                      title={c.files.eyebrow}
                      onClick={(e) => {
                        e.stopPropagation();
                        const cluster = sc.clusters[0] || "";
                        if (!cluster) {
                          alert(
                            zh
                              ? "该存储类未关联集群"
                              : "This storage class has no associated cluster",
                          );
                          return;
                        }
                        const url = `/files/${encodeURIComponent(cluster)}/${encodeURIComponent(sc.name)}`;
                        window.open(url, "_blank");
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
      </section>
      <Pagination
        page={currentPage}
        pageSize={pageSize}
        total={filtered.length}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
        zh={zh}
      />
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
        <button
          type="button"
          className="secondary-button"
          aria-label={zh ? "关闭" : "Close"}
          onClick={onBack}
        >
          <ChevronLeft size={17} />
          {zh ? "返回" : "Back"}
        </button>
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
            {zh
              ? "访问端点、集群范围与连接方式"
              : "Endpoint, cluster scope and connection mode"}
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
              <span className="muted">{c.storageClass.clusters}</span>
              <strong>{storageClass.clusters.join(", ") || "—"}</strong>
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
  const [clusterOptions, setClusterOptions] = useState<ClusterOption[]>(
    mockClusters.map((cluster) => ({
      id: cluster.name,
      name: cluster.name,
      description: cluster.region,
    })),
  );
  const [form, setForm] = useState<StorageClassFormState>(() =>
    storageClassToForm(initialStorageClass),
  );

  useEffect(() => {
    setForm(storageClassToForm(initialStorageClass));
  }, [initialStorageClass]);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/v1/clusters")
      .then((r) =>
        r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)),
      )
      .then((data) => {
        if (cancelled) return;
        const options: ClusterOption[] = (data.data ?? [])
          .map((cluster: any) => ({
            id: cluster.id ?? cluster.name ?? "",
            name: cluster.name ?? cluster.id ?? "",
            description: cluster.region ?? cluster.location ?? "",
          }))
          .filter((cluster: ClusterOption) => cluster.id || cluster.name);
        if (options.length > 0) setClusterOptions(options);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !submitting) onBack();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onBack, submitting]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (form.clusters.length === 0) {
      setError(
        zh ? "请至少选择一个关联集群。" : "Select at least one linked cluster.",
      );
      return;
    }
    if (!isEdit && !form.access_key_secret.trim()) {
      setError(
        zh ? "Access Key Secret 不能为空。" : "Access Key Secret is required.",
      );
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      const resp = await fetch(
        isEdit
          ? `/api/v1/storage/storageclass/${encodeURIComponent(form.name)}`
          : "/api/v1/storage/storageclass",
        {
          method: isEdit ? "PUT" : "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(form),
        },
      );
      if (!resp.ok) {
        const msg = await resp.text();
        setError(
          (isEdit
            ? zh
              ? "更新存储类失败"
              : "Failed to update storage class"
            : c.storageClass.createFailed) +
            ": " +
            msg,
        );
        return;
      }
      onCreated?.();
      onBack();
    } catch (err) {
      setError(c.storageClass.createFailed);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="modal-backdrop storage-create-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !submitting) onBack();
      }}
    >
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
                  ? "调整对象存储配置、访问参数和关联集群。"
                  : "Update object storage settings, access parameters and linked clusters."
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
                {c.storageClass.namespace}
                <input
                  value={form.namespace}
                  onChange={(e) =>
                    setForm({ ...form, namespace: e.target.value })
                  }
                  placeholder="default"
                />
              </label>
              <label>
                {c.storageClass.provider} *
                <select
                  value={form.provider}
                  onChange={(e) =>
                    setForm({
                      ...form,
                      provider: e.target.value as StorageProvider,
                    })
                  }
                >
                  {storageProviders.map((p) => (
                    <option key={p} value={p}>
                      {p}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                {c.storageClass.clusters}
                <ClusterMultiSelect
                  zh={zh}
                  options={clusterOptions}
                  value={form.clusters}
                  onChange={(clusters) => setForm({ ...form, clusters })}
                />
              </label>
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
                  required
                  value={form.access_key_id}
                  onChange={(e) =>
                    setForm({ ...form, access_key_id: e.target.value })
                  }
                  placeholder="AKIAIOSFODNN7EXAMPLE"
                />
              </label>
              <label>
                {c.storageClass.accessKeySecret} *
                <input
                  required={!isEdit}
                  type="password"
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
          {error && <p className="error-text">{error}</p>}
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

function ClusterMultiSelect({
  zh,
  options,
  value,
  onChange,
}: {
  zh: boolean;
  options: ClusterOption[];
  value: string[];
  onChange: (next: string[]) => void;
}) {
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const selected = new Set(value);
  const toggle = (id: string) => {
    if (!id) return;
    const next = selected.has(id)
      ? value.filter((item) => item !== id)
      : [...value, id];
    onChange(next);
  };
  const selectedOptions = value
    .map((id) => options.find((option) => option.id === id) ?? { id, name: id })
    .filter((option) => option.id);
  const normalizedQuery = query.trim().toLowerCase();
  const filteredOptions = normalizedQuery
    ? options.filter(
        (option) =>
          option.name.toLowerCase().includes(normalizedQuery) ||
          option.id.toLowerCase().includes(normalizedQuery) ||
          option.description?.toLowerCase().includes(normalizedQuery),
      )
    : options;
  const allFilteredSelected =
    filteredOptions.length > 0 &&
    filteredOptions.every((option) => selected.has(option.id));
  const toggleAllFiltered = () => {
    if (filteredOptions.length === 0) return;
    if (allFilteredSelected) {
      const filteredIDs = new Set(filteredOptions.map((option) => option.id));
      onChange(value.filter((id) => !filteredIDs.has(id)));
    } else {
      onChange([
        ...value,
        ...filteredOptions
          .map((option) => option.id)
          .filter((id) => !selected.has(id)),
      ]);
    }
  };

  return (
    <div className="storage-cluster-picker">
      <button
        type="button"
        className={`storage-cluster-select ${open ? "open" : ""}`}
        onClick={() => setOpen((next) => !next)}
      >
        {selectedOptions.length > 0 ? (
          <>
            <span>
              {zh
                ? `已选择 ${selectedOptions.length} 个集群`
                : `${selectedOptions.length} clusters selected`}
            </span>
            <strong>
              {selectedOptions
                .slice(0, 2)
                .map((option) => option.name)
                .join("、")}
              {selectedOptions.length > 2
                ? ` +${selectedOptions.length - 2}`
                : ""}
            </strong>
          </>
        ) : (
          <span>
            {zh ? "请选择一个或多个集群" : "Select one or more clusters"}
          </span>
        )}
        <ChevronRight size={16} />
      </button>
      {open && (
        <div className="storage-cluster-dropdown">
          <div className="storage-cluster-picker-search">
            <Search size={14} />
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={
                zh
                  ? "搜索集群名称、ID 或地域"
                  : "Search cluster name, ID or region"
              }
            />
            <small>
              {zh
                ? `${filteredOptions.length}/${options.length} 个`
                : `${filteredOptions.length}/${options.length}`}
            </small>
          </div>
          <button
            type="button"
            className="storage-cluster-select-all"
            onClick={toggleAllFiltered}
          >
            <input type="checkbox" readOnly checked={allFilteredSelected} />
            <span>
              {allFilteredSelected
                ? zh
                  ? "取消全选当前结果"
                  : "Clear current results"
                : zh
                  ? "全选当前结果"
                  : "Select all current results"}
            </span>
            <em>{zh ? `已选 ${value.length}` : `${value.length} selected`}</em>
          </button>
          <div className="storage-cluster-picker-options">
            {filteredOptions.map((option) => (
              <label
                key={option.id}
                className={selected.has(option.id) ? "active" : ""}
              >
                <input
                  type="checkbox"
                  checked={selected.has(option.id)}
                  onChange={() => toggle(option.id)}
                />
                <span>
                  <strong>{option.name}</strong>
                </span>
              </label>
            ))}
            {filteredOptions.length === 0 && (
              <p>{zh ? "没有匹配的集群" : "No matching clusters"}</p>
            )}
          </div>
        </div>
      )}
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
  const parts = sub.split("/");
  const cluster = decodeURIComponent(parts[0] || "");
  const name = decodeURIComponent(parts[1] || "");
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
    if (!cluster || !name) {
      setError(
        zh
          ? "参数错误：缺少集群或存储类名称"
          : "Error: missing cluster or storage class name",
      );
      return;
    }
    setLoading(true);
    setError("");
    try {
      const params = new URLSearchParams();
      params.set("prefix", prefix);
      params.set("maxKeys", "100");
      const resp = await fetch(
        `/api/v1/storage/storageclass/${encodeURIComponent(name)}/${encodeURIComponent(cluster)}/list?${params}`,
      );
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
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
  }, [cluster, name, prefix]);

  const handleUpload = async (files: FileList) => {
    if (!cluster || !name) return;
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
        const resp = await fetch(
          `/api/v1/storage/storageclass/${encodeURIComponent(name)}/${encodeURIComponent(cluster)}/upload`,
          { method: "POST", body: formData },
        );
        if (!resp.ok) {
          const err = await resp.json().catch(() => ({}));
          throw new Error(err.error || `Upload failed: ${resp.status}`);
        }
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
      const resp = await fetch(
        `/api/v1/storage/storageclass/${encodeURIComponent(name)}/${encodeURIComponent(cluster)}/object/${encodeURIComponent(key)}?expire=3600`,
      );
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
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
      const resp = await fetch(
        `/api/v1/storage/storageclass/${encodeURIComponent(name)}/${encodeURIComponent(cluster)}/object/${encodeURIComponent(key)}`,
        { method: "DELETE" },
      );
      if (!resp.ok) {
        const data = await resp.json().catch(() => ({}));
        throw new Error(data.error || `Delete failed: ${resp.status}`);
      }
      fetchFiles();
    } catch (e: any) {
      alert(e?.message || (zh ? "删除失败" : "Delete failed"));
    }
  };

  const navigateFolder = (folderPrefix: string) => {
    setPrefix(folderPrefix);
  };

  const goUp = () => {
    if (!prefix) return;
    const lastSlash = prefix.lastIndexOf("/");
    setPrefix(lastSlash >= 0 ? prefix.slice(0, lastSlash + 1) : "");
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
      crumbs.push(
        <button type="button" key={path} onClick={() => setPrefix(path)}>
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
            {c.files.cluster}: {cluster || "—"} · {c.files.storageClass}:{" "}
            {name || "—"}
          </p>
          <div className="storage-detail-badges">
            <span>{cluster || "—"}</span>
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
            <Server size={17} />
          </span>
          <small>{zh ? "所属集群" : "Cluster"}</small>
          <strong>{cluster || "—"}</strong>
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
          <RefreshCw size={16} />
          {c.common.refresh}
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

      <section className="table-panel files-table storage-files-table-panel">
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
            {loading && (
              <tr>
                <td colSpan={4}>
                  <div className="storage-files-state">
                    <span className="storage-files-spinner">
                      <RefreshCw size={20} />
                    </span>
                    <strong>
                      {zh
                        ? "正在加载目录内容..."
                        : "Loading directory contents..."}
                    </strong>
                  </div>
                </td>
              </tr>
            )}
            {!loading &&
              filteredPrefixes.map((folder) => {
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
            {!loading &&
              pagedObjects.map((obj) => {
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
