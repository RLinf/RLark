import { ChevronDown, Network } from "lucide-react";
import {
  nodeLinkHealth,
  nodeLinkLine,
  type NodeNetworkDisplay,
} from "../lib/nodeNetwork.js";
import { formatChinaDateTime } from "../../utils/time.js";

// The node's site-network links as each Hub sees them, for the node page.
// network is undefined until the first read.
export function NodeLinks({ network, zh, collapsible }: { network?: NodeNetworkDisplay; zh: boolean; collapsible?: boolean }) {
  const site = network?.site;
  const title = zh ? "站点网络链路" : "Site network links";
  const description = zh
    ? "每台 Hub 看到的这台节点，约 30 秒更新"
    : "This node as each hub sees it, refreshed about every 30 s";
  const states = {
    Connected: zh ? "已连通" : "Connected",
    Unanswered: zh ? "有请求无会话" : "Unanswered",
    Silent: zh ? "收不到" : "Silent",
    Unknown: zh ? "无近况" : "Unknown",
  };
  const links = site && <>
    {site.summary && <p className="node-link-summary">{site.summary}</p>}
    {site.links.length > 0 && (
      <dl className="node-info-list">
        {site.links.map((link) => (
          <div key={link.hub}>
            <dt>{link.hub}</dt>
            <dd>
              <span className={`node-link ${link.state === "Connected" ? "connected" : link.state === "Unknown" ? "unknown" : "down"}`}>
                {states[link.state]}
              </span>
              <small>
                {zh ? "自 " : "since "}
                {formatChinaDateTime(link.since)}
                {link.latest_handshake &&
                  ` · ${zh ? "最后握手" : "last handshake"} ${formatChinaDateTime(link.latest_handshake)}`}
              </small>
            </dd>
          </div>
        ))}
      </dl>
    )}
  </>;

  if (collapsible && site) {
    return (
      <details className="station-network-links">
        <summary>
          <Network size={14} aria-hidden="true" />
          <span className="station-network-links-title">{title}</span>
          <span className={`node-link ${nodeLinkHealth(site)}`}>{nodeLinkLine(site, zh)}</span>
          <ChevronDown className="station-network-links-chevron" size={16} aria-hidden="true" />
        </summary>
        <div className="station-network-links-body">
          <p className="station-detail-note">{description}</p>
          {links}
        </div>
      </details>
    );
  }

  return (
    <section className="node-insight-section node-links">
      <div className="node-insight-section-head">
        <div>
          <span>{title}</span>
          <small>{description}</small>
        </div>
      </div>
      {site ? (
        <>
          <p className={`node-link ${nodeLinkHealth(site)}`}>{nodeLinkLine(site, zh)}</p>
          {links}
        </>
      ) : (
        <p className="muted">
          {network ? (zh ? "不在站点网络中" : "Not in the site network") : zh ? "读取中…" : "Loading…"}
        </p>
      )}
    </section>
  );
}
