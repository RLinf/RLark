import { ChevronRight, Search } from "lucide-react";
import type { Copy } from "../i18n";

export function ApiPage({ copy: c }: { copy: Copy }) {
  const endpoints = [
    ["GET", "/api/v1/clusters", c.api.endpointDesc[0]],
    ["GET", "/api/v1/nodes", c.api.endpointDesc[1]],
    ["POST", "/api/v1/jobs", c.api.endpointDesc[2]],
    ["GET", "/api/v1/jobs/{id}/workers", c.api.endpointDesc[3]],
    ["GET", "/api/v1/workers/{id}/logs", c.api.endpointDesc[4]],
  ];
  const example =
    '{\\n  "kind": "Job",\\n  "type": "RL",\\n  "workers": [\\n    { "role": "Learner", "node": "gpu-cloud-03" },\\n    { "role": "Env Worker", "node": "robot-g1-12" }\\n  ]\\n}';
  return (
    <div className="page-content resource-page">
      <div className="section-heading">
        <div>
          <span className="eyebrow">{c.api.eyebrow}</span>
          <h2>{c.api.title}</h2>
          <p>{c.api.desc}</p>
        </div>
      </div>
      <div className="api-layout">
        <aside>
          <div className="search-field">
            <Search size={15} />
            <input placeholder={c.common.search} />
          </div>
          {c.api.sections.map((x, i) => (
            <button className={i === 4 ? "active" : ""} key={x}>
              {x}
              <ChevronRight size={14} />
            </button>
          ))}
        </aside>
        <main>
          <span className="eyebrow">JOB API</span>
          <h2>Jobs & Workers</h2>
          <p>{c.api.desc}</p>
          <div className="endpoint-list">
            {endpoints.map(([method, path, desc]) => (
              <div key={method + path}>
                <span className={"method " + method.toLowerCase()}>
                  {method}
                </span>
                <code>{path}</code>
                <p>{desc}</p>
                <ChevronRight size={16} />
              </div>
            ))}
          </div>
          <div className="code-block">
            <div>
              <span>{c.api.example}</span>
              <button>{c.api.copy}</button>
            </div>
            <pre>{example}</pre>
          </div>
        </main>
      </div>
    </div>
  );
}
