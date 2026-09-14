import { useEffect, useState } from "react";
import {
  Settings,
  Save,
  Check,
  Copy as CopyIcon,
  RefreshCw,
} from "lucide-react";
import type { Copy } from "../i18n";

interface LogBackendConfig {
  endpoint: string;
  project: string;
  logstore: string;
  accessKeyId: string;
  accessKeySecret: string;
}

interface LogConfig {
  backend: string;
  config: LogBackendConfig;
}

interface SystemConfig {
  sshJumpHost: string;
  sshJumpPort: string;
  log: LogConfig;
  isAccessKeySecretSet: boolean; // 标记 accessKeySecret 是否已设置（用于区分掩码和用户输入）
}

export function SystemConfigPage({ copy: c }: { copy: Copy }) {
  const zh = c.nav.overview === "总览";
  const [config, setConfig] = useState<SystemConfig>({
    sshJumpHost: "",
    sshJumpPort: "",
    log: {
      backend: "sls",
      config: {
        endpoint: "",
        project: "",
        logstore: "",
        accessKeyId: "",
        accessKeySecret: "",
      },
    },
    isAccessKeySecretSet: false,
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [copied, setCopied] = useState(false);
  // 记录日志配置是否被修改过，用于决定是否在保存时发送 log 字段
  const [isLogConfigDirty, setIsLogConfigDirty] = useState(false);

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
      const secret = data.log?.config?.accessKeySecret || "";
      setConfig({
        sshJumpHost: data.ssh?.jumpHost || data.sshJumpHost || "",
        sshJumpPort: data.ssh?.jumpPort || data.sshJumpPort || "",
        log: {
          backend: data.log?.backend || "sls",
          config: {
            endpoint: data.log?.config?.endpoint || "",
            project: data.log?.config?.project || "",
            logstore: data.log?.config?.logstore || "",
            accessKeyId: data.log?.config?.accessKeyId || "",
            // 后端返回掩码时展示掩码；用于让用户知道已设置
            accessKeySecret: secret,
          },
        },
        isAccessKeySecretSet: Boolean(secret),
      });
      // 加载完成后，重置脏标记
      setIsLogConfigDirty(false);
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
      // 构建请求体：如果日志配置没有被修改，则不包含 log 字段
      const requestBody: any = {
        ssh: {
          jumpHost: config.sshJumpHost,
          jumpPort: config.sshJumpPort,
        },
      };

      if (isLogConfigDirty) {
        requestBody.log = config.log;
      }

      const resp = await fetch("/api/v1/system-config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(requestBody),
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
    ? `ssh -J ${config.sshJumpHost}${config.sshJumpPort ? ":" + config.sshJumpPort : ""} root@<pod-name>`
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
            aria-busy={loading}
          >
            <RefreshCw
              size={16}
              className={loading ? "job-action-loading" : ""}
            />
            {loading
              ? zh
                ? "刷新中..."
                : "Refreshing..."
              : zh
                ? "刷新"
                : "Refresh"}
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
        <section className="table-panel" style={{ marginBottom: 20 }}>
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

      <section className="table-panel">
        <div className="storage-table-heading">
          <div>
            <strong>{zh ? "日志后端配置" : "Log Backend Configuration"}</strong>
            <small>
              {zh
                ? "配置日志后端的连接参数，用于查询历史日志"
                : "Configure log backend connection parameters for querying historical logs"}
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
              style={{ gridTemplateColumns: "1fr 1fr" }}
            >
              <label>
                {zh ? "后端类型" : "Backend Type"}
                <select
                  value={config.log.backend}
                  onChange={(e) => {
                    setConfig({
                      ...config,
                      log: { ...config.log, backend: e.target.value },
                    });
                    setIsLogConfigDirty(true);
                  }}
                >
                  <option value="sls">阿里云 SLS</option>
                  <option value="loki" disabled>
                    Loki (待支持)
                  </option>
                  <option value="elasticsearch" disabled>
                    Elasticsearch (待支持)
                  </option>
                </select>
              </label>
              <label>
                {zh ? "接入地址 (Endpoint)" : "Endpoint"}
                <input
                  value={config.log.config.endpoint}
                  onChange={(e) => {
                    setConfig({
                      ...config,
                      log: {
                        ...config.log,
                        config: {
                          ...config.log.config,
                          endpoint: e.target.value,
                        },
                      },
                    });
                    setIsLogConfigDirty(true);
                  }}
                  placeholder="rlark.cn-beijing.log.aliyuncs.com:10012"
                />
              </label>
              <label>
                {zh ? "项目/组织名 (Project)" : "Project"}
                <input
                  value={config.log.config.project}
                  onChange={(e) => {
                    setConfig({
                      ...config,
                      log: {
                        ...config.log,
                        config: {
                          ...config.log.config,
                          project: e.target.value,
                        },
                      },
                    });
                    setIsLogConfigDirty(true);
                  }}
                  placeholder="rlark"
                />
              </label>
              <label>
                {zh ? "日志库/索引名 (Logstore)" : "Logstore"}
                <input
                  value={config.log.config.logstore}
                  onChange={(e) => {
                    setConfig({
                      ...config,
                      log: {
                        ...config.log,
                        config: {
                          ...config.log.config,
                          logstore: e.target.value,
                        },
                      },
                    });
                    setIsLogConfigDirty(true);
                  }}
                  placeholder="rlark"
                />
              </label>
              <label>
                {zh ? "认证 ID (Access Key ID)" : "Access Key ID"}
                <input
                  value={config.log.config.accessKeyId}
                  onChange={(e) => {
                    setConfig({
                      ...config,
                      log: {
                        ...config.log,
                        config: {
                          ...config.log.config,
                          accessKeyId: e.target.value,
                        },
                      },
                    });
                    setIsLogConfigDirty(true);
                  }}
                  placeholder="xxxxx"
                />
              </label>
              <label>
                {zh ? "认证 Secret (Access Key Secret)" : "Access Key Secret"}
                <input
                  type="password"
                  value={config.log.config.accessKeySecret}
                  onChange={(e) => {
                    setConfig({
                      ...config,
                      log: {
                        ...config.log,
                        config: {
                          ...config.log.config,
                          accessKeySecret: e.target.value,
                        },
                      },
                      isAccessKeySecretSet: false,
                    });
                    setIsLogConfigDirty(true);
                  }}
                  placeholder={
                    config.isAccessKeySecretSet
                      ? zh
                        ? "已设置，输入以更新"
                        : "Configured. Type to update"
                      : "xxxxx"
                  }
                />
              </label>
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
