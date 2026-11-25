import type { ReactNode } from "react";
import { CircleCheck, CircleHelp, CircleX } from "lucide-react";
import { CopyButton } from "../components/CopyButton";
import type { StationNetworkState } from "../lib/stationDisplay";

export function NetworkBadge({ state, zh }: { state: StationNetworkState; zh: boolean }) {
  const Icon = state === "verified" ? CircleCheck : state === "failed" ? CircleX : CircleHelp;
  const label = state === "verified" ? (zh ? "已验证" : "Verified") : state === "failed" ? (zh ? "探测失败" : "Probe failed") : (zh ? "未知" : "Unknown");
  return <span className={`station-network station-network-${state}`}><Icon size={14} aria-hidden="true" />{label}</span>;
}

export function CopyValue({ value, zh }: { value?: string; zh: boolean }) {
  if (!value) return <strong>—</strong>;
  return <span className="station-copy-value"><code className="inline-code" title={value}>{value}</code><CopyButton value={value} zh={zh} iconOnly /></span>;
}

export function DetailField({ label, children, wide }: { label: string; children: ReactNode; wide?: boolean }) {
  return <div className={`station-detail-field${wide ? " is-wide" : ""}`}><dt>{label}</dt><dd>{children}</dd></div>;
}
