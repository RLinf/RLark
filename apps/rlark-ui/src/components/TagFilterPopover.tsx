import { useEffect, useMemo, useRef, useState } from "react";
import { Search } from "lucide-react";

interface TagFilterPopoverProps {
  /** 当前所有可用标签（来自全量 jobs） */
  allTags: Array<{ key: string; values: string[] }>;
  /** 当前选中的筛选条件：key -> values[] 的映射 */
  selection: Record<string, string[]>;
  /** 选择变更回调 */
  onChange: (selection: Record<string, string[]>) => void;
  /** 重置筛选条件 */
  onReset: () => void;
  /** 是否包含该标签的任务被视为命中 */
  zh?: boolean;
  /** 触发器元素（通常是表头 label），用于定位弹层 */
  anchorRect: DOMRect | null;
  /** 关闭时调用 */
  onClose: () => void;
}

export function TagFilterPopover({
  allTags,
  selection,
  onChange,
  onReset,
  zh = true,
  anchorRect,
  onClose,
}: TagFilterPopoverProps) {
  const [search, setSearch] = useState("");
  // 右栏展示哪个 key 的 value 列表。只由点击切换：悬停切换太灵敏，指针从左栏
  // 移向右栏的途中会扫过其他 key，右栏内容跟着跳变，下方 key 的 value 就勾不中。
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const [valuePage, setValuePage] = useState(1);
  const popoverRef = useRef<HTMLDivElement>(null);

  // 按 key 分组并固定 key/value 的展示顺序。
  const grouped = useMemo(() => {
    const map = new Map<string, string[]>();
    for (const tag of allTags) {
      const values = map.get(tag.key) ?? [];
      for (const value of tag.values) {
        if (!values.includes(value)) values.push(value);
      }
      map.set(tag.key, values);
    }
    return new Map(
      [...map]
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, values]) => [
          key,
          values.sort((left, right) => left.localeCompare(right)),
        ]),
    );
  }, [allTags]);

  // 应用搜索过滤（key 或 value 模糊匹配）
  const filteredKeys = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return [...grouped.keys()];
    const result: string[] = [];
    for (const [k, vals] of grouped) {
      if (
        k.toLowerCase().includes(q) ||
        vals.some((v) => v.toLowerCase().includes(q))
      ) {
        result.push(k);
      }
    }
    return result;
  }, [grouped, search]);

  // 点击外部关闭
  useEffect(() => {
    if (!popoverRef.current) return;
    const handler = (e: MouseEvent) => {
      if (!popoverRef.current?.contains(e.target as Node)) onClose();
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [onClose]);

  const keySelected = (k: string) =>
    selection[k] !== undefined && selection[k].length > 0;

  const toggleKeyAll = (k: string) => {
    const allVals = grouped.get(k) ?? [];
    const existing = selection[k] ?? [];
    const newSel = { ...selection };
    // 如果已经全选了则取消全选
    if (existing.length === allVals.length) {
      delete newSel[k];
    } else {
      newSel[k] = allVals;
    }
    onChange(newSel);
  };

  const toggleValue = (k: string, v: string) => {
    const existing = selection[k] ?? [];
    const newSel = { ...selection };
    if (existing.includes(v)) {
      const rest = existing.filter((x) => x !== v);
      if (rest.length === 0) delete newSel[k];
      else newSel[k] = rest;
    } else {
      newSel[k] = [...existing, v];
    }
    onChange(newSel);
  };

  const reset = () => {
    onChange({});
    onReset();
  };

  // 计算定位位置
  const style: React.CSSProperties = {};
  if (anchorRect) {
    const popoverWidth = 440;
    const left = Math.min(
      Math.max(anchorRect.left, 16),
      window.innerWidth - popoverWidth - 16,
    );
    style.left = left;
    style.top = anchorRect.bottom + 6;
  }

  // activeKey 可能因搜索过滤而不在当前列表中，此时才回退到已选中的 key。
  const effectiveKey =
    activeKey && filteredKeys.includes(activeKey)
      ? activeKey
      : (filteredKeys.find(keySelected) ?? null);
  const selectedValues = useMemo(
    () => (effectiveKey ? (grouped.get(effectiveKey) ?? []) : []),
    [effectiveKey, grouped],
  );
  // 已选中 key 时，搜索同时筛选该 key 下不符合的 value
  const matchedValues = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return selectedValues;
    return selectedValues.filter((v) => v.toLowerCase().includes(q));
  }, [selectedValues, search]);
  const selectedValsForKey = effectiveKey
    ? (selection[effectiveKey] ?? [])
    : [];
  const valuePageSize = 10;
  const valuePageCount = Math.max(
    1,
    Math.ceil(matchedValues.length / valuePageSize),
  );
  const currentValuePage = Math.min(valuePage, valuePageCount);
  const pagedValues = matchedValues.slice(
    (currentValuePage - 1) * valuePageSize,
    currentValuePage * valuePageSize,
  );

  useEffect(() => {
    setValuePage(1);
  }, [effectiveKey, search]);

  return (
    <div
      ref={popoverRef}
      className="tag-filter-popover"
      style={style}
      role="dialog"
      aria-label={zh ? "标签筛选" : "Tag filter"}
    >
      <div className="tag-filter-head">
        <div className="tag-filter-search">
          <Search size={14} />
          <input
            type="text"
            placeholder={zh ? "搜索 key 或 value" : "Search key or value"}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <p className="tag-filter-hint">
          {zh
            ? "多个标签 key 之间为「且」，同一 key 下的多个值之间为「或」。"
            : "Different keys are ANDed; values within the same key are ORed."}
        </p>
      </div>
      <div className="tag-filter-body">
        {/* 左栏：key 列表 */}
        <div className="tag-filter-keys">
          <div className="tag-filter-col-head">
            <span className="tag-filter-col-title">
              {zh ? "标签 key" : "Keys"}
            </span>
          </div>
          {filteredKeys.length === 0 ? (
            <div className="tag-filter-empty">
              {zh ? "没有匹配的标签" : "No matching tags"}
            </div>
          ) : (
            filteredKeys.map((k) => {
              const totalVals = grouped.get(k)?.length ?? 0;
              const selVals = selection[k]?.length ?? 0;
              return (
                <div
                  key={k}
                  className={`tag-filter-key-item${keySelected(k) ? " selected" : ""}${effectiveKey === k ? " active" : ""}`}
                >
                  <input
                    type="checkbox"
                    aria-label={
                      zh ? `全选 ${k} 的值` : `Select all values of ${k}`
                    }
                    checked={keySelected(k)}
                    onChange={() => {
                      setActiveKey(k);
                      toggleKeyAll(k);
                    }}
                  />
                  <button
                    type="button"
                    className="tag-filter-key-main"
                    aria-pressed={effectiveKey === k}
                    onClick={() => setActiveKey(k)}
                  >
                    <span className="tag-filter-key-name">{k}</span>
                    <small className="tag-filter-key-count">
                      {selVals > 0 ? `${selVals}/${totalVals}` : `${totalVals}`}
                    </small>
                  </button>
                </div>
              );
            })
          )}
        </div>

        {/* 右栏：选中 key 的 values */}
        <div className="tag-filter-values">
          <div className="tag-filter-col-head">
            <span className="tag-filter-col-title">
              {effectiveKey
                ? zh
                  ? `标签值 — ${effectiveKey}`
                  : `Values — ${effectiveKey}`
                : zh
                  ? "标签值"
                  : "Values"}
            </span>
            {effectiveKey && selectedValues.length > 0 && (
              <button
                type="button"
                className="tag-filter-link-btn"
                onClick={() => toggleKeyAll(effectiveKey)}
              >
                {zh ? "全选/取消" : "All / None"}
              </button>
            )}
          </div>
          {!effectiveKey || selectedValues.length === 0 ? (
            <div className="tag-filter-empty">
              {zh
                ? "点击左侧的 key 查看对应的值"
                : "Click a key on the left to see its values"}
            </div>
          ) : matchedValues.length === 0 ? (
            <div className="tag-filter-empty">
              {zh ? "没有匹配的值" : "No matching values"}
            </div>
          ) : (
            <>
              {pagedValues.map((v) => (
                <label key={v} className="tag-filter-value-item">
                  <input
                    type="checkbox"
                    checked={selectedValsForKey.includes(v)}
                    onChange={() => toggleValue(effectiveKey, v)}
                  />
                  <span>{v}</span>
                </label>
              ))}
              {valuePageCount > 1 && (
                <div className="tag-filter-value-pagination">
                  <button
                    type="button"
                    aria-label={zh ? "上一页" : "Previous page"}
                    disabled={currentValuePage === 1}
                    onClick={() => setValuePage(currentValuePage - 1)}
                  >
                    {zh ? "上一页" : "Prev"}
                  </button>
                  <span>
                    {currentValuePage} / {valuePageCount}
                  </span>
                  <button
                    type="button"
                    aria-label={zh ? "下一页" : "Next page"}
                    disabled={currentValuePage === valuePageCount}
                    onClick={() => setValuePage(currentValuePage + 1)}
                  >
                    {zh ? "下一页" : "Next"}
                  </button>
                </div>
              )}
            </>
          )}
        </div>
      </div>
      <div className="tag-filter-foot">
        <button
          type="button"
          className="secondary-button tag-filter-reset"
          onClick={reset}
        >
          {zh ? "重置" : "Reset"}
        </button>
        <button
          type="button"
          className="primary-button tag-filter-confirm"
          onClick={onClose}
        >
          {zh ? "确定" : "OK"}
        </button>
      </div>
    </div>
  );
}
