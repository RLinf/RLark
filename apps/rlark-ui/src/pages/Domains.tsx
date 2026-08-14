import { useEffect, useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Plus, Trash2 } from "lucide-react";
import type { Copy } from "../i18n";
import type { CRDDomain } from "../types";
import { useAutoRefresh } from "../hooks";
import { PageToolbar } from "../components/shared";

export function DomainsPage({
  copy: c,
  selectedName,
  onSelect,
}: {
  copy: Copy;
  selectedName: string;
  onSelect: (name?: string) => void;
}) {
  const zh = c.nav.overview === "总览";
  const [domains, setDomains] = useState<CRDDomain[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [newCidr, setNewCidr] = useState("10.244.0.0/16");
  const [creating, setCreating] = useState(false);

  const fetchDomains = async (isInitial = true) => {
    if (isInitial) setLoading(true);
    setError("");
    try {
      const resp = await fetch("/api/v1/rlinf.io/v1alpha1/domains");
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      setDomains(data.items ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useAutoRefresh(fetchDomains, 10000);

  const handleCreate = async () => {
    setCreating(true);
    setError("");
    try {
      const resp = await fetch("/api/v1/rlinf.io/v1alpha1/domains", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          apiVersion: "rlinf.io/v1alpha1",
          kind: "Domain",
          metadata: { name: newName.trim() },
          spec: { cidr: newCidr.trim() },
        }),
      });
      if (!resp.ok)
        throw new Error(`HTTP ${resp.status}: ${await resp.text()}`);
      setShowCreate(false);
      setNewName("");
      setNewCidr("10.244.0.0/16");
      fetchDomains();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (name: string) => {
    if (!confirm(zh ? `确定删除域 "${name}" 吗?` : `Delete domain "${name}"?`))
      return;
    try {
      const resp = await fetch(`/api/v1/rlinf.io/v1alpha1/domains/${name}`, {
        method: "DELETE",
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      setDomains((prev) => prev.filter((d) => d.metadata.name !== name));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const selected =
    selectedName && domains.length > 0
      ? (domains.find((d) => d.metadata.name === selectedName) ?? null)
      : null;

  if (selected) {
    return (
      <DomainDetailPage
        domain={selected}
        copy={c}
        onBack={() => onSelect(undefined)}
      />
    );
  }

  return (
    <div className="page-content resource-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">
            {zh ? "跨集群网络" : "Cross-cluster Network"}
          </span>
          <h2>{c.nav.domains}</h2>
          <p>
            {zh
              ? "管理跨集群网络域，为 Pod 分配跨集群可达的 IP 地址。"
              : "Manage cross-cluster network domains for pod IP allocation."}
          </p>
        </div>
        <button className="primary-button" onClick={() => setShowCreate(true)}>
          <Plus size={17} />
          {zh ? "创建域" : "Create Domain"}
        </button>
      </div>
      <PageToolbar
        placeholder={zh ? "搜索域..." : "Search domains..."}
        value=""
        onChange={() => {}}
        count={domains.length}
        copy={c}
        onRefresh={() => fetchDomains()}
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
              <th>{zh ? "域名" : "Name"}</th>
              <th>CIDR</th>
              <th>{zh ? "IP 分配" : "IP Allocations"}</th>
              <th>{zh ? "创建时间" : "Created"}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {domains.map((d) => (
              <tr
                key={d.metadata.name}
                className="clickable-row"
                onClick={() => onSelect(d.metadata.name)}
              >
                <td>
                  <strong>{d.metadata.name}</strong>
                </td>
                <td>
                  <code className="inline-code">{d.spec.cidr}</code>
                </td>
                <td>
                  <small>
                    {d.status?.ipAllocations?.length ?? 0}{" "}
                    {zh ? "个分配" : "allocated"}
                  </small>
                </td>
                <td>
                  <small>{d.metadata.creationTimestamp ?? "—"}</small>
                </td>
                <td>
                  <div className="row-actions">
                    <button
                      className="icon-button danger"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDelete(d.metadata.name);
                      }}
                      title={zh ? "删除" : "Delete"}
                    >
                      <Trash2 size={15} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {domains.length === 0 && !loading && (
              <tr>
                <td
                  colSpan={5}
                  style={{ textAlign: "center", padding: "32px" }}
                >
                  <small className="muted">
                    {zh ? "暂无域" : "No domains"}
                  </small>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      {showCreate && (
        <div
          className="modal-backdrop"
          onMouseDown={(e) =>
            e.target === e.currentTarget && setShowCreate(false)
          }
        >
          <div className="modal" style={{ maxWidth: 480 }}>
            <div className="modal-head">
              <div>
                <span className="eyebrow">NEW DOMAIN</span>
                <h2>{zh ? "创建网络域" : "Create Domain"}</h2>
              </div>
              <button
                className="icon-button"
                onClick={() => setShowCreate(false)}
              >
                ×
              </button>
            </div>
            <div className="modal-body">
              <div className="form-section">
                <div className="form-section-head">
                  <small>{zh ? "域名" : "Domain Name"}</small>
                </div>
                <input
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder={zh ? "my-domain" : "my-domain"}
                />
              </div>
              <div className="form-section">
                <div className="form-section-head">
                  <small>CIDR</small>
                </div>
                <input
                  value={newCidr}
                  onChange={(e) => setNewCidr(e.target.value)}
                  placeholder="10.244.0.0/16"
                />
                <small
                  className="muted"
                  style={{ display: "block", marginTop: 4 }}
                >
                  {zh
                    ? "Pod 跨集群 IP 将从此网段中分配"
                    : "Cross-cluster pod IPs will be allocated from this subnet"}
                </small>
              </div>
              {error && (
                <div className="cert-error" style={{ marginBottom: 12 }}>
                  {error}
                </div>
              )}
              <div className="step-actions">
                <button
                  className="secondary-button"
                  onClick={() => setShowCreate(false)}
                >
                  {zh ? "取消" : "Cancel"}
                </button>
                <button
                  className="primary-button"
                  disabled={creating || !newName.trim() || !newCidr.trim()}
                  onClick={handleCreate}
                >
                  {creating
                    ? zh
                      ? "创建中…"
                      : "Creating…"
                    : zh
                      ? "创建"
                      : "Create"}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export function DomainDetailPage({
  domain,
  copy: c,
  onBack,
}: {
  domain: CRDDomain;
  copy: Copy;
  onBack: () => void;
}) {
  const zh = c.nav.overview === "总览";
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [query, setQuery] = useState("");
  const [sortKey, setSortKey] = useState<"ip" | "job" | "task" | "pod" | null>(
    null,
  );
  const [sortAsc, setSortAsc] = useState(true);

  const allAllocs = domain.status?.ipAllocations ?? [];

  const filtered = useMemo(() => {
    if (!query.trim()) return allAllocs;
    const q = query.toLowerCase();
    return allAllocs.filter((a) =>
      `${a.ip} ${a.job} ${a.task} ${a.pod}`.toLowerCase().includes(q),
    );
  }, [allAllocs, query]);

  const sorted = useMemo(() => {
    if (!sortKey) return filtered;
    const arr = [...filtered];
    arr.sort((a, b) => {
      const av = (a[sortKey] ?? "").toString();
      const bv = (b[sortKey] ?? "").toString();
      return sortAsc ? av.localeCompare(bv) : bv.localeCompare(av);
    });
    return arr;
  }, [filtered, sortKey, sortAsc]);

  const total = sorted.length;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const currentPage = Math.min(page, totalPages);
  const start = (currentPage - 1) * pageSize;
  const paged = sorted.slice(start, start + pageSize);

  useEffect(() => {
    if (page > totalPages) setPage(1);
  }, [totalPages, page]);

  const toggleSort = (key: "ip" | "job" | "task" | "pod") => {
    if (sortKey === key) {
      setSortAsc(!sortAsc);
    } else {
      setSortKey(key);
      setSortAsc(true);
    }
  };

  return (
    <div className="page-content resource-page domain-detail-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">
            {zh ? "跨集群网络" : "Cross-cluster Network"}
          </span>
          <h2>{domain.metadata.name}</h2>
          <p>
            {zh
              ? "管理跨集群网络域，为 Pod 分配跨集群可达的 IP 地址。"
              : "Manage cross-cluster network domains for pod IP allocation."}
          </p>
        </div>
        <button className="secondary-button" onClick={onBack}>
          <ChevronLeft size={17} />
          {zh ? "返回列表" : "Back"}
        </button>
      </div>
      <div className="form-section">
        <div className="form-section-head">
          <small>CIDR</small>
        </div>
        <code className="inline-code">{domain.spec.cidr}</code>
      </div>
      <div className="form-section">
        <div className="form-section-head">
          <small>
            {zh ? "IP 分配明细" : "IP Allocations"} ({allAllocs.length})
          </small>
        </div>
        {allAllocs.length > 0 ? (
          <>
            <div
              style={{
                display: "flex",
                gap: 12,
                alignItems: "center",
                marginBottom: 8,
                flexWrap: "wrap",
              }}
            >
              <input
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value);
                  setPage(1);
                }}
                placeholder={
                  zh ? "搜索 IP/任务/Pod..." : "Search IP/Job/Pod..."
                }
                style={{ minWidth: 240, flex: 1, maxWidth: 360 }}
              />
              <small className="muted">
                {zh ? `共 ${total} 条` : `${total} total`}
              </small>
              <div style={{ marginLeft: "auto" }}>
                <label
                  style={{
                    display: "inline-flex",
                    gap: 6,
                    alignItems: "center",
                  }}
                >
                  <small className="muted">{zh ? "每页" : "Page size"}</small>
                  <select
                    value={pageSize}
                    onChange={(e) => {
                      setPageSize(Number(e.target.value));
                      setPage(1);
                    }}
                    style={{ width: "auto" }}
                  >
                    {[10, 20, 50, 100].map((n) => (
                      <option key={n} value={n}>
                        {n}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
            </div>
            <div className="table-panel">
              <table>
                <thead>
                  <tr>
                    <th>
                      <button
                        className="sort-th"
                        onClick={() => toggleSort("ip")}
                      >
                        IP
                        {sortKey === "ip" && (sortAsc ? " ▲" : " ▼")}
                      </button>
                    </th>
                    <th>
                      <button
                        className="sort-th"
                        onClick={() => toggleSort("job")}
                      >
                        {zh ? "任务" : "Job"}
                        {sortKey === "job" && (sortAsc ? " ▲" : " ▼")}
                      </button>
                    </th>
                    <th>
                      <button
                        className="sort-th"
                        onClick={() => toggleSort("task")}
                      >
                        {zh ? "子任务" : "Task"}
                        {sortKey === "task" && (sortAsc ? " ▲" : " ▼")}
                      </button>
                    </th>
                    <th>
                      <button
                        className="sort-th"
                        onClick={() => toggleSort("pod")}
                      >
                        Pod
                        {sortKey === "pod" && (sortAsc ? " ▲" : " ▼")}
                      </button>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {paged.map((alloc, i) => (
                    <tr key={start + i}>
                      <td>
                        <code className="inline-code">{alloc.ip}</code>
                      </td>
                      <td>
                        <small>{alloc.job}</small>
                      </td>
                      <td>
                        <small>{alloc.task}</small>
                      </td>
                      <td>
                        <small>{alloc.pod}</small>
                      </td>
                    </tr>
                  ))}
                  {paged.length === 0 && (
                    <tr>
                      <td
                        colSpan={4}
                        style={{ textAlign: "center", padding: "32px" }}
                      >
                        <small className="muted">
                          {zh ? "暂无匹配记录" : "No matching records"}
                        </small>
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
            <div className="pagination-bar">
              <button
                className="icon-button"
                disabled={currentPage <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                title={zh ? "上一页" : "Prev"}
              >
                <ChevronLeft size={16} />
              </button>
              <small>
                {zh
                  ? `第 ${currentPage} / ${totalPages} 页`
                  : `Page ${currentPage} / ${totalPages}`}
              </small>
              <button
                className="icon-button"
                disabled={currentPage >= totalPages}
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                title={zh ? "下一页" : "Next"}
              >
                <ChevronRight size={16} />
              </button>
            </div>
          </>
        ) : (
          <small className="muted">
            {zh ? "暂无 IP 分配" : "No IP allocations"}
          </small>
        )}
      </div>
      <div className="form-section">
        <div className="form-section-head">
          <small>{zh ? "创建时间" : "Created"}</small>
        </div>
        <small>{domain.metadata.creationTimestamp ?? "—"}</small>
      </div>
    </div>
  );
}
