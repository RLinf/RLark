import { useEffect, useState } from "react";
import { KeyRound, RefreshCw, Server, X } from "lucide-react";
import { nodeAdmissionsApi, selfHealApi, stationsApi, type NodeOperation } from "../api";
import type { ClusterSummary } from "../../types";
import { formatChinaDateTime } from "../../utils/time.js";
import { NodeName, type NodeRef } from "../components/NodeName";
import { joinStageLabel } from "../lib/nodeJoin";
import {
  admissionStateClass,
  admissionStateLabel,
  keyFingerprint,
  type IssuedNodeAdmission,
  type NodeAdmission,
} from "../lib/siteNodeDisplay";

const operationStatusClass = (phase?: string) =>
  phase === "Succeeded" ? "status-online" : phase === "Failed" ? "status-failed" : "status-pending";

// The node operations of one cluster (joins and removals), each opened in a drawer that follows it while it runs.
export function NodeOperations({ clusterID, zh, nodeAt }: { clusterID: string; zh: boolean; nodeAt: (namespace: string, name: string) => NodeRef }) {
  const [operations, setOperations] = useState<NodeOperation[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [shown, setShown] = useState<NodeOperation>();
  const [shownError, setShownError] = useState("");

  const load = async () => {
    setLoading(true);
    setError("");
    try {
      setOperations((await stationsApi.listNodeOperations(clusterID)).items);
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    void load();
    // The panel is keyed by its cluster.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // The operation shown is read at once and every 3 s until it ends, for three minutes.
  const shownName = shown?.metadata.name;
  useEffect(() => {
    if (!shownName) return;
    const controller = new AbortController();
    let timer = 0;
    let left = 60;
    const poll = async () => {
      try {
        const operation = await stationsApi.getNodeOperation(shownName, clusterID, controller.signal);
        setShown((current) => current?.metadata.name === shownName ? operation : current);
        setOperations((current) => current.map((item) => item.metadata.name === shownName ? operation : item));
        if (operation.status?.phase === "Succeeded" || operation.status?.phase === "Failed") return;
      } catch (reason) {
        if (!controller.signal.aborted) setShownError((reason as Error).message);
      }
      if (--left > 0 && !controller.signal.aborted) timer = window.setTimeout(poll, 3000);
    };
    void poll();
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [shownName, clusterID]);

  return <>
    <section className="station-operation-history-panel">
      <div className="station-operation-history-heading">
        <strong>{zh ? "节点操作记录" : "Node operations"}</strong>
        <button type="button" className="icon-button" onClick={() => void load()} disabled={loading} aria-label={zh ? "刷新操作记录" : "Refresh operations"} title={zh ? "刷新" : "Refresh"}><RefreshCw size={15} /></button>
      </div>
      {error && <div className="cert-error" role="alert">{error}</div>}
      {operations.length === 0 ? <small className="muted">{loading ? (zh ? "正在加载…" : "Loading…") : (zh ? "该集群没有操作记录" : "No operations for this cluster")}</small> : <div className="station-operation-history-list">
        {operations.map((operation) => <button type="button" className="station-operation-history-row" key={`${operation.metadata.namespace}/${operation.metadata.name}`} onClick={() => { setShown(operation); setShownError(""); }}>
          <span><strong><NodeName node={nodeAt(operation.metadata.namespace, operation.spec.nodeName)} /></strong><small className="muted">{operation.spec.action} · {operation.status?.stage ?? "—"}</small></span>
          <span className={`status ${operationStatusClass(operation.status?.phase)}`}><i />{operation.status?.phase ?? "Pending"}</span>
        </button>)}
      </div>}
    </section>

    {shown && <>
      <div className="addon-drawer-overlay stations-operation-overlay" onClick={() => setShown(undefined)} />
      <aside className="addon-drawer station-drawer station-operation-drawer" role="dialog" aria-modal="true" aria-labelledby="station-operation-title">
        <div className="addon-drawer-header station-drawer-header">
          <div>
            <span className="eyebrow"><Server size={13} />Station</span>
            <h3 id="station-operation-title">{zh ? "节点操作状态" : "Node operation status"}</h3>
            <small className="muted">{shown.spec.action} · <NodeName node={nodeAt(shown.metadata.namespace, shown.spec.nodeName)} /></small>
          </div>
          <button type="button" className="icon-button" aria-label={zh ? "关闭" : "Close"} title={zh ? "关闭" : "Close"} onClick={() => setShown(undefined)}><X size={16} /></button>
        </div>
        <div className="addon-drawer-body station-drawer-body">
          <section className="station-drawer-section station-join-progress" aria-live="polite">
            <span className={`status ${operationStatusClass(shown.status?.phase)}`}><i />{shown.status?.phase ?? "Pending"}</span>
            <strong>{shown.status?.stage ?? (zh ? "等待控制器" : "Waiting for controller")}</strong>
            <small className="muted">{shown.metadata.name}</small>
            {shown.status?.conditions?.map((condition) => <div key={condition.type} className="station-join-condition"><strong>{condition.reason ?? condition.type}</strong><small className="muted">{condition.message}</small></div>)}
            {shownError && <div className="cert-error" role="alert">{shownError}</div>}
          </section>
        </div>
        <div className="addon-drawer-footer">
          <button type="button" className="secondary-button" onClick={() => setShown(undefined)}>{zh ? "关闭" : "Close"}</button>
        </div>
      </aside>
    </>}
  </>;
}

// One record per node that joined by a code or over SSH: reissue, revoke, or clear a join that never made a node
// (one Redeemed, not Joined).
export function AdmissionRecords({ admissions, clusters, loadError, zh, onJoin, onChanged }: {
  admissions: NodeAdmission[];
  clusters: ClusterSummary[];
  // Why the admissions of a cluster could not be read.
  loadError: string;
  zh: boolean;
  onJoin: (reissued?: IssuedNodeAdmission, sshJoin?: { clusterID: string; name: string }) => void;
  onChanged: () => Promise<void>;
}) {
  const [pending, setPending] = useState("");
  const [error, setError] = useState("");

  const act = async (admission: NodeAdmission, question: string, work: () => Promise<unknown>) => {
    if (pending || (question && !window.confirm(question))) return;
    setPending(admission.node_name);
    setError("");
    try {
      await work();
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setPending("");
    }
  };
  const reissue = (admission: NodeAdmission) => act(admission, "", async () => onJoin(await nodeAdmissionsApi.reissue(admission.node_name, admission.cluster_id)));
  const revoke = (admission: NodeAdmission) => act(admission, zh ? `作废 ${admission.node_name} 的入网码？` : `Revoke the join code of ${admission.node_name}?`, async () => {
    await nodeAdmissionsApi.revoke(admission.node_name, admission.cluster_id);
    await onChanged();
  });
  // A join that has not made a node starts over: the name is cleared (a removal by name, which clears what RLark
  // keeps of a name its cluster has no Node of) and the admission is gone or open for a new code.
  const restart = (admission: NodeAdmission) => act(admission, zh
    ? `清掉 ${admission.node_name} 这次没走完的入网？集群里已有这个节点就按删除节点处理；没有就退役它的场站节点记录、删掉这条入网记录，放出公钥和隧道地址。之后重新发码或 SSH 入网。机器上的安装程序如果还在跑，先等它停下或在机器上停掉它。`
    : `Clear the unfinished join of ${admission.node_name}? A Node of the name in its cluster is removed; otherwise its site node record is retired and this admission deleted, freeing its key and tunnel address. Then give out a new code or join it by SSH. If the installer still runs on the machine, wait for it to stop or stop it there first.`, async () => {
    let operation = await selfHealApi.removeNode(admission.cluster_id, admission.node_name, `restart-${admission.node_name}-${Date.now().toString(36)}`);
    while (!["Succeeded", "Blocked", "Failed"].includes(operation.status?.phase ?? "")) {
      await new Promise((resolve) => window.setTimeout(resolve, 2000));
      operation = await stationsApi.getNodeOperation(operation.metadata.name, admission.cluster_id);
    }
    const condition = operation.status?.conditions?.find((item) => item.type === "Progressing");
    if (operation.status?.phase !== "Succeeded") setError(`${zh ? "清理停下：" : "Clearing stopped: "}${condition?.reason ?? ""} ${condition?.message ?? ""}`);
    await onChanged();
  });

  return <section className="table-panel station-admissions" aria-label={zh ? "入网记录" : "Admission records"}>
    <div className="station-operation-history-heading">
      <strong><KeyRound size={14} /> {zh ? "入网记录" : "Admission records"}</strong>
      <small className="muted">{zh ? "每台用入网码加入的节点一条：码是否已用、谁发的、何时过期。码本身不保存。" : "One per node that joined by a code: whether the code was used, who issued it, when it expires. The code itself is not kept."}</small>
    </div>
    {(error || loadError) && <div className="cert-error" role="alert">{error || loadError}</div>}
    {admissions.length === 0 ? <div className="stations-state muted">{zh ? "没有入网记录" : "No admission records"}</div> : (
      <table className="stations-table">
        <thead><tr>
          <th>{zh ? "节点" : "Node"}</th>
          <th>{zh ? "集群" : "Cluster"}</th>
          <th>{zh ? "状态" : "State"}</th>
          <th>{zh ? "发码人" : "Issued by"}</th>
          <th>{zh ? "发码时间" : "Issued"}</th>
          <th>{zh ? "过期时间" : "Expires"}</th>
          <th>{zh ? "WireGuard 公钥" : "WireGuard key"}</th>
          <th>{zh ? "隧道地址" : "Tunnel address"}</th>
          <th />
        </tr></thead>
        <tbody>
          {admissions.map((admission) => {
            const stage = joinStageLabel(admission.join_stage ?? "", zh ? "zh" : "en");
            return <tr key={`${admission.cluster_id}/${admission.node_name}`}>
              <td><strong>{admission.node_name}</strong>{admission.ssh_login && <small className="muted station-admission-ssh" title={zh ? "首次连接时记下的主机指纹" : "Host key as first seen"}>SSH {admission.ssh_login} · {admission.ssh_host_key}</small>}
                {admission.join_phase && admission.join_phase !== "Succeeded" && <button type="button" className="table-action-button station-admission-join" onClick={() => onJoin(undefined, { clusterID: admission.cluster_id, name: `${admission.cluster_id}.${admission.node_name}` })}>
                  {admission.join_phase === "Failed"
                    ? (zh ? `SSH 入网：停在 ${stage}` : `SSH join: stopped at ${stage}`)
                    : (zh ? `SSH 入网：${stage}` : `SSH join: ${stage}`)}
                </button>}</td>
              <td>{clusters.find((cluster) => cluster.id === admission.cluster_id)?.name ?? admission.cluster_id}</td>
              <td>
                <span className={`status ${admission.stuck ? "status-failed" : admissionStateClass(admission.state)}`} title={admission.reason}><i />{admission.stuck ? (zh ? "卡住" : "Stuck") : admissionStateLabel(admission.state, zh)}</span>
                {admission.state !== "Joined" && admission.reason && <small className="muted station-admission-step">{admission.reason}</small>}
                {admission.self_healed && <small className="station-admission-healed">{zh ? "已自动处理：" : "Put right by RLark: "}{admission.self_healed}</small>}
              </td>
              <td>{admission.issued_by || <span className="muted">—</span>}</td>
              <td><small>{formatChinaDateTime(admission.issued_at)}</small></td>
              <td><small>{formatChinaDateTime(admission.expires_at)}</small></td>
              <td>{admission.public_key ? <code className="inline-code" title={admission.public_key}>{keyFingerprint(admission.public_key)}</code> : <span className="muted">—</span>}</td>
              <td>{admission.tunnel_address || <span className="muted">—</span>}</td>
              <td className="station-admission-actions">
                <button type="button" className="table-action-button" onClick={() => void reissue(admission)} disabled={!!pending}>{zh ? "重发" : "Reissue"}</button>
                {admission.state === "Pending" && <button type="button" className="table-action-button danger" onClick={() => void revoke(admission)} disabled={!!pending}>{zh ? "作废" : "Revoke"}</button>}
                {admission.state === "Redeemed" && <button type="button" className="table-action-button danger" onClick={() => void restart(admission)} disabled={!!pending}>{pending === admission.node_name ? (zh ? "清理中…" : "Clearing…") : (zh ? "清掉重来" : "Start over")}</button>}
              </td>
            </tr>;
          })}
        </tbody>
      </table>
    )}
  </section>;
}
