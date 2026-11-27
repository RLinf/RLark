import { useCallback, useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { createPortal } from "react-dom";
import { AlertTriangle, Info, LoaderCircle, Trash2, X } from "lucide-react";
import { clustersApi, podsApi } from "../../backend";
import { StatusBadge } from "../../components/shared";
import { copy } from "../../i18n";
import type { Phase } from "../../data";
import type { ClusterSummary, CRDNode } from "../../types";
import { categoryLabels, getNodeCategories } from "../../utils/nodes";
import { overlayContainer } from "../../utils/overlayContainer";
import { machinesApi, stationsApi } from "../api";
import { NodeName } from "../components/NodeName";
import { useAction } from "../components/useAction";
import { useRemovalProgress } from "../components/removalProgressContext";
import { nodeNetworkModel } from "../lib/nodeNetwork";
import { podsToStop, removalOutcome, removalStageLabel, removalWorkloadPreview, verifiedNodeUID } from "../lib/nodeRemoval";
import { seenTime, unseenMachineOf } from "../lib/siteNodeDisplay";
import "./RemoveNodeDialog.css";

interface RemovalTarget {
  clusterID: string;
  cluster: ClusterSummary;
  nodeUID: string;
  pods: string[];
  unseen?: string;
}

// Confirms node removal with a concise impact summary and preserves the existing preflight behavior.
export function RemoveNodeDialog({ zh, node, onClose }: { zh: boolean; node: CRDNode; onClose: () => void }) {
  const nodeName = node.metadata.name;
  const namespace = node.metadata.namespace ?? "";
  const [target, setTarget] = useState<RemovalTarget | null>(null);
  const [preflightLoading, setPreflightLoading] = useState(true);
  const [error, setError] = useState("");
  const { pending, error: submitError, run } = useAction();
  const removalProgress = useRemovalProgress();
  const entry = target ? removalProgress.entryFor(target.clusterID, nodeName) : undefined;
  const active = !!entry && (!entry.operation || removalOutcome(entry.operation) === "active");
  const cancelRef = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLElement>(null);
  const submitGuard = useRef(false);
  const preflightRequest = useRef(0);
  const currentPhase = (node.status?.phase ?? "Offline") as Phase;
  const categories = getNodeCategories(node);
  const nodeType = categoryLabels[categories[0] ?? "unknown"];
  const nodeIP = nodeNetworkModel(node).addresses.find((address) => address.kind === "node")?.address;
  const preview = removalWorkloadPreview(target?.pods ?? [], 6);
  const unseenAt = target?.unseen ? seenTime(target.unseen, Date.now()) : undefined;
  const unseenSince = unseenAt ? (zh ? `自 ${unseenAt} 起` : `since ${unseenAt}`) : zh ? "最近" : "recently";
  const c = copy[zh ? "zh" : "en"];
  const robot = categories.includes("robot");

  const loadTarget = useCallback(async () => {
    const requestID = ++preflightRequest.current;
    setPreflightLoading(true);
    setError("");
    setTarget(null);
    try {
      const cluster = (await clustersApi.list<ClusterSummary>()).find((item) => item.agentNamespace === namespace);
      if (!cluster) throw new Error(zh ? `找不到命名空间 ${namespace} 的集群` : `No cluster has namespace ${namespace}`);
      const [stations, nodePods, machines] = await Promise.all([
        stationsApi.list({ cluster_id: cluster.id, limit: "500" }),
        podsApi.list(),
        machinesApi.list(),
      ]);
      const nodeUID = verifiedNodeUID(stations.items, namespace, nodeName);
      const unseen = unseenMachineOf(machines.items, cluster.id, nodeName, Date.now());
      if (requestID !== preflightRequest.current) return;
      setTarget({
        clusterID: cluster.id,
        cluster,
        nodeUID,
        pods: podsToStop(nodePods, namespace, nodeName),
        unseen: unseen ? (unseen.last_seen ?? "") : undefined,
      });
    } catch (reason) {
      if (requestID === preflightRequest.current) setError((reason as Error).message);
    } finally {
      if (requestID === preflightRequest.current) setPreflightLoading(false);
    }
  }, [namespace, nodeName, zh]);

  useEffect(() => () => {
    preflightRequest.current++;
  }, []);

  useEffect(() => {
    void loadTarget();
  }, [loadTarget]);

  useEffect(() => {
    const previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    cancelRef.current?.focus();
    return () => {
      if (previouslyFocused?.isConnected) previouslyFocused.focus();
    };
  }, []);

  // The admin shell scrolls in .main-area, not on body; lock that container
  // while the destructive confirmation is open.
  useEffect(() => {
    const scroller = document.querySelector<HTMLElement>(".main-area");
    const previous = scroller?.style.overflow;
    if (scroller) scroller.style.overflow = "hidden";
    return () => {
      if (scroller && previous !== undefined) scroller.style.overflow = previous;
    };
  }, []);

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !pending) onClose();
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [onClose, pending]);

  const trapFocus = (event: ReactKeyboardEvent<HTMLElement>) => {
    if (event.key !== "Tab") return;
    const candidates = dialogRef.current?.querySelectorAll<HTMLElement>(
      "button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], [tabindex]:not([tabindex='-1'])",
    );
    const focusable = candidates ? [...candidates].filter((element) => element.getClientRects().length > 0) : [];
    if (!focusable.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };

  const submit = () => {
    if (!target || preflightLoading || pending || active || submitGuard.current) return;
    submitGuard.current = true;
    void run(async () => {
      await removalProgress.start({ clusterID: target.clusterID, nodeName, nodeUID: target.nodeUID });
      onClose();
    }).finally(() => {
      submitGuard.current = false;
    });
  };

  const body = (
    <div
      className="modal-backdrop remove-node-backdrop"
      onMouseDown={(event) => event.target === event.currentTarget && !pending && onClose()}
    >
      <section
        ref={dialogRef}
        className="modal remove-node-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="remove-node-title"
        aria-describedby="remove-node-description"
        aria-busy={preflightLoading || pending}
        onKeyDown={trapFocus}
      >
        <header className="remove-node-head">
          <span className="remove-node-icon"><Trash2 size={19} aria-hidden="true" /></span>
          <div className="remove-node-heading">
            <span className="eyebrow">{zh ? "删除节点" : "Remove node"}</span>
            <h2 id="remove-node-title">{zh ? "删除 " : "Remove "}<NodeName node={node} /></h2>
            {target && <div className="remove-node-subtitle"><StatusBadge phase={currentPhase} copy={c} /><span title={target.cluster.name}>{target.cluster.name}</span></div>}
            <p id="remove-node-description">{zh ? "请核对节点身份和删除影响。" : "Review the node identity and removal impact."}</p>
          </div>
          <button type="button" className="icon-button" onClick={onClose} aria-label={zh ? "关闭" : "Close"} disabled={pending}><X size={17} /></button>
        </header>

        <div className="modal-body remove-node-body">
          {preflightLoading && <div className="remove-node-loading" role="status"><LoaderCircle size={17} className="remove-node-spinner" />{zh ? "正在读取节点和删除影响…" : "Loading node and removal details…"}</div>}
          {error && <div className="remove-node-error" role="alert"><strong>{zh ? "无法完成删除预检" : "Removal preflight failed"}<span>{error}</span></strong><button type="button" className="secondary-button" onClick={() => void loadTarget()} disabled={preflightLoading || pending}>{zh ? "重试预检" : "Retry preflight"}</button></div>}

          {!preflightLoading && target && active && <section className="remove-node-active" aria-live="polite">
            <span className="remove-node-active-icon"><LoaderCircle size={18} /></span>
            <div><strong>{zh ? "已在删除中" : "Already being removed"}</strong><span>{entry?.operation?.status?.stage ? removalStageLabel(entry.operation.status.stage, zh) : zh ? "等待控制器" : "Waiting for controller"}</span></div>
            <button type="button" className="secondary-button" onClick={() => { removalProgress.openDrawer(target.clusterID, nodeName); onClose(); }}>{zh ? "查看进度" : "View progress"}</button>
          </section>}
          {submitError && <div className="remove-node-error" role="alert"><strong>{zh ? "提交删除请求失败" : "Could not submit removal"}<span>{submitError}</span></strong></div>}

          {!preflightLoading && target && !active && <>
            <section className="remove-node-section" aria-labelledby="remove-node-facts-title">
              <h3 id="remove-node-facts-title">{zh ? "节点信息" : "Node details"}</h3>
              <div className="remove-node-facts">
                <div><small>{zh ? "集群" : "Cluster"}</small><strong title={target.cluster.name}>{target.cluster.name}</strong></div>
                <div><small>{zh ? "节点名" : "Node name"}</small><strong title={node.metadata.name}><NodeName node={node} /></strong></div>
                <div><small>{zh ? "节点 IP" : "Node IP"}</small><strong>{nodeIP ?? "—"}</strong></div>
                <div><small>{zh ? "状态" : "Status"}</small><StatusBadge phase={currentPhase} copy={c} /></div>
                <div className="remove-node-type-fact"><small>{zh ? "类型" : "Type"}</small><span className="remove-node-type-chip">{zh ? nodeType.zh : nodeType.en}</span></div>
                <div className="remove-node-uid-fact"><span className={`remove-node-uid-badge${target.nodeUID ? " verified" : " unverified"}`}><span aria-hidden="true">{target.nodeUID ? "✓" : "!"}</span>{target.nodeUID ? zh ? "UID 已核实" : "UID verified" : zh ? "按名称删除" : "Removal by name"}</span></div>
              </div>
            </section>

            <section className="remove-node-section" aria-labelledby="remove-node-steps-title">
              <h3 id="remove-node-steps-title">{zh ? "将执行" : "What will happen"}</h3>
              <ol className="remove-node-steps">
                <li><span>1</span><div><strong>{zh ? "预检并准备节点" : "Check and prepare the node"}</strong><small>{robot ? zh ? "机器人会先停驱动并下电；Run 或会话占用时操作会被阻止。" : "The robot driver is stopped and power is turned off first. Removal is blocked while a Run or session is using it." : zh ? "检查节点状态和移除条件。" : "Check the node state and removal prerequisites."}</small></div></li>
                <li><span>2</span><div><strong>{zh ? `停止节点上的 ${target.pods.length} 个任务` : `Stop ${target.pods.length} ${target.pods.length === 1 ? "workload" : "workloads"} on this node`}</strong><small>{zh ? "任务会按现有调度逻辑在其他节点重新启动。" : "Workloads restart on other nodes according to the existing scheduler."}</small></div></li>
                <li><span>3</span><div><strong>{zh ? "移出隧道并重置机器" : "Leave the tunnel and reset the machine"}</strong><small>{zh ? "机器可达时执行 kubeadm reset。" : "Run kubeadm reset when the machine is reachable."}</small></div></li>
                <li><span>4</span><div><strong>{zh ? "删除 Node 并确认移除" : "Delete the Node and verify removal"}</strong><small>{zh ? "移除在控制面持续执行，可在任务中心查看阶段。" : "Removal continues on the control plane. Follow its stages in the task center."}</small></div></li>
              </ol>
            </section>

            <section className="remove-node-section" aria-labelledby="remove-node-workloads-title">
              <h3 id="remove-node-workloads-title">{zh ? `将停止的任务 · ${target.pods.length}` : `Workloads to stop · ${target.pods.length}`}</h3>
              {preview.visible.length ? <div className="remove-node-workloads">{preview.visible.map((workload) => <span className="tag-value-chip" title={workload} key={workload}>{workload}</span>)}{preview.hiddenCount > 0 && <span className="remove-node-workload-more">{zh ? `还有 ${preview.hiddenCount} 个` : `${preview.hiddenCount} more`}</span>}</div> : <p className="remove-node-empty-workloads">{zh ? "此节点上没有运行中的任务。" : "No running workloads are assigned to this node."}</p>}
            </section>

            {target.unseen !== undefined && <div className="remove-node-warning" role="note"><AlertTriangle size={17} aria-hidden="true" /><div><strong>{zh ? "机器可能无法远程到达" : "The machine may be unreachable remotely"}</strong><span>{zh ? `场地 Hub ${unseenSince}未观测到这台机器的入口。删除后隧道也会拆除，可能只能到机器现场重新入网。` : `No site Hub has observed this machine's entry ${unseenSince}. Removal also tears down its tunnel, so it may need to be rejoined from the machine.`}</span></div></div>}
            {!target.nodeUID && <div className="remove-node-info" role="note"><Info size={17} aria-hidden="true" /><div><strong>{zh ? "未核实节点身份，将按名称删除" : "Node identity is unverified. Removal will target this name."}</strong><span>{zh ? "集群中存在同名 Node 时会移除该对象；否则清理 RLark 保留的同名入网、场站节点和管理面记录。" : "A matching Node in the cluster will be removed. Otherwise, RLark will clear its retained join, site-node, and management records for this name."}</span></div></div>}
          </>}
        </div>

        <footer className="remove-node-footer">
          <p>{zh ? "删除会在服务端继续；关闭后可在任务中心查看进度。" : "Removal continues on the server. Close this dialog any time and follow progress in the task center."}</p>
          <div className="remove-node-actions">
            <button ref={cancelRef} type="button" className="secondary-button" onClick={onClose} disabled={pending}>{active ? zh ? "关闭" : "Close" : zh ? "取消" : "Cancel"}</button>
            {!active && <button type="button" className="remove-node-confirm" onClick={submit} disabled={!target || preflightLoading || pending || !!error}><Trash2 size={15} />{pending ? zh ? "提交中…" : "Submitting…" : zh ? "确认删除节点" : "Remove node"}</button>}
          </div>
        </footer>
      </section>
    </div>
  );

  return createPortal(body, overlayContainer());
}
