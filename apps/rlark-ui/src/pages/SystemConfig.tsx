import { useEffect, useState } from "react";
import {
  Settings,
  Save,
  Check,
  Copy as CopyIcon,
  RefreshCw,
  ServerCog,
  ScrollText,
  ShieldCheck,
} from "lucide-react";
import type { Copy } from "../i18n";
import { RefreshOverlay } from "../components/shared";
import {
  systemConfigApi,
  type DeploymentConfig,
  type SystemConfigResponse,
} from "../backend";
import { resolveDeploymentConfig } from "../utils/deployYaml";

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
  deployment: {
    controlPlaneAddress: string;
    sshAddress: string;
    insecureSkipTlsVerify: boolean;
    kubernetes: {
      kubeconfig: string;
      agentImage: string;
      image: string;
      imagePullPolicy: "" | "Always" | "IfNotPresent" | "Never";
      imagePullSecrets: string;
      containerdSocket: string;
    };
  };
}

export function SystemConfigPage({ copy: c }: { copy: Copy }) {
  const zh = c.nav.overview === "总览";
  const [activeCategory, setActiveCategory] = useState<
    "ssh" | "deployment" | "log"
  >("ssh");
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
    deployment: {
      controlPlaneAddress: "",
      sshAddress: "",
      insecureSkipTlsVerify: false,
      kubernetes: {
        kubeconfig: "",
        agentImage: "rlark:latest",
        image: "rlark:latest",
        imagePullPolicy: "",
        imagePullSecrets: "",
        containerdSocket: "",
      },
    },
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [copied, setCopied] = useState(false);
  // 记录日志配置是否被修改过，用于决定是否在保存时发送 log 字段
  const [isLogConfigDirty, setIsLogConfigDirty] = useState(false);

  const applyConfig = (data: SystemConfigResponse) => {
    const secret = data.log?.config?.accessKeySecret || "";
    const deployment = resolveDeploymentConfig(data.deployment);
    setConfig({
      sshJumpHost: data.ssh?.jumpHost || data.sshJumpHost || "",
      sshJumpPort: data.ssh?.jumpPort || data.sshJumpPort || "",
      log: {
        backend: data.log?.backend || "none",
        config: {
          endpoint: data.log?.config?.endpoint || "",
          project: data.log?.config?.project || "",
          logstore: data.log?.config?.logstore || "",
          accessKeyId: data.log?.config?.accessKeyId || "",
          accessKeySecret: secret,
        },
      },
      isAccessKeySecretSet: Boolean(secret),
      deployment: {
        controlPlaneAddress: deployment.controlPlaneAddress || "",
        sshAddress: deployment.sshAddress || "",
        insecureSkipTlsVerify: deployment.insecureSkipTlsVerify || false,
        kubernetes: {
          kubeconfig: deployment.kubernetes?.kubeconfig || "",
          agentImage: deployment.kubernetes?.agentImage || "",
          image: deployment.kubernetes?.image || "",
          imagePullPolicy: deployment.kubernetes?.imagePullPolicy || "",
          imagePullSecrets:
            deployment.kubernetes?.imagePullSecrets?.join(", ") || "",
          containerdSocket: deployment.kubernetes?.containerdSocket || "",
        },
      },
    });
    setIsLogConfigDirty(false);
  };

  useEffect(() => {
    fetchConfig();
  }, []);

  const fetchConfig = async () => {
    setLoading(true);
    setError("");
    try {
      applyConfig(await systemConfigApi.get({ refresh: true }));
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
      const host = config.sshJumpHost.trim();
      const port = config.sshJumpPort.trim();
      if (activeCategory === "ssh" && !host && port) {
        throw new Error(
          zh
            ? "设置端口时必须填写跳板地址"
            : "Jump host is required when port is set",
        );
      }
      if (
        activeCategory === "ssh" &&
        (/\s|\/|@/.test(host) || host.includes(":"))
      ) {
        throw new Error(
          zh
            ? "跳板地址必须是不含协议、用户、端口或路径的主机名/IP"
            : "Jump host must be a hostname/IP without scheme, user, port, or path",
        );
      }
      if (
        activeCategory === "ssh" &&
        port &&
        (!/^\d+$/.test(port) || Number(port) < 1 || Number(port) > 65535)
      ) {
        throw new Error(
          zh
            ? "端口必须是 1 到 65535 之间的整数"
            : "Port must be an integer between 1 and 65535",
        );
      }
      // 构建请求体：如果日志配置没有被修改，则不包含 log 字段
      const requestBody: {
        ssh?: { jumpHost: string; jumpPort: string };
        log?: LogConfig;
        deployment?: DeploymentConfig;
      } = {};

      if (activeCategory === "ssh") {
        requestBody.ssh = {
          jumpHost: host,
          jumpPort: port,
        };
      }
      if (activeCategory === "deployment") {
        requestBody.deployment = {
          apiVersion: "rlark.io/v1alpha1",
          kind: "DeployConfig",
          plane: "data",
          controlPlaneAddress: config.deployment.controlPlaneAddress,
          sshAddress: config.deployment.sshAddress,
          insecureSkipTlsVerify: config.deployment.insecureSkipTlsVerify,
          kubernetes: {
            ...config.deployment.kubernetes,
            imagePullSecrets: config.deployment.kubernetes.imagePullSecrets
              .split(",")
              .map((value) => value.trim())
              .filter(Boolean),
          },
        };
      }

      if (activeCategory === "log") {
        requestBody.log = config.log;
      }

      applyConfig(await systemConfigApi.update(requestBody));
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
    <div
      className={`page-content resource-page system-config-page refreshable-region page-refresh-region${loading ? " is-refreshing" : ""}`}
      aria-busy={loading}
    >
      <div className="section-heading system-config-heading">
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
            type="button"
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
            type="button"
            className="primary-button"
            onClick={handleSave}
            disabled={saving || loading}
            aria-busy={saving}
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

      <div
        className="system-config-overview"
        aria-label={zh ? "配置概览" : "Configuration overview"}
      >
        <div>
          <span>
            <ShieldCheck size={18} />
          </span>
          <small>{zh ? "接入入口" : "Access entry"}</small>
          <strong>
            {config.sshJumpHost || (zh ? "尚未配置" : "Not configured")}
          </strong>
        </div>
        <div>
          <span>
            <ServerCog size={18} />
          </span>
          <small>{zh ? "Agent 部署" : "Agent deployment"}</small>
          <strong>
            {config.deployment.kubernetes.agentImage ||
              (zh ? "使用默认镜像" : "Default image")}
          </strong>
        </div>
        <div>
          <span>
            <ScrollText size={18} />
          </span>
          <small>{zh ? "历史日志" : "Historical logs"}</small>
          <strong>
            {config.log.backend === "none"
              ? zh
                ? "未开启"
                : "Disabled"
              : config.log.backend.toUpperCase()}
          </strong>
        </div>
      </div>

      {error && (
        <div className="cert-error" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}

      <nav
        className="api-category-bar system-config-category-bar"
        aria-label={zh ? "系统配置分类" : "System configuration categories"}
      >
        <div className="api-category-tabs">
          <button
            type="button"
            className={activeCategory === "ssh" ? "active" : ""}
            onClick={() => setActiveCategory("ssh")}
            aria-pressed={activeCategory === "ssh"}
          >
            {zh ? "SSH 跳板配置" : "SSH Jump Host"}
          </button>
          <button
            type="button"
            className={activeCategory === "deployment" ? "active" : ""}
            onClick={() => setActiveCategory("deployment")}
            aria-pressed={activeCategory === "deployment"}
          >
            {zh ? "部署配置" : "Deployment"}
          </button>
          <button
            type="button"
            className={activeCategory === "log" ? "active" : ""}
            onClick={() => setActiveCategory("log")}
            aria-pressed={activeCategory === "log"}
          >
            {zh ? "日志后端配置" : "Log Backend"}
          </button>
        </div>
      </nav>

      {activeCategory === "ssh" && (
        <>
          <section
            className="table-panel system-config-panel"
            style={{ marginBottom: 20 }}
          >
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
            <section
              className="table-panel system-config-panel system-config-preview-panel"
              style={{ marginBottom: 20 }}
            >
              <div className="storage-table-heading">
                <div>
                  <strong>
                    {zh ? "预览 SSH 命令" : "SSH Command Preview"}
                  </strong>
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
        </>
      )}

      {activeCategory === "deployment" && (
        <section className="table-panel system-config-panel">
          <div className="storage-table-heading">
            <div>
              <strong>{zh ? "部署配置默认值" : "Deployment Defaults"}</strong>
              <small>
                {zh
                  ? "用于生成签发集群页面中的部署配置 YAML"
                  : "Defaults used to generate deployment YAML on the cluster signing page"}
              </small>
            </div>
          </div>
          <div
            className="storage-create-form"
            style={{ background: "transparent", padding: "18px 20px" }}
          >
            <div className="form-section">
              <div
                className="form-grid system-config-form-grid"
                style={{ gridTemplateColumns: "1fr 1fr" }}
              >
                <label>
                  {zh ? "控制面地址" : "Control Plane Address"}
                  <input
                    value={config.deployment.controlPlaneAddress}
                    onChange={(event) =>
                      setConfig({
                        ...config,
                        deployment: {
                          ...config.deployment,
                          controlPlaneAddress: event.target.value,
                        },
                      })
                    }
                    placeholder="https://rlark.example.com:8443"
                  />
                </label>
                <label>
                  {zh
                    ? "控制面 SSH 地址（可选）"
                    : "Control Plane SSH Address (optional)"}
                  <input
                    value={config.deployment.sshAddress}
                    onChange={(event) =>
                      setConfig({
                        ...config,
                        deployment: {
                          ...config.deployment,
                          sshAddress: event.target.value,
                        },
                      })
                    }
                    placeholder="client@rlark.example.com:2222"
                  />
                </label>
                <label>
                  {zh ? "Kubeconfig 路径" : "Kubeconfig Path"}
                  <input
                    value={config.deployment.kubernetes.kubeconfig}
                    onChange={(event) =>
                      setConfig({
                        ...config,
                        deployment: {
                          ...config.deployment,
                          kubernetes: {
                            ...config.deployment.kubernetes,
                            kubeconfig: event.target.value,
                          },
                        },
                      })
                    }
                    placeholder="~/.kube/config"
                  />
                </label>
                <label>
                  {zh ? "Agent 镜像" : "Agent Image"}
                  <input
                    value={config.deployment.kubernetes.agentImage}
                    onChange={(event) =>
                      setConfig({
                        ...config,
                        deployment: {
                          ...config.deployment,
                          kubernetes: {
                            ...config.deployment.kubernetes,
                            agentImage: event.target.value,
                          },
                        },
                      })
                    }
                    placeholder="rlark:latest"
                  />
                </label>
                <label>
                  {zh
                    ? "共享 RLark 镜像（可选）"
                    : "Shared RLark Image (optional)"}
                  <input
                    value={config.deployment.kubernetes.image}
                    onChange={(event) =>
                      setConfig({
                        ...config,
                        deployment: {
                          ...config.deployment,
                          kubernetes: {
                            ...config.deployment.kubernetes,
                            image: event.target.value,
                          },
                        },
                      })
                    }
                    placeholder="rlark:latest"
                  />
                </label>
                <label>
                  {zh ? "镜像拉取策略" : "Image Pull Policy"}
                  <select
                    value={config.deployment.kubernetes.imagePullPolicy}
                    onChange={(event) =>
                      setConfig({
                        ...config,
                        deployment: {
                          ...config.deployment,
                          kubernetes: {
                            ...config.deployment.kubernetes,
                            imagePullPolicy: event.target.value as
                              "" | "Always" | "IfNotPresent" | "Never",
                          },
                        },
                      })
                    }
                  >
                    <option value="">
                      {zh ? "使用默认值" : "Use default"}
                    </option>
                    <option value="Always">Always</option>
                    <option value="IfNotPresent">IfNotPresent</option>
                    <option value="Never">Never</option>
                  </select>
                </label>
                <label>
                  {zh
                    ? "镜像拉取 Secret（逗号分隔）"
                    : "Image Pull Secrets (comma-separated)"}
                  <input
                    value={config.deployment.kubernetes.imagePullSecrets}
                    onChange={(event) =>
                      setConfig({
                        ...config,
                        deployment: {
                          ...config.deployment,
                          kubernetes: {
                            ...config.deployment.kubernetes,
                            imagePullSecrets: event.target.value,
                          },
                        },
                      })
                    }
                    placeholder="registry-secret"
                  />
                </label>
                <label>
                  {zh
                    ? "Containerd Socket（可选）"
                    : "Containerd Socket (optional)"}
                  <input
                    value={config.deployment.kubernetes.containerdSocket}
                    onChange={(event) =>
                      setConfig({
                        ...config,
                        deployment: {
                          ...config.deployment,
                          kubernetes: {
                            ...config.deployment.kubernetes,
                            containerdSocket: event.target.value,
                          },
                        },
                      })
                    }
                    placeholder="/run/containerd/containerd.sock"
                  />
                </label>
                <label>
                  <span>{zh ? "TLS 验证" : "TLS Verification"}</span>
                  <span className="checkbox-row">
                    <input
                      type="checkbox"
                      checked={config.deployment.insecureSkipTlsVerify}
                      onChange={(event) =>
                        setConfig({
                          ...config,
                          deployment: {
                            ...config.deployment,
                            insecureSkipTlsVerify: event.target.checked,
                          },
                        })
                      }
                    />
                    {zh
                      ? "跳过控制面 TLS 证书验证（不推荐）"
                      : "Skip control-plane TLS certificate verification (not recommended)"}
                  </span>
                </label>
              </div>
            </div>
          </div>
        </section>
      )}

      {activeCategory === "log" && (
        <section className="table-panel system-config-panel">
          <div className="storage-table-heading">
            <div>
              <strong>
                {zh ? "日志后端配置" : "Log Backend Configuration"}
              </strong>
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
                    <option value="none">{zh ? "不开启" : "Disabled"}</option>
                    <option value="sls">阿里云 SLS</option>
                    <option value="loki" disabled>
                      Loki (待支持)
                    </option>
                    <option value="elasticsearch" disabled>
                      Elasticsearch (待支持)
                    </option>
                  </select>
                </label>
                {config.log.backend === "sls" && (
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
                )}
                {config.log.backend === "sls" && (
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
                )}
                {config.log.backend === "sls" && (
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
                )}
                {config.log.backend === "sls" && (
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
                )}
                {config.log.backend === "sls" && (
                  <label>
                    {zh
                      ? "认证 Secret (Access Key Secret)"
                      : "Access Key Secret"}
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
                )}
              </div>
            </div>
          </div>
        </section>
      )}
      <RefreshOverlay
        visible={loading}
        label={zh ? "正在刷新系统配置" : "Refreshing system configuration"}
      />
    </div>
  );
}
