import { useEffect, useState } from "react";
import {
  Check,
  ChevronRight,
  Copy,
  FileCode2,
  KeyRound,
  Server,
  Shield,
} from "lucide-react";
import type { Lang } from "../i18n";
import type { AgentCertListItem, SignAgentCertResponse } from "../types";
import {
  certificatesApi,
  systemConfigApi,
  type DeploymentConfig,
} from "../backend";
import { buildDeployYaml } from "../utils/deployYaml";

export function CreateClusterPage({ lang }: { lang: Lang }) {
  const [clusterId, setClusterId] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<SignAgentCertResponse | null>(null);
  const [copied, setCopied] = useState(false);
  const [certList, setCertList] = useState<AgentCertListItem[]>([]);
  const [, setCertListLoading] = useState(true);
  const [expandedCluster, setExpandedCluster] = useState<string | null>(null);
  const [expandedResult, setExpandedResult] =
    useState<SignAgentCertResponse | null>(null);
  const [expandedCopied, setExpandedCopied] = useState(false);
  const [deploymentConfig, setDeploymentConfig] = useState<DeploymentConfig>(
    {},
  );

  const zh = lang === "zh";

  const fetchCertList = async () => {
    setCertListLoading(true);
    try {
      setCertList(await certificatesApi.list());
    } catch {
    } finally {
      setCertListLoading(false);
    }
  };

  useEffect(() => {
    fetchCertList();
    systemConfigApi
      .get({ refresh: true })
      .then((config) => {
        setDeploymentConfig(config.deployment || {});
      })
      .catch(() => {});
  }, []);

  const handleSign = async () => {
    if (!clusterId.trim()) return;
    setLoading(true);
    setError("");
    setResult(null);
    try {
      setResult(await certificatesApi.sign(clusterId.trim()));
      fetchCertList();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const deployYaml = result ? buildDeployYaml(result, deploymentConfig) : "";

  const handleCopy = () => {
    navigator.clipboard.writeText(deployYaml).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  const handleExpand = async (cid: string) => {
    if (expandedCluster === cid) {
      setExpandedCluster(null);
      setExpandedResult(null);
      return;
    }
    setExpandedCluster(cid);
    setExpandedResult(null);
    try {
      setExpandedResult(await certificatesApi.get(cid));
    } catch {}
  };

  const handleExpandedCopy = () => {
    if (!expandedResult) return;
    navigator.clipboard
      .writeText(buildDeployYaml(expandedResult, deploymentConfig))
      .then(() => {
        setExpandedCopied(true);
        setTimeout(() => setExpandedCopied(false), 2000);
      });
  };

  return (
    <div className="page-content resource-page create-cluster-page">
      <div className="section-heading create-cluster-heading">
        <div>
          <span className="eyebrow">
            <Shield size={13} />
            {zh ? "创建集群" : "Create Cluster"}
          </span>
          <h2>{zh ? "创建集群" : "Create Cluster"}</h2>
          <p>
            {zh
              ? "部署数据面集群前，请自定义集群名称并签发证书。签发成功后，将下方 YAML 内容填入 deploy-conf.yaml 的 cert 字段即可。"
              : "Before deploying a data-plane cluster, customize the cluster name and sign certificates. After signing, paste the YAML content below into the cert field of your deploy-conf.yaml."}
          </p>
        </div>
      </div>

      <div className="cluster-enrollment-flow">
        <section className="cluster-enrollment-card">
          <div className="cluster-enrollment-card-head">
            <span className="cluster-enrollment-step">01</span>
            <div>
              <strong>
                {zh ? "命名并签发身份" : "Name and issue identity"}
              </strong>
              <small>
                {zh
                  ? "名称会成为集群在控制面中的唯一标识"
                  : "The name becomes the cluster identity in the control plane"}
              </small>
            </div>
          </div>
          <div className="cert-form">
            <label>
              <span>{zh ? "集群名称" : "Cluster Name"}</span>
              <input
                value={clusterId}
                onChange={(e) => setClusterId(e.target.value)}
                placeholder={
                  zh
                    ? "输入集群名称，例如 my-cluster-01"
                    : "Enter cluster name, e.g. my-cluster-01"
                }
                onKeyDown={(e) => e.key === "Enter" && handleSign()}
              />
            </label>
            <button
              className="primary-button"
              onClick={handleSign}
              disabled={loading || !clusterId.trim()}
            >
              {loading
                ? zh
                  ? "签发中..."
                  : "Signing..."
                : zh
                  ? "签发证书"
                  : "Sign Certificate"}
            </button>
          </div>

          {error && <div className="cert-error">{error}</div>}
          <div className="cluster-enrollment-notes">
            <span>
              <KeyRound size={16} />
              {zh
                ? "每个集群使用独立证书"
                : "Dedicated certificate per cluster"}
            </span>
            <span>
              <Server size={16} />
              {zh
                ? "生成 Kubernetes Agent 部署配置"
                : "Generates Kubernetes Agent deployment"}
            </span>
          </div>
        </section>

        {result && (
          <section className="cert-result cluster-enrollment-result">
            <div className="cert-result-header">
              <div>
                <Check size={18} />
                <strong>
                  {zh ? "证书签发成功" : "Certificate signed successfully"}
                </strong>
              </div>
              <small>
                {zh ? "集群" : "Cluster"}: {result.cluster_id} ·{" "}
                {zh ? "服务器" : "Server"}: {result.server_addr}
              </small>
            </div>
            <div className="cert-yaml-block cluster-yaml-card">
              <div className="cert-yaml-head">
                <div>
                  <FileCode2 size={16} />
                  <strong>{zh ? "部署配置 YAML" : "Deployment YAML"}</strong>
                </div>
                <button className="secondary-button" onClick={handleCopy}>
                  <Copy size={14} />
                  {copied ? (zh ? "已复制" : "Copied") : zh ? "复制" : "Copy"}
                </button>
              </div>
              <pre>{deployYaml}</pre>
            </div>
          </section>
        )}
      </div>

      {certList.length > 0 && (
        <section className="signed-clusters-panel">
          <div className="signed-clusters-heading">
            <div>
              <span className="eyebrow">
                <Shield size={13} />
                {zh ? "已签发集群" : "Signed Clusters"}
              </span>
              <h3>{zh ? "已签发集群" : "Signed Clusters"}</h3>
              <p>
                {zh
                  ? "展开集群可重新获取按当前默认值生成的部署 YAML。"
                  : "Expand a cluster to regenerate deployment YAML with current defaults."}
              </p>
            </div>
            <span>{certList.length}</span>
          </div>
          <div className="cert-list">
            {certList.map((item) => (
              <div key={item.cluster_id} className="cert-list-item">
                <div
                  className={
                    "cert-list-row" +
                    (expandedCluster === item.cluster_id ? " expanded" : "")
                  }
                  onClick={() => handleExpand(item.cluster_id)}
                >
                  <span className="cert-list-icon">
                    <Server size={15} />
                  </span>
                  <span className="cert-list-name">{item.cluster_id}</span>
                  <small className="cert-list-date">
                    {new Date(item.created_at).toLocaleString(
                      zh ? "zh-CN" : "en-US",
                    )}
                  </small>
                  <ChevronRight
                    size={16}
                    className={
                      "cert-list-chevron" +
                      (expandedCluster === item.cluster_id ? " rotated" : "")
                    }
                  />
                </div>
                {expandedCluster === item.cluster_id && (
                  <div className="signed-cluster-detail">
                    {expandedResult ? (
                      <div className="cert-yaml-block cluster-yaml-card signed-cluster-yaml">
                        <div className="cert-yaml-head">
                          <div>
                            <FileCode2 size={16} />
                            <strong>
                              {zh ? "部署配置 YAML" : "Deployment YAML"}
                            </strong>
                          </div>
                          <button
                            className="secondary-button"
                            onClick={(event) => {
                              event.stopPropagation();
                              handleExpandedCopy();
                            }}
                          >
                            <Copy size={14} />
                            {expandedCopied
                              ? zh
                                ? "已复制"
                                : "Copied"
                              : zh
                                ? "复制"
                                : "Copy"}
                          </button>
                        </div>
                        <pre>
                          {buildDeployYaml(expandedResult, deploymentConfig)}
                        </pre>
                      </div>
                    ) : (
                      <div className="signed-cluster-loading">
                        <span />
                        {zh
                          ? "正在获取证书与部署配置..."
                          : "Loading certificate and deployment configuration..."}
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
