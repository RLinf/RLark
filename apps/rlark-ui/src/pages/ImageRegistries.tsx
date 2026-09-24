import { useEffect, useState, type FormEvent } from "react";
import {
  ChevronLeft,
  Database,
  Image as ImageIcon,
  Lock,
  Pencil,
  Plus,
  Trash2,
  X,
} from "lucide-react";
import type { Copy } from "../i18n";
import { PageToolbar, RefreshOverlay } from "../components/shared";

type ClusterSelectionMode = "None" | "Selected" | "All";
type ClusterSelection = { mode: ClusterSelectionMode; clusters: string[] };
type ClusterOption = { id: string; name: string };

function useClusterOptions() {
  const [clusters, setClusters] = useState<ClusterOption[]>([]);
  useEffect(() => {
    clustersApi
      .list<{ id?: string; name?: string }>()
      .then((items) =>
        setClusters(
          items.map((cluster: { id?: string; name?: string }) => ({
            id: (cluster.name || cluster.id || "").replace(/^rlark-/, ""),
            name: (cluster.name || cluster.id || "").replace(/^rlark-/, ""),
          })),
        ),
      )
      .catch(() => setClusters([]));
  }, []);
  return clusters;
}

function selectionLabel(selection: ClusterSelection, zh: boolean) {
  if (selection.mode === "None") return zh ? "仅保存" : "Stored only";
  if (selection.mode === "All") return zh ? "所有集群" : "All clusters";
  return zh
    ? `${selection.clusters.length} 个集群`
    : `${selection.clusters.length} clusters`;
}

function ClusterSelectionFields({
  zh,
  selection,
  clusters,
  onChange,
}: {
  zh: boolean;
  selection: ClusterSelection;
  clusters: ClusterOption[];
  onChange: (selection: ClusterSelection) => void;
}) {
  const options = Array.from(
    new Set([...clusters.map((cluster) => cluster.id), ...selection.clusters]),
  ).sort();
  return (
    <div className="form-section">
      <strong>{zh ? "分发范围" : "Distribution Scope"}</strong>
      <div className="form-grid">
        <label>
          {zh ? "模式" : "Mode"}
          <select
            value={selection.mode}
            onChange={(event) =>
              onChange({
                mode: event.target.value as ClusterSelectionMode,
                clusters:
                  event.target.value === "Selected" ? selection.clusters : [],
              })
            }
          >
            <option value="None">{zh ? "仅保存" : "Stored only"}</option>
            <option value="Selected">
              {zh ? "指定集群" : "Selected clusters"}
            </option>
            <option value="All">{zh ? "所有集群" : "All clusters"}</option>
          </select>
        </label>
      </div>
      {selection.mode === "Selected" && (
        <div className="registry-cluster-options">
          {options.length === 0 ? (
            <p>{zh ? "暂无可选集群" : "No clusters available"}</p>
          ) : (
            options.map((id) => {
              const option = clusters.find((cluster) => cluster.id === id);
              return (
                <label
                  key={id}
                  className={selection.clusters.includes(id) ? "active" : ""}
                >
                  <input
                    type="checkbox"
                    checked={selection.clusters.includes(id)}
                    onChange={() =>
                      onChange({
                        mode: "Selected",
                        clusters: selection.clusters.includes(id)
                          ? selection.clusters.filter(
                              (cluster) => cluster !== id,
                            )
                          : [...selection.clusters, id].sort(),
                      })
                    }
                  />
                  <span>
                    <strong>{option?.name || id}</strong>
                    {option?.name && option.name !== id && <small>{id}</small>}
                  </span>
                </label>
              );
            })
          )}
        </div>
      )}
    </div>
  );
}

