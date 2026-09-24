import { useEffect, useMemo, useState } from "react";
import { Ban, ChevronRight, LayoutGrid, LockOpen } from "lucide-react";
import type { Phase } from "../data";
import type { Copy } from "../i18n";
import type { CRDNode, NodeCategory } from "../types";
import {
  categoryLabels,
  getNodeCategories,
  getNodeLocation,
  getNodeResourceSummary,
  hasNodeCategory,
} from "../utils/nodes";
import {
  ColumnFilterButton,
  PageToolbar,
  Pagination,
  RefreshOverlay,
  StatusBadge,
  useColumnFilter,
} from "./shared";
import { ColumnFilterPopover } from "./ColumnFilterPopover";

type CategoryFilter = "all" | NodeCategory;

const categoryOrder: NodeCategory[] = ["cloud", "edge", "robot", "unknown"];

export function NodeResourceBrowser({
  nodes,
  nodeWorkloads = {},
  copy: c,
  onSelectNode,
  onRefresh,
  refreshing,
  onToggleScheduling,
  updatingNode,
  initialCategory = "all",
  initialQuery = "",
  selectedNodeKeys,
  onSelectionChange,
  onSelectFiltered,
  onClearSelection,
}: {
  nodes: CRDNode[];
  nodeWorkloads?: Record<string, { jobs: string[]; workers: number }>;
  copy: Copy;
  onSelectNode: (name: string) => void;
  onRefresh?: () => void | Promise<void>;
  refreshing?: boolean;
  onToggleScheduling?: (node: CRDNode) => void;
  updatingNode?: string | null;
  initialCategory?: CategoryFilter;
  initialQuery?: string;
  selectedNodeKeys?: Set<string>;
  onSelectionChange?: (keys: Set<string>) => void;
  onSelectFiltered?: (keys: string[]) => void;
  onClearSelection?: () => void;
}) {
  const zh = c.nav.overview === "总览";
  const [category, setCategory] = useState<CategoryFilter>(initialCategory);
  const [query, setQuery] = useState(initialQuery);
  // 表头列多选筛选；空数组 = 全部
  const [typeFilter, setTypeFilter] = useState<string[]>([]);
  const [phaseFilter, setPhaseFilter] = useState<string[]>([]);
  const [clusterFilter, setClusterFilter] = useState<string[]>([]);
  const [locationFilter, setLocationFilter] = useState<string[]>([]);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const { openKey, anchorRect, openFor, close } = useColumnFilter();

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

  // 节点集群归属（与表头"所属集群"一致）
  const clusterOf = (node: CRDNode) =>
    node.metadata.namespace ??
    node.metadata.labels?.["rlark.io/cluster-id"] ??
    "";

  const filteredNodes = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return nodes
      .filter((node) => {
        const address =
          node.status?.addresses?.find((item) => item.type === "InternalIP")
            ?.address ??
          node.status?.addresses?.[0]?.address ??
          "";
        const taskName = (nodeWorkloads[node.metadata.name]?.jobs ?? []).join(
          " ",
        );
        const location = getNodeLocation(node);
        const searchable =
          `${node.metadata.name} ${node.metadata.namespace ?? ""} ${node.spec.agentType ?? ""} ${address} ${taskName} ${location}`.toLowerCase();
        const phase = (node.status?.phase ?? "Offline") as Phase;
        const typeHit =
          typeFilter.length === 0 ||
          getNodeCategories(node).some((c) => typeFilter.includes(c));
        const phaseHit =
          phaseFilter.length === 0 || phaseFilter.includes(phase);
        const clusterHit =
          clusterFilter.length === 0 ||
          clusterFilter.includes(clusterOf(node) || "—");
        const locationHit =
          locationFilter.length === 0 ||
          locationFilter.includes(location || "—");
        return (
          (category === "all" || hasNodeCategory(node, category)) &&
          typeHit &&
          phaseHit &&
          clusterHit &&
          locationHit &&
          (!normalizedQuery || searchable.includes(normalizedQuery))
        );
      })
      .sort((a, b) =>
        a.metadata.name.localeCompare(b.metadata.name, zh ? "zh-CN" : "en", {
          numeric: true,
        }),
      );
  }, [
    category,
    clusterFilter,
    locationFilter,
    nodeWorkloads,
    nodes,
    phaseFilter,
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
  const onlineCount = filteredNodes.filter(
    (node) => node.status?.phase === "Online",
  ).length;
  const selectable = Boolean(selectedNodeKeys && onSelectionChange);
  const nodeKey = (node: CRDNode) =>
    `${node.metadata.namespace ?? ""}/${node.metadata.name}`;
  const filteredKeys = filteredNodes.map(nodeKey);
  const allFilteredSelected =
    filteredKeys.length > 0 &&
    filteredKeys.every((key) => selectedNodeKeys?.has(key));
  const toggleAllFiltered = () => {
    if (!selectedNodeKeys || !onSelectionChange) return;
    const next = new Set(selectedNodeKeys);
    filteredKeys.forEach((key) =>
      allFilteredSelected ? next.delete(key) : next.add(key),
    );
    onSelectionChange(next);
  };

  useEffect(
    () => setPage(1),
    [
      category,
      clusterFilter,
      locationFilter,
      pageSize,
      phaseFilter,
      query,
      typeFilter,
    ],
  );
  useEffect(() => setCategory(initialCategory), [initialCategory]);
  useEffect(() => setQuery(initialQuery), [initialQuery]);

  // 表头筛选选项：从当前节点集合去重
  const typeOptions = categoryOrder.map((v) => ({
    value: v,
    label: zh ? categoryLabels[v].zh : categoryLabels[v].en,
  }));
  // 状态选项：从当前节点集合去重（节点实际只有 Online/Offline，但保留弹性）
  const phaseOptions = useMemo(() => {
    const labelOf = (p: string) =>
      p === "Online" ? c.status.Online : p === "Offline" ? c.status.Offline : p;
    const set = new Set<string>();
    nodes.forEach((n) => set.add(n.status?.phase ?? "Offline"));
    return [...set].sort().map((v) => ({ value: v, label: labelOf(v) }));
  }, [nodes, c]);
  const clusterOptions = useMemo(() => {
    const set = new Set<string>();
    nodes.forEach((n) => set.add(clusterOf(n) || "—"));
    return [...set]
      .sort((a, b) => a.localeCompare(b, zh ? "zh-CN" : "en"))
      .map((v) => ({ value: v, label: v }));
  }, [nodes, zh]);
  const locationOptions = useMemo(() => {
    const set = new Set<string>();
    nodes.forEach((n) => set.add(getNodeLocation(n) || "—"));
    return [...set]
      .sort((a, b) => a.localeCompare(b, zh ? "zh-CN" : "en"))
      .map((v) => ({ value: v, label: v }));
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
              {filteredNodes.length} {zh ? "个节点" : "nodes"} · {onlineCount}{" "}
              {zh ? "在线" : "online"}
            </small>
          </div>
          {selectable ? (
            <div className="node-batch-selection-actions">
              <label className="node-batch-select-all">
                <input
                  type="checkbox"
                  checked={allFilteredSelected}
                  onChange={toggleAllFiltered}
                />
                {zh ? "全选筛选结果" : "Select filtered"}
              </label>
              <button
                type="button"
                className="plain-button"
                onClick={() => onSelectFiltered?.(filteredKeys)}
                disabled={filteredKeys.length === 0}
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
            className={`node-resource-table-head${onToggleScheduling ? " has-admin-actions" : ""}${selectable ? " has-selection" : ""}`}
          >
            {selectable && <span aria-label={zh ? "选择" : "Select"} />}
            <span>{zh ? "节点名称" : "Node"}</span>
            <ColumnFilterButton
              label={zh ? "类型" : "Type"}
              selectedCount={typeFilter.length}
              onClick={openFor("type")}
            />
            <ColumnFilterButton
              label={zh ? "状态" : "Status"}
              selectedCount={phaseFilter.length}
              onClick={openFor("phase")}
            />
            <ColumnFilterButton
              label={zh ? "所属集群" : "Cluster"}
              selectedCount={clusterFilter.length}
              onClick={openFor("cluster")}
            />
            <ColumnFilterButton
              label={zh ? "物理位置" : "Location"}
              selectedCount={locationFilter.length}
              onClick={openFor("location")}
            />
            <span>{zh ? "节点 IP" : "Node IP"}</span>
            <span>{zh ? "资源与空闲" : "Resources"}</span>
            <span>{zh ? "任务" : "Task"}</span>
            <span>
              {onToggleScheduling ? (zh ? "调度管理" : "Scheduling") : ""}
            </span>
          </div>
          <div className="node-resource-table-body">
            {pagedNodes.map((node) => {
              const phase = (node.status?.phase ?? "Offline") as Phase;
              const nodeCategories = getNodeCategories(node);
              const labels = node.metadata.labels ?? {};
              const cluster =
                node.metadata.namespace ?? labels["rlark.io/cluster-id"] ?? "—";
              const address =
                node.status?.addresses?.find(
                  (item) => item.type === "InternalIP",
                )?.address ??
                node.status?.addresses?.[0]?.address ??
                "—";
              const workload = nodeWorkloads[node.metadata.name];
              const jobCount = workload?.jobs.length ?? 0;
              const workerCount = workload?.workers ?? 0;
              const taskSummary = jobCount
                ? zh
                  ? `${jobCount} 个任务 · ${workerCount} 个 Worker`
                  : `${jobCount} jobs · ${workerCount} workers`
                : zh
                  ? "无"
                  : "None";
              const location = getNodeLocation(node) || "—";
              const resource = getNodeResourceSummary(node, zh);
              const key = nodeKey(node);
              const checked = selectedNodeKeys?.has(key) ?? false;
              return (
                <div
                  className={`node-resource-row${onToggleScheduling ? " has-admin-actions" : ""}${selectable ? " has-selection" : ""}${checked ? " selected" : ""}`}
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
                  <span>
                    <StatusBadge phase={phase} copy={c} />
                  </span>
                  <span className="node-row-meta" title={cluster}>
                    {cluster}
                  </span>
                  <span className="node-row-location" title={location}>
                    {location}
                  </span>
                  <code className="node-row-ip">{address}</code>
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
                  <span
                    className="node-row-task"
                    title={workload?.jobs.join("、") ?? ""}
                  >
                    <i
                      className={`embodied-task-dot ${jobCount ? "active" : "idle"}`}
                    />
                    {taskSummary}
                  </span>
                  {onToggleScheduling ? (
                    <span className="node-scheduling-actions">
                      <span
                        className={`node-scheduling-state${node.spec.unschedulable ? " cordoned" : ""}`}
                      >
                        {node.spec.unschedulable
                          ? zh
                            ? "已封锁"
                            : "Cordoned"
                          : zh
                            ? "可调度"
                            : "Schedulable"}
                      </span>
                      <button
                        type="button"
                        className={
                          node.spec.unschedulable
                            ? "secondary-button node-scheduling-button"
                            : "secondary-button danger node-scheduling-button"
                        }
                        disabled={updatingNode === node.metadata.name}
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
                              ? "解封"
                              : "Uncordon"
                            : zh
                              ? "封锁"
                              : "Cordon"}
                      </button>
                    </span>
                  ) : (
                    <ChevronRight size={15} className="node-row-chevron" />
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
      {openKey === "phase" && (
        <ColumnFilterPopover
          label={zh ? "状态" : "Status"}
          options={phaseOptions}
          selected={phaseFilter}
          onChange={setPhaseFilter}
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
