import { useEffect, useRef, useState } from "react";
import {
  Bell,
  ChevronLeft,
  ChevronRight,
  Languages,
  ListFilter,
  Moon,
  Plus,
  RefreshCw,
  Search,
  Activity,
  ArrowDown,
  ArrowUp,
} from "lucide-react";
import type { Phase } from "../data";
import type { Copy, Lang, Theme } from "../i18n";
import type { ResourceRow } from "../types";

export function PlatformFooter() {
  return (
    <footer className="platform-footer">
      <div className="platform-footer-links">
        <strong>RLark Open Source</strong>
        <span aria-hidden="true">·</span>
        <a
          href="https://github.com/RLinf/RLark"
          target="_blank"
          rel="noreferrer"
        >
          GitHub
        </a>
        <span aria-hidden="true">·</span>
        <a
          href="https://github.com/RLinf/RLark/wiki"
          target="_blank"
          rel="noreferrer"
        >
          Docs
        </a>
        <span aria-hidden="true">·</span>
        <span className="platform-footer-maintainer">
          Initiated &amp; Maintained by{" "}
          <a
            href="https://www.infinigence-ai.com/"
            target="_blank"
            rel="noreferrer"
          >
            Infinigence AI
          </a>
        </span>
      </div>
    </footer>
  );
}

export type SortDirection = "asc" | "desc";

export function compareSortValues(
  left: string | number,
  right: string | number,
  direction: SortDirection,
  locale = "en",
) {
  const comparison =
    typeof left === "number" && typeof right === "number"
      ? left - right
      : String(left).localeCompare(String(right), locale, {
          numeric: true,
          sensitivity: "base",
        });
  return direction === "asc" ? comparison : -comparison;
}

export function SortButton({
  label,
  active,
  direction,
  onClick,
}: {
  label: string;
  active: boolean;
  direction: SortDirection;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={`table-sort-button${active ? " active" : ""}`}
      onClick={onClick}
      aria-label={`${label} ${active && direction === "desc" ? "descending" : "ascending"}`}
      aria-pressed={active}
    >
      <span>{label}</span>
      {active ? (
        direction === "asc" ? (
          <ArrowUp size={12} />
        ) : (
          <ArrowDown size={12} />
        )
      ) : (
        <span className="table-sort-placeholder">↕</span>
      )}
    </button>
  );
}

export function Logo() {
  return (
    <div className="brand">
      <img
        src="/rlark-logo.png"
        alt="RLark"
        className="brand-logo brand-logo-light"
      />
      <img
        src="/rlark-logo-white.png"
        alt="RLark"
        className="brand-logo brand-logo-dark"
      />
    </div>
  );
}

export function StatusBadge({ phase, copy: c }: { phase: Phase; copy: Copy }) {
  return (
    <span className={"status status-" + phase.toLowerCase()}>
      <i />
      {c.status[phase]}
    </span>
  );
}

