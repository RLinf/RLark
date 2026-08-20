import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { Check, CircleAlert, MapPin } from "lucide-react";
import type { CRDNodeLite } from "../types";
import {
  getGPUResourceKey,
  getNodeDeviceModel,
  getNodeGPUModel,
  getNodeLocation,
} from "../utils/nodes";
import { selectorToStr } from "../utils/job";

type Mode = "model" | "manual";
type ResourceKind = "gpu" | "device" | "compute";

type NodeResource = {
  node: CRDNodeLite;
  kind: ResourceKind;
  model: string;
  resourceKey: string;
  total: number;
  free: number;
};

type ResourceOption = {
  id: string;
  kind: ResourceKind;
  model: string;
  resources: NodeResource[];
  totalDevices: number;
  freeDevices: number;
  totalNodes: number;
  availableNodes: number;
};

const isSchedulable = (resource: NodeResource) =>
  resource.node.status?.phase !== "Offline" &&
  !resource.node.spec?.unschedulable;

const quantity = (value?: string) => {
  const parsed = Number.parseFloat(value ?? "0");
  return Number.isFinite(parsed) ? parsed : 0;
};

const requestedResourceAmount = (gpu: string, device?: string) => {
  const parsed = Number.parseInt(device ?? gpu, 10);
  return Number.isFinite(parsed) ? Math.max(0, parsed) : 1;
};

function nodeResource(node: CRDNodeLite): NodeResource | null {
  const fullNode = node as Parameters<typeof getGPUResourceKey>[0];
  const capacity = node.status?.capacity ?? node.status?.allocatable ?? {};
  const allocatable = node.status?.allocatable ?? capacity;
  const used = node.status?.used ?? {};
  const gpuKey = getGPUResourceKey(fullNode);
  const gpuTotal = quantity(capacity[gpuKey] ?? allocatable[gpuKey]);
  if (gpuTotal > 0) {
    return {
      node,
      kind: "gpu",
      model: getNodeGPUModel(fullNode) || "未标注 GPU",
      resourceKey: gpuKey,
      total: gpuTotal,
      free: Math.max(0, quantity(allocatable[gpuKey]) - quantity(used[gpuKey])),
    };
  }

  const deviceKey = Array.from(
    new Set([...Object.keys(capacity), ...Object.keys(allocatable)]),
  ).find(
    (key) => key === "rlinf.io/device" || key.startsWith("rlinf.io/device-"),
  );
  if (!deviceKey) {
    return {
      node,
      kind: "compute",
      model: "CPU",
      resourceKey: "",
      total: 1,
      free:
        node.status?.phase !== "Offline" && !node.spec?.unschedulable ? 1 : 0,
    };
  }
  const deviceTotal = quantity(capacity[deviceKey] ?? allocatable[deviceKey]);
  if (deviceTotal <= 0) return null;
  return {
    node,
    kind: "device",
    model: getNodeDeviceModel(fullNode) || deviceKey,
    resourceKey: deviceKey,
    total: deviceTotal,
    free: Math.max(
      0,
      quantity(allocatable[deviceKey]) - quantity(used[deviceKey]),
    ),
  };
}

