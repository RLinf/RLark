import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { ChevronDown, Plus, Trash2, X } from "lucide-react";
import type { JobTag } from "../data";

export interface TagSuggestion {
  key: string;
  values: string[];
}

const MAX_TAGS = 10;
const MAX_VALUES_PER_KEY = 10;
const MAX_LEN = 10;

function newTagId(): string {
  const bytes = new Uint8Array(4);
  crypto.getRandomValues(bytes);
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join(
    "",
  );
  return `tag-new-${hex}`;
}

interface TagEditRow {
  id: string;
  key: string;
  values: string[];
  input: string;
  existing?: boolean;
  /** 该行立即占用一个标签计数："添加标签"按钮创建的行与默认空行 */
  counted?: boolean;
}

function blankRow(counted = false): TagEditRow {
  return {
    id: newTagId(),
    key: "",
    values: [],
    input: "",
    existing: false,
    counted,
  };
}

function collectKeys(suggestions: TagSuggestion[]): string[] {
  return suggestions.map((suggestion) => suggestion.key);
}

function collectValuesForKey(
  suggestions: TagSuggestion[],
  key: string,
): string[] {
  return suggestions.find((suggestion) => suggestion.key === key)?.values ?? [];
}

interface TagEditorProps {
  tags: JobTag[];
  onChange: (tags: JobTag[]) => void;
  suggestions?: TagSuggestion[];
  zh?: boolean;
  compact?: boolean;
  sectioned?: boolean;
}

