import { Fragment, useCallback, useEffect, useMemo, useState } from "react";
import { ChevronRight, X } from "lucide-react";
import type { ImageRegistryItem } from "../../backend";
import { registryBrowseApi, registryError, type RegistryTagDetailPage } from "../api";
import { CopyButton } from "../components/CopyButton";
import { PageToolbar, RefreshOverlay } from "../../components/shared";
import type { Copy } from "../../i18n";
import { pullCommand, registryCommands } from "../lib/registryCommands";
import { forEachLimited } from "../lib/forEachLimited";
import { formatChinaDateTime } from "../../utils/time.js";
import { formatRelativeTime } from "../lib/stationDisplay";
import {
  formatBytes,
  groupRegistryVersions,
  isRecent,
  splitRepository,
  type RegistryVersion,
} from "../lib/registryVersions";
import "./RegistryBrowser.css";

type Page = { items: string[]; next: string };
type DetailPreview = { page: RegistryTagDetailPage } | { error: unknown };
type SortMode = "time" | "name";

function TagChips({ tags, max }: { tags: string[]; max: number }) {
  const [main, ...aliases] = tags;
  return (
    <span className="registry-tag-previews">
      <code className="registry-tag-chip" title={main}>
        {main}
      </code>
      {aliases.slice(0, max).map((alias) => (
        <code className="registry-tag-chip is-alias" key={alias} title={alias}>
          {alias}
        </code>
      ))}
      {aliases.length > max && (
        <span className="muted registry-more">+{aliases.length - max}</span>
      )}
    </span>
  );
}