export function ResourcePlacementPicker({
  zh,
  cluster,
  nodes,
  loading,
  replicas,
  gpu,
  devices,
  onChange,
}: {
  zh: boolean;
  cluster: string;
  nodes: CRDNodeLite[];
  loading: boolean;
  replicas: number;
  gpu: string;
  devices: Array<{ name: string; quantity: string }>;
  onChange: (next: {
    nodeSelector: string;
    replicas: number;
    gpu: string;
    devices: Array<{ name: string; quantity: string }>;
  }) => void;
}) {
  const pickerId = useId();
  const [mode, setMode] = useState<Mode>("model");
  const [selectedOptionId, setSelectedOptionId] = useState("");
  const [resourceAmount, setResourceAmount] = useState(() =>
    requestedResourceAmount(gpu, devices[0]?.quantity),
  );
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const gridRef = useRef<HTMLDivElement>(null);
  const dragStartRef = useRef<{
    x: number;
    y: number;
    initial: Set<string>;
    targetName: string | null;
  } | null>(null);
  const pendingSelectionRef = useRef<Set<string> | null>(null);
  const [selectionBox, setSelectionBox] = useState<{
    left: number;
    top: number;
    width: number;
    height: number;
  } | null>(null);
  const externalDeviceAmount = devices[0]?.quantity;

  useEffect(() => {
    const nextAmount = requestedResourceAmount(gpu, externalDeviceAmount);
    setResourceAmount(nextAmount);
  }, [externalDeviceAmount, gpu]);

  const resources = useMemo(
    () =>
      nodes
        .filter((node) => node.metadata.namespace === cluster)
        .map(nodeResource)
        .filter((item): item is NodeResource => item !== null),
    [cluster, nodes],
  );
  const resourceOptions = useMemo<ResourceOption[]>(() => {
    const groups = new Map<string, NodeResource[]>();
    resources.forEach((resource) => {
      const id = `${resource.kind}:${resource.model}`;
      const group = groups.get(id) ?? [];
      group.push(resource);
      groups.set(id, group);
    });
    return Array.from(groups, ([id, groupedResources]) => ({
      id,
      kind: groupedResources[0].kind,
      model: groupedResources[0].model,
      resources: groupedResources,
      totalDevices: groupedResources.reduce(
        (sum, resource) => sum + resource.total,
        0,
      ),
      freeDevices: groupedResources
        .filter(isSchedulable)
        .reduce((sum, resource) => sum + resource.free, 0),
      totalNodes: groupedResources.length,
      availableNodes: groupedResources.filter(
        (resource) => isSchedulable(resource) && resource.free > 0,
      ).length,
    })).sort((left, right) => {
      const order: Record<ResourceKind, number> = {
        gpu: 0,
        device: 1,
        compute: 2,
      };
      if (left.kind !== right.kind) return order[left.kind] - order[right.kind];
      return left.model.localeCompare(right.model);
    });
  }, [resources]);

  useEffect(() => {
    if (!resourceOptions.some((option) => option.id === selectedOptionId)) {
      setSelectedOptionId(resourceOptions[0]?.id ?? "");
    }
  }, [resourceOptions, selectedOptionId]);

  const selectedOption = resourceOptions.find(
    (option) => option.id === selectedOptionId,
  );
  const kind = selectedOption?.kind ?? "compute";
  const model = selectedOption?.model ?? "";
  const resourceUnit =
    kind === "gpu"
      ? zh
        ? "个 GPU"
        : resourceAmount === 1
          ? "GPU"
          : "GPUs"
      : kind === "device"
        ? zh
          ? "个设备"
          : resourceAmount === 1
            ? "device"
            : "devices"
        : zh
          ? "个计算槽"
          : resourceAmount === 1
            ? "compute slot"
            : "compute slots";
  const candidates = selectedOption?.resources ?? [];
  const free = selectedOption?.freeDevices ?? 0;
  const requested = replicas * resourceAmount;
  const eligibleNames = candidates
    .filter(
      (resource) => resource.free >= resourceAmount && isSchedulable(resource),
    )
    .map((resource) => resource.node.metadata.name);

  const commitModel = (nextReplicas: number, nextPerWorker: number) => {
    const names = candidates
      .filter(
        (resource) => resource.free >= nextPerWorker && isSchedulable(resource),
      )
      .map((resource) => resource.node.metadata.name);
    onChange({
      nodeSelector: names.length
        ? selectorToStr({ "kubernetes.io/hostname": names.join(",") })
        : "",
      replicas: nextReplicas,
      gpu: kind === "gpu" ? String(nextPerWorker) : "0",
      devices:
        kind === "device" && candidates[0]
          ? [
              {
                name: candidates[0].resourceKey,
                quantity: String(nextPerWorker),
              },
            ]
          : [],
    });
  };

  useEffect(() => {
    if (!model) return;
    if (mode === "model") {
      commitModel(Math.max(1, replicas), resourceAmount);
    } else {
      onChange({
        nodeSelector: "",
        replicas: 0,
        gpu: kind === "gpu" ? String(resourceAmount) : "0",
        devices:
          kind === "device" && candidates[0]
            ? [
                {
                  name: candidates[0].resourceKey,
                  quantity: String(resourceAmount),
                },
              ]
            : [],
      });
    }
    // Rebuild the hostname selector whenever the chosen resource pool changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cluster, selectedOptionId]);

  useEffect(() => {
    setSelected(new Set());
  }, [cluster, selectedOptionId]);

  const chooseResourceOption = (optionId: string) => {
    setSelectedOptionId(optionId);
    setSelected(new Set());
  };

  const commitSelection = (next: Set<string>) => {
    setSelected(next);
    const selectedResources = candidates.filter((item) =>
      next.has(item.node.metadata.name),
    );
    const resourceKey =
      selectedResources[0]?.resourceKey ?? candidates[0]?.resourceKey;
    onChange({
      nodeSelector: next.size
        ? selectorToStr({
            "kubernetes.io/hostname": Array.from(next).join(","),
          })
        : "",
      replicas: next.size,
      gpu: kind === "gpu" ? String(resourceAmount) : "0",
      devices:
        kind === "device" && resourceKey
          ? [{ name: resourceKey, quantity: String(resourceAmount) }]
          : [],
    });
  };

  const handleSelectionStart = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0 || !gridRef.current) return;
    event.preventDefault();
    dragStartRef.current = {
      x: event.clientX,
      y: event.clientY,
      initial: new Set(selected),
      targetName:
        (event.target as HTMLElement).closest<HTMLButtonElement>(
          ".placement-node-card:not(:disabled)",
        )?.dataset.nodeName ?? null,
    };
    pendingSelectionRef.current = new Set(selected);
    const gridRect = gridRef.current.getBoundingClientRect();
    setSelectionBox({
      left: event.clientX - gridRect.left,
      top: event.clientY - gridRect.top,
      width: 0,
      height: 0,
    });
  };

  const handleSelectionMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const start = dragStartRef.current;
    const grid = gridRef.current;
    if (!start || !grid) return;
    if (
      Math.hypot(event.clientX - start.x, event.clientY - start.y) >= 4 &&
      !event.currentTarget.hasPointerCapture(event.pointerId)
    ) {
      event.currentTarget.setPointerCapture(event.pointerId);
    }
    const gridRect = grid.getBoundingClientRect();
    const leftClient = Math.min(start.x, event.clientX);
    const rightClient = Math.max(start.x, event.clientX);
    const topClient = Math.min(start.y, event.clientY);
    const bottomClient = Math.max(start.y, event.clientY);
    setSelectionBox({
      left: leftClient - gridRect.left,
      top: topClient - gridRect.top,
      width: rightClient - leftClient,
      height: bottomClient - topClient,
    });

    const next = new Set(start.initial);
    grid
      .querySelectorAll<HTMLButtonElement>(
        ".placement-node-card:not(:disabled)",
      )
      .forEach((card) => {
        const rect = card.getBoundingClientRect();
        const intersects =
          rect.left < rightClient &&
          rect.right > leftClient &&
          rect.top < bottomClient &&
          rect.bottom > topClient;
        if (!intersects) return;
        const name = card.dataset.nodeName;
        if (!name) return;
        if (start.initial.has(name)) next.delete(name);
        else next.add(name);
      });
    pendingSelectionRef.current = next;
  };

  const handleSelectionEnd = (event: ReactPointerEvent<HTMLDivElement>) => {
    const start = dragStartRef.current;
    if (!start) return;
    const moved = Math.hypot(event.clientX - start.x, event.clientY - start.y);
    if (moved < 4) {
      const name = start.targetName;
      if (name) {
        const next = new Set(start.initial);
        if (next.has(name)) next.delete(name);
        else next.add(name);
        commitSelection(next);
      }
    } else if (pendingSelectionRef.current) {
      commitSelection(pendingSelectionRef.current);
    }
    dragStartRef.current = null;
    pendingSelectionRef.current = null;
    setSelectionBox(null);
  };

  return (
    <section className="placement-picker">
      {loading ? (
        <div className="placement-empty">
          {zh ? "正在加载节点资源…" : "Loading resources…"}
        </div>
      ) : resourceOptions.length === 0 ? (
        <div className="placement-empty">
          {zh
            ? "该集群暂无匹配的可调度资源"
            : "No matching schedulable resources"}
        </div>
      ) : (
        <>
          <div className="placement-resource-config">
            <fieldset className="placement-resource-options">
              <legend>
                {zh ? "选择设备规格" : "Select device specification"}
              </legend>
              <div
                className="placement-resource-option-head"
                aria-hidden="true"
              >
                <span>{zh ? "资源类型与型号" : "Resource and model"}</span>
                <span>
                  {zh
                    ? "资源槽位 可用 / 总量"
                    : "Resource slots available / total"}
                </span>
                <span>
                  {zh ? "节点 可用 / 总量" : "Nodes available / total"}
                </span>
              </div>
              {resourceOptions.map((option) => (
                <label
                  key={option.id}
                  className={
                    selectedOptionId === option.id ? "active" : undefined
                  }
                >
                  <input
                    type="radio"
                    name={`${pickerId}-resource-option`}
                    value={option.id}
                    checked={selectedOptionId === option.id}
                    onChange={() => chooseResourceOption(option.id)}
                  />
                  <span className="placement-resource-option-name">
                    <small>
                      {option.kind === "gpu"
                        ? "GPU"
                        : option.kind === "device"
                          ? zh
                            ? "端侧设备"
                            : "Edge device"
                          : zh
                            ? "通用计算"
                            : "General compute"}
                    </small>
                    <strong>{option.model}</strong>
                  </span>
                  <span className="placement-resource-option-stat">
                    <strong>{option.freeDevices}</strong>
                    <small>/ {option.totalDevices}</small>
                  </span>
                  <span className="placement-resource-option-stat">
                    <strong>{option.availableNodes}</strong>
                    <small>/ {option.totalNodes}</small>
                  </span>
                </label>
              ))}
            </fieldset>
          </div>

          <div className="placement-plan-controls">
            {kind !== "compute" && (
              <label className="placement-field">
                <span className="placement-field-label">
                  {zh
                    ? `单 Worker ${kind === "gpu" ? "GPU" : "设备"}数量`
                    : "Resources per worker"}
                </span>
                <input
                  type="number"
                  min={0}
                  value={resourceAmount}
                  onChange={(event) => {
                    const nextAmount = Math.max(0, Number(event.target.value));
                    setResourceAmount(nextAmount);
                    if (mode === "model") {
                      commitModel(Math.max(1, replicas), nextAmount);
                    } else {
                      setSelected(new Set());
                      onChange({
                        nodeSelector: "",
                        replicas: 0,
                        gpu: kind === "gpu" ? String(nextAmount) : "0",
                        devices:
                          kind === "device" && candidates[0]
                            ? [
                                {
                                  name: candidates[0].resourceKey,
                                  quantity: String(nextAmount),
                                },
                              ]
                            : [],
                      });
                    }
                  }}
                />
                <small>
                  {zh
                    ? "自动选择和指定节点共用此规格"
                    : "Shared by automatic selection and node selection modes"}
                </small>
              </label>
            )}

            <fieldset className="placement-scheduling-choice">
              <legend>{zh ? "调度方式" : "Scheduling mode"}</legend>
              <div>
                <label className={mode === "model" ? "active" : ""}>
                  <input
                    type="radio"
                    name={`${pickerId}-placement-mode`}
                    checked={mode === "model"}
                    onChange={() => {
                      setMode("model");
                      commitModel(
                        Math.max(1, replicas || selected.size),
                        resourceAmount,
                      );
                    }}
                  />
                  <span>{zh ? "自动选择" : "Automatic selection"}</span>
                </label>
                <label className={mode === "manual" ? "active" : ""}>
                  <input
                    type="radio"
                    name={`${pickerId}-placement-mode`}
                    checked={mode === "manual"}
                    onChange={() => {
                      setMode("manual");
                      setSelected(new Set());
                      onChange({
                        nodeSelector: "",
                        replicas: 0,
                        gpu: kind === "gpu" ? String(resourceAmount) : "0",
                        devices:
                          kind === "device" && candidates[0]
                            ? [
                                {
                                  name: candidates[0].resourceKey,
                                  quantity: String(resourceAmount),
                                },
                              ]
                            : [],
                      });
                    }}
                  />
                  <span>{zh ? "指定节点" : "Select nodes"}</span>
                </label>
              </div>
              <small>
                {mode === "model"
                  ? zh
                    ? "平台按规格自动选择可调度节点"
                    : "The platform selects eligible nodes"
                  : zh
                    ? "每个选中节点创建一个 Worker"
                    : "Each selected node creates one Worker"}
              </small>
            </fieldset>

            {mode === "model" ? (
              <label className="placement-field">
                <span className="placement-field-label">
                  {zh ? "Worker 数量" : "Workers"}
                </span>
                <input
                  type="number"
                  min={1}
                  value={replicas || 1}
                  onChange={(event) =>
                    commitModel(
                      Math.max(1, Number(event.target.value)),
                      resourceAmount,
                    )
                  }
                />
                <small>
                  {zh ? "手动填写需要创建的数量" : "Enter the desired count"}
                </small>
              </label>
            ) : (
              <div className="placement-derived-workers">
                <span className="placement-field-label">
                  {zh ? "Worker 数量" : "Workers"}
                </span>
                <div>
                  <strong>{selected.size}</strong>
                  <small>{zh ? "由已选节点生成" : "From selected nodes"}</small>
                </div>
                <small>
                  {zh ? "在下方点击或框选节点" : "Select nodes below"}
                </small>
              </div>
            )}
          </div>

          {mode === "manual" && (
            <div>
              <p className="placement-manual-hint">
                {zh
                  ? `仅显示空闲数量不少于 ${resourceAmount} ${resourceUnit}${kind === "gpu" ? " " : ""}的节点。点击单选，拖拽可批量选择或取消。`
                  : `Showing nodes with at least ${resourceAmount} ${resourceUnit} available. Click or drag to select and deselect.`}
              </p>
              <div
                className="placement-node-grid"
                ref={gridRef}
                onPointerDown={handleSelectionStart}
                onPointerMove={handleSelectionMove}
                onPointerUp={handleSelectionEnd}
                onPointerCancel={handleSelectionEnd}
              >
                {selectionBox && (
                  <div
                    className="placement-selection-box"
                    style={selectionBox}
                    aria-hidden="true"
                  />
                )}
                {candidates
                  .filter((resource) => resource.free >= resourceAmount)
                  .map((resource) => {
                    const name = resource.node.metadata.name;
                    const offline = resource.node.status?.phase === "Offline";
                    const cordoned = Boolean(resource.node.spec?.unschedulable);
                    const disabled = offline || cordoned;
                    const active = selected.has(name);
                    const location =
                      getNodeLocation(
                        resource.node as Parameters<typeof getNodeLocation>[0],
                      ) || (zh ? "位置未标注" : "Location not set");
                    const statusText = offline
                      ? zh
                        ? "节点离线"
                        : "Node offline"
                      : cordoned
                        ? zh
                          ? "已暂停调度"
                          : "Scheduling disabled"
                        : zh
                          ? "可调度"
                          : "Schedulable";
                    return (
                      <button
                        type="button"
                        key={name}
                        data-node-name={name}
                        disabled={disabled}
                        className={`placement-node-card${active ? " active" : ""}${disabled ? " unavailable" : ""}`}
                      >
                        <div className="placement-node-head">
                          <strong>{name}</strong>
                          <em>{statusText}</em>
                        </div>
                        <span className="placement-node-location">
                          <MapPin size={12} /> {location}
                        </span>
                        <div className="placement-node-specs">
                          <span>{resource.model}</span>
                          <span>
                            <b>{resource.free}</b> / {resource.total}
                            {zh ? " 空闲" : " free"}
                          </span>
                        </div>
                        <span className="placement-node-check">
                          {active && <Check size={14} />}
                        </span>
                        {disabled && (
                          <CircleAlert
                            className="placement-node-alert"
                            size={15}
                          />
                        )}
                      </button>
                    );
                  })}
              </div>
            </div>
          )}
          <div className="placement-selection-summary">
            <div>
              <strong>
                {kind === "compute"
                  ? mode === "model"
                    ? zh
                      ? `已配置 ${Math.max(1, replicas)} 个 Worker`
                      : `${Math.max(1, replicas)} workers configured`
                    : zh
                      ? `已选择 ${selected.size} 个节点，将创建 ${selected.size} 个 Worker`
                      : `${selected.size} nodes selected, ${selected.size} workers`
                  : mode === "model"
                    ? zh
                      ? `已配置 ${Math.max(1, replicas)} 个 Worker，共申请 ${requested} 个${kind === "gpu" ? " GPU" : "设备"}`
                      : `${Math.max(1, replicas)} workers configured, ${requested} resources requested`
                    : zh
                      ? `已选择 ${selected.size} 个节点，将创建 ${selected.size} 个 Worker，共申请 ${selected.size * resourceAmount} 个${kind === "gpu" ? " GPU" : "设备"}`
                      : `${selected.size} nodes selected, ${selected.size} workers, ${selected.size * resourceAmount} resources requested`}
              </strong>
              <small>
                {kind === "compute"
                  ? zh
                    ? `通用计算 · ${model}`
                    : `General compute · ${model}`
                  : `${zh ? "单 Worker" : "Per worker"}：${resourceAmount} × ${model}`}
              </small>
            </div>
            {mode === "model" ? (
              <div
                className={`placement-summary-validation${requested > free || eligibleNames.length === 0 ? " invalid" : ""}`}
              >
                <strong>
                  {requested > free || eligibleNames.length === 0
                    ? zh
                      ? "资源不足"
                      : "Unavailable"
                    : zh
                      ? "资源充足"
                      : "Available"}
                </strong>
                <small>
                  {requested > free
                    ? zh
                      ? `申请量超过可用数量 ${free}`
                      : `Request exceeds ${free} available`
                    : eligibleNames.length === 0
                      ? zh
                        ? "没有单节点可满足该规格"
                        : "No node satisfies this request"
                      : zh
                        ? `${eligibleNames.length} 个节点可承载`
                        : `${eligibleNames.length} eligible nodes`}
                </small>
              </div>
            ) : (
              <button
                type="button"
                onClick={() => {
                  setSelected(new Set());
                  onChange({
                    nodeSelector: "",
                    replicas: 0,
                    gpu: "0",
                    devices: [],
                  });
                }}
              >
                {zh ? "清空选择" : "Clear"}
              </button>
            )}
          </div>
        </>
      )}
    </section>
  );
}