export function TagEditor({
  tags,
  onChange,
  suggestions = [],
  zh = true,
  compact = false,
  sectioned = false,
}: TagEditorProps) {
  const allSuggestionKeys = collectKeys(suggestions);
  const [editRows, setEditRows] = useState<TagEditRow[]>(() => {
    const initial = rowsFromTags(tags);
    // 默认空行与已有标签一并计数：已有标签已达上限时不再展示默认空行
    return sectioned && initial.length < MAX_TAGS
      ? [...initial, blankRow(true)]
      : initial;
  });

  useEffect(() => {
    setEditRows((rows) => {
      if (rows.length > 0) return rows;
      const next = rowsFromTags(tags);
      return sectioned && next.length > 0 && next.length < MAX_TAGS
        ? [...next, blankRow(true)]
        : next;
    });
  }, [tags, sectioned]);

  const [errorMsg, setErrorMsg] = useState("");
  const errorTimerRef = useRef<number | undefined>(undefined);

  const showError = (msg: string) => {
    setErrorMsg(msg);
    if (errorTimerRef.current) window.clearTimeout(errorTimerRef.current);
    errorTimerRef.current = window.setTimeout(() => setErrorMsg(""), 3000);
  };

  const toTags = (rows: typeof editRows): JobTag[] =>
    rows.flatMap((row) =>
      row.values.map((value) => ({
        id: `${row.id}-${value}`,
        key: row.key.trim(),
        value,
      })),
    );

  const tagKeyCount = (rows: typeof editRows) =>
    new Set(
      rows
        .filter((row) => row.key.trim() && row.values.length > 0)
        .map((row) => row.key.trim()),
    ).size;

  // 已占用的标签槽位：添加按钮创建的行立即计数，其余行填写后计数
  const tagSlotCount = (rows: typeof editRows) =>
    rows.filter((row) => row.counted || row.key.trim() || row.values.length > 0)
      .length;

  const isDuplicateKey = (rows: typeof editRows, id: string) => {
    const row = rows.find((r) => r.id === id);
    if (!row || !row.key.trim()) return false;
    return rows.some((r) => r.id !== id && r.key.trim() === row.key.trim());
  };

  const applyChanges = (nextRows: typeof editRows) => {
    const validRows = nextRows.filter(
      (row) => row.key.trim() && row.values.length,
    );
    const valid = toTags(validRows);
    if (tagKeyCount(validRows) > MAX_TAGS) {
      showError(
        zh
          ? `一个任务最多添加 ${MAX_TAGS} 个标签。`
          : `A job can have at most ${MAX_TAGS} tags.`,
      );
      return false;
    }
    // 安全兜底：有效行之间不允许存在重复的标签键
    const validKeyCounts = new Map<string, number>();
    for (const row of validRows) {
      const k = row.key.trim();
      validKeyCounts.set(k, (validKeyCounts.get(k) ?? 0) + 1);
    }
    for (const [k, count] of validKeyCounts) {
      if (count > 1) {
        showError(
          zh
            ? `标签键「${k}」重复，请先修改。`
            : `Tag key "${k}" is duplicated.`,
        );
        return false;
      }
    }
    for (const row of validRows) {
      if (row.values.length > MAX_VALUES_PER_KEY) {
        showError(
          zh
            ? `标签键「${row.key.trim()}」的标签值不能超过 ${MAX_VALUES_PER_KEY} 个。`
            : `Tag key "${row.key.trim()}" can have at most ${MAX_VALUES_PER_KEY} values.`,
        );
        return false;
      }
    }
    onChange(valid);
    return true;
  };

  const availableKeys = (currentRowId: string) => {
    const usedKeys = new Set(
      editRows
        .filter((row) => row.id !== currentRowId)
        .map((row) => row.key.trim())
        .filter(Boolean),
    );
    return allSuggestionKeys.filter((key) => !usedKeys.has(key));
  };

  const availableValuesFor = (key: string) => {
    const values = collectValuesForKey(suggestions, key);
    const row = editRows.find((item) => item.key.trim() === key.trim());
    return [...new Set([...values, ...(row?.values ?? [])])];
  };

  const addRow = () => {
    if (tagSlotCount(editRows) >= MAX_TAGS) {
      showError(
        zh
          ? `最多添加 ${MAX_TAGS} 个标签。`
          : `At most ${MAX_TAGS} tags allowed.`,
      );
      return;
    }
    setEditRows((rows) => [...rows, blankRow(true)]);
  };

  const updateKey = (id: string, key: string) => {
    const next = editRows.map((row) => (row.id === id ? { ...row, key } : row));
    setEditRows(next);
    applyChanges(next);
  };

  const updateInput = (id: string, input: string) => {
    setEditRows((rows) =>
      rows.map((row) => (row.id === id ? { ...row, input } : row)),
    );
  };

  const addValue = (id: string, value: string) => {
    const trimmed = value.trim();
    if (!trimmed) return;
    const row = editRows.find((item) => item.id === id);
    if (!row || !row.key.trim()) return;
    if (isDuplicateKey(editRows, id)) {
      showError(
        zh
          ? `标签键「${row.key.trim()}」与其他标签键重复，请先修改。`
          : `Tag key "${row.key.trim()}" conflicts with another row.`,
      );
      return;
    }
    if (row.values.includes(trimmed)) {
      showError(
        zh
          ? `标签「${row.key.trim()}:${trimmed}」已存在，不能重复添加。`
          : `Tag "${row.key.trim()}:${trimmed}" already exists.`,
      );
      return;
    }
    if (row.values.length >= MAX_VALUES_PER_KEY) {
      showError(
        zh
          ? `标签键「${row.key.trim()}」的标签值不能超过 ${MAX_VALUES_PER_KEY} 个。`
          : `Tag key "${row.key.trim()}" can have at most ${MAX_VALUES_PER_KEY} values.`,
      );
      return;
    }
    if (tagKeyCount(editRows) >= MAX_TAGS && !row.values.length) {
      showError(
        zh
          ? `一个任务最多添加 ${MAX_TAGS} 个标签。`
          : `A job can have at most ${MAX_TAGS} tags.`,
      );
      return;
    }
    const next = editRows.map((item) =>
      item.id === id
        ? { ...item, values: [...item.values, trimmed], input: "" }
        : item,
    );
    if (applyChanges(next)) setEditRows(next);
  };

  const removeValue = (id: string, value: string) => {
    const next = editRows.map((row) =>
      row.id === id
        ? { ...row, values: row.values.filter((item) => item !== value) }
        : row,
    );
    setEditRows(next);
    applyChanges(next);
  };

  const removeRow = (id: string) => {
    const next = editRows.filter((row) => row.id !== id);
    setEditRows(next);
    applyChanges(next);
  };

  const renderRow = (row: TagEditRow) => (
    <TagRow
      key={row.id}
      row={row}
      keys={availableKeys(row.id)}
      values={availableValuesFor(row.key)}
      duplicate={isDuplicateKey(editRows, row.id)}
      onChangeKey={(value) => updateKey(row.id, value)}
      onAddValue={(value) => addValue(row.id, value)}
      onChangeInput={(value) => updateInput(row.id, value)}
      onRemoveValue={(value) => removeValue(row.id, value)}
      onRemove={() => removeRow(row.id)}
      zh={zh}
      sectioned={sectioned}
    />
  );

  if (sectioned) {
    const existingRows = editRows.filter((row) => row.existing);
    const newRows = editRows.filter((row) => !row.existing);
    return (
      <div
        className={`tag-editor tag-editor-sectioned${compact ? " tag-editor-compact" : ""}`}
      >
        {errorMsg && <div className="tag-editor-error">{errorMsg}</div>}
        <div className="tag-section">
          <div className="tag-section-title">
            {zh ? "已有标签" : "Existing tags"}
          </div>
          {existingRows.length > 0 ? (
            <div className="tag-editor-rows">{existingRows.map(renderRow)}</div>
          ) : (
            <div className="tag-section-empty">
              {zh ? "暂无标签" : "No tags yet"}
            </div>
          )}
        </div>
        <div className="tag-section">
          <div className="tag-section-title">
            {zh ? "添加标签" : "Add tags"}
          </div>
          <div className="tag-editor-rows">{newRows.map(renderRow)}</div>
          <button
            type="button"
            className="tag-add-link"
            onClick={addRow}
            disabled={tagSlotCount(editRows) >= MAX_TAGS}
          >
            <Plus size={14} />
            {zh
              ? `添加标签 (${tagSlotCount(editRows)}/${MAX_TAGS})`
              : `Add tag (${tagSlotCount(editRows)}/${MAX_TAGS})`}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={`tag-editor${compact ? " tag-editor-compact" : ""}`}>
      {errorMsg && <div className="tag-editor-error">{errorMsg}</div>}
      <div className="tag-editor-rows">{editRows.map(renderRow)}</div>
      <button
        type="button"
        className="secondary-button tag-add-btn"
        onClick={addRow}
        disabled={tagSlotCount(editRows) >= MAX_TAGS}
      >
        <Plus size={14} />
        {zh
          ? `添加标签 (${tagSlotCount(editRows)}/${MAX_TAGS})`
          : `Add tag (${tagSlotCount(editRows)}/${MAX_TAGS})`}
      </button>
    </div>
  );
}

function rowsFromTags(tags: JobTag[]): TagEditRow[] {
  const rows = new Map<string, TagEditRow>();
  for (const tag of tags) {
    const row = rows.get(tag.key);
    if (row) {
      row.values.push(tag.value);
    } else {
      rows.set(tag.key, {
        id: tag.id || newTagId(),
        key: tag.key,
        values: [tag.value],
        input: "",
        existing: true,
      });
    }
  }
  return [...rows.values()];
}

function TagRow({
  row,
  keys,
  values,
  duplicate,
  onChangeKey,
  onAddValue,
  onChangeInput,
  onRemoveValue,
  onRemove,
  zh,
  sectioned = false,
}: {
  row: TagEditRow;
  keys: string[];
  values: string[];
  duplicate: boolean;
  onChangeKey: (value: string) => void;
  onAddValue: (value: string) => void;
  onChangeInput: (value: string) => void;
  onRemoveValue: (value: string) => void;
  onRemove: () => void;
  zh: boolean;
  sectioned?: boolean;
}) {
  return (
    <div className="tag-row">
      {sectioned && (
        <span className="tag-row-label">
          <span className="tag-row-required">*</span>
          {zh ? "标签" : "Tag"}
        </span>
      )}
      <div
        className={`tag-input-field${duplicate ? " tag-input-field-duplicate" : ""}`}
      >
        <Combobox
          value={row.key}
          options={keys}
          placeholder=""
          onChange={onChangeKey}
          onSelect={onChangeKey}
          chevron={sectioned}
        />
        <span className="tag-input-hint">
          {duplicate
            ? zh
              ? "标签键与其他行重复"
              : "Duplicate key"
            : sectioned
              ? zh
                ? "请输入标签键，不超过10个字符"
                : "Enter or select key (≤10 chars)"
              : zh
                ? "请输入或选择标签键，不超过 10 个字符"
                : "Enter or select key (≤10 chars)"}
        </span>
      </div>
      <span className="tag-sep">:</span>
      <div className="tag-input-field">
        <div className="tag-values-editor">
          {row.values.map((value) => (
            <span className="tag-value-chip" key={value}>
              {value}
              <button
                type="button"
                onClick={() => onRemoveValue(value)}
                aria-label={zh ? `删除 ${value}` : `Remove ${value}`}
              >
                <X size={12} />
              </button>
            </span>
          ))}
          <Combobox
            value={row.input}
            options={values.filter((value) => !row.values.includes(value))}
            placeholder=""
            onChange={onChangeInput}
            onSelect={onAddValue}
            onEnter={onAddValue}
            disabled={
              !row.key.trim() || row.values.length >= MAX_VALUES_PER_KEY
            }
            chevron={sectioned}
          />
        </div>
        <span className="tag-input-hint">
          {row.values.length >= MAX_VALUES_PER_KEY
            ? ""
            : sectioned
              ? zh
                ? "请输入标签值不超过10个字符，回车确认"
                : "Enter value (≤10 chars), press Enter"
              : zh
                ? "输入后按回车添加标签值"
                : "Press Enter to add value"}
        </span>
      </div>
      <button
        type="button"
        className="tag-remove-btn"
        onClick={onRemove}
        title={zh ? "删除此标签" : "Remove this tag"}
        aria-label={zh ? "删除" : "Remove"}
      >
        <Trash2 size={14} />
      </button>
    </div>
  );
}

function Combobox({
  value,
  options,
  placeholder,
  onChange,
  onSelect,
  onEnter,
  disabled = false,
  chevron = false,
}: {
  value: string;
  options: string[];
  placeholder: string;
  onChange: (value: string) => void;
  onSelect?: (value: string) => void;
  onEnter?: (value: string) => void;
  disabled?: boolean;
  chevron?: boolean;
}) {
  const [focused, setFocused] = useState(false);
  const [highlight, setHighlight] = useState(-1);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const listRef = useRef<HTMLUListElement | null>(null);
  const [anchor, setAnchor] = useState<
    | undefined
    | {
        left: number;
        top: number;
        minWidth: number;
        maxHeight: number;
        openUp: boolean;
      }
  >(undefined);
  // 与输入内容完全相同的选项不展示，避免出现重复的下拉提示
  const filtered = options.filter(
    (option) =>
      option !== value && option.toLowerCase().includes(value.toLowerCase()),
  );
  const showDropdown = focused && filtered.length > 0;

  /**
   * 下拉列表通过 portal 渲染到 body 并用 fixed 定位，
   * 避免被弹窗 overflow: hidden 或相邻表单区块遮盖；
   * 输入框下方空间不足时向上翻转。
   */
  const updateAnchor = useCallback(() => {
    const input = inputRef.current;
    if (!input) return;
    const rect = input.getBoundingClientRect();
    const spaceBelow = window.innerHeight - rect.bottom;
    const spaceAbove = rect.top;
    const openUp = spaceBelow < 150 && spaceAbove > spaceBelow;
    const available = Math.max(openUp ? spaceAbove : spaceBelow, 60) - 8;
    setAnchor({
      left: rect.left,
      top: openUp ? rect.top : rect.bottom + 4,
      minWidth: rect.width,
      maxHeight: Math.min(200, available),
      openUp,
    });
  }, []);

  useLayoutEffect(() => {
    if (!showDropdown) return;
    updateAnchor();
    window.addEventListener("resize", updateAnchor);
    // capture 阶段监听，覆盖弹窗内部滚动容器的滚动
    window.addEventListener("scroll", updateAnchor, true);
    return () => {
      window.removeEventListener("resize", updateAnchor);
      window.removeEventListener("scroll", updateAnchor, true);
    };
  }, [showDropdown, updateAnchor]);

  // 键盘导航时保证高亮项在可滚动区域内可见
  useEffect(() => {
    if (highlight < 0 || !listRef.current) return;
    const item = listRef.current.children[highlight] as HTMLElement | undefined;
    item?.scrollIntoView({ block: "nearest" });
  }, [highlight]);

  return (
    <div className="tag-combobox">
      <input
        ref={inputRef}
        className={`tag-combobox-input${chevron ? " has-chevron" : ""}`}
        value={value}
        maxLength={MAX_LEN}
        placeholder={placeholder}
        disabled={disabled}
        autoComplete="off"
        onChange={(event) => {
          onChange(event.target.value);
          setHighlight(-1);
        }}
        onFocus={() => {
          setFocused(true);
          setHighlight(-1);
        }}
        onBlur={() => setFocused(false)}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" && showDropdown) {
            event.preventDefault();
            setHighlight((current) =>
              Math.min(current + 1, filtered.length - 1),
            );
          } else if (event.key === "ArrowUp" && showDropdown) {
            event.preventDefault();
            setHighlight((current) => Math.max(current - 1, -1));
          } else if (event.key === "Enter") {
            event.preventDefault();
            const selected = highlight >= 0 ? filtered[highlight] : value;
            if (selected) onEnter?.(selected);
            setFocused(false);
          }
        }}
      />
      {chevron && (
        <ChevronDown size={14} className="tag-combobox-chevron" aria-hidden />
      )}
      {showDropdown &&
        anchor &&
        createPortal(
          <ul
            ref={listRef}
            className={`tag-combobox-list${anchor.openUp ? " open-up" : ""}`}
            role="listbox"
            style={{
              left: anchor.left,
              top: anchor.top,
              minWidth: anchor.minWidth,
              maxHeight: anchor.maxHeight,
            }}
          >
            {filtered.map((option, index) => (
              <li
                key={option}
                role="option"
                aria-selected={index === highlight}
                className={index === highlight ? "active" : undefined}
                onMouseDown={(event) => {
                  event.preventDefault();
                  onSelect?.(option);
                  setFocused(false);
                }}
              >
                {option}
              </li>
            ))}
          </ul>,
          document.body,
        )}
    </div>
  );
}
