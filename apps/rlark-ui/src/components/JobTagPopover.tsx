import { useLayoutEffect, useRef, useState } from "react";
import { X } from "lucide-react";
import type { JobTag } from "../data";

interface JobTagPopoverProps {
  /** 全部标签 */
  tags: JobTag[];
  /** 触发元素（通常是 "+N" 按钮）的位置，用于定位弹层 */
  anchorRect: DOMRect;
  zh?: boolean;
  onClose: () => void;
}

const GAP = 6;
const VIEWPORT_MARGIN = 12;

/**
 * 全部标签浮层：优先在触发元素下方展开；
 * 下方空间不足时自动翻转到上方，左右越界时收回视口内，
 * 保证靠近屏幕底部的行也能完整展示。
 */
export function JobTagPopover({
  tags,
  anchorRect,
  zh = true,
  onClose,
}: JobTagPopoverProps) {
  const popoverRef = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState({
    top: anchorRect.bottom + GAP,
    left: anchorRect.left,
  });

  // 渲染后测量浮层实际尺寸，再根据视口剩余空间调整位置
  useLayoutEffect(() => {
    const el = popoverRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();

    let top = anchorRect.bottom + GAP;
    if (top + rect.height > window.innerHeight - VIEWPORT_MARGIN) {
      const above = anchorRect.top - GAP - rect.height;
      if (above >= VIEWPORT_MARGIN) {
        top = above;
      } else {
        top = Math.max(
          VIEWPORT_MARGIN,
          window.innerHeight - VIEWPORT_MARGIN - rect.height,
        );
      }
    }

    let left = anchorRect.left;
    if (left + rect.width > window.innerWidth - VIEWPORT_MARGIN) {
      left = Math.max(
        VIEWPORT_MARGIN,
        window.innerWidth - VIEWPORT_MARGIN - rect.width,
      );
    }

    setPosition({ top, left });
  }, [anchorRect]);

  return (
    <div
      ref={popoverRef}
      className="job-tag-popover"
      style={{ top: position.top, left: position.left }}
    >
      <div className="job-tag-popover-header">
        <strong>{zh ? "全部标签" : "All tags"}</strong>
        <button
          type="button"
          className="job-tag-popover-close"
          onClick={onClose}
          aria-label={zh ? "关闭" : "Close"}
        >
          <X size={14} />
        </button>
      </div>
      <div className="job-tag-popover-list">
        {tags.map((tag) => (
          <span key={tag.id} className="job-tag-chip">
            {tag.key}: {tag.value}
          </span>
        ))}
      </div>
    </div>
  );
}
