import { useEffect, useState } from "react";
import {
  Braces,
  Check,
  Copy as CopyIcon,
  KeyRound,
  Layers3,
  Search,
} from "lucide-react";
import {
  apiReferenceApi,
  type ApiReferenceEndpoint,
  type ApiReferenceResponse,
} from "../backend";
import type { Copy } from "../i18n";

export function ApiPage({ copy: c }: { copy: Copy }) {
  const lang = c.nav.overview === "总览" ? "zh" : "en";
  const zh = lang === "zh";
  const [reference, setReference] = useState<ApiReferenceResponse | null>(null);
  const [loadError, setLoadError] = useState(false);
  const [activeSectionID, setActiveSectionID] = useState("");
  const [query, setQuery] = useState("");
  const [selectedKey, setSelectedKey] = useState("");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    let cancelled = false;
    apiReferenceApi
      .get()
      .then((result) => {
        if (cancelled) return;
        setReference(result);
        const firstSection = result.sections[0];
        setActiveSectionID(firstSection?.id ?? "");
        const firstEndpoint = firstSection?.endpoints?.[0];
        setSelectedKey(
          firstEndpoint ? `${firstEndpoint.method} ${firstEndpoint.path}` : "",
        );
      })
      .catch(() => {
        if (!cancelled) setLoadError(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const activeSection = reference?.sections.find(
    (section) => section.id === activeSectionID,
  );
  const normalizedQuery = query.trim().toLowerCase();
  const filteredEndpoints = (activeSection?.endpoints ?? []).filter(
    (endpoint) =>
      !normalizedQuery ||
      `${endpoint.method} ${endpoint.path} ${endpoint.description[lang]}`
        .toLowerCase()
        .includes(normalizedQuery),
  );
  const selectedEndpoint =
    filteredEndpoints.find(
      (endpoint) => `${endpoint.method} ${endpoint.path}` === selectedKey,
    ) ?? filteredEndpoints[0];
  const resourceSections = (reference?.sections ?? []).filter(
    (section) => section.id !== "overview",
  );
  const endpointCount = (reference?.sections ?? []).reduce(
    (total, section) => total + (section.endpoints?.length ?? 0),
    0,
  );

  const selectSection = (sectionID: string) => {
    setActiveSectionID(sectionID);
    setQuery("");
    const firstEndpoint = reference?.sections.find(
      (section) => section.id === sectionID,
    )?.endpoints?.[0];
    setSelectedKey(
      firstEndpoint ? `${firstEndpoint.method} ${firstEndpoint.path}` : "",
    );
  };

  const copyExample = async (endpoint: ApiReferenceEndpoint) => {
    await navigator.clipboard.writeText(
      JSON.stringify(endpoint.example, null, 2),
    );
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  };

  if (!reference) {
    return (
      <div className="page-content resource-page api-reference-page">
        <div className="section-heading">
          <div>
            <span className="eyebrow">{c.api.eyebrow}</span>
            <h2>{c.api.title}</h2>
            <p>{c.api.desc}</p>
          </div>
        </div>
        <div className="api-load-state">
          {loadError ? c.api.loadError : c.api.loading}
        </div>
      </div>
    );
  }

  return (
    <div className="page-content resource-page api-reference-page">
      <div className="section-heading api-page-heading">
        <div>
          <span className="eyebrow">{c.api.eyebrow}</span>
          <h2>{reference.title[lang]}</h2>
          <p>{reference.description[lang]}</p>
        </div>
        <div className="api-page-meta">
          <span>{zh ? "Gateway 实时数据" : "Live Gateway data"}</span>
          <strong>{endpointCount}</strong>
          <small>{zh ? "个已收录接口" : "documented endpoints"}</small>
        </div>
      </div>
      <nav
        className="api-category-bar"
        aria-label={zh ? "接口分类" : "API categories"}
      >
        <div className="api-category-tabs">
          {reference.sections.map((section) => (
            <button
              type="button"
              className={section.id === activeSectionID ? "active" : ""}
              onClick={() => selectSection(section.id)}
              aria-pressed={section.id === activeSectionID}
              key={section.id}
            >
              {section.title[lang]}
              <small>{section.endpoints?.length ?? 0}</small>
            </button>
          ))}
        </div>
        {activeSection?.id !== "overview" && (
          <div className="search-field">
            <Search size={15} />
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={c.api.search}
              aria-label={zh ? "搜索接口" : "Search endpoints"}
            />
          </div>
        )}
      </nav>
      <main className="api-content">
        <div className="api-section-heading">
          <div>
            <span className="eyebrow">
              {activeSection?.id === "overview"
                ? zh
                  ? "快速开始"
                  : "Quick start"
                : zh
                  ? "接口分类"
                  : "API category"}
            </span>
            <h2>{activeSection?.title[lang]}</h2>
            <p>{activeSection?.description[lang]}</p>
          </div>
          {activeSection?.id !== "overview" && (
            <span className="api-endpoint-count">
              {filteredEndpoints.length} {zh ? "个接口" : "endpoints"}
            </span>
          )}
        </div>
        {activeSection?.id === "overview" ? (
          <div className="api-overview">
            <div className="api-overview-hero">
              <span>
                <Braces size={18} />
              </span>
              <div>
                <strong>
                  {zh ? "RLark Gateway API" : "RLark Gateway API"}
                </strong>
                <p>{activeSection.description[lang]}</p>
              </div>
              <code>/api/v1/rlinf.io/v1alpha1</code>
            </div>
            <div className="api-overview-facts">
              <div>
                <span className="violet">
                  <KeyRound size={17} />
                </span>
                <small>{zh ? "认证方式" : "Authentication"}</small>
                <strong>Bearer JWT</strong>
                <p>
                  {zh
                    ? "登录后将令牌放入 Authorization 请求头。"
                    : "Send the login token in the Authorization header."}
                </p>
              </div>
              <div>
                <span className="mint">
                  <Layers3 size={17} />
                </span>
                <small>{zh ? "资源分类" : "Resource groups"}</small>
                <strong>{resourceSections.length}</strong>
                <p>
                  {zh
                    ? "按业务资源组织，可从下方直接进入。"
                    : "Organized by resource; open one directly below."}
                </p>
              </div>
            </div>
            <div className="api-overview-resources">
              <div className="api-overview-title">
                <div>
                  <span>{zh ? "资源目录" : "Resource catalog"}</span>
                  <strong>
                    {zh ? "选择分类开始浏览" : "Choose a category"}
                  </strong>
                </div>
                <small>
                  {resourceSections.reduce(
                    (total, section) =>
                      total + (section.endpoints?.length ?? 0),
                    0,
                  )}{" "}
                  {zh ? "个接口" : "endpoints"}
                </small>
              </div>
              <div className="api-overview-resource-grid">
                {resourceSections.map((section) => (
                  <button
                    type="button"
                    onClick={() => selectSection(section.id)}
                    key={section.id}
                  >
                    <span>{section.title[lang].slice(0, 1)}</span>
                    <div>
                      <strong>{section.title[lang]}</strong>
                      <small>
                        {section.endpoints?.length ?? 0}{" "}
                        {zh ? "个接口" : "endpoints"}
                      </small>
                    </div>
                  </button>
                ))}
              </div>
            </div>
            <div className="api-overview-flow">
              <span>{zh ? "调用流程" : "Request flow"}</span>
              <ol>
                <li>
                  <b>01</b>
                  <div>
                    <strong>{zh ? "获取令牌" : "Get a token"}</strong>
                    <small>POST /api/v1/auth/login</small>
                  </div>
                </li>
                <li>
                  <b>02</b>
                  <div>
                    <strong>{zh ? "添加请求头" : "Add the header"}</strong>
                    <small>Authorization: Bearer &lt;token&gt;</small>
                  </div>
                </li>
                <li>
                  <b>03</b>
                  <div>
                    <strong>{zh ? "调用资源接口" : "Call a resource"}</strong>
                    <small>Content-Type: application/json</small>
                  </div>
                </li>
              </ol>
            </div>
          </div>
        ) : filteredEndpoints.length > 0 ? (
          <div className="api-endpoint-stack">
            {filteredEndpoints.map((endpoint) => {
              const endpointKey = `${endpoint.method} ${endpoint.path}`;
              const expanded =
                endpointKey ===
                `${selectedEndpoint?.method} ${selectedEndpoint?.path}`;
              return (
                <article
                  className={expanded ? "expanded" : ""}
                  key={endpointKey}
                >
                  <button
                    type="button"
                    className="api-endpoint-summary"
                    onClick={() => setSelectedKey(endpointKey)}
                    aria-expanded={expanded}
                  >
                    <span className={"method " + endpoint.method.toLowerCase()}>
                      {endpoint.method}
                    </span>
                    <code>{endpoint.path}</code>
                    <p>{endpoint.description[lang]}</p>
                    <span className="api-expand-label">
                      {expanded
                        ? zh
                          ? "收起"
                          : "Collapse"
                        : zh
                          ? "查看示例"
                          : "View example"}
                    </span>
                  </button>
                  {expanded && (
                    <div className="api-endpoint-detail">
                      <div className="api-detail-meta">
                        <div>
                          <span>{zh ? "请求路径" : "Request path"}</span>
                          <code>{endpoint.path}</code>
                        </div>
                        <span>{c.api.example}</span>
                      </div>
                      <div className="code-block">
                        <div>
                          <span>application/json</span>
                          <button
                            type="button"
                            onClick={() => copyExample(endpoint)}
                          >
                            {copied ? (
                              <Check size={13} />
                            ) : (
                              <CopyIcon size={13} />
                            )}
                            {copied ? (zh ? "已复制" : "Copied") : c.api.copy}
                          </button>
                        </div>
                        <pre>{JSON.stringify(endpoint.example, null, 2)}</pre>
                      </div>
                    </div>
                  )}
                </article>
              );
            })}
          </div>
        ) : (
          <div className="api-guide-card">
            {normalizedQuery
              ? zh
                ? "没有匹配的接口"
                : "No matching endpoints"
              : activeSection?.description[lang]}
          </div>
        )}
      </main>
    </div>
  );
}