export function Header({
  title,
  lang,
  theme,
  copy: c,
  onLangChange,
  onThemeChange,
  onCreate,
  onLogout,
  createLabel,
  showMockEnvironment = false,
  notificationSummary,
  userName,
}: {
  title: string;
  lang: Lang;
  theme: Theme;
  copy: Copy;
  onLangChange: (lang: Lang) => void;
  onThemeChange: (theme: Theme) => void;
  onCreate: () => void;
  onLogout?: () => void;
  createLabel?: string;
  showMockEnvironment?: boolean;
  notificationSummary?: { runningJobs: number; attentionNodes: number };
  userName?: string;
}) {
  const zh = lang === "zh";
  const displayUserName = userName?.trim() || (zh ? "用户" : "User");
  const avatarText = Array.from(displayUserName.replace(/\s+/g, ""))
    .slice(0, 2)
    .join("")
    .toUpperCase();
  const [notificationsOpen, setNotificationsOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);
  const notificationMenuRef = useRef<HTMLDivElement>(null);
  const accountMenuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!notificationsOpen && !accountOpen) return;

    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!notificationMenuRef.current?.contains(target)) {
        setNotificationsOpen(false);
      }
      if (!accountMenuRef.current?.contains(target)) {
        setAccountOpen(false);
      }
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setNotificationsOpen(false);
        setAccountOpen(false);
      }
    };

    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [notificationsOpen, accountOpen]);

  return (
    <header className="topbar">
      <div className="topbar-context">
        <span>{zh ? "当前页面" : "Current page"}</span>
        <h1>{title}</h1>
      </div>
      <div className="topbar-actions">
        {showMockEnvironment && (
          <div
            className="cluster-picker environment-status"
            title={zh ? "当前使用 Mock 数据" : "Using mock data"}
          >
            <span className="online-pulse" />
            {zh ? "Mock 环境" : "Mock"}
          </div>
        )}
        <div className="segmented-control">
          <button
            className={lang === "zh" ? "active" : ""}
            onClick={() => onLangChange("zh")}
          >
            <Languages size={14} />中
          </button>
          <button
            className={lang === "en" ? "active" : ""}
            onClick={() => onLangChange("en")}
          >
            EN
          </button>
        </div>
        <div className="segmented-control theme-control">
          <button
            className={theme === "light" ? "active" : ""}
            onClick={() => onThemeChange("light")}
          >
            {c.common.light}
          </button>
          <button
            className={theme === "dark" ? "active" : ""}
            onClick={() => onThemeChange("dark")}
          >
            <Moon size={14} />
            {c.common.dark}
          </button>
        </div>
        <div className="topbar-menu" ref={notificationMenuRef}>
          <button
            type="button"
            className="icon-button notification-button"
            aria-label={zh ? "通知" : "Notifications"}
            aria-expanded={notificationsOpen}
            onClick={() => {
              setNotificationsOpen((open) => !open);
              setAccountOpen(false);
            }}
          >
            <Bell size={18} />
            {notificationSummary &&
              notificationSummary.runningJobs +
                notificationSummary.attentionNodes >
                0 && (
                <em>
                  {notificationSummary.runningJobs +
                    notificationSummary.attentionNodes}
                </em>
              )}
          </button>
          {notificationsOpen && (
            <div className="topbar-popover notification-popover">
              <strong>{zh ? "运行通知" : "Notifications"}</strong>
              {notificationSummary ? (
                <>
                  <span>
                    {zh
                      ? `${notificationSummary.runningJobs} 个任务正在运行`
                      : `${notificationSummary.runningJobs} jobs are running`}
                  </span>
                  <span>
                    {zh
                      ? `${notificationSummary.attentionNodes} 个节点需要关注`
                      : `${notificationSummary.attentionNodes} nodes need attention`}
                  </span>
                </>
              ) : (
                <span>
                  {zh ? "暂无新的运行通知" : "No new runtime notifications"}
                </span>
              )}
            </div>
          )}
        </div>
        <button className="primary-button" onClick={onCreate}>
          <Plus size={17} />
          <span>{createLabel ?? c.common.createJob}</span>
        </button>
        <div className="topbar-menu" ref={accountMenuRef}>
          <button
            type="button"
            className="avatar"
            aria-label={zh ? "用户菜单" : "User menu"}
            aria-expanded={accountOpen}
            onClick={() => {
              setAccountOpen((open) => !open);
              setNotificationsOpen(false);
            }}
          >
            {avatarText}
          </button>
          {accountOpen && (
            <div className="topbar-popover account-popover">
              <strong>{displayUserName}</strong>
              <span>{zh ? "平台用户" : "Platform user"}</span>
              {onLogout && (
                <button
                  type="button"
                  onClick={() => {
                    setAccountOpen(false);
                    onLogout();
                  }}
                >
                  {zh ? "退出登录" : "Sign out"}
                </button>
              )}
            </div>
          )}
        </div>
      </div>
    </header>
  );
}

export function MetricCard({
  icon: Icon,
  tone,
  label,
  value,
  note,
  onClick,
}: {
  icon: typeof Activity;
  tone: string;
  label: string;
  value: string;
  note: string;
  onClick?: () => void;
}) {
  const content = (
    <>
      <div className="metric-head">
        <span className="metric-icon">
          <Icon size={18} />
        </span>
      </div>
      <span className="metric-label">{label}</span>
      <div className="metric-value-row">
        <strong>{value}</strong>
      </div>
      <small>{note}</small>
    </>
  );

  if (onClick) {
    return (
      <button
        type="button"
        className={"metric-card metric-card-action tone-" + tone}
        onClick={onClick}
        aria-label={label}
      >
        {content}
      </button>
    );
  }

  return <div className={"metric-card tone-" + tone}>{content}</div>;
}

