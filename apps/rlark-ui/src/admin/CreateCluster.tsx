import { useEffect, useState } from "react";
import { Check, ChevronRight, Shield } from "lucide-react";
import type { Copy, Lang } from "../i18n";
import type { AgentCertListItem, SignAgentCertResponse } from "../types";

export function CreateClusterPage({
  copy: c,
  lang,
}: {
  copy: Copy;
  lang: Lang;
}) {
  const [clusterId, setClusterId] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<SignAgentCertResponse | null>(null);
  const [copied, setCopied] = useState(false);
  const [certList, setCertList] = useState<AgentCertListItem[]>([]);
  const [certListLoading, setCertListLoading] = useState(true);
  const [expandedCluster, setExpandedCluster] = useState<string | null>(null);
  const [expandedResult, setExpandedResult] =
    useState<SignAgentCertResponse | null>(null);
  const [expandedCopied, setExpandedCopied] = useState(false);

  const zh = lang === "zh";

  const fetchCertList = async () => {
    setCertListLoading(true);
    try {
      const resp = await fetch("/api/v1/certificates/agent");
      if (resp.ok) {
        setCertList(await resp.json());
      }
    } catch {
    } finally {
      setCertListLoading(false);
    }
  };

  useEffect(() => {
    fetchCertList();
  }, []);

  const handleSign = async () => {
    if (!clusterId.trim()) return;
    setLoading(true);
    setError("");
    setResult(null);
    try {
      const resp = await fetch("/api/v1/certificates/agent", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ cluster_id: clusterId.trim() }),
      });
      if (!resp.ok) {
        const body = await resp.text();
        throw new Error(`HTTP ${resp.status}: ${body}`);
      }
      setResult(await resp.json());
      fetchCertList();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const buildDeployYaml = (
    r: SignAgentCertResponse,
  ) => `apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: data
control-plane-address: ${r.server_addr}

cert:
  ca-cert: |
${r.ca_cert
  .split("\n")
  .map((l: string) => "    " + l)
  .join("\n")}
  agent-cert: |
${r.agent_cert
  .split("\n")
  .map((l: string) => "    " + l)
  .join("\n")}
  agent-key: |
${r.agent_key
  .split("\n")
  .map((l: string) => "    " + l)
  .join("\n")}

kubernetes:
  kubeconfig: /path/to/kubeconfig.yaml
  agent-image: rlark-agent:latest
`;

  const deployYaml = result ? buildDeployYaml(result) : "";

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
      const resp = await fetch(
        `/api/v1/certificates/agent/${encodeURIComponent(cid)}`,
      );
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      setExpandedResult(await resp.json());
    } catch {}
  };

  const handleExpandedCopy = () => {
    if (!expandedResult) return;
    navigator.clipboard.writeText(buildDeployYaml(expandedResult)).then(() => {
      setExpandedCopied(true);
      setTimeout(() => setExpandedCopied(false), 2000);
    });
  };

  return (
    <div className="page-content resource-page">
      <div className="section-heading">
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

      <div className="panel cert-panel">
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

        {result && (
          <div className="cert-result">
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
            <div className="cert-yaml-block">
              <div className="cert-yaml-head">
                <strong>
                  {zh
                    ? "部署配置 YAML（可直接复制到 deploy-conf.yaml）"
                    : "Deploy YAML (copy to deploy-conf.yaml)"}
                </strong>
                <button className="secondary-button" onClick={handleCopy}>
                  {copied ? (zh ? "已复制" : "Copied") : zh ? "复制" : "Copy"}
                </button>
              </div>
              <pre>{deployYaml}</pre>
            </div>
          </div>
        )}
      </div>

      {certList.length > 0 && (
        <div className="panel cert-panel" style={{ marginTop: 24 }}>
          <div className="section-heading" style={{ marginBottom: 16 }}>
            <div>
              <span className="eyebrow">
                <Shield size={13} />
                {zh ? "已签发集群" : "Signed Clusters"}
              </span>
              <h3>{zh ? "已签发集群" : "Signed Clusters"}</h3>
            </div>
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
                  <span className="cert-list-dot" />
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
                  <div className="cert-yaml-block" style={{ marginTop: 8 }}>
                    {expandedResult ? (
                      <>
                        <div className="cert-yaml-head">
                          <strong>
                            {zh ? "部署配置 YAML" : "Deploy YAML"}
                          </strong>
                          <button
                            className="secondary-button"
                            onClick={handleExpandedCopy}
                          >
                            {expandedCopied
                              ? zh
                                ? "已复制"
                                : "Copied"
                              : zh
                                ? "复制"
                                : "Copy"}
                          </button>
                        </div>
                        <pre>{buildDeployYaml(expandedResult)}</pre>
                      </>
                    ) : (
                      <p className="muted">{zh ? "加载中..." : "Loading..."}</p>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
