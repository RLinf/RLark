import { useEffect, useRef, useState } from "react";
import { ChevronDown, X } from "lucide-react";
import type { CRDNodeLite } from "../types";
import { parseNodeSelectorStr, selectorToStr } from "../utils/job";

export function NodeSelectorPicker({
  value,
  onChange,
  zh,
  onMatchedCount,
  onMaxGPU,
  cluster,
  nodes,
  loading,
}: {
  value: string;
  onChange: (v: string) => void;
  zh: boolean;
  onMatchedCount?: (n: number) => void;
  onMaxGPU?: (gpu: number) => void;
  cluster?: string;
  nodes: CRDNodeLite[];
  loading: boolean;
}) {
  const [open, setOpen] = useState(false);
  const pickerRef = useRef<HTMLDivElement>(null);
  const selectorMap = parseNodeSelectorStr(value);

  const clusterNodes = cluster
    ? nodes.filter((n) => n.metadata.namespace === cluster)
    : nodes;

  const labelMap: Record<string, Set<string>> = {};
  for (const n of clusterNodes) {
    const labels = n.metadata.labels ?? {};
    for (const [k, v] of Object.entries(labels)) {
      if (!k.startsWith("kubernetes.io/") && !k.startsWith("rlark.io/"))
        continue;
      if (k === "rlark.io/cluster-id") continue;
      if (!labelMap[k]) labelMap[k] = new Set();
      labelMap[k].add(v);
    }
  }

  const matchedNodes = clusterNodes.filter((n) => {
    const labels = n.metadata.labels ?? {};
    return Object.entries(selectorMap).every(([k, v]) => {
      const values = v.split(",");
      return labels[k] !== undefined && values.includes(labels[k]);
    });
  });

  const matchedCount =
    Object.keys(selectorMap).length === 0
      ? clusterNodes.length
      : matchedNodes.length;

  const maxGPU = (() => {
    const nodesToCheck =
      Object.keys(selectorMap).length === 0 ? clusterNodes : matchedNodes;
    let min = Infinity;
    for (const n of nodesToCheck) {
      const g = n.status?.allocatable?.["nvidia.com/gpu"];
      if (g !== undefined) {
        const num = parseInt(g, 10);
        if (!isNaN(num) && num < min) min = num;
      }
    }
    return min === Infinity ? 0 : min;
  })();

  useEffect(() => {
    onMatchedCount?.(matchedCount);
  }, [matchedCount]);

  useEffect(() => {
    onMaxGPU?.(maxGPU);
  }, [maxGPU]);

  useEffect(() => {
    if (!open) return;

    const handlePointerDown = (event: PointerEvent) => {
      if (!pickerRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };

    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [open]);

  const toggleLabel = (key: string, val: string) => {
    const next = { ...selectorMap };
    const current = next[key] ? next[key].split(",") : [];
    const idx = current.indexOf(val);
    if (idx >= 0) {
      current.splice(idx, 1);
    } else {
      current.push(val);
    }
    if (current.length === 0) {
      delete next[key];
    } else {
      next[key] = current.join(",");
    }
    onChange(selectorToStr(next));
  };

  const removeLabel = (key: string) => {
    const next = { ...selectorMap };
    delete next[key];
    onChange(selectorToStr(next));
  };

  const labelKeys = Object.keys(labelMap).sort();

  return (
    <div className="node-selector-picker" ref={pickerRef}>
      <div
        className="selector-chips-area"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        aria-haspopup="listbox"
        role="button"
        tabIndex={0}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            setOpen(!open);
          }
        }}
      >
        {Object.keys(selectorMap).length === 0 ? (
          <span className="selector-placeholder">
            {zh ? "点击选择节点标签…" : "Click to select node labels…"}
          </span>
        ) : (
          Object.entries(selectorMap).map(([k, v]) => (
            <span
              key={k}
              className="selector-chip"
              onClick={(e) => {
                e.stopPropagation();
                removeLabel(k);
              }}
            >
              {k}={v}
              <X size={12} />
            </span>
          ))
        )}
        <ChevronDown size={14} className="selector-chevron" />
      </div>
      {open && (
        <div className="selector-dropdown">
          {loading ? (
            <div className="selector-loading">
              {zh ? "加载中…" : "Loading…"}
            </div>
          ) : labelKeys.length === 0 ? (
            <div className="selector-empty">
              {zh ? "暂无节点标签数据" : "No node labels available"}
            </div>
          ) : (
            labelKeys.map((key) => (
              <div key={key} className="selector-group">
                <div className="selector-group-head">
                  <code>{key}</code>
                </div>
                <div className="selector-group-values">
                  {Array.from(labelMap[key])
                    .sort()
                    .map((val) => (
                      <button
                        key={val}
                        className={
                          "selector-value-chip " +
                          ((selectorMap[key] ?? "").split(",").includes(val)
                            ? "active"
                            : "")
                        }
                        onClick={(e) => {
                          e.stopPropagation();
                          toggleLabel(key, val);
                        }}
                      >
                        {val}
                      </button>
                    ))}
                </div>
              </div>
            ))
          )}
        </div>
      )}
      <input
        className="selector-text-input"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={
          zh
            ? "或手动输入，如 gpu=h800,robot=online"
            : "Or type manually, e.g. gpu=h800,robot=online"
        }
      />
      {!loading &&
        Object.keys(selectorMap).length === 0 &&
        labelKeys.length > 0 && (
          <div className="selector-hint">
            {zh
              ? "选择标签可筛选出一批满足条件的节点；非必选，不选则匹配集群内所有节点；选择多个标签取交集"
              : "Select labels to filter nodes meeting all conditions. Optional — no selection matches all nodes. Multiple labels are intersected."}
          </div>
        )}
      {!loading &&
        Object.keys(selectorMap).length > 0 &&
        (matchedNodes.length > 0 ? (
          <div className="selector-matched selector-matched-inline">
            <div className="selector-matched-head">
              {zh
                ? `匹配节点 (${matchedNodes.length})`
                : `Matched nodes (${matchedNodes.length})`}
            </div>
            <div className="selector-matched-list">
              {matchedNodes.map((n) => (
                <div key={n.metadata.name} className="selector-matched-node">
                  <span
                    className={
                      "node-dot " + (n.status?.phase ?? "").toLowerCase()
                    }
                  />
                  <code>{n.metadata.name}</code>
                </div>
              ))}
            </div>
          </div>
        ) : (
          labelKeys.length > 0 && (
            <div className="selector-no-match">
              {zh ? "⚠ 没有匹配的节点" : "⚠ No matching nodes"}
            </div>
          )
        ))}
    </div>
  );
}

export function RoleNameInput({
  role,
  onRename,
}: {
  role: string;
  onRename: (old: string, newName: string) => void;
}) {
  const [draft, setDraft] = useState(role);
  useEffect(() => setDraft(role), [role]);
  return (
    <input
      value={draft}
      onClick={(e) => e.stopPropagation()}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={() => {
        const trimmed = draft.trim();
        if (trimmed && trimmed !== role) onRename(role, trimmed);
        else setDraft(role);
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          (e.target as HTMLInputElement).blur();
        }
      }}
    />
  );
}
