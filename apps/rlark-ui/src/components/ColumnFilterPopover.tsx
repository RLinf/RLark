import { useEffect, useRef } from "react";

interface ColumnFilterPopoverProps {
  label: string;
  options: Array<{ value: string; label: string }>;
  selected: string[];
  onChange: (selected: string[]) => void;
  anchorRect: DOMRect | null;
  onClose: () => void;
  zh?: boolean;
}

// 表头列多选筛选弹层。单栏 checkbox 列表，与 TagFilterPopover 风格一致
// 但只服务"单值多选"场景（状态、类型、集群等），不需要 key→values 双栏。
export function ColumnFilterPopover({
  label,
  options,
  selected,
  onChange,
  anchorRect,
  onClose,
  zh = true,
}: ColumnFilterPopoverProps) {
  const popoverRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!popoverRef.current) return;
    const handler = (e: MouseEvent) => {
      if (!popoverRef.current?.contains(e.target as Node)) onClose();
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [onClose]);

  const style: React.CSSProperties = {};
  if (anchorRect) {
    const popoverWidth = 240;
    const left = Math.min(
      Math.max(anchorRect.left, 16),
      window.innerWidth - popoverWidth - 16,
    );
    style.left = left;
    style.top = anchorRect.bottom + 6;
  }

  const allSelected = options.length > 0 && selected.length === options.length;
  const noneSelected = selected.length === 0;

  const toggleValue = (v: string) => {
    if (selected.includes(v)) {
      onChange(selected.filter((x) => x !== v));
    } else {
      onChange([...selected, v]);
    }
  };

  const toggleAll = () => {
    if (allSelected) {
      onChange([]);
    } else {
      onChange(options.map((o) => o.value));
    }
  };

  const reset = () => onChange([]);

  return (
    <div
      ref={popoverRef}
      className="col-filter-popover"
      style={style}
      role="dialog"
      aria-label={label}
    >
      <div className="col-filter-head">
        <span className="col-filter-title">{label}</span>
        <button
          type="button"
          className="col-filter-link-btn"
          onClick={toggleAll}
        >
          {allSelected ? (zh ? "清空" : "Clear") : zh ? "全选" : "Select all"}
        </button>
      </div>
      <div className="col-filter-list">
        {options.length === 0 ? (
          <div className="col-filter-empty">
            {zh ? "暂无可选项" : "No options"}
          </div>
        ) : (
          options.map((opt) => {
            const checked = selected.includes(opt.value);
            return (
              <label
                key={opt.value}
                className={`col-filter-item${checked ? " selected" : ""}`}
              >
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={() => toggleValue(opt.value)}
                />
                <span>{opt.label}</span>
              </label>
            );
          })
        )}
      </div>
      <div className="col-filter-foot">
        <button
          type="button"
          className="secondary-button col-filter-reset"
          onClick={reset}
          disabled={noneSelected}
        >
          {zh ? "重置" : "Reset"}
        </button>
        <button
          type="button"
          className="primary-button col-filter-confirm"
          onClick={onClose}
        >
          {zh ? "确定" : "OK"}
        </button>
      </div>
    </div>
  );
}
