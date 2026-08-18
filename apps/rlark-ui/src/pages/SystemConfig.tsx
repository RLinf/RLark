import { useEffect, useState } from "react";
import { Settings, Save, Check, Copy as CopyIcon } from "lucide-react";
import type { Copy } from "../i18n";

interface SystemConfig {
  sshJumpHost: string;
  sshJumpPort: string;
}

export function SystemConfigPage({ copy: c }: { copy: Copy }) {
  const zh = c.nav.overview === "总览";
  const [config, setConfig] = useState<SystemConfig>({
    sshJumpHost: "",
    sshJumpPort: "",
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    fetchConfig();
  }, []);

  const fetchConfig = async () => {
    setLoading(true);
    setError("");
    try {
      const resp = await fetch("/api/v1/system-config");
      if (!resp.ok) throw new Error(await resp.text());
      const data = await resp.json();
      setConfig({
        sshJumpHost: data.sshJumpHost || "",
        sshJumpPort: data.sshJumpPort || "",
      });
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    setSaving(true);
    setError("");
    setSaved(false);
    try {
      const resp = await fetch("/api/v1/system-config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(config),
      });
      if (!resp.ok) throw new Error(await resp.text());
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const sshCommand = config.sshJumpHost
    ? `ssh -J <ssh-user>@${config.sshJumpHost}${config.sshJumpPort ? ":" + config.sshJumpPort : ""} root@<pod-name>`
    : "";

  const copySSHCommand = async () => {
    await navigator.clipboard.writeText(sshCommand);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="page-content resource-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">
            <Settings size={13} />
            {zh ? "系统配置" : "System Config"}
          </span>
          <h2>{zh ? "系统配置" : "System Configuration"}</h2>
          <p>
            {zh
              ? "管理平台级别的系统配置，包括 SSH 跳板地址等。"
              : "Manage platform-level system configuration, including SSH jump host settings."}
          </p>
        </div>
        <div className="section-actions">
          <button
            className="secondary-button"
            onClick={fetchConfig}
            title={zh ? "刷新" : "Refresh"}
            disabled={loading}
          >
            {loading ? "…" : zh ? "刷新" : "Refresh"}
          </button>
          <button
            className="primary-button"
            onClick={handleSave}
            disabled={saving}
          >
            {saved ? <Check size={16} /> : <Save size={16} />}
            {saving
              ? zh
                ? "保存中…"
                : "Saving…"
              : saved
                ? zh
                  ? "已保存"
                  : "Saved"
                : zh
                  ? "保存"
                  : "Save"}
          </button>
        </div>
      </div>

      {error && (
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}

      <section className="table-panel" style={{ marginBottom: 20 }}>
        <div className="storage-table-heading">
          <div>
            <strong>{zh ? "SSH 跳板配置" : "SSH Jump Host"}</strong>
            <small>
              {zh
                ? "用户通过 SSH 代理连接到任务 Pod 时使用的跳板地址和端口"
                : "Jump host address and port for SSH proxy access to task pods"}
            </small>
          </div>
        </div>
        <div
          className="storage-create-form"
          style={{ background: "transparent", padding: "18px 20px" }}
        >
          <div className="form-section">
            <div
              className="form-grid"
              style={{ gridTemplateColumns: "1fr 200px" }}
            >
              <label>
                {zh ? "跳板地址" : "Jump Host"}
                <input
                  value={config.sshJumpHost}
                  onChange={(e) =>
                    setConfig({ ...config, sshJumpHost: e.target.value })
                  }
                  placeholder="nlb-xxx.cn-beijing.nlb.aliyuncsslb.com"
                />
              </label>
              <label>
                {zh ? "端口" : "Port"}
                <input
                  value={config.sshJumpPort}
                  onChange={(e) =>
                    setConfig({ ...config, sshJumpPort: e.target.value })
                  }
                  placeholder="2222"
                />
              </label>
            </div>
          </div>
        </div>
      </section>

      {sshCommand && (
        <section className="table-panel">
          <div className="storage-table-heading">
            <div>
              <strong>{zh ? "预览 SSH 命令" : "SSH Command Preview"}</strong>
              <small>
                {zh
                  ? "用户在任务页面看到的 SSH 连接命令"
                  : "The SSH command users will see on the job page"}
              </small>
            </div>
          </div>
          <div style={{ padding: "16px 20px" }}>
            <div className="system-config-ssh-preview">
              <code>{sshCommand}</code>
              <button
                type="button"
                className="system-config-copy-button"
                onClick={copySSHCommand}
                aria-label={zh ? "复制 SSH 命令" : "Copy SSH command"}
                title={zh ? "复制 SSH 命令" : "Copy SSH command"}
              >
                {copied ? <Check size={14} /> : <CopyIcon size={14} />}
                {copied ? (zh ? "已复制" : "Copied") : zh ? "复制" : "Copy"}
              </button>
            </div>
          </div>
        </section>
      )}
    </div>
  );
}