export function ImageRegistriesPage({
  copy: c,
  selectedID,
  onSelect,
  onCreate,
}: {
  copy: Copy;
  selectedID?: string;
  onSelect?: (id?: string) => void;
  onCreate?: () => void;
}) {
  const zh = c.nav.overview === "总览";
  const [items, setItems] = useState<ImageRegistryItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [editingItem, setEditingItem] = useState<ImageRegistryItem | null>(
    null,
  );

  const fetchItems = async () => {
    setLoading(true);
    setError("");
    try {
      setItems(await imageRegistriesApi.list());
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchItems();
  }, []);

  const handleDelete = async (item: ImageRegistryItem) => {
    if (
      !confirm(
        zh
          ? `确认删除“${item.name}”（${item.registry}）？分发副本将异步清理。`
          : `Delete “${item.name}” (${item.registry})? Distributed copies will be removed asynchronously.`,
      )
    )
      return;
    try {
      await imageRegistriesApi.remove(item.id);
      fetchItems();
    } catch (e) {
      setError(String(e));
    }
  };

  const providers = Array.from(
    new Set(items.map((i) => i.registry).filter(Boolean)),
  );
  const filteredItems = items.filter((item) =>
    `${item.name} ${item.registry} ${item.username}`
      .toLowerCase()
      .includes(query.trim().toLowerCase()),
  );

  if (selectedID) {
    const item = items.find((i) => i.id === selectedID);
    if (item) {
      return (
        <>
          <ImageRegistryDetailPage
            copy={c}
            item={item}
            onBack={() => onSelect?.(undefined)}
            onEdit={() => setEditingItem(item)}
          />
          {editingItem && (
            <ImageRegistryEditModal
              copy={c}
              item={editingItem}
              onBack={() => setEditingItem(null)}
              onSaved={() => {
                setEditingItem(null);
                fetchItems();
              }}
            />
          )}
        </>
      );
    }
  }

  return (
    <div className="page-content resource-page storage-class-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">
            <ImageIcon size={13} />
            {zh ? "镜像管理" : "Image Registries"}
          </span>
          <h2>{zh ? "镜像仓库凭据" : "Image Registry Credentials"}</h2>
          <p>
            {zh
              ? "管理私有镜像仓库的访问凭据。创建任务时自动匹配镜像前缀并注入 imagePullSecrets。"
              : "Manage credentials for private image registries. Automatically matched by image prefix and injected as imagePullSecrets."}
          </p>
        </div>
        <div className="section-actions">
          <button
            className="primary-button"
            onClick={() => setCreateOpen(true)}
          >
            <Plus size={17} />
            {zh ? "添加凭据" : "Add Registry"}
          </button>
        </div>
      </div>

      <section
        className="storage-overview-grid"
        aria-label={zh ? "镜像仓库概况" : "Registry overview"}
      >
        <div className="storage-overview-card purple">
          <span>
            <Lock size={18} />
          </span>
          <div>
            <small>{zh ? "凭据数量" : "Credentials"}</small>
            <strong>{items.length}</strong>
            <em>{zh ? "个镜像仓库凭据" : "registry credentials"}</em>
          </div>
        </div>
        <div className="storage-overview-card green">
          <span>
            <Database size={18} />
          </span>
          <div>
            <small>{zh ? "仓库地址" : "Registries"}</small>
            <strong>{providers.length}</strong>
            <em>{providers.join(" · ") || "—"}</em>
          </div>
        </div>
        <div className="storage-overview-card orange">
          <span>
            <ImageIcon size={18} />
          </span>
          <div>
            <small>{zh ? "自动注入" : "Auto-injection"}</small>
            <strong>{zh ? "已启用" : "Enabled"}</strong>
            <em>{zh ? "匹配镜像前缀自动填充" : "Matched by image prefix"}</em>
          </div>
        </div>
      </section>

      {error && (
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}

      <PageToolbar
        placeholder={
          zh
            ? "搜索名称、仓库或用户名..."
            : "Search name, registry or username..."
        }
        value={query}
        onChange={setQuery}
        count={filteredItems.length}
        copy={c}
        onRefresh={fetchItems}
        refreshing={loading}
      />

      <section
        className={`table-panel refreshable-region${loading ? " is-refreshing" : ""}`}
        aria-busy={loading}
      >
        <div className="storage-table-heading">
          <div>
            <strong>{zh ? "凭据列表" : "Registries"}</strong>
            <small>
              {zh
                ? "管理私有镜像仓库访问凭据"
                : "Manage private registry credentials"}
            </small>
          </div>
          <span>
            {zh
              ? `共 ${filteredItems.length} 项`
              : `${filteredItems.length} items`}
          </span>
        </div>
        {filteredItems.length === 0 ? (
          <p className="muted" style={{ padding: "20px" }}>
            {loading
              ? zh
                ? "加载中…"
                : "Loading…"
              : zh
                ? "暂无镜像仓库凭据"
                : "No image registries"}
          </p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>{zh ? "名称" : "Name"}</th>
                <th>{zh ? "仓库地址" : "Registry"}</th>
                <th>{zh ? "用户名" : "Username"}</th>
                <th>{zh ? "分发范围" : "Scope"}</th>
                <th className="table-actions-col">{zh ? "操作" : "Actions"}</th>
              </tr>
            </thead>
            <tbody>
              {filteredItems.map((item) => (
                <tr
                  key={item.id}
                  style={{ cursor: "pointer" }}
                  onClick={() => onSelect?.(item.id)}
                >
                  <td style={{ fontWeight: 500 }}>
                    <div
                      style={{ display: "flex", alignItems: "center", gap: 8 }}
                    >
                      <Lock size={14} style={{ color: "var(--muted)" }} />
                      {item.name}
                    </div>
                  </td>
                  <td>
                    <code
                      style={{
                        fontFamily: "var(--font-mono, monospace)",
                        fontSize: 12,
                      }}
                    >
                      {item.registry}
                    </code>
                  </td>
                  <td className="muted">{item.username}</td>
                  <td>{selectionLabel(item.clusterSelection, zh)}</td>
                  <td
                    className="table-actions-col"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <div className="row-actions">
                      <button
                        type="button"
                        className="icon-button danger"
                        title={zh ? "删除" : "Delete"}
                        aria-label={zh ? "删除" : "Delete"}
                        onClick={() => handleDelete(item)}
                      >
                        <Trash2 size={15} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <RefreshOverlay
          visible={loading}
          label={zh ? "正在刷新镜像仓库列表" : "Refreshing registry list"}
        />
      </section>
      {createOpen && (
        <ImageRegistryCreatePage
          copy={c}
          onBack={() => setCreateOpen(false)}
          onCreated={() => {
            void fetchItems();
            onCreate?.();
          }}
        />
      )}
    </div>
  );
}

function ImageRegistryDetailPage({
  copy: c,
  item,
  onBack,
  onEdit,
}: {
  copy: Copy;
  item: ImageRegistryItem;
  onBack: () => void;
  onEdit: () => void;
}) {
  const zh = c.nav.overview === "总览";
  return (
    <div className="page-content resource-page storage-detail-page">
      <div className="section-heading storage-detail-hero">
        <div>
          <span className="eyebrow">
            <ImageIcon size={13} />
            {zh ? "镜像管理" : "Image Registries"}
          </span>
          <h2>{item.name}</h2>
          <p>
            {item.registry} · {item.username}
          </p>
          <div className="storage-detail-badges">
            <span>{zh ? "dockerconfigjson" : "dockerconfigjson"}</span>
            <span>{selectionLabel(item.clusterSelection, zh)}</span>
          </div>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <button className="secondary-button" onClick={onEdit}>
            <Pencil size={15} />
            {zh ? "编辑" : "Edit"}
          </button>
          <button className="secondary-button" onClick={onBack}>
            <ChevronLeft size={17} />
            {zh ? "返回" : "Back"}
          </button>
        </div>
      </div>
      <div className="node-detail-body storage-detail-layout">
        <section className="node-detail-section storage-detail-panel">
          <span className="node-detail-label">
            <Database size={15} />
            {zh ? "凭据信息" : "Registry Info"}
          </span>
          <p className="storage-detail-section-copy">
            {zh
              ? "镜像仓库的访问地址与认证信息"
              : "Registry access address and credentials"}
          </p>
          <div className="node-detail-grid storage-detail-info-grid">
            <div>
              <span className="muted">{zh ? "名称" : "Name"}</span>
              <strong>{item.name}</strong>
            </div>
            <div>
              <span className="muted">{zh ? "分发范围" : "Distribution"}</span>
              <strong>{selectionLabel(item.clusterSelection, zh)}</strong>
            </div>
            <div>
              <span className="muted">
                {zh ? "目标命名空间" : "Target Namespace"}
              </span>
              <strong>rlark-system</strong>
            </div>
            <div>
              <span className="muted">{zh ? "仓库地址" : "Registry"}</span>
              <strong>{item.registry}</strong>
            </div>
            <div>
              <span className="muted">{zh ? "用户名" : "Username"}</span>
              <strong>{item.username}</strong>
            </div>
            <div>
              <span className="muted">{zh ? "密码" : "Password"}</span>
              <strong>••••••••</strong>
            </div>
            <div>
              <span className="muted">
                {zh ? "Secret 类型" : "Secret Type"}
              </span>
              <strong>kubernetes.io/dockerconfigjson</strong>
            </div>
            <div>
              <span className="muted">
                {zh ? "自动注入" : "Auto-injection"}
              </span>
              <strong>
                {zh
                  ? "匹配镜像前缀自动填充 imagePullSecrets"
                  : "Matched by image prefix"}
              </strong>
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}

export function ImageRegistryCreatePage({
  copy: c,
  onBack,
  onCreated,
}: {
  copy: Copy;
  onBack: () => void;
  onCreated?: () => void;
}) {
  const zh = c.nav.overview === "总览";
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const clusters = useClusterOptions();
  const [form, setForm] = useState({
    name: "",
    registry: "",
    username: "",
    password: "",
    clusterSelection: { mode: "All", clusters: [] } as ClusterSelection,
  });

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !submitting) onBack();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onBack, submitting]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      await imageRegistriesApi.create({
        name: form.name.trim(),
        registry: form.registry.trim(),
        username: form.username.trim(),
        password: form.password,
        clusterSelection: form.clusterSelection,
      });
      onCreated?.();
      onBack();
    } catch (err) {
      setError(String(err));
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
              <ImageIcon size={13} />
              {zh ? "镜像管理" : "Image Registries"}
            </span>
            <h2>{zh ? "添加镜像仓库凭据" : "Add Image Registry"}</h2>
            <p>
              {zh
                ? "填写镜像仓库地址和登录凭据，创建任务时自动匹配并注入。"
                : "Enter the registry address and login credentials. Automatically matched and injected on job creation."}
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
            <strong>{zh ? "凭据信息" : "Registry Info"}</strong>
            <div className="form-grid">
              <label>
                {zh ? "名称" : "Name"} *
                <input
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="my-registry"
                  required
                />
              </label>
              <label>
                {zh ? "仓库地址" : "Registry"} *
                <input
                  value={form.registry}
                  onChange={(e) =>
                    setForm({ ...form, registry: e.target.value })
                  }
                  placeholder="registry.example.com"
                  required
                />
              </label>
            </div>
            <div className="form-grid" style={{ marginTop: 16 }}>
              <label>
                {zh ? "用户名" : "Username"} *
                <input
                  value={form.username}
                  onChange={(e) =>
                    setForm({ ...form, username: e.target.value })
                  }
                  placeholder="username"
                  required
                />
              </label>
              <label>
                {zh ? "密码" : "Password"} *
                <input
                  type="password"
                  value={form.password}
                  onChange={(e) =>
                    setForm({ ...form, password: e.target.value })
                  }
                  placeholder="••••••••"
                  required
                />
              </label>
            </div>
          </div>
          <ClusterSelectionFields
            zh={zh}
            selection={form.clusterSelection}
            clusters={clusters}
            onChange={(clusterSelection) =>
              setForm({ ...form, clusterSelection })
            }
          />
          {error && (
            <div className="cert-error" style={{ marginBottom: 12 }}>
              {error}
            </div>
          )}
          <div className="form-actions">
            <button
              type="button"
              className="secondary-button"
              onClick={onBack}
              disabled={submitting}
            >
              {zh ? "取消" : "Cancel"}
            </button>
            <button
              type="submit"
              className="primary-button"
              disabled={
                submitting ||
                !form.name.trim() ||
                !form.registry.trim() ||
                !form.username.trim() ||
                !form.password.trim() ||
                (form.clusterSelection.mode === "Selected" &&
                  form.clusterSelection.clusters.length === 0)
              }
            >
              {submitting
                ? zh
                  ? "提交中…"
                  : "Submitting…"
                : zh
                  ? "确认添加"
                  : "Add Registry"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

function ImageRegistryEditModal({
  copy: c,
  item,
  onBack,
  onSaved,
}: {
  copy: Copy;
  item: ImageRegistryItem;
  onBack: () => void;
  onSaved: () => void;
}) {
  const zh = c.nav.overview === "总览";
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const clusters = useClusterOptions();
  const [form, setForm] = useState({
    name: item.name,
    registry: item.registry,
    username: item.username,
    password: "",
    clusterSelection: item.clusterSelection,
  });

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !submitting) onBack();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onBack, submitting]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      await imageRegistriesApi.update(item.id, {
        name: form.name.trim(),
        registry: form.registry.trim(),
        username: form.username.trim(),
        ...(form.password ? { password: form.password } : {}),
        clusterSelection: form.clusterSelection,
      });
      onSaved();
    } catch (err) {
      setError(String(err));
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
              <Pencil size={13} />
              {zh ? "镜像管理" : "Image Registries"}
            </span>
            <h2>{zh ? "编辑镜像仓库凭据" : "Edit Image Registry"}</h2>
            <p>{item.name}</p>
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
            <strong>{zh ? "凭据信息" : "Registry Info"}</strong>
            <div className="form-grid">
              <label>
                {zh ? "名称" : "Name"}
                <input
                  value={form.name}
                  onChange={(event) =>
                    setForm({ ...form, name: event.target.value })
                  }
                  required
                />
              </label>
              <label>
                {zh ? "仓库地址" : "Registry"} *
                <input
                  value={form.registry}
                  onChange={(e) =>
                    setForm({ ...form, registry: e.target.value })
                  }
                  placeholder="registry.example.com"
                  required
                />
              </label>
            </div>
            <div className="form-grid" style={{ marginTop: 16 }}>
              <label>
                {zh ? "用户名" : "Username"} *
                <input
                  value={form.username}
                  onChange={(e) =>
                    setForm({ ...form, username: e.target.value })
                  }
                  placeholder="username"
                  required
                />
              </label>
              <label>
                {zh ? "新密码" : "New Password"}
                <input
                  type="password"
                  value={form.password}
                  onChange={(e) =>
                    setForm({ ...form, password: e.target.value })
                  }
                  placeholder={zh ? "留空则不修改" : "Leave empty to keep"}
                />
              </label>
            </div>
          </div>
          <ClusterSelectionFields
            zh={zh}
            selection={form.clusterSelection}
            clusters={clusters}
            onChange={(clusterSelection) =>
              setForm({ ...form, clusterSelection })
            }
          />
          {error && (
            <div className="cert-error" style={{ marginBottom: 12 }}>
              {error}
            </div>
          )}
          <div className="form-actions">
            <button
              type="button"
              className="secondary-button"
              onClick={onBack}
              disabled={submitting}
            >
              {zh ? "取消" : "Cancel"}
            </button>
            <button
              type="submit"
              className="primary-button"
              disabled={
                submitting ||
                !form.name.trim() ||
                !form.registry.trim() ||
                !form.username.trim() ||
                (form.clusterSelection.mode === "Selected" &&
                  form.clusterSelection.clusters.length === 0)
              }
            >
              {submitting
                ? zh
                  ? "保存中…"
                  : "Saving…"
                : zh
                  ? "保存修改"
                  : "Save Changes"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
import {
  clustersApi,
  imageRegistriesApi,
  type ImageRegistryItem,
} from "../backend";
