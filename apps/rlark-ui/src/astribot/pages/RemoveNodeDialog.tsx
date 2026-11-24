import { useEffect, useState } from "react";
import { Trash2, X } from "lucide-react";
import { clustersApi, podsApi } from "../../backend";
import { machinesApi, stationsApi } from "../api";
import type { CRDNode, ClusterSummary } from "../../types";
import { podsToStop, removalOutcome, verifiedNodeUID } from "../lib/nodeRemoval";
import { NodeName } from "../components/NodeName";
import { useAction } from "../components/useAction";
import { useRemovalProgress } from "../components/removalProgressContext";
import { nodeLabel } from "../lib/displayName";
import { unseenMachineOf } from "../lib/siteNodeDisplay";
import "./Stations.css";

// Confirms a node removal after preserving the station, pod, and machine preflight checks.
export function RemoveNodeDialog({ zh, node, onClose }: { zh: boolean; node: CRDNode; onClose: () => void }) {
  const nodeName = node.metadata.name;
  const namespace = node.metadata.namespace ?? "";
  const [target, setTarget] = useState<{ clusterID: string; nodeUID: string; pods: string[]; unseen?: string } | null>(null);
  const { pending, error, setError, run } = useAction();
  const removalProgress = useRemovalProgress();
  const entry = target ? removalProgress.entryFor(target.clusterID, nodeName) : undefined;
  const active = !!entry && (!entry.operation || removalOutcome(entry.operation) === "active");
  const pods = target?.pods ?? [];

  useEffect(() => {
    let current = true;
    (async () => {
      const cluster = (await clustersApi.list<ClusterSummary>()).find((item) => item.agentNamespace === namespace);
      if (!cluster) throw new Error(zh ? `找不到命名空间 ${namespace} 的集群` : `No cluster has namespace ${namespace}`);
      const [stations, nodePods, machines] = await Promise.all([stationsApi.list({ cluster_id: cluster.id, limit: "500" }), podsApi.list(), machinesApi.list()]);
      const nodeUID = verifiedNodeUID(stations.items, namespace, nodeName);
      const unseen = unseenMachineOf(machines.items, cluster.id, nodeName, Date.now());
      if (current) setTarget({ clusterID: cluster.id, nodeUID, pods: podsToStop(nodePods, namespace, nodeName), unseen: unseen && (unseen.last_seen ?? "—") });
    })().catch((reason) => current && setError((reason as Error).message));
    return () => { current = false; };
  }, [namespace, nodeName, zh, setError]);

  const submit = () => {
    if (!target || pending) return;
    setError("");
    void run(async () => {
      await removalProgress.start({ clusterID: target.clusterID, nodeName, nodeUID: target.nodeUID });
      onClose();
    });
  };

  return <>
    <div className="addon-drawer-overlay stations-operation-overlay" onClick={() => !pending && onClose()} />
    <aside className="addon-drawer station-drawer station-operation-drawer" role="dialog" aria-modal="true" aria-labelledby="remove-node-title">
      <div className="addon-drawer-header station-drawer-header">
        <div><span className="eyebrow"><Trash2 size={13} />{zh ? "删除节点" : "Remove node"}</span><h3 id="remove-node-title">{zh ? "删除 " : "Remove "}<NodeName node={node} /></h3><small className="muted">{namespace}</small></div>
        <button type="button" className="icon-button" aria-label={zh ? "关闭" : "Close"} title={zh ? "关闭" : "Close"} onClick={onClose} disabled={pending}><X size={16} /></button>
      </div>
      <div className="addon-drawer-body station-drawer-body">
        {!target && !error && <p className="muted">{zh ? "读取中…" : "Loading…"}</p>}
        {target && !active && <p>{zh
          ? `${nodeLabel(node)} 会从集群删除：是机器人的话先停驱动、下电（有 Run 或会话在用就不删）；${pods.length ? `停下它上面的 ${pods.length} 个任务（${pods.join("、")}），任务会在别处重新启动；` : ""}把它移出隧道，在机器上执行 kubeadm reset，再删除节点。机器连不上的话，它上面的旧安装会在它下次加入时清掉。`
          : `${nodeLabel(node)} leaves the cluster: a robot has its driver stopped and is powered off first (not while a Run or a session uses it); ${pods.length ? `its ${pods.length} task pods (${pods.join(", ")}) stop and start elsewhere; ` : ""}it leaves the tunnel, kubeadm reset runs on the machine, and the Node is deleted. If the machine cannot be reached, its old installation is cleared when it joins again.`}</p>}
        {target && !active && target.unseen && <div className="cert-error" role="alert">{zh
          ? `场地的 Hub 从 ${target.unseen} 起就没再扫到这台机器的入口：删除后它的隧道也会拆掉，RLark 再也到不了它，只能到现场重新入网。`
          : `No hub of its site has seen this machine's entry since ${target.unseen}: once removed, its tunnel goes too and RLark cannot reach it again; it can only join again from the machine itself.`}</div>}
        {target && !active && !target.nodeUID && <p className="muted">{zh
          ? "RLark 没有核实过的场站节点记录对应它（入网没走完，或记录还属于这个名字以前的节点）：按名字删除。集群里有这个节点就删它；没有就只清掉 RLark 里关于这个名字的记录（入网记录、场站节点记录、管理面节点），之后可以重新入网。"
          : "No site node record RLark verified stands for it (its join never finished, or the record is of an earlier node of the name): it is removed by its name. The Node its cluster has is removed; if there is none, only what RLark keeps of the name is cleared (admission, site node record, management Node), and it can join again."}</p>}
        {active && <section className="station-join-progress"><strong>{zh ? "已在删除中" : "Already being removed"}</strong><span className="muted">{entry?.operation?.status?.stage ?? (zh ? "等待控制器" : "Waiting for controller")}</span><button type="button" className="secondary-button" onClick={() => { removalProgress.openDrawer(entry!.clusterID, entry!.nodeName); onClose(); }}>{zh ? "查看进度" : "View progress"}</button></section>}
        {error && <div className="cert-error" role="alert">{error}</div>}
      </div>
      <div className="addon-drawer-footer">
        <button type="button" className="secondary-button" onClick={onClose} disabled={pending}>{active ? (zh ? "关闭" : "Close") : (zh ? "取消" : "Cancel")}</button>
        {!active && <button type="button" className="primary-button danger" onClick={submit} disabled={!target || pending}><Trash2 size={14} />{pending ? (zh ? "提交中…" : "Submitting…") : (zh ? "确认删除" : "Remove")}</button>}
      </div>
    </aside>
  </>;
}
