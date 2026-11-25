import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import {
  Ban,
  ChevronRight,
  Loader2,
  LayoutGrid,
  LockOpen,
  MoreVertical,
  Unlink2,
} from "lucide-react";
import type { Phase } from "../data";
import type { Copy } from "../i18n";
import type { CRDNode, NodeCategory } from "../types";
import {
  nodeAvailabilityNote,
  nodeAvailabilityState,
  nodeConnectionState,
  nodePressureState,
  nodeSchedulingState,
  nodeStateLabel,
  type NodeAvailability,
  type NodeStateKind,
} from "../utils/nodeState";
import {
  categoryLabels,
  getNodeCategories,
  getNodeLocation,
  getNodeResourceSummary,
  hasNodeCategory,
} from "../utils/nodes";
import { nodeWorkloadKey } from "../utils/nodeWorkloads";
import {
  ColumnFilterButton,
  PageToolbar,
  Pagination,
  RefreshOverlay,
  useColumnFilter,
} from "./shared";
import { ColumnFilterPopover } from "./ColumnFilterPopover";

type CategoryFilter = "all" | NodeCategory;

const categoryOrder: NodeCategory[] = ["cloud", "edge", "robot", "unknown"];

function StateBadge({ kind, zh }: { kind: NodeStateKind; zh: boolean }) {
  return (
    <span className={`node-state-badge ${kind}`}>
      {nodeStateLabel(kind, zh)}
    </span>
  );
}

/**
 * AvailabilityBadge — single synthesized "can this node run tasks" status.
 * Detailed reasons are rendered in the Notes column, so the badge itself
 * carries no tooltip.
 */
function AvailabilityBadge({
  availability,
  zh,
}: {
  availability: NodeAvailability;
  zh: boolean;
}) {
  return (
    <span className="node-row-status">
      <StateBadge kind={availability.kind} zh={zh} />
    </span>
  );
}

/**
 * AdminNodeRowMenu — compact "⋯" dropdown for an admin node row.
 * Provides cordon/uncordon, drain/undrain, and unmanage (remove RLark Node CR) actions.
 */
