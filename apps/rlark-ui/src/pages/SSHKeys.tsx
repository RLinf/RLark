import { useEffect, useState } from "react";
import {
  Plus,
  Trash2,
  Terminal,
  KeyRound,
  Copy as CopyIcon,
  Check,
  Info,
} from "lucide-react";
import type { Copy } from "../i18n";
import { formatChinaDateTime } from "../utils/time";
import { findSSHKeyDuplicate } from "../utils/sshKeys";
import {
  ColumnFilterButton,
  compareSortValues,
  RefreshOverlay,
  PageToolbar,
  SortButton,
  useColumnFilter,
  type SortDirection,
} from "../components/shared";
import { ColumnFilterPopover } from "../components/ColumnFilterPopover";

export function SSHKeysPage({
  copy: c,
  userName,
}: {
  copy: Copy;
  userName?: string;
}) {
  const zh = c.nav.overview === "总览";
  const [keys, setKeys] = useState<SSHKeyItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showAdd, setShowAdd] = useState(false);
  const [newUser, setNewUser] = useState(() => userName || "user");
  const [query, setQuery] = useState("");
  const [newKey, setNewKey] = useState("");
  const [adding, setAdding] = useState(false);
  const [copied, setCopied] = useState(false);
  const [copiedKey, setCopiedKey] = useState("");
  const [sort, setSort] = useState<{
    key: "added_at";
    direction: SortDirection;
  }>({ key: "added_at", direction: "desc" });
  // 公钥名称多选筛选；空数组 = 全部
  const [userFilter, setUserFilter] = useState<string[]>([]);
  const columnFilter = useColumnFilter();
  const toggleSort = (key: typeof sort.key) =>
    setSort((current) => ({
      key,
      direction:
        current.key === key && current.direction === "asc" ? "desc" : "asc",
    }));
  const userOptions = Array.from(new Set(keys.map((k) => k.user)))
    .sort()
    .map((v) => ({ value: v, label: v }));
  const filteredKeys = keys.filter((k) => {
    const queryHit = `${k.user} ${k.public_key}`
      .toLowerCase()
      .includes(query.trim().toLowerCase());
    return queryHit && (userFilter.length === 0 || userFilter.includes(k.user));
  });
  const sortedKeys = [...filteredKeys].sort((a, b) =>
    compareSortValues(
      a[sort.key],
      b[sort.key],
      sort.direction,
      zh ? "zh-CN" : "en",
    ),
  );

  const fetchKeys = async () => {
    setLoading(true);
    setError("");
    try {
      setKeys(await sshKeysApi.list());
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchKeys();
  }, []);

  const handleAdd = async () => {
    if (!newUser.trim() || !newKey.trim()) return;

    const duplicate = findSSHKeyDuplicate(newUser, newKey, keys);
    if (duplicate) {
      setError(
        duplicate === "name"
          ? zh
            ? "公钥名称已存在，请使用其他名称。"
            : "The public key name already exists. Use another name."
          : zh
            ? "该公钥已上传，请勿重复添加。"
            : "This public key has already been uploaded.",
      );
      return;
    }

    setAdding(true);
    setError("");
    try {
      try {
        await sshKeysApi.create(newUser.trim(), newKey.trim());
      } catch (error) {
        if (error instanceof ApiError && error.status === 409) {
          const data = error.body as { error?: string };
          setError(
            data.error === "public key name already exists"
              ? zh
                ? "公钥名称已存在，请使用其他名称。"
                : "The public key name already exists. Use another name."
              : zh
                ? "该公钥已上传，请勿重复添加。"
                : "This public key has already been uploaded.",
          );
          return;
        }
        throw error;
      }
      setNewKey("");
      setShowAdd(false);
      fetchKeys();
    } catch (e) {
      setError(String(e));
    } finally {
      setAdding(false);
    }
  };

  const handleDelete = async (user: string, index: number) => {
    if (
      !confirm(
        zh
          ? `确认删除 ${user} 的第 ${index + 1} 个公钥？`
          : `Delete key ${index + 1} for ${user}?`,
      )
    )
      return;
    try {
      await sshKeysApi.remove(user, index);
      fetchKeys();
    } catch (e) {
      setError(String(e));
    }
  };

  const [sshConfig, setSSHConfig] = useState<{
    sshJumpHost: string;
    sshJumpPort: string;
  } | null>(null);
  useEffect(() => {
    systemConfigApi
      .get()
      .then((d) => {
        setSSHConfig({
          sshJumpHost: d.ssh?.jumpHost || d.sshJumpHost || "",
          sshJumpPort: d.ssh?.jumpPort || d.sshJumpPort || "",
        });
      })
      .catch(() => {});
  }, []);
  const sshCommand = sshConfig?.sshJumpHost
    ? `ssh -J ${sshConfig.sshJumpHost}${sshConfig.sshJumpPort ? ":" + sshConfig.sshJumpPort : ""} root@<pod-name>`
    : `ssh -J <server>:<port> root@<pod-name>`;

  const handleCopy = () => {
    navigator.clipboard.writeText(sshCommand);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="page-content resource-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">
            <Terminal size={13} />
            {zh ? "SSH 公钥" : "SSH Keys"}
          </span>
          <h2>{zh ? "SSH 公钥管理" : "SSH Key Management"}</h2>
          <p>
            {zh
              ? "上传 SSH 公钥后，即可通过 SSH 代理登录连接到任务 Pod。"
              : "Upload your SSH public key, then connect to task pods via SSH proxy."}
          </p>
        </div>
        <div className="section-actions">
          <button
            className="primary-button"
            onClick={() => setShowAdd(!showAdd)}
          >
            <Plus size={16} />
            {zh ? "添加公钥" : "Add Key"}
          </button>
        </div>
      </div>

      <section
        className="storage-overview-grid"
        aria-label={zh ? "SSH 概况" : "SSH Overview"}
      >
        <div className="storage-overview-card purple">
          <span>
            <KeyRound size={18} />
          </span>
          <div>
            <small>{zh ? "已添加公钥" : "Added Keys"}</small>
            <strong>{keys.length}</strong>
            <em>{zh ? "个公钥可用" : "public keys available"}</em>
          </div>
        </div>
        <div className="storage-overview-card green">
          <span>
            <Terminal size={18} />
          </span>
          <div>
            <small>{zh ? "连接用户" : "SSH User"}</small>
            <strong>{newUser}</strong>
            <em>{zh ? "用于 SSH 代理登录" : "for SSH proxy login"}</em>
          </div>
        </div>
        <div className="storage-overview-card orange">
          <span>
            <Terminal size={18} />
          </span>
          <div>
            <small>{zh ? "连接命令" : "Connect Command"}</small>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                marginTop: 4,
              }}
            >
              <code
                style={{
                  fontSize: 11,
                  fontFamily: "var(--font-mono, monospace)",
                  color: "var(--ink)",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                  maxWidth: 280,
                }}
              >
                {sshCommand}
              </code>
              <button
                className="icon-button"
                title={zh ? "复制" : "Copy"}
                onClick={handleCopy}
                style={{ flexShrink: 0 }}
              >
                {copied ? <Check size={14} /> : <CopyIcon size={14} />}
              </button>
            </div>
          </div>
        </div>
      </section>

      <PageToolbar
        placeholder={
          zh ? "搜索公钥名称或内容..." : "Search key name or content..."
        }
        value={query}
        onChange={setQuery}
        count={filteredKeys.length}
        copy={c}
        onRefresh={fetchKeys}
        refreshing={loading}
      />

      {showAdd && (
        <div
          className="modal-backdrop storage-create-backdrop"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget && !adding)
              setShowAdd(false);
          }}
        >
          <section
            className="modal storage-create-modal"
            role="dialog"
            aria-modal="true"
          >
            <div className="modal-head storage-create-head">
              <div>
                <strong>{zh ? "添加 SSH 公钥" : "Add SSH Public Key"}</strong>
                <small>
                  {zh
                    ? "粘贴你的公钥内容，支持 ssh-ed25519、ssh-rsa 等格式"
                    : "Paste your public key. Supports ssh-ed25519, ssh-rsa, etc."}
                </small>
              </div>
            </div>
            <div
              className="storage-create-form"
              style={{ background: "transparent", padding: "18px 20px" }}
            >
              <div className="form-section">
                <strong>{zh ? "公钥信息" : "Key Info"}</strong>
                <div
                  className="form-grid"
                  style={{ gridTemplateColumns: "1fr" }}
                >
                  <label>
                    {zh ? "公钥名称" : "Public Key Name"}
                    <input
                      value={newUser}
                      onChange={(e) => {
                        setNewUser(e.target.value);
                        setError("");
                      }}
                      placeholder="user"
                    />
                  </label>
                </div>
                <div
                  className="form-grid"
                  style={{ marginTop: 16, gridTemplateColumns: "1fr" }}
                >
                  <label>
                    {zh ? "公钥内容" : "Public Key"}
                    <textarea
                      value={newKey}
                      onChange={(e) => {
                        setNewKey(e.target.value);
                        setError("");
                      }}
                      placeholder="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... or ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB..."
                      rows={4}
                    />
                  </label>
                </div>
              </div>
              {error && (
                <div className="form-error-banner" role="alert">
                  {error}
                </div>
              )}
              <div
                className="form-actions"
                style={{
                  position: "static",
                  margin: 0,
                  padding: 0,
                  borderTop: "none",
                  background: "transparent",
                  backdropFilter: "none",
                }}
              >
                <button
                  type="button"
                  className="secondary-button"
                  onClick={() => {
                    setShowAdd(false);
                    setNewKey("");
                    setError("");
                  }}
                >
                  {zh ? "取消" : "Cancel"}
                </button>
                <button
                  type="button"
                  className="primary-button"
                  onClick={handleAdd}
                  disabled={adding || !newKey.trim()}
                >
                  <Plus size={16} />
                  {adding
                    ? zh
                      ? "添加中…"
                      : "Adding…"
                    : zh
                      ? "确认添加"
                      : "Add"}
                </button>
              </div>
            </div>
          </section>
        </div>
      )}

      <div
        className="ssh-tip-banner"
        style={{
          display: "flex",
          alignItems: "flex-start",
          gap: 10,
          marginBottom: 16,
          padding: "12px 14px",
          border: "1px solid rgba(124, 58, 237, 0.15)",
          borderRadius: 12,
          background: "rgba(124, 58, 237, 0.04)",
        }}
      >
        <Info
          size={16}
          style={{ flexShrink: 0, marginTop: 1, color: "var(--blue)" }}
        />
        <div style={{ fontSize: 12, lineHeight: 1.6, color: "var(--muted)" }}>
          {zh
            ? "创建任务时可在「公共配置」中选择已上传的公钥。连接时使用 root 用户登录 Pod。"
            : "When creating a job, select your uploaded key in 'Common Config'. Use root user to connect to the Pod."}
        </div>
      </div>

      {error && !showAdd && (
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}

      <section
        className={`table-panel refreshable-region${loading ? " is-refreshing" : ""}`}
        aria-busy={loading}
      >
        <div className="storage-table-heading">
          <div>
            <strong>{zh ? "公钥列表" : "SSH Keys"}</strong>
            <small>
              {zh
                ? "管理用于 SSH 登录的公钥"
                : "Manage public keys for SSH access"}
            </small>
          </div>
          <span>
            {zh
              ? `共 ${filteredKeys.length} 项`
              : `${filteredKeys.length} items`}
          </span>
        </div>
        {filteredKeys.length === 0 ? (
          <p className="muted" style={{ padding: "20px" }}>
            {loading
              ? zh
                ? "加载中…"
                : "Loading…"
              : zh
                ? "暂无公钥"
                : "No SSH keys found"}
          </p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>
                  <ColumnFilterButton
                    label={zh ? "公钥名称" : "Public Key Name"}
                    selectedCount={userFilter.length}
                    onClick={columnFilter.openFor("user")}
                  />
                </th>
                <th>{zh ? "公钥" : "Public Key"}</th>
                <th>
                  <SortButton
                    label={zh ? "添加时间" : "Added"}
                    active={sort.key === "added_at"}
                    direction={sort.direction}
                    onClick={() => toggleSort("added_at")}
                  />
                </th>
                <th className="table-actions-col">{zh ? "操作" : "Actions"}</th>
              </tr>
            </thead>
            <tbody>
              {sortedKeys.map((k) => {
                const keyId = `${k.user}-${k.index}`;
                const isCopied = copiedKey === keyId;
                return (
                  <tr key={keyId}>
                    <td style={{ fontWeight: 500 }}>{k.user}</td>
                    <td>
                      <div
                        style={{
                          display: "flex",
                          alignItems: "center",
                          gap: 8,
                        }}
                      >
                        <code
                          title={k.public_key}
                          style={{
                            fontFamily: "var(--font-mono, monospace)",
                            fontSize: 12,
                            maxWidth: 360,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                            cursor: "help",
                          }}
                        >
                          {k.public_key}
                        </code>
                        <button
                          className="icon-button"
                          title={zh ? "复制公钥" : "Copy key"}
                          onClick={() => {
                            navigator.clipboard.writeText(k.public_key);
                            setCopiedKey(keyId);
                            setTimeout(() => setCopiedKey(""), 2000);
                          }}
                          style={{ flexShrink: 0 }}
                        >
                          {isCopied ? (
                            <Check size={14} />
                          ) : (
                            <CopyIcon size={14} />
                          )}
                        </button>
                      </div>
                    </td>
                    <td className="muted">{formatChinaDateTime(k.added_at)}</td>
                    <td className="table-actions-col">
                      <div className="row-actions">
                        <button
                          type="button"
                          className="icon-button danger"
                          title={zh ? "删除" : "Delete"}
                          aria-label={zh ? "删除" : "Delete"}
                          onClick={() => handleDelete(k.user, k.index)}
                        >
                          <Trash2 size={15} />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
        <RefreshOverlay
          visible={loading}
          label={zh ? "正在刷新 SSH 公钥列表" : "Refreshing SSH key list"}
        />
      </section>
      {columnFilter.openKey === "user" && (
        <ColumnFilterPopover
          label={zh ? "公钥名称" : "Public Key Name"}
          options={userOptions}
          selected={userFilter}
          onChange={setUserFilter}
          anchorRect={columnFilter.anchorRect}
          onClose={columnFilter.close}
          zh={zh}
        />
      )}
    </div>
  );
}
import { ApiError } from "../api";
import { sshKeysApi, systemConfigApi, type SSHKeyItem } from "../backend";
