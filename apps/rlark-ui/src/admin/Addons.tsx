import { useEffect, useState } from "react";
import { Package } from "lucide-react";
import type { Copy, Lang } from "../i18n";

export function AddonsPage({ copy: c, lang }: { copy: Copy; lang: Lang }) {
  const zh = lang === "zh";
  const [clusters, setClusters] = useState<{ id: string; name: string }[]>([]);
  const [catalog, setCatalog] = useState<any[]>([]);
  const [installed, setInstalled] = useState<any[]>([]);
  const [clusterFilter, setClusterFilter] = useState("");
  const [page, setPage] = useState(1);
  const pageSize = 10;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [installAddonName, setInstallAddonName] = useState("");
  const [configInstalled, setConfigInstalled] = useState<any>(null);

  useEffect(() => {
    fetch("/api/v1/clusters")
      .then((r) => r.json())
      .then((data) => setClusters(data.data || []))
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  useEffect(() => {
    fetch("/api/v1/addons")
      .then((r) => r.json())
      .then((data) => setCatalog(data.data || []))
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  const fetchInstalled = () => {
    const q = clusterFilter ? `?cluster=${clusterFilter}` : "";
    fetch(`/api/v1/installed-addons${q}`)
      .then((r) => r.json())
      .then((data) => setInstalled(data.data || []))
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  };

  useEffect(() => {
    fetchInstalled();
    const interval = setInterval(fetchInstalled, 5000);
    return () => clearInterval(interval);
  }, [clusterFilter]);

  useEffect(() => {
    setPage(1);
  }, [clusterFilter]);

  const phaseColor = (phase: string) => {
    switch (phase) {
      case "Ready":
        return "#26b985";
      case "Failed":
        return "#ef4444";
      case "Installing":
      case "Upgrading":
        return "#f59e35";
      default:
        return "#7f8998";
    }
  };

  const totalPages = Math.ceil(installed.length / pageSize);
  const pagedInstalled = installed.slice(
    (page - 1) * pageSize,
    page * pageSize,
  );

  if (installAddonName) {
    const addon = catalog.find((a) => a.name === installAddonName);
    if (addon) {
      return (
        <AddonInstallPage
          addon={addon}
          clusters={clusters}
          lang={lang}
          onBack={() => setInstallAddonName("")}
          onInstalled={() => {
            setInstallAddonName("");
            fetchInstalled();
          }}
        />
      );
    }
  }

  if (configInstalled) {
    const addon = catalog.find(
      (a) => a.name === configInstalled.spec?.addonName,
    );
    if (addon) {
      return (
        <AddonConfigPage
          addon={addon}
          installed={configInstalled}
          lang={lang}
          onBack={() => setConfigInstalled(null)}
          onSaved={() => {
            setConfigInstalled(null);
            fetchInstalled();
          }}
        />
      );
    }
  }

  return (
    <div
      className="page-content"
      style={{ display: "flex", flexDirection: "column", gap: 18 }}
    >
      <div className="section-heading">
        <div>
          <span className="eyebrow">
            <Package size={13} />
            {zh ? "组件市场" : "Addons"}
          </span>
          <h2>{zh ? "组件市场" : "Addon Catalog"}</h2>
        </div>
      </div>

      {error && <div style={{ color: "#ef4444", fontSize: 13 }}>{error}</div>}

      {/* Catalog grid */}
      <div className="addon-catalog-grid">
        {catalog.map((addon) => (
          <div key={addon.name} className="addon-card">
            <div className="addon-card-header">
              <Package size={18} />
              <div>
                <strong>{addon.displayName}</strong>
                <span className="addon-card-version">{addon.version}</span>
              </div>
            </div>
            <p className="addon-card-desc">{addon.description}</p>
            <div className="addon-card-footer">
              <span className="addon-card-category">{addon.category}</span>
              <button
                onClick={() => setInstallAddonName(addon.name)}
                style={{
                  padding: "4px 14px",
                  borderRadius: 6,
                  border: "none",
                  background: "var(--blue)",
                  color: "#fff",
                  cursor: "pointer",
                  fontSize: 12,
                  fontWeight: 500,
                }}
              >
                {zh ? "安装" : "Install"}
              </button>
            </div>
          </div>
        ))}
      </div>

      {/* Installed addons table */}
      <div>
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            marginBottom: 12,
          }}
        >
          <h3 style={{ fontSize: 15, margin: 0 }}>
            {zh ? "已安装组件" : "Installed Addons"}
            <span className="muted" style={{ fontSize: 12, marginLeft: 8 }}>
              ({installed.length})
            </span>
          </h3>
          <select
            value={clusterFilter}
            onChange={(e) => setClusterFilter(e.target.value)}
            style={{
              padding: "6px 10px",
              borderRadius: 6,
              border: "1px solid var(--line)",
              fontSize: 13,
            }}
          >
            <option value="">{zh ? "所有集群" : "All clusters"}</option>
            {clusters.map((cl) => (
              <option key={cl.id} value={cl.id}>
                {cl.name || cl.id}
              </option>
            ))}
          </select>
        </div>
        {installed.length > 0 ? (
          <>
            <table className="addon-installed-table">
              <thead>
                <tr>
                  <th>{zh ? "组件" : "Addon"}</th>
                  <th>{zh ? "集群" : "Cluster"}</th>
                  <th>{zh ? "版本" : "Version"}</th>
                  <th>{zh ? "状态" : "Phase"}</th>
                  <th>{zh ? "信息" : "Message"}</th>
                  <th>{zh ? "操作" : "Actions"}</th>
                </tr>
              </thead>
              <tbody>
                {pagedInstalled.map((a) => (
                  <tr key={`${a.clusterId}-${a.metadata?.name}`}>
                    <td>{a.spec?.addonName}</td>
                    <td className="muted">{a.clusterId}</td>
                    <td>{a.status?.version || a.spec?.version || "-"}</td>
                    <td>
                      <span style={{ color: phaseColor(a.status?.phase) }}>
                        {a.status?.phase || "Pending"}
                      </span>
                    </td>
                    <td className="muted">{a.status?.message || "-"}</td>
                    <td>
                      <div style={{ display: "flex", gap: 6 }}>
                        <button
                          onClick={() => setConfigInstalled(a)}
                          style={{
                            padding: "4px 10px",
                            borderRadius: 6,
                            border: "1px solid var(--line)",
                            background: "transparent",
                            cursor: "pointer",
                            fontSize: 12,
                            color: "var(--blue)",
                          }}
                        >
                          {zh ? "配置" : "Config"}
                        </button>
                        <button
                          onClick={() => {
                            const label = a.spec?.addonName || a.metadata?.name;
                            if (
                              !window.confirm(
                                zh
                                  ? `确定从集群 ${a.clusterId} 卸载组件“${label}”吗？`
                                  : `Uninstall “${label}” from ${a.clusterId}?`,
                              )
                            )
                              return;
                            fetch(
                              `/api/v1/clusters/${a.clusterId}/addons/${a.metadata?.name}`,
                              {
                                method: "DELETE",
                              },
                            )
                              .then((r) => {
                                if (!r.ok) throw new Error("Uninstall failed");
                                return r.json();
                              })
                              .then(() => fetchInstalled())
                              .catch((e) => setError(e.message));
                          }}
                          style={{
                            padding: "4px 10px",
                            borderRadius: 6,
                            border: "1px solid var(--line)",
                            background: "transparent",
                            cursor: "pointer",
                            fontSize: 12,
                            color: "#ef4444",
                          }}
                        >
                          {zh ? "卸载" : "Uninstall"}
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {totalPages > 1 && (
              <div
                style={{
                  display: "flex",
                  justifyContent: "flex-end",
                  alignItems: "center",
                  gap: 8,
                  marginTop: 12,
                }}
              >
                <button
                  onClick={() => setPage(page - 1)}
                  disabled={page <= 1}
                  style={{
                    padding: "4px 12px",
                    borderRadius: 6,
                    border: "1px solid var(--line)",
                    background: page <= 1 ? "#f5f5f5" : "#fff",
                    cursor: page <= 1 ? "not-allowed" : "pointer",
                    fontSize: 12,
                  }}
                >
                  {zh ? "上一页" : "Prev"}
                </button>
                <span style={{ fontSize: 12, color: "var(--text-muted)" }}>
                  {page} / {totalPages}
                </span>
                <button
                  onClick={() => setPage(page + 1)}
                  disabled={page >= totalPages}
                  style={{
                    padding: "4px 12px",
                    borderRadius: 6,
                    border: "1px solid var(--line)",
                    background: page >= totalPages ? "#f5f5f5" : "#fff",
                    cursor: page >= totalPages ? "not-allowed" : "pointer",
                    fontSize: 12,
                  }}
                >
                  {zh ? "下一页" : "Next"}
                </button>
              </div>
            )}
          </>
        ) : (
          <p className="muted" style={{ fontSize: 13 }}>
            {zh ? "暂无已安装组件" : "No installed addons"}
          </p>
        )}
      </div>
    </div>
  );
}

export function AddonInstallPage({
  addon,
  clusters,
  lang,
  onBack,
  onInstalled,
}: {
  addon: any;
  clusters: { id: string; name: string }[];
  lang: Lang;
  onBack: () => void;
  onInstalled: () => void;
}) {
  const zh = lang === "zh";
  const [installCluster, setInstallCluster] = useState("");
  const [values, setValues] = useState<Record<string, string>>(() => {
    const defaults: Record<string, string> = {};
    (addon.parameters || []).forEach((p: any) => {
      defaults[p.name] = p.default || "";
    });
    return defaults;
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const handleInstall = () => {
    if (!installCluster) return;
    setLoading(true);
    setError("");
    fetch(`/api/v1/clusters/${installCluster}/addons`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        addonName: addon.name,
        version: addon.version,
        values,
      }),
    })
      .then((r) => {
        if (!r.ok) throw new Error("Install failed");
        return r.json();
      })
      .then(() => {
        setLoading(false);
        onInstalled();
      })
      .catch((e) => {
        setError(e.message);
        setLoading(false);
      });
  };

  return (
    <div
      className="page-content"
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
        ← {zh ? "返回组件市场" : "Back to catalog"}
      </button>

      <div className="section-heading">
        <div>
          <span className="eyebrow">
            <Package size={13} />
            {zh ? "安装组件" : "Install Addon"}
          </span>
          <h2>{addon.displayName}</h2>
          <p>{addon.description}</p>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <span className="addon-card-version">{addon.version}</span>
          <span className="addon-card-category">{addon.category}</span>
        </div>
      </div>

      {error && <div style={{ color: "#ef4444", fontSize: 13 }}>{error}</div>}

      <div className="addon-install-form">
        <div className="form-section">
          <div className="form-section-head">
            <strong>{zh ? "目标集群" : "Target Cluster"}</strong>
            <span style={{ color: "#ef4444" }}>*</span>
          </div>
          <select
            value={installCluster}
            onChange={(e) => setInstallCluster(e.target.value)}
            style={{
              width: "100%",
              padding: "10px 12px",
              borderRadius: 8,
              border: "1px solid var(--line)",
              fontSize: 14,
              boxSizing: "border-box",
            }}
          >
            <option value="">{zh ? "选择集群" : "Select cluster"}</option>
            {clusters.map((cl) => (
              <option key={cl.id} value={cl.id}>
                {cl.name || cl.id}
              </option>
            ))}
          </select>
        </div>

        {(addon.parameters || []).length > 0 && (
          <div className="form-section">
            <div className="form-section-head">
              <strong>{zh ? "参数配置" : "Parameters"}</strong>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
              {addon.parameters.map((p: any) => (
                <div
                  key={p.name}
                  style={{ display: "flex", flexDirection: "column", gap: 6 }}
                >
                  <label
                    style={{
                      fontSize: 13,
                      fontWeight: 600,
                      color: "var(--text)",
                    }}
                  >
                    {p.displayName}
                    {p.required && <span style={{ color: "#ef4444" }}> *</span>}
                  </label>
                  {p.description && (
                    <span
                      className="muted"
                      style={{ fontSize: 12, whiteSpace: "pre-wrap" }}
                    >
                      {p.description}
                    </span>
                  )}
                  {p.type === "enum" ? (
                    <select
                      value={values[p.name] || ""}
                      onChange={(e) =>
                        setValues({ ...values, [p.name]: e.target.value })
                      }
                      style={{
                        padding: "10px 12px",
                        borderRadius: 8,
                        border: "1px solid var(--line)",
                        fontSize: 14,
                        boxSizing: "border-box",
                        width: "100%",
                      }}
                    >
                      {(p.options || []).map((opt: string) => (
                        <option key={opt} value={opt}>
                          {opt}
                        </option>
                      ))}
                    </select>
                  ) : p.type === "text" ? (
                    <textarea
                      value={values[p.name] || ""}
                      onChange={(e) =>
                        setValues({ ...values, [p.name]: e.target.value })
                      }
                      rows={8}
                      placeholder={p.description || ""}
                      className="addon-textarea"
                    />
                  ) : (
                    <input
                      type={
                        p.type === "int"
                          ? "number"
                          : p.type === "bool"
                            ? "checkbox"
                            : "text"
                      }
                      value={
                        p.type === "bool" ? undefined : values[p.name] || ""
                      }
                      checked={
                        p.type === "bool"
                          ? values[p.name] === "true"
                          : undefined
                      }
                      onChange={(e) =>
                        setValues({
                          ...values,
                          [p.name]:
                            p.type === "bool"
                              ? e.target.checked
                                ? "true"
                                : "false"
                              : e.target.value,
                        })
                      }
                      style={{
                        padding: p.type === "bool" ? undefined : "10px 12px",
                        borderRadius: 8,
                        border: "1px solid var(--line)",
                        fontSize: 14,
                        boxSizing: "border-box",
                        width: p.type === "bool" ? undefined : "100%",
                      }}
                    />
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        <div
          style={{
            display: "flex",
            justifyContent: "flex-end",
            gap: 8,
            marginTop: 8,
          }}
        >
          <button
            onClick={onBack}
            style={{
              padding: "10px 20px",
              borderRadius: 10,
              border: "1px solid var(--line)",
              background: "#fff",
              cursor: "pointer",
              fontSize: 13,
              fontWeight: 600,
              color: "var(--text)",
            }}
          >
            {zh ? "取消" : "Cancel"}
          </button>
          <button
            onClick={handleInstall}
            disabled={loading || !installCluster}
            style={{
              padding: "10px 24px",
              borderRadius: 10,
              border: "none",
              background: loading || !installCluster ? "#ccc" : "var(--blue)",
              color: "#fff",
              cursor: loading || !installCluster ? "not-allowed" : "pointer",
              fontSize: 13,
              fontWeight: 600,
            }}
          >
            {loading
              ? zh
                ? "安装中..."
                : "Installing..."
              : zh
                ? "安装"
                : "Install"}
          </button>
        </div>
      </div>
    </div>
  );
}

export function AddonConfigPage({
  addon,
  installed,
  lang,
  onBack,
  onSaved,
}: {
  addon: any;
  installed: any;
  lang: Lang;
  onBack: () => void;
  onSaved: () => void;
}) {
  const zh = lang === "zh";
  const clusterId = installed.clusterId;
  const addonName = installed.metadata?.name;
  const [values, setValues] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {};
    (addon.parameters || []).forEach((p: any) => {
      init[p.name] = installed.spec?.values?.[p.name] ?? p.default ?? "";
    });
    return init;
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const handleSave = () => {
    setLoading(true);
    setError("");
    fetch(`/api/v1/clusters/${clusterId}/addons/${addonName}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        addonName: addon.name,
        version: addon.version,
        values,
      }),
    })
      .then((r) => {
        if (!r.ok) throw new Error("Update failed");
        return r.json();
      })
      .then(() => {
        setLoading(false);
        onSaved();
      })
      .catch((e) => {
        setError(e.message);
        setLoading(false);
      });
  };

  return (
    <div
      className="page-content"
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
        ← {zh ? "返回组件市场" : "Back to catalog"}
      </button>

      <div className="section-heading">
        <div>
          <span className="eyebrow">
            <Package size={13} />
            {zh ? "配置组件" : "Configure Addon"}
          </span>
          <h2>{addon.displayName}</h2>
          <p>{addon.description}</p>
        </div>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <span className="addon-card-version">{addon.version}</span>
          <span className="addon-card-category">{addon.category}</span>
          <span style={{ fontSize: 12, color: "var(--text-muted)" }}>
            {zh ? "集群" : "Cluster"}: <strong>{clusterId}</strong>
          </span>
        </div>
      </div>

      {error && <div style={{ color: "#ef4444", fontSize: 13 }}>{error}</div>}

      <div className="addon-install-form">
        {(addon.parameters || []).length > 0 ? (
          <div className="form-section">
            <div className="form-section-head">
              <strong>{zh ? "参数配置" : "Parameters"}</strong>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
              {addon.parameters.map((p: any) => (
                <div
                  key={p.name}
                  style={{ display: "flex", flexDirection: "column", gap: 6 }}
                >
                  <label
                    style={{
                      fontSize: 13,
                      fontWeight: 600,
                      color: "var(--text)",
                    }}
                  >
                    {p.displayName}
                    {p.required && <span style={{ color: "#ef4444" }}> *</span>}
                  </label>
                  {p.description && (
                    <span
                      className="muted"
                      style={{ fontSize: 12, whiteSpace: "pre-wrap" }}
                    >
                      {p.description}
                    </span>
                  )}
                  {p.type === "enum" ? (
                    <select
                      value={values[p.name] || ""}
                      onChange={(e) =>
                        setValues({ ...values, [p.name]: e.target.value })
                      }
                      style={{
                        padding: "10px 12px",
                        borderRadius: 8,
                        border: "1px solid var(--line)",
                        fontSize: 14,
                        boxSizing: "border-box",
                        width: "100%",
                      }}
                    >
                      {(p.options || []).map((opt: string) => (
                        <option key={opt} value={opt}>
                          {opt}
                        </option>
                      ))}
                    </select>
                  ) : p.type === "text" ? (
                    <textarea
                      value={values[p.name] || ""}
                      onChange={(e) =>
                        setValues({ ...values, [p.name]: e.target.value })
                      }
                      rows={8}
                      placeholder={p.description || ""}
                      className="addon-textarea"
                    />
                  ) : (
                    <input
                      type={
                        p.type === "int"
                          ? "number"
                          : p.type === "bool"
                            ? "checkbox"
                            : "text"
                      }
                      value={
                        p.type === "bool" ? undefined : values[p.name] || ""
                      }
                      checked={
                        p.type === "bool"
                          ? values[p.name] === "true"
                          : undefined
                      }
                      onChange={(e) =>
                        setValues({
                          ...values,
                          [p.name]:
                            p.type === "bool"
                              ? e.target.checked
                                ? "true"
                                : "false"
                              : e.target.value,
                        })
                      }
                      style={{
                        padding: p.type === "bool" ? undefined : "10px 12px",
                        borderRadius: 8,
                        border: "1px solid var(--line)",
                        fontSize: 14,
                        boxSizing: "border-box",
                        width: p.type === "bool" ? undefined : "100%",
                      }}
                    />
                  )}
                </div>
              ))}
            </div>
          </div>
        ) : (
          <p className="muted" style={{ fontSize: 13 }}>
            {zh
              ? "该组件无可配置参数"
              : "This addon has no configurable parameters"}
          </p>
        )}

        <div
          style={{
            display: "flex",
            justifyContent: "flex-end",
            gap: 8,
            marginTop: 8,
          }}
        >
          <button
            onClick={onBack}
            style={{
              padding: "10px 20px",
              borderRadius: 10,
              border: "1px solid var(--line)",
              background: "#fff",
              cursor: "pointer",
              fontSize: 13,
              fontWeight: 600,
              color: "var(--text)",
            }}
          >
            {zh ? "取消" : "Cancel"}
          </button>
          <button
            onClick={handleSave}
            disabled={loading}
            style={{
              padding: "10px 24px",
              borderRadius: 10,
              border: "none",
              background: loading ? "#ccc" : "var(--blue)",
              color: "#fff",
              cursor: loading ? "not-allowed" : "pointer",
              fontSize: 13,
              fontWeight: 600,
            }}
          >
            {loading ? (zh ? "保存中..." : "Saving...") : zh ? "保存" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}
