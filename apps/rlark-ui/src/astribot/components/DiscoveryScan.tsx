import { useEffect, useRef, useState } from "react";
import { Radar } from "lucide-react";
import { ApiError } from "../../api";
import { useAutoRefresh } from "../../hooks";
import { siteNetworkApi, type SiteDiscoveryScan } from "../api";
import "./DiscoveryScan.css";

function newScanID() {
  // The management console may be served over HTTP, where randomUUID is unavailable.
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 15) | 64;
  bytes[8] = (bytes[8] & 63) | 128;
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
  return [hex.slice(0, 8), hex.slice(8, 12), hex.slice(12, 16), hex.slice(16, 20), hex.slice(20)].join("-");
}

export function useDiscoveryScan(site: string, zh: boolean) {
  const [scan, setScan] = useState<SiteDiscoveryScan | null>();
  const [starting, setStarting] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [startError, setStartError] = useState("");
  const read = useRef<AbortController | null>(null);
  const writing = useRef(false);
  const requestID = useRef<string | undefined>(undefined);

  useEffect(() => () => {
    read.current?.abort();
    read.current = null;
  }, []);
  useAutoRefresh(async () => {
    if (!site || writing.current || read.current) return;
    const controller = new AbortController();
    read.current = controller;
    try {
      const current = await siteNetworkApi.getScan(site, controller.signal);
      if (!controller.signal.aborted) {
        setScan(current);
        setLoadError("");
        if (current?.id === requestID.current) {
          requestID.current = undefined;
          setStartError("");
        }
      }
    } catch {
      if (!controller.signal.aborted) setLoadError(zh ? "读取扫描状态失败。" : "Could not load scan status.");
    } finally {
      if (read.current === controller) read.current = null;
    }
  }, scan?.phase === "Running" ? 3000 : 10000, [site]);

  const start = async () => {
    if (!site || writing.current || scan?.phase === "Running") return;
    writing.current = true;
    read.current?.abort();
    read.current = null;
    setStarting(true);
    setStartError("");
    const id = requestID.current ?? newScanID();
    requestID.current = id;
    try {
      setScan(await siteNetworkApi.startScan(site, id));
      requestID.current = undefined;
      setLoadError("");
    } catch (reason) {
      if (reason instanceof ApiError && reason.status === 409) {
        setStartError(zh ? "站点还没有可扫描的 Hub。请检查扫描设置，等待配置下发后重试。" : "No Hub is ready to scan. Check the site settings and retry after they apply.");
      } else {
        setStartError(zh ? "发起扫描失败，请重试。" : "Could not start the scan. Try again.");
      }
    } finally {
      writing.current = false;
      setStarting(false);
    }
  };

  return { scan, starting, start, error: startError || loadError };
}

export function DiscoveryScanControl({ zh, scan, starting, start, error, disabledReason, variant = "primary" }: {
  zh: boolean;
  scan: SiteDiscoveryScan | null | undefined;
  starting: boolean;
  start: () => Promise<void>;
  error: string;
  disabledReason?: string;
  variant?: "primary" | "secondary";
}) {
  const running = starting || scan?.phase === "Running";
  let detail = disabledReason;
  if (!detail && scan) {
    const completed = `${scan.completed}/${scan.total} Hub`;
    const time = new Date(scan.startedAt).toLocaleString(zh ? "zh-CN" : "en-GB", { hour12: false });
    switch (scan.phase) {
      case "Running":
        detail = zh ? `${completed} 已返回结果` : `${completed} reported`;
        break;
      case "Succeeded":
        detail = zh ? `上次扫描 ${time}` : `Last scan ${time}`;
        break;
      case "Failed":
        detail = zh ? `${scan.failed} 个 Hub 扫描失败，可重试。` : `${scan.failed} Hub scan(s) failed. Try again.`;
        break;
      case "TimedOut":
        detail = zh ? `扫描超时，${completed} 已返回结果。` : `Scan timed out. ${completed} reported.`;
        break;
    }
  }
  const buttonClass = `${variant === "secondary" ? "secondary-button" : "primary-button"} station-discovery-btn${running ? " is-running" : ""}`;
  return <div className="station-discovery-control">
    <button type="button" className={buttonClass} disabled={running || Boolean(disabledReason)} onClick={() => void start()}>
      <Radar size={15} className="station-discovery-icon" aria-hidden="true" />{running ? (zh ? "扫描中…" : "Scanning…") : (zh ? "开始扫描" : "Start scan")}
    </button>
    {detail && <small className="station-discovery-detail" role="status">{detail}</small>}
    {error && <small className="station-discovery-error" role="alert">{error}</small>}
  </div>;
}
