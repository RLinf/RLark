import { useEffect, useMemo, useState } from "react";
import { ChevronRight, LayoutGrid } from "lucide-react";
import type { Phase } from "../data";
import type { Copy } from "../i18n";
import type { CRDNode, NodeCategory } from "../types";
import {
  categoryLabels,
  getNodeCategory,
  getNodeLocation,
  getNodeResourceSummary,
} from "../utils/nodes";
import {
  compareSortValues,
  PageToolbar,
  Pagination,
  SortButton,
  StatusBadge,
  type SortDirection,
} from "./shared";

type CategoryFilter = "all" | NodeCategory;

const categoryOrder: NodeCategory[] = ["cloud", "edge", "robot", "unknown"];

export function NodeResourceBrowser({
  nodes,
  copy: c,
  onSelectNode,
  onRefresh,
  initialCategory = "all",
  initialQuery = "",
}: {
  nodes: CRDNode[];
  copy: Copy;
  onSelectNode: (name: string) => void;
  onRefresh?: () => void;
  initialCategory?: CategoryFilter;
  initialQuery?: string;
}) {
  const zh = c.nav.overview === "总览";
  const [category, setCategory] = useState<CategoryFilter>(initialCategory);
  const [query, setQuery] = useState(initialQuery);
  const [phaseFilter, setPhaseFilter] = useState<"All" | Phase>("All");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [sort, setSort] = useState<{
    key:
      | "name"
      | "type"
      | "phase"
      | "cluster"
      | "location"
      | "ip"
      | "resource"
      | "task";
    direction: SortDirection;
  }>({ key: "cluster", direction: "asc" });
  const toggleSort = (key: typeof sort.key) =>
    setSort((current) => ({
      key,
      direction:
        current.key === key && current.direction === "asc" ? "desc" : "asc",
    }));

  const categoryCounts = useMemo(() => {
    const counts: Record<CategoryFilter, number> = {
      all: nodes.length,
      cloud: 0,
      edge: 0,
      robot: 0,
      unknown: 0,
    };
    nodes.forEach((node) => counts[getNodeCategory(node)]++);
    return counts;
  }, [nodes]);

  const filteredNodes = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return nodes
      .filter((node) => {
        const labels = node.metadata.labels ?? {};
        const address =
          node.status?.addresses?.find((item) => item.type === "InternalIP")
            ?.address ??
          node.status?.addresses?.[0]?.address ??
          "";
        const taskName =
          labels["rlark.io/embodied-task-name"] ??
          labels["rlark.io/task-name"] ??
          "";
        const location = getNodeLocation(node);
        const searchable =
          `${node.metadata.name} ${node.metadata.namespace ?? ""} ${node.spec.agentType ?? ""} ${address} ${taskName} ${location}`.toLowerCase();
        const phase = (node.status?.phase ?? "Offline") as Phase;
        return (
          (category === "all" || getNodeCategory(node) === category) &&
          (phaseFilter === "All" || phase === phaseFilter) &&
          (!normalizedQuery || searchable.includes(normalizedQuery))
        );
      })
      .sort((a, b) => {
        const value = (node: CRDNode): string | number => {
          const labels = node.metadata.labels ?? {};
          const address =
            node.status?.addresses?.find((item) => item.type === "InternalIP")
              ?.address ??
            node.status?.addresses?.[0]?.address ??
            "";
          const task =
            labels["rlark.io/embodied-task-name"] ??
            labels["rlark.io/task-name"] ??
            "";
          if (sort.key === "name") return node.metadata.name;
          if (sort.key === "type") return getNodeCategory(node);
          if (sort.key === "phase") return node.status?.phase ?? "Offline";
          if (sort.key === "cluster")
            return (
              node.metadata.namespace ?? labels["rlark.io/cluster-id"] ?? ""
            );
          if (sort.key === "location") return getNodeLocation(node);
          if (sort.key === "ip") return address;
          if (sort.key === "resource")
            return (
              Number.parseFloat(getNodeResourceSummary(node, zh).primary) || 0
            );
          return task;
        };
        const order = compareSortValues(
          value(a),
          value(b),
          sort.direction,
          zh ? "zh-CN" : "en",
        );
        return (
          order ||
          a.metadata.name.localeCompare(b.metadata.name, zh ? "zh-CN" : "en", {
            numeric: true,
          })
        );
      });
  }, [category, nodes, phaseFilter, query, sort, zh]);

  const totalPages = Math.max(1, Math.ceil(filteredNodes.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pagedNodes = filteredNodes.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );
  const onlineCount = filteredNodes.filter(
    (node) => node.status?.phase === "Online",
  ).length;

  useEffect(() => setPage(1), [category, pageSize, phaseFilter, query]);
  useEffect(() => setCategory(initialCategory), [initialCategory]);
  useEffect(() => setQuery(initialQuery), [initialQuery]);

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
        filterValue={phaseFilter}
        onFilterChange={(value) => setPhaseFilter(value as "All" | Phase)}
        filterOptions={[
          { value: "All", label: zh ? "全部状态" : "All statuses" },
          { value: "Online", label: c.status.Online },
          { value: "Offline", label: c.status.Offline },
        ]}
      />

      <section className="panel node-resource-table-panel">
        <div className="node-resource-table-summary">
          <div>
            <strong>{tabs.find((tab) => tab.value === category)?.label}</strong>
            <small>
              {filteredNodes.length} {zh ? "个节点" : "nodes"} · {onlineCount}{" "}
              {zh ? "在线" : "online"}
            </small>
          </div>
          <span>{zh ? "点击节点查看详情" : "Select a node for details"}</span>
        </div>
        <div className="node-resource-table">
          <div className="node-resource-table-head">
            <SortButton
              label={zh ? "节点名称" : "Node"}
              active={sort.key === "name"}
              direction={sort.direction}
              onClick={() => toggleSort("name")}
            />
            <SortButton
              label={zh ? "类型" : "Type"}
              active={sort.key === "type"}
              direction={sort.direction}
              onClick={() => toggleSort("type")}
            />
            <SortButton
              label={zh ? "状态" : "Status"}
              active={sort.key === "phase"}
              direction={sort.direction}
              onClick={() => toggleSort("phase")}
            />
            <SortButton
              label={zh ? "所属集群" : "Cluster"}
              active={sort.key === "cluster"}
              direction={sort.direction}
              onClick={() => toggleSort("cluster")}
            />
            <SortButton
              label={zh ? "物理位置" : "Location"}
              active={sort.key === "location"}
              direction={sort.direction}
              onClick={() => toggleSort("location")}
            />
            <SortButton
              label={zh ? "节点 IP" : "Node IP"}
              active={sort.key === "ip"}
              direction={sort.direction}
              onClick={() => toggleSort("ip")}
            />
            <SortButton
              label={zh ? "资源与空闲" : "Resources"}
              active={sort.key === "resource"}
              direction={sort.direction}
              onClick={() => toggleSort("resource")}
            />
            <SortButton
              label={zh ? "任务" : "Task"}
              active={sort.key === "task"}
              direction={sort.direction}
              onClick={() => toggleSort("task")}
            />
            <span />
          </div>
          <div className="node-resource-table-body">
            {pagedNodes.map((node) => {
              const phase = (node.status?.phase ?? "Offline") as Phase;
              const nodeCategory = getNodeCategory(node);
              const categoryInfo = categoryLabels[nodeCategory];
              const CategoryIcon = categoryInfo.icon;
              const labels = node.metadata.labels ?? {};
              const cluster =
                node.metadata.namespace ?? labels["rlark.io/cluster-id"] ?? "—";
              const address =
                node.status?.addresses?.find(
                  (item) => item.type === "InternalIP",
                )?.address ??
                node.status?.addresses?.[0]?.address ??
                "—";
              const taskName =
                labels["rlark.io/embodied-task-name"] ??
                labels["rlark.io/task-name"] ??
                "";
              const hasEmbodiedTask =
                labels["rlark.io/embodied-task"] === "true" ||
                Boolean(taskName);
              const location = getNodeLocation(node) || "—";
              const resource = getNodeResourceSummary(node, zh);
              return (
                <button
                  type="button"
                  className="node-resource-row"
                  key={`${node.metadata.namespace ?? ""}/${node.metadata.name}`}
                  onClick={() => onSelectNode(node.metadata.name)}
                  aria-label={`${zh ? "查看节点" : "View node"} ${node.metadata.name}`}
                >
                  <span className="node-row-primary">
                    <span
                      className={`node-status-ring ${phase.toLowerCase()}`}
                    />
                    <strong className="node-row-name">
                      {node.metadata.name}
                    </strong>
                  </span>
                  <span className={`node-type-cell cat-${nodeCategory}`}>
                    <CategoryIcon size={14} />
                    {zh ? categoryInfo.zh : categoryInfo.en}
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
                    title={`${resource.primary} ${resource.secondary}`}
                  >
                    <strong>{resource.primary}</strong>
                    <small>{resource.secondary}</small>
                  </span>
                  <span className="node-row-task" title={taskName}>
                    <i
                      className={`embodied-task-dot ${hasEmbodiedTask ? "active" : "idle"}`}
                    />
                    {taskName || (zh ? "无" : "None")}
                  </span>
                  <ChevronRight size={15} className="node-row-chevron" />
                </button>
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
      </section>

      <Pagination
        page={currentPage}
        pageSize={pageSize}
        total={filteredNodes.length}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
        zh={zh}
      />
    </div>
  );
}