function RegistryDrawer({
  item,
  repository,
  initial,
  zh,
  now,
  onFirstPage,
  onClose,
}: {
  item: ImageRegistryItem;
  repository: string;
  initial?: RegistryTagDetailPage;
  zh: boolean;
  now: number;
  onFirstPage: (repository: string, preview: DetailPreview) => void;
  onClose: () => void;
}) {
  const [cursor, setCursor] = useState("");
  const [page, setPage] = useState<RegistryTagDetailPage | null>(initial ?? null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(!initial);
  const [retry, setRetry] = useState(0);

  useEffect(() => {
    if (!cursor && initial && retry === 0) return;
    const controller = new AbortController();
    setLoading(true);
    setError("");
    registryBrowseApi
      .tagDetails(item.id, repository, cursor, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        setPage(result);
        if (!cursor) onFirstPage(repository, { page: result });
      })
      .catch((err: unknown) => {
        if (!controller.signal.aborted) setError(registryError(err, zh));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
    // `initial` only seeds the first page; later preview updates must not refetch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.id, repository, cursor, retry, zh, onFirstPage]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const versions = page ? groupRegistryVersions(page.items) : [];
  const latest = versions.find((version) => !version.unavailable);
  const { namespace } = splitRepository(repository);
  const base = `${item.registry}/${repository}`;
  const reference = (version: RegistryVersion) => `${base}:${version.tags[0]}`;

  return (
    <>
      <div className="addon-drawer-overlay" onClick={onClose} />
      <aside
        className="addon-drawer registry-drawer"
        role="dialog"
        aria-modal="true"
        aria-labelledby="registry-drawer-title"
      >
        <div className="addon-drawer-header registry-drawer-header">
          <div>
            {namespace && <span className="eyebrow">{namespace}</span>}
            <h3 id="registry-drawer-title">{repository}</h3>
            <div className="registry-drawer-ref">
              <code>{base}</code>
              <CopyButton value={base} label={zh ? "复制仓库地址" : "Copy repository"} zh={zh} iconOnly />
            </div>
          </div>
          <button type="button" className="icon-button" onClick={onClose} aria-label={zh ? "关闭" : "Close"} title={zh ? "关闭" : "Close"}>
            <X size={16} />
          </button>
        </div>
        <div className="addon-drawer-body registry-drawer-body">
          <div className="registry-facts">
            <div>
              <span>{zh ? "版本数" : "Versions"}</span>
              <strong>{page ? versions.length : "—"}</strong>
              <small className="muted">
                {page ? (zh ? `${page.items.length} 个标签${page.next ? "（当前页）" : ""}` : `${page.items.length} tags${page.next ? " (this page)" : ""}`) : ""}
              </small>
            </div>
            <div>
              <span>{zh ? "最近构建" : "Latest build"}</span>
              <strong>{formatChinaDateTime(latest?.created)}</strong>
              <small className="muted">{formatRelativeTime(latest?.created, now, zh)}</small>
            </div>
            <div>
              <span>{zh ? "最新大小" : "Latest size"}</span>
              <strong>{formatBytes(latest?.size)}</strong>
              <small className="muted">{zh ? "config + 压缩层" : "config + compressed layers"}</small>
            </div>
          </div>

          <div className="registry-section-title">
            <span>{zh ? "版本（按构建时间倒序）" : "Versions (newest build first)"}</span>
            <small className="muted">{zh ? "同一 digest 的标签合并显示" : "Tags with the same digest are merged"}</small>
          </div>
          {loading && <p className="muted" role="status">{zh ? "加载中…" : "Loading…"}</p>}
          {error && (
            <div className="cert-error" role="alert">
              <p>{error}</p>
              <button type="button" className="secondary-button" onClick={() => setRetry((value) => value + 1)}>
                {zh ? "重试" : "Retry"}
              </button>
            </div>
          )}
          {!loading && !error && versions.length === 0 && <p className="muted">{zh ? "没有标签" : "No tags"}</p>}
          {!error &&
            versions.map((version) => (
              <article className={`registry-version${version === latest && !cursor ? " is-latest" : ""}`} key={version.key}>
                <div className="registry-version-top">
                  <TagChips tags={version.tags} max={4} />
                  {version === latest && !cursor && <span className="status status-online"><i />{zh ? "最新" : "Latest"}</span>}
                </div>
                {version.unavailable ? (
                  <small className="muted">{zh ? "无法读取镜像信息（可能是不支持的格式或仓库重定向）" : "Image metadata unavailable (unsupported format or registry redirect)"}</small>
                ) : !version.created && version.size === undefined && version.platforms.length === 0 ? null : (
                  <div className="registry-version-meta">
                    <span>{zh ? "构建" : "Built"} <strong>{formatChinaDateTime(version.created)}</strong></span>
                    <span>{formatRelativeTime(version.created, now, zh)}</span>
                    <span>{zh ? "大小" : "Size"} <strong>{formatBytes(version.size)}</strong></span>
                    <span>{version.platforms.join(" · ") || "—"}</span>
                  </div>
                )}
                <div className="registry-version-actions">
                  {version.digest && <code className="registry-digest" title={version.digest}>{version.digest.slice(0, 19)}…</code>}
                  <CopyButton value={reference(version)} label={zh ? "复制地址" : "Copy reference"} zh={zh} className="table-action-button" />
                  <CopyButton value={pullCommand(reference(version))} label={zh ? "复制 Pull" : "Copy pull"} zh={zh} className="table-action-button" />
                  {version.digest && (
                    <CopyButton value={pullCommand(`${base}@${version.digest}`)} label={zh ? "按 digest Pull" : "Pull by digest"} zh={zh} className="table-action-button" />
                  )}
                </div>
              </article>
            ))}
          {(cursor || page?.next) && (
            <div className="registry-pager">
              {cursor && <button className="secondary-button" type="button" onClick={() => { setCursor(""); setRetry((value) => value + 1); }}>{zh ? "第一页" : "First page"}</button>}
              {!error && page?.next && (
                <button className="secondary-button" type="button" disabled={loading} onClick={() => setCursor(page.next)}>
                  {zh ? "下一页标签" : "Next tags"}
                </button>
              )}
            </div>
          )}
          <p className="registry-source-note muted">
            {zh
              ? "构建时间、大小和平台由 Gateway 读取镜像 manifest 与 config 得到，并按 digest 缓存。多平台镜像显示全部平台，时间和大小取 linux/amd64。"
              : "Build time, size and platforms come from the image manifest and config read by the Gateway and are cached by digest. Multi-platform images list every platform; time and size describe linux/amd64."}
          </p>
        </div>
      </aside>
    </>
  );
}

export function RegistryBrowser({
  item,
  zh,
  copy,
}: {
  item: ImageRegistryItem;
  zh: boolean;
  copy: Copy;
}) {
  const [page, setPage] = useState<Page | null>(null);
  const [cursor, setCursor] = useState("");
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<SortMode>("time");
  const [namespace, setNamespace] = useState<string | null>(null);
  const [refresh, setRefresh] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [repository, setRepository] = useState("");
  const [now, setNow] = useState(() => Date.now());
  const [previews, setPreviews] = useState<Record<string, DetailPreview>>({});
  const setPreview = useCallback((name: string, preview: DetailPreview) => {
    setPreviews((current) => ({ ...current, [name]: preview }));
  }, []);
  const closeDrawer = useCallback(() => setRepository(""), []);
  const commands = registryCommands(item.registry, item.username);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    setPreviews({});
    setNow(Date.now());
    registryBrowseApi
      .repositories(item.id, cursor, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) {
          setPage(result);
          // The first page of each repository's tags, four at a time.
          void forEachLimited(result.items, 4, controller.signal, async (name) => {
            const preview = await registryBrowseApi.tagDetails(item.id, name, "", controller.signal).then((page) => ({ page }), (error: unknown) => ({ error }));
            if (!controller.signal.aborted) setPreview(name, preview);
          });
        }
      })
      .catch((err: unknown) => {
        if (!controller.signal.aborted) setError(registryError(err, zh));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [item.id, cursor, refresh, zh, setPreview]);

  const rows = useMemo(() => {
    const term = query.trim().toLowerCase();
    return (page?.items ?? []).map((name) => {
      const preview = previews[name];
      const versions = preview && "page" in preview ? groupRegistryVersions(preview.page.items) : undefined;
      return { name, ...splitRepository(name), preview, versions, latest: versions?.find((version) => !version.unavailable) };
    }).filter((row) =>
      (namespace === null || row.namespace === namespace) &&
      (!term || row.name.toLowerCase().includes(term) ||
        (row.versions?.some((version) => version.tags.some((tag) => tag.toLowerCase().includes(term))) ?? false)),
    ).sort((a, b) => {
      if (sort === "name") return a.name.localeCompare(b.name);
      const at = a.latest?.created ? Date.parse(a.latest.created) : -Infinity;
      const bt = b.latest?.created ? Date.parse(b.latest.created) : -Infinity;
      return bt - at || a.name.localeCompare(b.name);
    });
  }, [page, previews, query, namespace, sort]);

  const namespaces = useMemo(() => {
    const counts = new Map<string, number>();
    for (const name of page?.items ?? []) {
      const key = splitRepository(name).namespace;
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
    return [...counts.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [page]);
  const namespaceLabel = (value: string) => value || (zh ? "根目录" : "root");
  const selectedPreview = repository ? previews[repository] : undefined;

  return (
    <>
      <section className="node-detail-section storage-detail-panel registry-browser-panel">
        <div className="registry-browser-heading">
          <div>
            <h3>{zh ? "仓库镜像" : "Registry images"}</h3>
            <p className="muted">
              {zh
                ? "实时读取仓库，每页最多 50 个镜像；搜索和筛选仅针对当前页。"
                : "Read live from the registry, up to 50 repositories per page; search and filters apply to the current page."}
            </p>
          </div>
        </div>
        <PageToolbar
          placeholder={zh ? "搜索镜像或标签..." : "Search images or tags..."}
          value={query}
          onChange={setQuery}
          count={rows.length}
          copy={copy}
          refreshing={loading}
          onRefresh={() => {
            setRepository("");
            setRefresh(refresh + 1);
          }}
          filterValue={sort}
          onFilterChange={(value) => setSort(value as SortMode)}
          filterOptions={[
            { value: "time", label: zh ? "最近更新优先" : "Recently updated" },
            { value: "name", label: zh ? "按名称" : "By name" },
          ]}
        />
        {namespaces.length > 1 && (
          <div className="registry-namespaces" role="group" aria-label={zh ? "命名空间" : "Namespaces"}>
            {[["", page?.items.length ?? 0] as [string, number], ...namespaces].map(([value, count], index) => (
              <button
                key={index === 0 ? "*all" : value || "*root"}
                type="button"
                className="registry-namespace-chip"
                aria-pressed={index === 0 ? namespace === null : namespace === value}
                onClick={() => setNamespace(index === 0 ? null : value)}
              >
                {index === 0 ? (zh ? "全部" : "All") : namespaceLabel(value)}
                <em>{count}</em>
              </button>
            ))}
          </div>
        )}
        <div
          className={`table-panel refreshable-region${loading ? " is-refreshing" : ""}`}
          aria-busy={loading}
        >
          {error && (
            <p className="cert-error" role="alert">
              {error}
            </p>
          )}
          {!loading && !error && rows.length === 0 && (
            <p className="table-empty-state">
              {zh ? "当前页没有匹配的镜像" : "No matching images on this page"}
            </p>
          )}
          {!error && rows.length > 0 && (
            <table className="registry-table">
              <thead>
                <tr>
                  <th>{zh ? "镜像" : "Repository"}</th>
                  <th>{zh ? "最新版本" : "Latest version"}</th>
                  <th>{zh ? "更新时间" : "Updated"}</th>
                  <th className="registry-num">{zh ? "大小" : "Size"}</th>
                  <th>{zh ? "平台" : "Platform"}</th>
                  <th className="table-actions-col">{zh ? "操作" : "Actions"}</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row, index) => {
                  const groupStart = sort === "name" && (index === 0 || rows[index - 1].namespace !== row.namespace);
                  const otherVersions = row.versions ? row.versions.length - (row.latest ? 1 : 0) : 0;
                  const reference = row.latest ? `${item.registry}/${row.name}:${row.latest.tags[0]}` : "";
                  return (
                    <Fragment key={row.name}>
                      {groupStart && namespaces.length > 1 && (
                        <tr className="registry-group-row">
                          <td colSpan={6}>{namespaceLabel(row.namespace)}</td>
                        </tr>
                      )}
                      <tr
                        className={`clickable-row${repository === row.name ? " selected" : ""}`}
                        tabIndex={0}
                        onClick={() => setRepository(row.name)}
                        onKeyDown={(event) => {
                          if (event.key === "Enter") setRepository(row.name);
                        }}
                      >
                        <td>
                          <div className="registry-repo">
                            <strong>{row.short}</strong>
                            {row.namespace && <small>{row.namespace}/</small>}
                          </div>
                        </td>
                        <td>
                          {!row.preview ? (
                            <span className="muted" role="status">{zh ? "正在加载标签…" : "Loading tags…"}</span>
                          ) : "error" in row.preview ? (
                            <span className="registry-tag-preview-error" role="status">
                              {zh ? "标签加载失败，请刷新重试" : "Failed to load tags; refresh to retry"}
                            </span>
                          ) : !row.versions?.length ? (
                            <span className="muted">{zh ? "没有标签" : "No tags"}</span>
                          ) : !row.latest ? (
                            <span className="muted">{zh ? "无法读取镜像信息" : "Metadata unavailable"}</span>
                          ) : (
                            <span className="registry-latest">
                              <TagChips tags={row.latest.tags} max={1} />
                              {(otherVersions > 0 || row.preview.page.next) && (
                                <span className="muted registry-more">
                                  {row.preview.page.next
                                    ? zh ? "还有更多版本" : "More versions"
                                    : zh ? `另 ${otherVersions} 个版本` : `+${otherVersions} versions`}
                                </span>
                              )}
                            </span>
                          )}
                        </td>
                        <td>
                          {row.latest?.created ? (
                            <span className="registry-when">
                              <span>
                                {isRecent(row.latest.created, now) && <i className="registry-fresh" aria-hidden="true" />}
                                {formatChinaDateTime(row.latest.created)}
                              </span>
                              <small>{formatRelativeTime(row.latest.created, now, zh)}</small>
                            </span>
                          ) : (
                            <span className="muted">—</span>
                          )}
                        </td>
                        <td className="registry-num">{formatBytes(row.latest?.size)}</td>
                        <td className="registry-platform">{row.latest?.platforms.join(" · ") || "—"}</td>
                        <td className="table-actions-col">
                          <div className="row-actions">
                            {reference && <CopyButton value={pullCommand(reference)} label={zh ? "复制 Pull" : "Copy pull"} zh={zh} className="table-action-button" />}
                            <button
                              className="table-action-button"
                              type="button"
                              aria-haspopup="dialog"
                              onClick={(event) => {
                                event.stopPropagation();
                                setRepository(row.name);
                              }}
                            >
                              {zh ? "详情" : "Details"}
                              <ChevronRight size={14} />
                            </button>
                          </div>
                        </td>
                      </tr>
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
          )}
          <RefreshOverlay visible={loading} label={zh ? "正在读取仓库镜像" : "Loading registry images"} />
        </div>
        {(cursor || (!error && page?.next)) && (
          <div className="registry-pager">
            {cursor && (
              <button className="secondary-button" type="button" onClick={() => { setCursor(""); setRepository(""); }}>
                {zh ? "第一页" : "First page"}
              </button>
            )}
            {!error && page?.next && (
              <button disabled={loading} className="secondary-button" type="button" onClick={() => { setCursor(page.next); setRepository(""); }}>
                {zh ? "下一页" : "Next page"}
              </button>
            )}
          </div>
        )}
      </section>
      <section className="node-detail-section storage-detail-panel registry-browser-panel">
        <h3>{zh ? "Push / Pull 操作指南" : "Push / pull guide"}</h3>
        <p>
          {zh
            ? "客户端需先具备网络访问并信任仓库 CA。密码在登录提示中输入，不会显示在命令中。请替换示例镜像名称及标签。"
            : "The client must reach the registry and trust its CA. Enter the password at the login prompt; it is not included in the commands. Replace the example image and tag."}
        </p>
        {commands.map((command) => (
          <div className="registry-command" key={command}>
            <code>{command}</code>{" "}
            <CopyButton value={command} label={zh ? "复制" : "Copy"} zh={zh} />
          </div>
        ))}
        <p className="muted">
          {zh
            ? "上传和下载由 Docker 客户端执行；不要关闭 TLS 校验。"
            : "Push and pull run in the Docker client; do not disable TLS verification."}
        </p>
      </section>
      {repository && (
        <RegistryDrawer
          key={repository}
          item={item}
          repository={repository}
          initial={selectedPreview && "page" in selectedPreview ? selectedPreview.page : undefined}
          zh={zh}
          now={now}
          onFirstPage={setPreview}
          onClose={closeDrawer}
        />
      )}
    </>
  );
}
