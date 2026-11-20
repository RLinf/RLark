import { useEffect, useState, type ReactElement } from "react";
import { imageRegistriesApi, type ImageRegistryItem } from "../../backend";
import { registryBrowseApi, registryError } from "../api";
import { formatRelativeTime } from "../lib/stationDisplay";
import "./RegistryImagePicker.css";

type ImageUsage = {
  image: string;
  useCount: number;
  lastUsedAt: string;
};

type Props = {
  zh: boolean;
  value: string;
  onChange: (image: string) => void;
  recentImages: ImageUsage[];
};

type Page = { items: string[]; next: string };

export function RegistryImagePicker({
  zh,
  value,
  onChange,
  recentImages,
}: Props): ReactElement {
  const [historyOpen, setHistoryOpen] = useState(false);
  const [browserOpen, setBrowserOpen] = useState(false);
  const [registries, setRegistries] = useState<ImageRegistryItem[]>([]);
  const [registryId, setRegistryId] = useState("");
  const [repository, setRepository] = useState("");
  const [cursors, setCursors] = useState<string[]>([""]);
  const [page, setPage] = useState<Page | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const selectedRegistry = registries.find((item) => item.id === registryId);
  const filteredRecent = recentImages.filter((item) =>
    item.image.toLowerCase().includes(value.trim().toLowerCase()),
  );

  // What the open browser shows: the registries, then a registry's repositories or a repository's tags.
  useEffect(() => {
    if (!browserOpen) return;
    const controller = new AbortController();
    const { signal } = controller;
    const cursor = cursors[cursors.length - 1];
    setPage(null);
    setLoading(true);
    setError("");
    const read: Promise<() => void> = !registryId
      ? imageRegistriesApi.list(signal).then((items) => () => setRegistries(items))
      : (repository ? registryBrowseApi.tags(registryId, repository, cursor, signal) : registryBrowseApi.repositories(registryId, cursor, signal))
          .then((result) => () => setPage(result));
    read
      .then((show) => {
        if (!controller.signal.aborted) show();
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) setError(registryError(cause, zh));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [browserOpen, registryId, repository, cursors, zh, retry]);

  function resetBrowser() {
    setRegistryId("");
    setRepository("");
    setCursors([""]);
    setPage(null);
    setError("");
  }

  function chooseRepository(name: string) {
    setRepository(name);
    setCursors([""]);
    setPage(null);
  }

  return (
    <div className="image-picker">
      <div
        className="image-picker-input"
        onBlur={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget))
            setHistoryOpen(false);
        }}
      >
        <input
          value={value}
          onFocus={() => setHistoryOpen(true)}
          onChange={(event) => {
            onChange(event.target.value);
            setHistoryOpen(true);
          }}
          placeholder={
            zh
              ? "例如：docker.io/library/ubuntu:22.04（需确保集群可访问）"
              : "Example: docker.io/library/ubuntu:22.04 (must be cluster-accessible)"
          }
        />
        {historyOpen && filteredRecent.length > 0 && (
          <div className="image-picker-list">
            <div className="image-picker-heading">
              {zh
                ? "最近使用（可能已失效，请确认地址）"
                : "Recently used (may be stale; verify the reference)"}
            </div>
            {filteredRecent.map((item) => (
              <button
                key={item.image}
                type="button"
                className="image-picker-option"
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => {
                  onChange(item.image);
                  setHistoryOpen(false);
                }}
              >
                <strong>{item.image}</strong>
                <small>
                  {zh
                    ? `${formatRelativeTime(item.lastUsedAt, Date.now(), zh)}使用 · ${item.useCount} 次`
                    : `${formatRelativeTime(item.lastUsedAt, Date.now(), zh)} · used ${item.useCount} times`}
                </small>
              </button>
            ))}
          </div>
        )}
      </div>
      <button
        type="button"
        className="secondary-button image-picker-browse"
        aria-expanded={browserOpen}
        onClick={() => {
          resetBrowser();
          setRegistries([]);
          setBrowserOpen((open) => !open);
        }}
      >
        {browserOpen
          ? zh
            ? "关闭仓库浏览"
            : "Close registry browser"
          : zh
            ? "浏览已配置镜像仓库"
            : "Browse configured registries"}
      </button>
      {browserOpen && (
        <div className="image-picker-browser">
          <label>
            {zh ? "镜像仓库" : "Registry"}
            <select
              value={registryId}
              onChange={(event) => {
                resetBrowser();
                setRegistryId(event.target.value);
              }}
            >
              <option value="">
                {zh ? "请选择仓库" : "Select a registry"}
              </option>
              {registries.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name} · {item.registry}
                </option>
              ))}
            </select>
          </label>
          {loading && <p role="status">{zh ? "加载中…" : "Loading…"}</p>}
          {error && (
            <div role="alert">
              <p>{error}</p>
              <button
                type="button"
                className="secondary-button"
                onClick={() => setRetry((count) => count + 1)}
              >
                {zh ? "重试" : "Retry"}
              </button>
            </div>
          )}
          {!loading && !error && registries.length === 0 && (
            <p>
              {zh
                ? "没有已配置的镜像仓库，请手动输入镜像地址。"
                : "No registries are configured. Enter an image reference manually."}
            </p>
          )}
          {selectedRegistry && repository && (
            <button
              type="button"
              className="secondary-button"
              onClick={() => chooseRepository("")}
            >
              {zh ? "返回镜像列表" : "Back to repositories"}
            </button>
          )}
          {selectedRegistry && page && !loading && !error && (
            <>
              <div className="image-picker-heading">
                {repository
                  ? `${zh ? "标签" : "Tags"}: ${repository}`
                  : zh
                    ? "仓库镜像"
                    : "Repository images"}
              </div>
              {page.items.length === 0 && (
                <p>{zh ? "当前页没有内容。" : "This page is empty."}</p>
              )}
              <div className="image-picker-results">
                {page.items.map((item) => (
                  <button
                    key={item}
                    type="button"
                    className="image-picker-option"
                    onClick={() => {
                      if (repository) {
                        onChange(`${selectedRegistry.registry}/${repository}:${item}`);
                        setBrowserOpen(false);
                        resetBrowser();
                      } else chooseRepository(item);
                    }}
                  >
                    <strong>{item}</strong>
                  </button>
                ))}
              </div>
              <div className="image-picker-pagination">
                <button
                  type="button"
                  className="secondary-button"
                  disabled={cursors.length === 1}
                  onClick={() => setCursors((current) => current.slice(0, -1))}
                >
                  {zh ? "上一页" : "Previous"}
                </button>
                <button
                  type="button"
                  className="secondary-button"
                  disabled={!page.next || cursors.includes(page.next)}
                  onClick={() => setCursors((current) => [...current, page.next])}
                >
                  {zh ? "下一页" : "Next"}
                </button>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}