function AdminNodeRowMenu({
  node,
  schedulingKind,
  updating,
  zh,
  runningJobs,
  onToggleScheduling,
  onDrain,
  onUndrain,
  onUnmanage,
}: {
  node: CRDNode;
  schedulingKind: string;
  updating: boolean;
  zh: boolean;
  // RLark 托管的工作负载数量（node.status.activeWorkloads.rlarkManaged）。不能
  // 用总运行 Pod 数，否则任何带系统 Pod 的节点都会永远无法取消纳管。
  runningJobs: number;
  onToggleScheduling: () => void;
  onDrain: () => void;
  onUndrain: () => void;
  onUnmanage: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [menuStyle, setMenuStyle] = useState<CSSProperties>({});
  const rootRef = useRef<HTMLDivElement>(null);
  const btnRef = useRef<HTMLButtonElement>(null);

  useLayoutEffect(() => {
    if (!open || !rootRef.current) return;
    const btn = btnRef.current;
    if (!btn) return;
    const rect = btn.getBoundingClientRect();
    const ddEl = rootRef.current.querySelector(
      ".action-dropdown",
    ) as HTMLElement | null;
    const ddHeight = ddEl?.offsetHeight ?? 120;
    const ddWidth = ddEl?.offsetWidth ?? 160;
    const spaceBelow = window.innerHeight - rect.bottom;
    const dropUp = spaceBelow < ddHeight + 8;
    const menuLeft = Math.max(
      8,
      Math.min(rect.right - ddWidth, window.innerWidth - ddWidth - 8),
    );
    setMenuStyle(
      dropUp
        ? {
            position: "fixed",
            left: menuLeft,
            bottom: window.innerHeight - rect.top + 4,
            right: "auto",
            top: "auto",
          }
        : {
            position: "fixed",
            left: menuLeft,
            top: rect.bottom + 4,
            right: "auto",
            bottom: "auto",
          },
    );
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handlePointer = (e: PointerEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    const handleClose = () => setOpen(false);
    document.addEventListener("pointerdown", handlePointer);
    document.addEventListener("keydown", handleKey);
    window.addEventListener("scroll", handleClose, true);
    window.addEventListener("resize", handleClose);
    return () => {
      document.removeEventListener("pointerdown", handlePointer);
      document.removeEventListener("keydown", handleKey);
      window.removeEventListener("scroll", handleClose, true);
      window.removeEventListener("resize", handleClose);
    };
  }, [open]);

  const cordoned = node.spec.unschedulable;
  const draining = schedulingKind === "draining";
  // An unreachable Node has no heartbeat, so the controller reports it Offline
  // (never Cordoned) and already marks it unschedulable — the scheduler fails
  // closed on placement while AgentConnected is not True. Gating unmanage on the
  // Cordoned phase would leave such a Node permanently un-unmanageable, so treat
  // "not connected" as unable to take new Tasks and skip both prerequisites.
  const agentUnreachable = nodeConnectionState(node).kind !== "connected";
  // Gate on the durable cordon intent (spec.unschedulable) rather than the
  // controller-reported Cordoned phase: the phase is written on a later pass, so
  // using it would disable drain/unmanage for the whole window after an operator
  // pauses scheduling. The Gateway validates unmanage against the same spec field.
  const canUnmanage = agentUnreachable || (cordoned && runningJobs === 0);

  return (
    <div
      ref={rootRef}
      style={{ position: "relative" }}
      className="admin-node-row-menu"
    >
      <span className="action-tooltip" data-tooltip={zh ? "更多操作" : "More"}>
        <button
          ref={btnRef}
          type="button"
          className="icon-button"
          onClick={() => setOpen((v) => !v)}
          aria-label={
            zh
              ? `节点 ${node.metadata.name} 的操作菜单`
              : `Actions for ${node.metadata.name}`
          }
          aria-expanded={open}
          disabled={updating}
        >
          {updating ? (
            <span className="admin-node-row-spinner" aria-hidden />
          ) : (
            <MoreVertical size={16} />
          )}
        </button>
      </span>
      {open && (
        <div className="action-dropdown" style={menuStyle}>
          <button
            type="button"
            className="action-dropdown-item"
            disabled={updating || draining}
            onClick={() => {
              setOpen(false);
              onToggleScheduling();
            }}
          >
            {cordoned ? <LockOpen size={14} /> : <Ban size={14} />}
            {cordoned
              ? zh
                ? "恢复调度"
                : "Resume Schedule"
              : zh
                ? "暂停调度"
                : "Pause Schedule"}
          </button>
          {draining ? (
            <button
              type="button"
              className="action-dropdown-item"
              disabled={updating || runningJobs === 0}
              title={
                runningJobs === 0
                  ? zh
                    ? "节点上的运行任务已全部驱逐"
                    : "All running jobs on the node have been drained"
                  : undefined
              }
              onClick={() => {
                setOpen(false);
                onUndrain();
              }}
            >
              <Loader2 size={14} />
              {zh ? "取消驱逐" : "Cancel drain"}
            </button>
          ) : (
            <button
              type="button"
              className="action-dropdown-item"
              disabled={updating || !cordoned || runningJobs === 0}
              onClick={() => {
                setOpen(false);
                onDrain();
              }}
              title={
                runningJobs === 0
                  ? zh
                    ? "节点没有运行任务"
                    : "No running jobs on the node"
                  : !cordoned
                    ? zh
                      ? "请先对该节点执行暂停调度，停止放置新的 RLark 任务"
                      : "Pause scheduling on the node first to stop placing new RLark tasks"
                    : undefined
              }
            >
              <Loader2 size={14} />
              {zh ? "驱逐任务" : "Drain Jobs"}
            </button>
          )}
          <button
            type="button"
            className="action-dropdown-item danger"
            disabled={updating || !canUnmanage}
            title={
              canUnmanage
                ? undefined
                : !cordoned
                  ? zh
                    ? "请先对该节点执行暂停调度，停止放置新的 RLark 任务后再取消纳管"
                    : "Pause scheduling on the node first to stop placing new RLark tasks before unmanaging"
                  : zh
                    ? "节点上还有运行中的任务，请等待任务完成或先驱逐任务"
                    : "There are still running jobs on the node. Wait for them to complete or drain first"
            }
            onClick={() => {
              setOpen(false);
              onUnmanage();
            }}
          >
            <Unlink2 size={14} />
            {zh ? "取消纳管" : "Unmanage"}
          </button>
        </div>
      )}
    </div>
  );
}

export function NodeResourceBrowser({
  nodes,
  nodeWorkloads = {},
  copy: c,
  onSelectNode,
  onRefresh,
  refreshing,
  onToggleScheduling,
  updatingNode,
  onUnmanageNode,
  onDrainNode,
  onUndrainNode,
  initialCategory = "all",
  initialQuery = "",
  selectedNodeKeys,
  onSelectionChange,
  onSelectFiltered,
  onClearSelection,
  compact,
}: {
  nodes: CRDNode[];
  nodeWorkloads?: Record<string, { jobs: string[]; workers: number }>;
  copy: Copy;
  onSelectNode: (name: string) => void;
  onRefresh?: () => void | Promise<void>;
  refreshing?: boolean;
  onToggleScheduling?: (node: CRDNode) => void;
  updatingNode?: string | null;
  onUnmanageNode?: (node: CRDNode) => void;
  onDrainNode?: (node: CRDNode) => void;
  onUndrainNode?: (node: CRDNode) => void;
  initialCategory?: CategoryFilter;
  initialQuery?: string;
  selectedNodeKeys?: Set<string>;
  onSelectionChange?: (keys: Set<string>) => void;
  onSelectFiltered?: (keys: string[]) => void;
  onClearSelection?: () => void;
  compact?: boolean;
}) {
  const zh = c.nav.overview === "总览";
  const [category, setCategory] = useState<CategoryFilter>(initialCategory);
  const [query, setQuery] = useState(initialQuery);
  const [typeFilter, setTypeFilter] = useState<string[]>([]);
  const [availabilityFilter, setAvailabilityFilter] = useState<string[]>([]);
  const [pressureFilter, setPressureFilter] = useState<string[]>([]);
  const [clusterFilter, setClusterFilter] = useState<string[]>([]);
  const [locationFilter, setLocationFilter] = useState<string[]>([]);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const { openKey, anchorRect, openFor, close } = useColumnFilter();

  const clusterOf = (node: CRDNode) =>
    node.metadata.namespace ??
    node.status?.observedNodeLabels?.["rlark.io/cluster-id"] ??
    "";
  const nodeKey = (node: CRDNode) =>
    `${node.metadata.namespace ?? ""}/${node.metadata.name}`;

  const categoryCounts = useMemo(() => {
    const counts: Record<CategoryFilter, number> = {
      all: nodes.length,
      cloud: 0,
      edge: 0,
      robot: 0,
      unknown: 0,
    };
    nodes.forEach((node) =>
      getNodeCategories(node).forEach((nodeCategory) => counts[nodeCategory]++),
    );
    return counts;
  }, [nodes]);

  const filteredNodes = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return nodes
      .filter((node) => {
        const address =
          node.status?.addresses?.find((item) => item.type === "InternalIP")
            ?.address ??
          node.status?.addresses?.[0]?.address ??
          "";
        const taskName = (
          nodeWorkloads[nodeWorkloadKey(node)]?.jobs ?? []
        ).join(" ");
        const location = getNodeLocation(node);
        const searchable =
          `${node.metadata.name} ${node.metadata.namespace ?? ""} ${node.spec.agentType ?? ""} ${address} ${taskName} ${location}`.toLowerCase();
        const availability = nodeAvailabilityState(node);
        const pressure = nodePressureState(node);
        return (
          (category === "all" || hasNodeCategory(node, category)) &&
          (typeFilter.length === 0 ||
            getNodeCategories(node).some((value) =>
              typeFilter.includes(value),
            )) &&
          (availabilityFilter.length === 0 ||
            availabilityFilter.includes(availability.kind)) &&
          (pressureFilter.length === 0 ||
            pressureFilter.includes(pressure.kind)) &&
          (clusterFilter.length === 0 ||
            clusterFilter.includes(clusterOf(node) || "—")) &&
          (locationFilter.length === 0 ||
            locationFilter.includes(location || "—")) &&
          (!normalizedQuery || searchable.includes(normalizedQuery))
        );
      })
      .sort((a, b) =>
        a.metadata.name.localeCompare(b.metadata.name, zh ? "zh-CN" : "en", {
          numeric: true,
        }),
      );
  }, [
    availabilityFilter,
    category,
    clusterFilter,
    locationFilter,
    nodeWorkloads,
    nodes,
    pressureFilter,
    query,
    typeFilter,
    zh,
  ]);

  const totalPages = Math.max(1, Math.ceil(filteredNodes.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pagedNodes = filteredNodes.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );
  const connectedCount = filteredNodes.filter(
    (node) => nodeConnectionState(node).kind === "connected",
  ).length;
  const selectable = Boolean(selectedNodeKeys && onSelectionChange);
  const pagedKeys = pagedNodes.map(nodeKey);
  const allPagedSelected =
    pagedKeys.length > 0 &&
    pagedKeys.every((key) => selectedNodeKeys?.has(key));

  useEffect(
    () => setPage(1),
    [
      availabilityFilter,
      category,
      clusterFilter,
      locationFilter,
      pageSize,
      pressureFilter,
      query,
      typeFilter,
    ],
  );
  useEffect(() => setCategory(initialCategory), [initialCategory]);
  useEffect(() => setQuery(initialQuery), [initialQuery]);

  const typeOptions = categoryOrder.map((value) => ({
    value,
    label: zh ? categoryLabels[value].zh : categoryLabels[value].en,
  }));
  const availabilityOptions = ["available", "unavailable"];
  const pressureOptions = ["pressure", "noPressure", "pressureUnknown"];
  const clusterOptions = useMemo(() => {
    const values = new Set<string>();
    nodes.forEach((node) => values.add(clusterOf(node) || "—"));
    return [...values]
      .sort((a, b) => a.localeCompare(b, zh ? "zh-CN" : "en"))
      .map((value) => ({ value, label: value }));
  }, [nodes, zh]);
  const locationOptions = useMemo(() => {
    const values = new Set<string>();
    nodes.forEach((node) => values.add(getNodeLocation(node) || "—"));
    return [...values]
      .sort((a, b) => a.localeCompare(b, zh ? "zh-CN" : "en"))
      .map((value) => ({ value, label: value }));
  }, [nodes, zh]);

  const tabs: Array<{
    value: CategoryFilter;
    label: string;
    icon: typeof LayoutGrid;
  }> = [
    { value: "all", label: zh ? "全部节点" : "All nodes", icon: LayoutGrid },
    ...categoryOrder.map((value) => ({
      value,
      label: zh ? categoryLabels[value].zh : categoryLabels[value].en,
      icon: categoryLabels[value].icon,
    })),
  ];

  return (
    <div className="node-resource-browser">
      <div
        className="node-category-tabs"
        role="tablist"
        aria-label={zh ? "节点类型" : "Node types"}
      >
        {tabs.map((tab) => {
          const Icon = tab.icon;
          const active = category === tab.value;
          return (
            <button
              key={tab.value}
              type="button"
              role="tab"
              aria-selected={active}
              className={`node-category-tab cat-${tab.value}${active ? " active" : ""}`}
              onClick={() => setCategory(tab.value)}
            >
              <span className="node-category-tab-icon">
                <Icon size={16} />
              </span>
              <span>{tab.label}</span>
              <b>{categoryCounts[tab.value]}</b>
            </button>
          );
        })}
      </div>

      <PageToolbar
        placeholder={
          zh
            ? "搜索节点名称、集群、IP 或任务名称..."
            : "Search node, cluster, IP or task..."
        }
        value={query}
        onChange={setQuery}
        count={filteredNodes.length}
        copy={c}
        onRefresh={onRefresh}
        refreshing={refreshing}
      />

      <section
        className={`panel node-resource-table-panel refreshable-region${refreshing ? " is-refreshing" : ""}`}
        aria-busy={refreshing}
      >
        <div className="node-resource-table-summary">
          <div>
            <strong>{tabs.find((tab) => tab.value === category)?.label}</strong>
            <small>
              {filteredNodes.length} {zh ? "个节点" : "nodes"} ·{" "}
              {connectedCount} {zh ? "已连接" : "connected"}
            </small>
          </div>
          {selectable ? (
            <div className="node-batch-selection-actions">
              <label className="node-batch-select-all">
                <input
                  type="checkbox"
                  checked={allPagedSelected}
                  onChange={() => {
                    if (!selectedNodeKeys || !onSelectionChange) return;
                    const next = new Set(selectedNodeKeys);
                    pagedKeys.forEach((key) =>
                      allPagedSelected ? next.delete(key) : next.add(key),
                    );
                    onSelectionChange(next);
                  }}
                />
                {zh ? "全选本页" : "Select page"}
              </label>
              <button
                type="button"
                className="plain-button"
                onClick={() => onSelectFiltered?.(pagedKeys)}
                disabled={pagedKeys.length === 0}
              >
                {zh ? "全选" : "Select all"}
              </button>
              <button
                type="button"
                className="plain-button"
                onClick={onClearSelection}
                disabled={!selectedNodeKeys?.size}
              >
                {zh ? "取消全选" : "Clear all"}
              </button>
            </div>
          ) : (
            <span>{zh ? "点击节点查看详情" : "Select a node for details"}</span>
          )}
        </div>
        <div className="node-resource-table">
          <div
            className={`node-resource-table-head${onToggleScheduling ? " has-admin-actions" : ""}${selectable ? " has-selection" : ""}${compact ? " compact" : ""}`}
          >
            {selectable && <span aria-label={zh ? "选择" : "Select"} />}
            {onToggleScheduling ? (
              <span className="node-scheduling-column">
                {zh ? "调度管理" : "Scheduling"}
              </span>
            ) : (
              <span aria-hidden="true" />
            )}
            <span>{zh ? "节点名称" : "Node"}</span>
            <ColumnFilterButton
              label={zh ? "类型" : "Type"}
              selectedCount={typeFilter.length}
              onClick={openFor("type")}
            />
            <ColumnFilterButton
              label={zh ? "可用性" : "Availability"}
              selectedCount={availabilityFilter.length}
              onClick={openFor("availability")}
            />
            {!compact && (
              <ColumnFilterButton
                label={zh ? "资源压力" : "Pressure"}
                selectedCount={pressureFilter.length}
                onClick={openFor("pressure")}
              />
            )}
            <span>{zh ? "说明" : "Notes"}</span>
            {!compact && (
              <ColumnFilterButton
                label={zh ? "所属集群" : "Cluster"}
                selectedCount={clusterFilter.length}
                onClick={openFor("cluster")}
              />
            )}
            <span>{zh ? "资源与空闲" : "Resources"}</span>
            {!compact && <span>{zh ? "任务数" : "Task Count"}</span>}
          </div>
          <div className="node-resource-table-body">
            {pagedNodes.map((node) => {
              const phase = (node.status?.phase ?? "Offline") as Phase;
              const availability = nodeAvailabilityState(node);
              const scheduling = nodeSchedulingState(node);
              const pressure = nodePressureState(node);
              const nodeCategories = getNodeCategories(node);
              const labels = node.status?.observedNodeLabels ?? {};
              const cluster =
                node.metadata.namespace ?? labels["rlark.io/cluster-id"] ?? "—";
              const workload = nodeWorkloads[nodeWorkloadKey(node)];
              const jobCount = workload?.jobs.length ?? 0;
              const resource = getNodeResourceSummary(node, zh);
              const key = nodeKey(node);
              const checked = selectedNodeKeys?.has(key) ?? false;
              const availabilityNote = nodeAvailabilityNote(node, zh);
              return (
                <div
                  className={`node-resource-row${onToggleScheduling ? " has-admin-actions" : ""}${selectable ? " has-selection" : ""}${compact ? " compact" : ""}${checked ? " selected" : ""}`}
                  key={key}
                >
                  {selectable && (
                    <span className="node-batch-checkbox">
                      <input
                        type="checkbox"
                        checked={checked}
                        aria-label={`${zh ? "选择节点" : "Select node"} ${node.metadata.name}`}
                        onClick={(event) => event.stopPropagation()}
                        onChange={() => {
                          if (!selectedNodeKeys || !onSelectionChange) return;
                          const next = new Set(selectedNodeKeys);
                          if (checked) next.delete(key);
                          else next.add(key);
                          onSelectionChange(next);
                        }}
                      />
                    </span>
                  )}
                  {onToggleScheduling ? (
                    <span className="node-scheduling-actions">
                      {onUnmanageNode ? (
                        <AdminNodeRowMenu
                          node={node}
                          schedulingKind={scheduling.kind}
                          updating={updatingNode === node.metadata.name}
                          zh={zh}
                          runningJobs={
                            node.status?.activeWorkloads?.rlarkManaged ?? 0
                          }
                          onToggleScheduling={() => onToggleScheduling(node)}
                          onDrain={
                            onDrainNode ? () => onDrainNode(node) : () => {}
                          }
                          onUndrain={
                            onUndrainNode ? () => onUndrainNode(node) : () => {}
                          }
                          onUnmanage={() => onUnmanageNode(node)}
                        />
                      ) : (
                        <button
                          type="button"
                          className={
                            node.spec.unschedulable
                              ? "secondary-button node-scheduling-button"
                              : "secondary-button danger node-scheduling-button"
                          }
                          disabled={
                            updatingNode === node.metadata.name ||
                            scheduling.kind === "draining"
                          }
                          onClick={(event) => {
                            event.stopPropagation();
                            onToggleScheduling(node);
                          }}
                        >
                          {node.spec.unschedulable ? (
                            <LockOpen size={14} />
                          ) : (
                            <Ban size={14} />
                          )}
                          {updatingNode === node.metadata.name
                            ? zh
                              ? "处理中"
                              : "Updating"
                            : node.spec.unschedulable
                              ? zh
                                ? "恢复调度"
                                : "Resume Schedule"
                              : zh
                                ? "暂停调度"
                                : "Pause Schedule"}
                        </button>
                      )}
                    </span>
                  ) : (
                    <ChevronRight size={15} className="node-row-chevron" />
                  )}
                  <button
                    type="button"
                    className="node-row-primary node-detail-link"
                    onClick={() => onSelectNode(node.metadata.name)}
                    aria-label={`${zh ? "查看节点" : "View node"} ${node.metadata.name}`}
                  >
                    <span
                      className={`node-status-ring ${phase.toLowerCase()}`}
                    />
                    <strong className="node-row-name">
                      {node.metadata.name}
                    </strong>
                  </button>
                  <span className="node-type-list">
                    {nodeCategories.map((item) => {
                      const info = categoryLabels[item];
                      const Icon = info.icon;
                      return (
                        <span
                          key={item}
                          className={`node-type-cell cat-${item}`}
                        >
                          <Icon size={13} />
                          {zh ? info.zh : info.en}
                        </span>
                      );
                    })}
                  </span>
                  <AvailabilityBadge availability={availability} zh={zh} />
                  {!compact && <StateBadge kind={pressure.kind} zh={zh} />}
                  <span
                    className="node-row-meta"
                    title={
                      availabilityNote === "—" ? undefined : availabilityNote
                    }
                  >
                    {availabilityNote}
                  </span>
                  {!compact && (
                    <span className="node-row-meta" title={cluster}>
                      {cluster}
                    </span>
                  )}
                  <span
                    className="node-row-resource"
                    title={resource.lines
                      .map((line) => `${line.secondary} ${line.primary}`)
                      .join("\n")}
                  >
                    {resource.lines.length ? (
                      resource.lines.map((line) => (
                        <span
                          className={`node-row-resource-line ${line.kind}`}
                          key={line.key}
                        >
                          <strong
                            className={
                              line.label === (zh ? "未标注" : "Unlabeled")
                                ? "unlabeled"
                                : undefined
                            }
                          >
                            {line.label}
                          </strong>
                          <i aria-hidden="true">·</i>
                          <small>{line.amount}</small>
                        </span>
                      ))
                    ) : (
                      <small>{resource.primary}</small>
                    )}
                  </span>
                  {!compact && (
                    <span
                      className="node-row-task"
                      title={workload?.jobs.join(zh ? "、" : ", ") ?? ""}
                    >
                      {jobCount > 0 && (
                        <>
                          <i className="embodied-task-dot active" />
                          {jobCount}
                        </>
                      )}
                    </span>
                  )}
                </div>
              );
            })}
            {pagedNodes.length === 0 && (
              <div className="node-resource-empty">
                <LayoutGrid size={22} />
                <strong>
                  {zh ? "没有符合条件的节点" : "No matching nodes"}
                </strong>
                <small>
                  {zh
                    ? "尝试切换分类或清除筛选条件"
                    : "Try another category or filter"}
                </small>
              </div>
            )}
          </div>
        </div>
        <RefreshOverlay
          visible={!!refreshing}
          label={zh ? "正在刷新全部节点" : "Refreshing all nodes"}
        />
      </section>

      <Pagination
        page={currentPage}
        pageSize={pageSize}
        total={filteredNodes.length}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
        zh={zh}
      />

      {openKey === "type" && (
        <ColumnFilterPopover
          label={zh ? "类型" : "Type"}
          options={typeOptions}
          selected={typeFilter}
          onChange={setTypeFilter}
          anchorRect={anchorRect}
          onClose={close}
          zh={zh}
        />
      )}
      {openKey === "availability" && (
        <ColumnFilterPopover
          label={zh ? "可用性" : "Availability"}
          options={availabilityOptions.map((value) => ({
            value,
            label: nodeStateLabel(value as NodeStateKind, zh),
          }))}
          selected={availabilityFilter}
          onChange={setAvailabilityFilter}
          anchorRect={anchorRect}
          onClose={close}
          zh={zh}
        />
      )}
      {openKey === "pressure" && (
        <ColumnFilterPopover
          label={zh ? "资源压力" : "Pressure"}
          options={pressureOptions.map((value) => ({
            value,
            label: nodeStateLabel(value as NodeStateKind, zh),
          }))}
          selected={pressureFilter}
          onChange={setPressureFilter}
          anchorRect={anchorRect}
          onClose={close}
          zh={zh}
        />
      )}
      {openKey === "cluster" && (
        <ColumnFilterPopover
          label={zh ? "所属集群" : "Cluster"}
          options={clusterOptions}
          selected={clusterFilter}
          onChange={setClusterFilter}
          anchorRect={anchorRect}
          onClose={close}
          zh={zh}
        />
      )}
      {openKey === "location" && (
        <ColumnFilterPopover
          label={zh ? "物理位置" : "Location"}
          options={locationOptions}
          selected={locationFilter}
          onChange={setLocationFilter}
          anchorRect={anchorRect}
          onClose={close}
          zh={zh}
        />
      )}
    </div>
  );
}
