import type { CRDNode } from "../../types";
import { CopyButton } from "./CopyButton.js";
import { nodeLinkHealth, nodeLinkLine, nodeNetworkModel, type NodeNetworkDisplay } from "../lib/nodeNetwork.js";
import { formatChinaDateTime } from "../../utils/time.js";

export function NodeNetworkAddresses({ node, network, zh }: { node: CRDNode; network?: NodeNetworkDisplay; zh: boolean }) {
  const model = nodeNetworkModel(node, network);
  const messages = {
    unreported: zh ? "未上报" : "Not reported",
    stale: zh ? "上次上报" : "Last reported",
    unavailable: zh ? "数据不可用" : "Unavailable",
  };
  return (
    <span className="node-network-addresses">
      {!model.addresses.some((entry) => entry.kind === "node") && (
        <span className="node-network-line">
          <span className="node-network-label">{zh ? "节点" : "Node"}</span>
          <span>—</span>
        </span>
      )}
      {model.addresses.map((entry) => (
        <span className="node-network-line" key={`${entry.kind}/${entry.address}`}>
          <span className="node-network-label">{entry.kind === "wg" ? "WG" : zh ? "节点" : "Node"}</span>
          <code className="node-row-ip" title={[entry.interface, entry.cidr ?? entry.address].filter(Boolean).join(" · ")}>
            {entry.address}
          </code>
          {!(entry.kind === "wg" && model.message === "stale") && (
            <CopyButton
              value={entry.address}
              zh={zh}
              iconOnly
              className="node-network-copy"
              label={`${zh ? "复制" : "Copy"} ${entry.kind === "wg" ? "WG" : zh ? "节点 IP" : "node IP"} ${entry.address}`}
            />
          )}
        </span>
      ))}
      {model.message && (
        <span className={`node-network-line node-network-note ${model.message}`}>
          <span className="node-network-label">WG</span>
          <span>
            {messages[model.message]}
            {model.message === "stale" && model.reportedAt ? ` · ${formatChinaDateTime(model.reportedAt)}` : ""}
          </span>
        </span>
      )}
      {network?.site && (
        <span className={`node-network-line node-link ${nodeLinkHealth(network.site)}`} title={network.site.summary || undefined}>
          <span className="node-network-label">{zh ? "链路" : "Link"}</span>
          <span>{nodeLinkLine(network.site, zh)}</span>
        </span>
      )}
    </span>
  );
}