export function Pagination({
  page,
  pageSize,
  total,
  onPageChange,
  onPageSizeChange,
  zh,
  pageSizeOptions = [10, 20, 50],
}: {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
  zh: boolean;
  pageSizeOptions?: number[];
}) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const currentPage = Math.min(Math.max(page, 1), totalPages);
  const start = total === 0 ? 0 : (currentPage - 1) * pageSize + 1;
  const end = Math.min(currentPage * pageSize, total);

  return (
    <div className="pagination-bar" aria-label={zh ? "分页" : "Pagination"}>
      <small className="pagination-summary">
        {zh
          ? `共 ${total} 条，当前 ${start}-${end}`
          : `${start}-${end} of ${total}`}
      </small>
      {onPageSizeChange && (
        <label className="pagination-size">
          <span>{zh ? "每页" : "Rows"}</span>
          <select
            value={pageSize}
            onChange={(event) => onPageSizeChange(Number(event.target.value))}
          >
            {pageSizeOptions.map((size) => (
              <option key={size} value={size}>
                {size}
              </option>
            ))}
          </select>
        </label>
      )}
      <button
        type="button"
        className="icon-button"
        disabled={currentPage <= 1}
        onClick={() => onPageChange(currentPage - 1)}
        aria-label={zh ? "上一页" : "Previous page"}
      >
        <ChevronLeft size={16} />
      </button>
      <small>
        {zh
          ? `${currentPage} / ${totalPages} 页`
          : `${currentPage} / ${totalPages}`}
      </small>
      <button
        type="button"
        className="icon-button"
        disabled={currentPage >= totalPages}
        onClick={() => onPageChange(currentPage + 1)}
        aria-label={zh ? "下一页" : "Next page"}
      >
        <ChevronRight size={16} />
      </button>
    </div>
  );
}

export function Gauge({ value, label }: { value: number; label: string }) {
  return (
    <div className="gauge">
      <div
        style={{
          background: `conic-gradient(#36c98f ${value * 3.6}deg, #e8edf4 0)`,
        }}
      >
        <span>{value}%</span>
      </div>
      <small>{label}</small>
    </div>
  );
}

export function PageToolbar({
  placeholder,
  value,
  onChange,
  count,
  copy: c,
  onRefresh,
  filterValue,
  onFilterChange,
  filterOptions,
}: {
  placeholder: string;
  value: string;
  onChange: (value: string) => void;
  count: number;
  copy: Copy;
  onRefresh?: () => void;
  filterValue?: string;
  onFilterChange?: (value: string) => void;
  filterOptions?: Array<{ value: string; label: string }>;
}) {
  return (
    <div className="page-toolbar">
      <div className="search-field">
        <Search size={16} />
        <input
          placeholder={placeholder}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      </div>
      {filterOptions && filterOptions.length > 0 && (
        <label className="toolbar-filter">
          <ListFilter size={16} />
          <span className="sr-only">{c.common.filters}</span>
          <select
            value={filterValue}
            onChange={(event) => onFilterChange?.(event.target.value)}
            aria-label={c.common.filters}
          >
            {filterOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
      )}
      {onRefresh && (
        <button type="button" className="secondary-button" onClick={onRefresh}>
          <RefreshCw size={16} />
          {c.common.refresh}
        </button>
      )}
      <small>
        {count} {c.common.results}
      </small>
    </div>
  );
}

export function ResourceDistribution({
  copy: c,
  rows,
}: {
  copy: Copy;
  rows: ResourceRow[];
}) {
  const total = rows.reduce((s, r) => s + r.count, 0) || 1;
  return (
    <div className="resource-split">
      {rows.map((row) => {
        const pct = Math.round((row.count / total) * 100);
        return (
          <div key={row.label} className={"resource-row " + row.color}>
            <div>
              <strong>{row.label}</strong>
              <small>
                {row.count} nodes{row.models ? " · " + row.models : ""}
              </small>
            </div>
            <span>
              <i style={{ width: pct + "%" }} />
            </span>
            <b>{pct}%</b>
          </div>
        );
      })}
    </div>
  );
}
